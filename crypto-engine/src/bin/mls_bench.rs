use clap::Parser;
use crypto_engine::mls::{
    add_members, create_group, generate_identity, generate_key_package, CachedOperationContext,
    RuntimeCache,
};
use hpke_rs_crypto::{types::*, HpkeCrypto};
use hpke_rs_rust_crypto::HpkeRustCrypto;
use sha2::{Digest, Sha256};
use std::fs::File;
use std::io::{BufWriter, Write};
use std::time::{Duration, Instant};

#[derive(Parser, Debug)]
#[command(author, version, about = "MLS optimization research benchmark")]
struct Args {
    #[arg(long, default_value = "16,32,64,128,256,512,1024,2048,4096")]
    sizes: String,

    #[arg(long, default_value_t = 100)]
    samples: usize,

    #[arg(long, default_value_t = 1024)]
    payload_size: usize,

    #[arg(long, default_value = "mls_optimization_benchmark.csv")]
    output: String,
}

struct BenchRow {
    n: usize,
    operation: String,
    median_ms: f64,
    p95_ms: f64,
    p99_ms: f64,
    samples: usize,
    payload_size: usize,
    state_size_bytes: usize,
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let sizes = parse_sizes(&args.sizes)?;
    let payload = vec![0x42; args.payload_size];

    let file = File::create(&args.output)?;
    let mut out = BufWriter::new(file);
    writeln!(
        out,
        "n,operation,median_ms,p95_ms,p99_ms,samples,payload_size,state_size_bytes"
    )?;

    for n in sizes {
        eprintln!("preparing group n={n}");
        let group_state = build_group_state(n)?;
        let state_size_bytes = group_state.len();
        let pairwise_recipients = build_pairwise_recipients(n)?;

        let pairwise = measure(args.samples, || {
            pairwise_baseline(&pairwise_recipients, &payload);
        });
        write_row(
            &mut out,
            BenchRow {
                n,
                operation: "pairwise_baseline".to_string(),
                median_ms: pairwise.0,
                p95_ms: pairwise.1,
                p99_ms: pairwise.2,
                samples: args.samples,
                payload_size: args.payload_size,
                state_size_bytes,
            },
        )?;

        let pairwise_hash = measure(args.samples, || {
            pairwise_hash_sanity(n, &payload);
        });
        write_row(
            &mut out,
            BenchRow {
                n,
                operation: "pairwise_hash_sanity_not_e2ee".to_string(),
                median_ms: pairwise_hash.0,
                p95_ms: pairwise_hash.1,
                p99_ms: pairwise_hash.2,
                samples: args.samples,
                payload_size: args.payload_size,
                state_size_bytes,
            },
        )?;

        let full_blob = measure(args.samples, || {
            let _ = crypto_engine::mls::encrypt_message(&group_state, &payload)
                .expect("full-blob encrypt");
        });
        write_row(
            &mut out,
            BenchRow {
                n,
                operation: "current_full_blob_mls_encrypt".to_string(),
                median_ms: full_blob.0,
                p95_ms: full_blob.1,
                p99_ms: full_blob.2,
                samples: args.samples,
                payload_size: args.payload_size,
                state_size_bytes,
            },
        )?;

        let cache = RuntimeCache::default();
        let meta = cache
            .load_group(&format!("bench-{n}"), &group_state, 0)
            .expect("load cached group");
        let mut epoch = meta.epoch;
        let mut version = meta.state_version;
        let cached_encrypt = measure(args.samples, || {
            let result = cache
                .encrypt_message_cached(&ctx(format!("bench-{n}"), epoch, version), &payload)
                .expect("cached encrypt");
            epoch = result.epoch;
            version = result.state_version;
        });
        write_row(
            &mut out,
            BenchRow {
                n,
                operation: "hot_cache_sidecar_encrypt_core".to_string(),
                median_ms: cached_encrypt.0,
                p95_ms: cached_encrypt.1,
                p99_ms: cached_encrypt.2,
                samples: args.samples,
                payload_size: args.payload_size,
                state_size_bytes,
            },
        )?;

        let mut self_update_parts = Vec::with_capacity(args.samples);
        let mut merge_parts = Vec::with_capacity(args.samples);
        let mut serialize_parts = Vec::with_capacity(args.samples);
        let update = measure(args.samples, || {
            let profile = cache
                .create_update_commit_cached_profiled(&ctx(format!("bench-{n}"), epoch, version))
                .expect("cached update commit");
            self_update_parts.push(profile.self_update);
            merge_parts.push(profile.merge_pending_commit);
            serialize_parts.push(profile.serialize_commit);
            epoch = profile.result.epoch;
            version = profile.result.state_version;
        });
        write_row(
            &mut out,
            BenchRow {
                n,
                operation: "hot_cache_sidecar_update_commit_core".to_string(),
                median_ms: update.0,
                p95_ms: update.1,
                p99_ms: update.2,
                samples: args.samples,
                payload_size: args.payload_size,
                state_size_bytes,
            },
        )?;
        write_duration_row(
            &mut out,
            n,
            "hot_cache_update_self_update_part",
            &self_update_parts,
            args.samples,
            args.payload_size,
            state_size_bytes,
        )?;
        write_duration_row(
            &mut out,
            n,
            "hot_cache_update_merge_pending_part",
            &merge_parts,
            args.samples,
            args.payload_size,
            state_size_bytes,
        )?;
        write_duration_row(
            &mut out,
            n,
            "hot_cache_update_serialize_commit_part",
            &serialize_parts,
            args.samples,
            args.payload_size,
            state_size_bytes,
        )?;
    }
    out.flush()?;
    Ok(())
}

fn parse_sizes(raw: &str) -> Result<Vec<usize>, String> {
    raw.split(',')
        .map(|part| {
            part.trim()
                .parse::<usize>()
                .map_err(|e| format!("invalid size '{part}': {e}"))
        })
        .collect()
}

fn build_group_state(n: usize) -> Result<Vec<u8>, String> {
    if n == 0 {
        return Err("group size must be > 0".into());
    }
    let creator = generate_identity()?;
    let group_id = format!("bench-{n}");
    let mut state = create_group(&group_id, &creator.signing_key_private)?.group_state;
    let mut key_packages = Vec::with_capacity(n.saturating_sub(1));
    for _ in 1..n {
        let identity = generate_identity()?;
        let kp = generate_key_package(&identity.signing_key_private)?;
        key_packages.push(kp.key_package_bytes);
    }
    for chunk in key_packages.chunks(64) {
        state = add_members(&state, chunk)?.new_group_state;
    }
    Ok(state)
}

fn build_pairwise_recipients(n: usize) -> Result<Vec<Vec<u8>>, String> {
    let mut prng = HpkeRustCrypto::prng();
    let mut recipients = Vec::with_capacity(n);
    for _ in 0..n {
        let (pk, _sk) = HpkeRustCrypto::kem_key_gen(KemAlgorithm::DhKemP256, &mut prng)
            .map_err(|e| format!("pairwise kem_key_gen: {e:?}"))?;
        recipients.push(pk);
    }
    Ok(recipients)
}

fn pairwise_baseline(recipient_public_keys: &[Vec<u8>], payload: &[u8]) {
    let mut prng = HpkeRustCrypto::prng();
    let mut sink = 0usize;
    for (recipient, pk) in recipient_public_keys.iter().enumerate() {
        let (ephemeral_public, ephemeral_secret) =
            HpkeRustCrypto::kem_key_gen(KemAlgorithm::DhKemP256, &mut prng)
                .expect("pairwise ephemeral kem_key_gen");
        let shared_secret = HpkeRustCrypto::dh(KemAlgorithm::DhKemP256, pk, &ephemeral_secret)
            .expect("pairwise dh");
        let prk =
            HpkeRustCrypto::kdf_extract(KdfAlgorithm::HkdfSha256, b"pairwise", &shared_secret)
                .expect("pairwise kdf_extract");
        let mut info = Vec::with_capacity(32);
        info.extend_from_slice(b"pairwise-wrap");
        info.extend_from_slice(&(recipient as u64).to_be_bytes());
        let key = HpkeRustCrypto::kdf_expand(KdfAlgorithm::HkdfSha256, &prk, &info, 16)
            .expect("pairwise key expand");
        let nonce = HpkeRustCrypto::kdf_expand(KdfAlgorithm::HkdfSha256, &prk, b"nonce", 12)
            .expect("pairwise nonce expand");
        let wrapped = HpkeRustCrypto::aead_seal(
            AeadAlgorithm::Aes128Gcm,
            &key,
            &nonce,
            &ephemeral_public,
            payload,
        )
        .expect("pairwise aead seal");
        sink ^= ephemeral_public.len() ^ wrapped.len();
    }
    std::hint::black_box(sink);
}

fn pairwise_hash_sanity(n: usize, payload: &[u8]) {
    let mut sink = [0u8; 32];
    for recipient in 0..n {
        let mut hasher = Sha256::new();
        hasher.update(b"pairwise-baseline");
        hasher.update((recipient as u64).to_be_bytes());
        hasher.update(payload);
        sink.copy_from_slice(&hasher.finalize());
    }
    std::hint::black_box(sink);
}

fn measure<F>(samples: usize, mut f: F) -> (f64, f64, f64)
where
    F: FnMut(),
{
    let mut durations = Vec::with_capacity(samples);
    for _ in 0..samples {
        let started = Instant::now();
        f();
        durations.push(started.elapsed());
    }
    durations.sort_unstable();
    (
        as_ms(percentile(&durations, 0.50)),
        as_ms(percentile(&durations, 0.95)),
        as_ms(percentile(&durations, 0.99)),
    )
}

fn percentile(values: &[Duration], p: f64) -> Duration {
    if values.is_empty() {
        return Duration::ZERO;
    }
    let idx = ((values.len() - 1) as f64 * p).round() as usize;
    values[idx]
}

fn as_ms(duration: Duration) -> f64 {
    duration.as_secs_f64() * 1000.0
}

fn write_row(out: &mut BufWriter<File>, row: BenchRow) -> std::io::Result<()> {
    writeln!(
        out,
        "{},{},{:.6},{:.6},{:.6},{},{},{}",
        row.n,
        row.operation,
        row.median_ms,
        row.p95_ms,
        row.p99_ms,
        row.samples,
        row.payload_size,
        row.state_size_bytes
    )
}

fn write_duration_row(
    out: &mut BufWriter<File>,
    n: usize,
    operation: &str,
    durations: &[Duration],
    samples: usize,
    payload_size: usize,
    state_size_bytes: usize,
) -> std::io::Result<()> {
    let mut sorted = durations.to_vec();
    sorted.sort_unstable();
    write_row(
        out,
        BenchRow {
            n,
            operation: operation.to_string(),
            median_ms: as_ms(percentile(&sorted, 0.50)),
            p95_ms: as_ms(percentile(&sorted, 0.95)),
            p99_ms: as_ms(percentile(&sorted, 0.99)),
            samples,
            payload_size,
            state_size_bytes,
        },
    )
}

fn ctx(group_id: String, epoch: u64, state_version: u64) -> CachedOperationContext {
    CachedOperationContext {
        group_id,
        expected_epoch: epoch,
        expected_state_version: state_version,
        operation_id: format!("bench-op-{epoch}-{state_version}"),
    }
}
