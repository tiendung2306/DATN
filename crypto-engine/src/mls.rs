use std::collections::HashMap;
use std::sync::Mutex;

use ed25519_dalek::SigningKey;
use openmls::prelude::*;
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::crypto::OpenMlsCrypto;
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

// ─── In-memory group store ───────────────────────────────────────────────────

struct GroupEntry {
    group: MlsGroup,
    provider: OpenMlsRustCrypto,
    signer: SignatureKeyPair,
}

pub struct MlsGroupStore {
    groups: Mutex<HashMap<String, GroupEntry>>,
}

impl MlsGroupStore {
    pub fn new() -> Self {
        Self {
            groups: Mutex::new(HashMap::new()),
        }
    }
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

// ─── Proposal descriptor (coordination-layer concept, not raw MLS) ───────────

#[derive(serde::Serialize, serde::Deserialize)]
struct ProposalDescriptor {
    proposal_type: i32,
    data: Vec<u8>,
}

// ─── Helper functions ────────────────────────────────────────────────────────

fn make_group_state(group_id: &str, epoch: u64) -> Vec<u8> {
    serde_json::to_vec(&serde_json::json!({
        "group_id": group_id,
        "epoch": epoch,
    }))
    .unwrap_or_default()
}

fn parse_group_state(state: &[u8]) -> Result<(String, u64), String> {
    let v: serde_json::Value =
        serde_json::from_slice(state).map_err(|e| format!("invalid group state JSON: {e}"))?;
    let group_id = v["group_id"]
        .as_str()
        .ok_or("missing group_id in state")?
        .to_string();
    let epoch = v["epoch"].as_u64().unwrap_or(0);
    Ok((group_id, epoch))
}

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
    let epoch = group.epoch();
    // GroupEpoch implements Into<u64> or has as_u64()
    epoch.as_u64()
}

// ─── MLS Group Operations ────────────────────────────────────────────────────

pub fn create_group(
    store: &MlsGroupStore,
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
    let state = make_group_state(group_id, epoch);

    store
        .groups
        .lock()
        .unwrap()
        .insert(group_id.to_string(), GroupEntry { group, provider, signer });

    Ok(CreateGroupResult {
        group_state: state,
        tree_hash,
    })
}

pub fn encrypt_message(
    store: &MlsGroupStore,
    group_state: &[u8],
    plaintext: &[u8],
) -> Result<EncryptResult, String> {
    let (group_id, _) = parse_group_state(group_state)?;

    let mut groups = store.groups.lock().unwrap();
    let entry = groups
        .get_mut(&group_id)
        .ok_or_else(|| format!("group '{group_id}' not found in memory"))?;

    let mls_out = entry
        .group
        .create_message(&entry.provider, &entry.signer, plaintext)
        .map_err(|e| format!("create_message: {e:?}"))?;

    let ciphertext = mls_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize MlsMessageOut: {e:?}"))?;

    let epoch = get_epoch(&entry.group);
    let new_state = make_group_state(&group_id, epoch);

    Ok(EncryptResult {
        ciphertext,
        new_group_state: new_state,
    })
}

pub fn decrypt_message(
    store: &MlsGroupStore,
    group_state: &[u8],
    ciphertext: &[u8],
) -> Result<DecryptResult, String> {
    let (group_id, _) = parse_group_state(group_state)?;

    let mut groups = store.groups.lock().unwrap();
    let entry = groups
        .get_mut(&group_id)
        .ok_or_else(|| format!("group '{group_id}' not found in memory"))?;

    let mls_msg = MlsMessageIn::tls_deserialize_exact(ciphertext)
        .map_err(|e| format!("deserialize ciphertext: {e:?}"))?;

    let protocol_msg = mls_msg
        .try_into_protocol_message()
        .map_err(|e| format!("extract protocol message: {e:?}"))?;

    let processed = entry
        .group
        .process_message(&entry.provider, protocol_msg)
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

    let epoch = get_epoch(&entry.group);
    let new_state = make_group_state(&group_id, epoch);

    Ok(DecryptResult {
        plaintext,
        new_group_state: new_state,
    })
}

pub fn create_proposal(
    _store: &MlsGroupStore,
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
    store: &MlsGroupStore,
    group_state: &[u8],
    proposals: &[Vec<u8>],
) -> Result<CommitResult, String> {
    let (group_id, _) = parse_group_state(group_state)?;

    let mut groups = store.groups.lock().unwrap();
    let entry = groups
        .get_mut(&group_id)
        .ok_or_else(|| format!("group '{group_id}' not found in memory"))?;

    if proposals.is_empty() {
        return Err("no proposals to commit".into());
    }

    // Parse all proposal descriptors
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

    // Self-update: creates a commit with an Update proposal
    let bundle = entry
        .group
        .self_update(&entry.provider, &entry.signer, LeafNodeParameters::default())
        .map_err(|e| format!("self_update: {e:?}"))?;

    let (commit_out, welcome_out, _group_info) = bundle.into_contents();

    entry
        .group
        .merge_pending_commit(&entry.provider)
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

    let epoch = get_epoch(&entry.group);
    let new_tree_hash = compute_tree_hash(&group_id, epoch);
    let new_state = make_group_state(&group_id, epoch);

    Ok(CommitResult {
        commit_bytes,
        welcome_bytes,
        new_group_state: new_state,
        new_tree_hash,
    })
}

pub fn process_commit(
    store: &MlsGroupStore,
    group_state: &[u8],
    commit_bytes: &[u8],
) -> Result<ProcessCommitResult, String> {
    let (group_id, _) = parse_group_state(group_state)?;

    let mut groups = store.groups.lock().unwrap();
    let entry = groups
        .get_mut(&group_id)
        .ok_or_else(|| format!("group '{group_id}' not found in memory"))?;

    let mls_msg = MlsMessageIn::tls_deserialize_exact(commit_bytes)
        .map_err(|e| format!("deserialize commit: {e:?}"))?;

    let protocol_msg = mls_msg
        .try_into_protocol_message()
        .map_err(|e| format!("extract protocol message: {e:?}"))?;

    let processed = entry
        .group
        .process_message(&entry.provider, protocol_msg)
        .map_err(|e| format!("process_message (commit): {e:?}"))?;

    match processed.into_content() {
        ProcessedMessageContent::StagedCommitMessage(staged_commit) => {
            entry
                .group
                .merge_staged_commit(&entry.provider, *staged_commit)
                .map_err(|e| format!("merge_staged_commit: {e:?}"))?;
        }
        other => {
            return Err(format!(
                "expected StagedCommit, got {:?}",
                std::mem::discriminant(&other)
            ))
        }
    }

    let epoch = get_epoch(&entry.group);
    let new_tree_hash = compute_tree_hash(&group_id, epoch);
    let new_state = make_group_state(&group_id, epoch);

    Ok(ProcessCommitResult {
        new_group_state: new_state,
        new_tree_hash,
    })
}

pub fn process_welcome(
    store: &MlsGroupStore,
    welcome_bytes: &[u8],
    signing_key: &[u8],
) -> Result<WelcomeResult, String> {
    let provider = OpenMlsRustCrypto::default();
    let signer = reconstruct_signer(&provider, signing_key)?;

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
    let state = make_group_state(&group_id, epoch);

    store.groups.lock().unwrap().insert(
        group_id,
        GroupEntry {
            group,
            provider,
            signer,
        },
    );

    Ok(WelcomeResult {
        group_state: state,
        tree_hash,
    })
}

pub fn external_join(
    _store: &MlsGroupStore,
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
    store: &MlsGroupStore,
    group_state: &[u8],
    label: &str,
    length: u32,
) -> Result<Vec<u8>, String> {
    let (group_id, _) = parse_group_state(group_state)?;

    let groups = store.groups.lock().unwrap();
    let entry = groups
        .get(&group_id)
        .ok_or_else(|| format!("group '{group_id}' not found in memory"))?;

    let secret = entry
        .group
        .export_secret(entry.provider.crypto(), label, &[], length as usize)
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
        let store = MlsGroupStore::new();
        let sk = test_signing_key();
        let result = create_group(&store, "test-group-1", &sk).expect("create_group failed");

        assert!(!result.group_state.is_empty());
        assert!(!result.tree_hash.is_empty());

        let (gid, epoch) = parse_group_state(&result.group_state).unwrap();
        assert_eq!(gid, "test-group-1");
        assert_eq!(epoch, 0);
    }

    #[test]
    fn test_encrypt_message() {
        let store = MlsGroupStore::new();
        let sk = test_signing_key();
        let cr = create_group(&store, "test-encrypt", &sk).expect("create_group");

        let enc = encrypt_message(&store, &cr.group_state, b"Hello, MLS!")
            .expect("encrypt_message");
        assert!(!enc.ciphertext.is_empty());
        assert!(!enc.new_group_state.is_empty());
    }

    #[test]
    fn test_create_proposal_descriptor() {
        let store = MlsGroupStore::new();
        let sk = test_signing_key();
        let cr = create_group(&store, "test-proposal", &sk).expect("create_group");

        let prop = create_proposal(&store, &cr.group_state, 2, b"")
            .expect("create_proposal");
        let desc: ProposalDescriptor = serde_json::from_slice(&prop).unwrap();
        assert_eq!(desc.proposal_type, 2);
    }

    #[test]
    fn test_export_secret() {
        let store = MlsGroupStore::new();
        let sk = test_signing_key();
        let cr = create_group(&store, "test-export", &sk).expect("create_group");

        let secret = export_secret(&store, &cr.group_state, "test-label", 32)
            .expect("export_secret");
        assert_eq!(secret.len(), 32);

        // Deterministic: same label produces same secret
        let secret2 = export_secret(&store, &cr.group_state, "test-label", 32)
            .expect("export_secret 2");
        assert_eq!(secret, secret2);
    }
}
