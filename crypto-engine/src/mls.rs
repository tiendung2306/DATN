use ed25519_dalek::SigningKey;
use openmls::prelude::*;
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::crypto::OpenMlsCrypto;
use openmls_traits::storage::StorageProvider;
use sha2::{Digest, Sha256};
use tls_codec::{Deserialize as TlsDeserializeTrait, Serialize as TlsSerializeTrait};

const CIPHERSUITE: Ciphersuite = Ciphersuite::MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519;

// ─── Stateless identity generation (unchanged from Phase 2) ──────────────────

pub struct GeneratedIdentity {
    pub signing_key_private: Vec<u8>,
    pub public_key: Vec<u8>,
    pub credential: Vec<u8>,
}

pub fn generate_identity() -> Result<GeneratedIdentity, String> {
    let provider = OpenMlsRustCrypto::default();

    let (signing_key_private, public_key) = provider
        .crypto()
        .signature_key_gen(SignatureScheme::ED25519)
        .map_err(|e| format!("Failed to generate Ed25519 key pair: {e:?}"))?;

    Ok(GeneratedIdentity {
        signing_key_private,
        public_key,
        credential: Vec::new(),
    })
}

// ─── Persisted group state (replaces the old in-memory HashMap) ──────────────

const STATE_VERSION: u8 = 1;

/// Self-contained serializable snapshot of an MLS group.
/// Go stores this as an opaque blob in SQLite; Rust reconstructs the
/// full OpenMLS `MlsGroup` from it on every RPC call.
#[derive(serde::Serialize, serde::Deserialize)]
struct PersistedGroupState {
    version: u8,
    group_id: String,
    epoch: u64,
    signing_key: Vec<u8>,
    entries: Vec<(Vec<u8>, Vec<u8>)>,
}

/// Intermediate result of importing a persisted group state.
struct ImportedGroup {
    group_id: String,
    signing_key: Vec<u8>,
    provider: OpenMlsRustCrypto,
    signer: SignatureKeyPair,
    group: MlsGroup,
}

fn export_state(
    provider: &OpenMlsRustCrypto,
    group_id: &str,
    epoch: u64,
    signing_key: &[u8],
) -> Vec<u8> {
    let values = provider.storage().values.read().unwrap();
    let persisted = PersistedGroupState {
        version: STATE_VERSION,
        group_id: group_id.to_string(),
        epoch,
        signing_key: signing_key.to_vec(),
        entries: values.iter().map(|(k, v)| (k.clone(), v.clone())).collect(),
    };
    serde_json::to_vec(&persisted).unwrap_or_default()
}

fn import_state(state_bytes: &[u8]) -> Result<ImportedGroup, String> {
    let persisted: PersistedGroupState = serde_json::from_slice(state_bytes).map_err(|e| {
        if serde_json::from_slice::<serde_json::Value>(state_bytes)
            .ok()
            .and_then(|v| v.get("version").cloned())
            .is_none()
        {
            "group_state is in legacy format (no crypto data); please recreate the group".to_string()
        } else {
            format!("invalid persisted group state: {e}")
        }
    })?;

    if persisted.version != STATE_VERSION {
        return Err(format!(
            "unsupported group_state version {} (expected {})",
            persisted.version, STATE_VERSION
        ));
    }

    let provider = OpenMlsRustCrypto::default();

    // Populate storage with the persisted entries
    {
        let mut values = provider.storage().values.write().unwrap();
        for (k, v) in persisted.entries {
            values.insert(k, v);
        }
    }

    let signer = reconstruct_signer(&provider, &persisted.signing_key)?;

    let group_id_mls = GroupId::from_slice(persisted.group_id.as_bytes());
    let group = MlsGroup::load(provider.storage(), &group_id_mls)
        .map_err(|e| format!("storage error loading group '{}': {e:?}", persisted.group_id))?
        .ok_or_else(|| {
            format!(
                "group '{}' not found in restored storage",
                persisted.group_id
            )
        })?;

    Ok(ImportedGroup {
        group_id: persisted.group_id,
        signing_key: persisted.signing_key,
        provider,
        signer,
        group,
    })
}

// ─── Result types (wire-compatible with gRPC responses) ──────────────────────

pub struct CreateGroupResult {
    pub group_state: Vec<u8>,
    pub tree_hash: Vec<u8>,
}

pub struct CommitResult {
    pub commit_bytes: Vec<u8>,
    pub welcome_bytes: Vec<u8>,
    pub new_group_state: Vec<u8>,
    pub new_tree_hash: Vec<u8>,
}

pub struct ProcessCommitResult {
    pub new_group_state: Vec<u8>,
    pub new_tree_hash: Vec<u8>,
}

pub struct WelcomeResult {
    pub group_state: Vec<u8>,
    pub tree_hash: Vec<u8>,
    pub epoch: u64,
}

pub struct EncryptResult {
    pub ciphertext: Vec<u8>,
    pub new_group_state: Vec<u8>,
}

pub struct DecryptResult {
    pub plaintext: Vec<u8>,
    pub new_group_state: Vec<u8>,
}

pub struct ExternalJoinResult {
    pub group_state: Vec<u8>,
    pub commit_bytes: Vec<u8>,
    pub tree_hash: Vec<u8>,
}

pub struct GenerateKeyPackageResult {
    pub key_package_bytes: Vec<u8>,
    pub key_package_bundle_private: Vec<u8>,
}

// ─── Proposal descriptor (coordination-layer concept, not raw MLS) ───────────

#[derive(serde::Serialize, serde::Deserialize)]
struct ProposalDescriptor {
    proposal_type: i32,
    data: Vec<u8>,
}

// ─── Helper functions ────────────────────────────────────────────────────────

fn reconstruct_signer(
    provider: &OpenMlsRustCrypto,
    signing_key: &[u8],
) -> Result<SignatureKeyPair, String> {
    let seed: [u8; 32] = signing_key
        .try_into()
        .map_err(|_| format!("signing key must be 32 bytes, got {}", signing_key.len()))?;
    let ed_sk = SigningKey::from_bytes(&seed);
    let public = ed_sk.verifying_key().to_bytes().to_vec();

    let signer = SignatureKeyPair::from_raw(SignatureScheme::ED25519, signing_key.to_vec(), public);
    signer
        .store(provider.storage())
        .map_err(|e| format!("store signer: {e:?}"))?;
    Ok(signer)
}

fn compute_tree_hash(group_id: &str, epoch: u64) -> Vec<u8> {
    let mut hasher = Sha256::new();
    hasher.update(b"tree_hash:");
    hasher.update(group_id.as_bytes());
    hasher.update(b":");
    hasher.update(epoch.to_be_bytes());
    hasher.finalize().to_vec()
}

fn get_epoch(group: &MlsGroup) -> u64 {
    group.epoch().as_u64()
}

// ─── MLS Group Operations (stateless) ───────────────────────────────────────

/// Build a self-signed MLS KeyPackage for the given identity (out-of-band add flow).
/// Returns the public KeyPackage bytes (share with the group creator) and an opaque
/// private blob that the invitee must retain until [`process_welcome`] (never share OOB).
pub fn generate_key_package(signing_key: &[u8]) -> Result<GenerateKeyPackageResult, String> {
    let provider = OpenMlsRustCrypto::default();
    let signer = reconstruct_signer(&provider, signing_key)?;

    let credential = BasicCredential::new(signer.to_public_vec());
    let credential_with_key = CredentialWithKey {
        credential: credential.into(),
        signature_key: signer.to_public_vec().into(),
    };

    let bundle = KeyPackageBuilder::new()
        .build(
            CIPHERSUITE,
            &provider,
            &signer,
            credential_with_key,
        )
        .map_err(|e| format!("KeyPackageBuilder::build: {e:?}"))?;

    let key_package_bytes = bundle
        .key_package()
        .tls_serialize_detached()
        .map_err(|e| format!("serialize KeyPackage: {e:?}"))?;

    let key_package_bundle_private = serde_json::to_vec(&bundle)
        .map_err(|e| format!("serialize KeyPackageBundle: {e}"))?;

    Ok(GenerateKeyPackageResult {
        key_package_bytes,
        key_package_bundle_private,
    })
}

/// Add one or more members via their KeyPackages (commit + welcome in one step).
pub fn add_members(
    group_state: &[u8],
    key_packages_bytes: &[Vec<u8>],
) -> Result<CommitResult, String> {
    let mut imp = import_state(group_state)?;

    if key_packages_bytes.is_empty() {
        return Err("no key packages".into());
    }

    let mut key_packages: Vec<KeyPackage> = Vec::with_capacity(key_packages_bytes.len());
    for raw in key_packages_bytes {
        let mut rd = raw.as_slice();
        let kpin = KeyPackageIn::tls_deserialize(&mut rd)
            .map_err(|e| format!("deserialize KeyPackageIn: {e:?}"))?;
        let kp = kpin
            .validate(imp.provider.crypto(), ProtocolVersion::Mls10)
            .map_err(|e| format!("invalid KeyPackage: {e:?}"))?;
        key_packages.push(kp);
    }

    let (commit_out, welcome_out, _group_info) = imp
        .group
        .add_members(&imp.provider, &imp.signer, &key_packages)
        .map_err(|e| format!("add_members: {e:?}"))?;

    imp.group
        .merge_pending_commit(&imp.provider)
        .map_err(|e| format!("merge_pending_commit: {e:?}"))?;

    let commit_bytes = commit_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize commit: {e:?}"))?;

    let welcome_bytes = welcome_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize welcome: {e:?}"))?;

    let epoch = get_epoch(&imp.group);
    let new_tree_hash = compute_tree_hash(&imp.group_id, epoch);
    let new_state = export_state(&imp.provider, &imp.group_id, epoch, &imp.signing_key);

    Ok(CommitResult {
        commit_bytes,
        welcome_bytes,
        new_group_state: new_state,
        new_tree_hash,
    })
}

pub fn create_group(
    group_id: &str,
    signing_key: &[u8],
) -> Result<CreateGroupResult, String> {
    let provider = OpenMlsRustCrypto::default();
    let signer = reconstruct_signer(&provider, signing_key)?;

    let credential = BasicCredential::new(group_id.as_bytes().to_vec());
    let credential_with_key = CredentialWithKey {
        credential: credential.into(),
        signature_key: signer.to_public_vec().into(),
    };

    let group_id_mls = GroupId::from_slice(group_id.as_bytes());

    let config = MlsGroupCreateConfig::builder()
        .use_ratchet_tree_extension(true)
        .ciphersuite(CIPHERSUITE)
        .build();

    let group = MlsGroup::new_with_group_id(
        &provider,
        &signer,
        &config,
        group_id_mls,
        credential_with_key,
    )
    .map_err(|e| format!("MlsGroup::new_with_group_id: {e:?}"))?;

    let epoch = get_epoch(&group);
    let tree_hash = compute_tree_hash(group_id, epoch);
    let state = export_state(&provider, group_id, epoch, signing_key);

    Ok(CreateGroupResult {
        group_state: state,
        tree_hash,
    })
}

pub fn encrypt_message(
    group_state: &[u8],
    plaintext: &[u8],
) -> Result<EncryptResult, String> {
    let mut imp = import_state(group_state)?;

    let mls_out = imp
        .group
        .create_message(&imp.provider, &imp.signer, plaintext)
        .map_err(|e| format!("create_message: {e:?}"))?;

    let ciphertext = mls_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize MlsMessageOut: {e:?}"))?;

    let epoch = get_epoch(&imp.group);
    let new_state = export_state(&imp.provider, &imp.group_id, epoch, &imp.signing_key);

    Ok(EncryptResult {
        ciphertext,
        new_group_state: new_state,
    })
}

pub fn decrypt_message(
    group_state: &[u8],
    ciphertext: &[u8],
) -> Result<DecryptResult, String> {
    let mut imp = import_state(group_state)?;

    let mls_msg = MlsMessageIn::tls_deserialize_exact(ciphertext)
        .map_err(|e| format!("deserialize ciphertext: {e:?}"))?;

    let protocol_msg = mls_msg
        .try_into_protocol_message()
        .map_err(|e| format!("extract protocol message: {e:?}"))?;

    let processed = imp
        .group
        .process_message(&imp.provider, protocol_msg)
        .map_err(|e| format!("process_message: {e:?}"))?;

    let plaintext = match processed.into_content() {
        ProcessedMessageContent::ApplicationMessage(app_msg) => app_msg.into_bytes(),
        ProcessedMessageContent::StagedCommitMessage(_) => {
            return Err("expected application message, got commit".into())
        }
        ProcessedMessageContent::ProposalMessage(_) => {
            return Err("expected application message, got proposal".into())
        }
        ProcessedMessageContent::ExternalJoinProposalMessage(_) => {
            return Err("expected application message, got external join".into())
        }
    };

    let epoch = get_epoch(&imp.group);
    let new_state = export_state(&imp.provider, &imp.group_id, epoch, &imp.signing_key);

    Ok(DecryptResult {
        plaintext,
        new_group_state: new_state,
    })
}

pub fn create_proposal(
    _group_state: &[u8],
    proposal_type: i32,
    data: &[u8],
) -> Result<Vec<u8>, String> {
    let desc = ProposalDescriptor {
        proposal_type,
        data: data.to_vec(),
    };
    serde_json::to_vec(&desc).map_err(|e| format!("serialize proposal descriptor: {e}"))
}

pub fn create_commit(
    group_state: &[u8],
    proposals: &[Vec<u8>],
) -> Result<CommitResult, String> {
    let mut imp = import_state(group_state)?;

    if proposals.is_empty() {
        return Err("no proposals to commit".into());
    }

    let mut update_count = 0u32;
    for raw in proposals {
        let desc: ProposalDescriptor =
            serde_json::from_slice(raw).map_err(|e| format!("parse proposal descriptor: {e}"))?;
        match desc.proposal_type {
            2 => update_count += 1, // ProposalUpdate
            0 => {
                return Err(
                    "Add proposals require KeyPackage generation (not yet implemented)".into(),
                )
            }
            1 => {
                return Err("Remove proposals not yet implemented with real OpenMLS".into())
            }
            _ => return Err(format!("unknown proposal type: {}", desc.proposal_type)),
        }
    }

    if update_count == 0 {
        return Err("no supported proposals to commit".into());
    }

    let bundle = imp
        .group
        .self_update(&imp.provider, &imp.signer, LeafNodeParameters::default())
        .map_err(|e| format!("self_update: {e:?}"))?;

    let (commit_out, welcome_out, _group_info) = bundle.into_contents();

    imp.group
        .merge_pending_commit(&imp.provider)
        .map_err(|e| format!("merge_pending_commit: {e:?}"))?;

    let commit_bytes = commit_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize commit: {e:?}"))?;

    let welcome_bytes: Vec<u8> = match welcome_out {
        Some(w) => w
            .tls_serialize_detached()
            .map_err(|e| format!("serialize welcome: {e:?}"))?,
        None => Vec::new(),
    };

    let epoch = get_epoch(&imp.group);
    let new_tree_hash = compute_tree_hash(&imp.group_id, epoch);
    let new_state = export_state(&imp.provider, &imp.group_id, epoch, &imp.signing_key);

    Ok(CommitResult {
        commit_bytes,
        welcome_bytes,
        new_group_state: new_state,
        new_tree_hash,
    })
}

pub fn process_commit(
    group_state: &[u8],
    commit_bytes: &[u8],
) -> Result<ProcessCommitResult, String> {
    let mut imp = import_state(group_state)?;

    let mls_msg = MlsMessageIn::tls_deserialize_exact(commit_bytes)
        .map_err(|e| format!("deserialize commit: {e:?}"))?;

    let protocol_msg = mls_msg
        .try_into_protocol_message()
        .map_err(|e| format!("extract protocol message: {e:?}"))?;

    let processed = imp
        .group
        .process_message(&imp.provider, protocol_msg)
        .map_err(|e| format!("process_message (commit): {e:?}"))?;

    match processed.into_content() {
        ProcessedMessageContent::StagedCommitMessage(staged_commit) => {
            imp.group
                .merge_staged_commit(&imp.provider, *staged_commit)
                .map_err(|e| format!("merge_staged_commit: {e:?}"))?;
        }
        other => {
            return Err(format!(
                "expected StagedCommit, got {:?}",
                std::mem::discriminant(&other)
            ))
        }
    }

    let epoch = get_epoch(&imp.group);
    let new_tree_hash = compute_tree_hash(&imp.group_id, epoch);
    let new_state = export_state(&imp.provider, &imp.group_id, epoch, &imp.signing_key);

    Ok(ProcessCommitResult {
        new_group_state: new_state,
        new_tree_hash,
    })
}

pub fn process_welcome(
    welcome_bytes: &[u8],
    signing_key: &[u8],
    key_package_bundle_private: &[u8],
) -> Result<WelcomeResult, String> {
    let provider = OpenMlsRustCrypto::default();
    let _signer = reconstruct_signer(&provider, signing_key)?;

    let bundle: KeyPackageBundle = serde_json::from_slice(key_package_bundle_private)
        .map_err(|e| format!("deserialize KeyPackageBundle: {e}"))?;
    let hash_ref = bundle
        .key_package()
        .hash_ref(provider.crypto())
        .map_err(|e| format!("key package hash_ref: {e:?}"))?;
    provider
        .storage()
        .write_key_package(&hash_ref, &bundle)
        .map_err(|e| format!("store KeyPackageBundle: {e:?}"))?;

    let mls_msg = MlsMessageIn::tls_deserialize_exact(welcome_bytes)
        .map_err(|e| format!("deserialize welcome: {e:?}"))?;

    let welcome = match mls_msg.extract() {
        MlsMessageBodyIn::Welcome(w) => w,
        _ => return Err("expected Welcome message body".into()),
    };

    let join_config = MlsGroupJoinConfig::builder()
        .use_ratchet_tree_extension(true)
        .build();

    let staged = StagedWelcome::new_from_welcome(&provider, &join_config, welcome, None)
        .map_err(|e| format!("StagedWelcome::new_from_welcome: {e:?}"))?;

    let group = staged
        .into_group(&provider)
        .map_err(|e| format!("into_group: {e:?}"))?;

    let group_id = String::from_utf8_lossy(group.group_id().as_slice()).to_string();
    let epoch = get_epoch(&group);
    let tree_hash = compute_tree_hash(&group_id, epoch);
    let state = export_state(&provider, &group_id, epoch, signing_key);

    Ok(WelcomeResult {
        group_state: state,
        tree_hash,
        epoch,
    })
}

pub fn external_join(
    group_info: &[u8],
    signing_key: &[u8],
) -> Result<ExternalJoinResult, String> {
    // External join requires a verifiable GroupInfo from the winning branch.
    // Full implementation deferred — return deterministic placeholder for now.
    let state = format!(
        "stub:external_join:info:{}:key:{}",
        group_info.len(),
        signing_key.len()
    );
    let tree_hash = {
        let mut h = Sha256::new();
        h.update(state.as_bytes());
        h.finalize().to_vec()
    };
    let commit = format!("stub:external_commit:info:{}", group_info.len());

    Ok(ExternalJoinResult {
        group_state: state.into_bytes(),
        commit_bytes: commit.into_bytes(),
        tree_hash,
    })
}

pub fn export_secret(
    group_state: &[u8],
    label: &str,
    length: u32,
) -> Result<Vec<u8>, String> {
    let imp = import_state(group_state)?;

    let secret = imp
        .group
        .export_secret(imp.provider.crypto(), label, &[], length as usize)
        .map_err(|e| format!("export_secret: {e:?}"))?;

    Ok(secret)
}

// ─── Tests ───────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    fn test_signing_key() -> Vec<u8> {
        let id = generate_identity().expect("generate_identity failed");
        id.signing_key_private
    }

    #[test]
    fn test_create_group() {
        let sk = test_signing_key();
        let result = create_group("test-group-1", &sk).expect("create_group failed");

        assert!(!result.group_state.is_empty());
        assert!(!result.tree_hash.is_empty());

        let persisted: PersistedGroupState =
            serde_json::from_slice(&result.group_state).unwrap();
        assert_eq!(persisted.group_id, "test-group-1");
        assert_eq!(persisted.epoch, 0);
        assert_eq!(persisted.version, STATE_VERSION);
        assert!(!persisted.entries.is_empty());
    }

    #[test]
    fn test_encrypt_message() {
        let sk = test_signing_key();
        let cr = create_group("test-encrypt", &sk).expect("create_group");

        let enc = encrypt_message(&cr.group_state, b"Hello, MLS!")
            .expect("encrypt_message");
        assert!(!enc.ciphertext.is_empty());
        assert!(!enc.new_group_state.is_empty());
    }

    #[test]
    fn test_encrypt_survives_reimport() {
        let sk = test_signing_key();
        let cr = create_group("test-reimport", &sk).expect("create_group");

        let enc1 = encrypt_message(&cr.group_state, b"msg-1").expect("encrypt 1");

        // The returned new_group_state can be used for the next operation
        // (simulates Go saving to SQLite and reading it back).
        let enc2 = encrypt_message(&enc1.new_group_state, b"msg-2").expect("encrypt 2");
        assert!(!enc2.ciphertext.is_empty());
    }

    #[test]
    fn test_create_proposal_descriptor() {
        let prop = create_proposal(b"{}", 2, b"").expect("create_proposal");
        let desc: ProposalDescriptor = serde_json::from_slice(&prop).unwrap();
        assert_eq!(desc.proposal_type, 2);
    }

    #[test]
    fn test_export_secret() {
        let sk = test_signing_key();
        let cr = create_group("test-export", &sk).expect("create_group");

        let secret = export_secret(&cr.group_state, "test-label", 32)
            .expect("export_secret");
        assert_eq!(secret.len(), 32);
    }

    #[test]
    fn test_generate_key_package() {
        let sk = test_signing_key();
        let kp = generate_key_package(&sk).expect("generate_key_package");
        assert!(!kp.key_package_bytes.is_empty());
        assert!(!kp.key_package_bundle_private.is_empty());
    }

    #[test]
    fn test_add_member_and_welcome() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let cr = create_group("add-member-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package for B");

        let commit = add_members(&cr.group_state, &[kp_b.key_package_bytes.clone()]).expect("add_members");

        let welcome_b = process_welcome(
            &commit.welcome_bytes,
            &sk_b,
            &kp_b.key_package_bundle_private,
        )
        .expect("process_welcome B");

        let enc_a = encrypt_message(&commit.new_group_state, b"hello from A")
            .expect("encrypt A");
        let dec_b = decrypt_message(&welcome_b.group_state, &enc_a.ciphertext)
            .expect("decrypt B");
        assert_eq!(dec_b.plaintext, b"hello from A");
    }

    #[test]
    fn test_legacy_state_rejected() {
        let legacy = serde_json::to_vec(&serde_json::json!({
            "group_id": "old-group",
            "epoch": 0
        }))
        .unwrap();
        match import_state(&legacy) {
            Ok(_) => panic!("expected error for legacy state, got Ok"),
            Err(e) => assert!(
                e.contains("legacy format"),
                "expected legacy format error, got: {e}"
            ),
        }
    }
}
