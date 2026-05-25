use dashmap::DashMap;
use ed25519_dalek::SigningKey;
use openmls::prelude::*;
use openmls_basic_credential::SignatureKeyPair;
use openmls_rust_crypto::OpenMlsRustCrypto;
use openmls_traits::crypto::OpenMlsCrypto;
use openmls_traits::storage::StorageProvider;
use sha2::{Digest, Sha256};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
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

struct GroupRuntime {
    group_id: String,
    signing_key: Vec<u8>,
    provider: OpenMlsRustCrypto,
    signer: SignatureKeyPair,
    group: MlsGroup,
    state_version: u64,
    dirty: bool,
}

/// In-memory group registry used only by optimization benchmark RPCs.
/// Production stateless RPCs continue to import/export full GroupState bytes.
#[derive(Default)]
pub struct RuntimeCache {
    groups: DashMap<String, Arc<Mutex<GroupRuntime>>>,
}

#[derive(Clone, Debug)]
pub struct CachedOperationContext {
    pub group_id: String,
    pub expected_epoch: u64,
    pub expected_state_version: u64,
    pub operation_id: String,
}

pub struct CachedGroupMetadata {
    pub group_id: String,
    pub epoch: u64,
    pub state_version: u64,
    pub tree_hash: Vec<u8>,
    pub dirty: bool,
    pub state_size_bytes: u64,
}

#[derive(Debug)]
pub struct CachedEncryptResult {
    pub ciphertext: Vec<u8>,
    pub epoch: u64,
    pub state_version: u64,
}

pub struct CachedDecryptResult {
    pub plaintext: Vec<u8>,
    pub epoch: u64,
    pub state_version: u64,
}

pub struct CachedUpdateCommitResult {
    pub commit_bytes: Vec<u8>,
    pub tree_hash: Vec<u8>,
    pub epoch: u64,
    pub state_version: u64,
}

#[allow(dead_code)]
pub struct CachedUpdateCommitProfileResult {
    pub result: CachedUpdateCommitResult,
    pub self_update: Duration,
    pub merge_pending_commit: Duration,
    pub serialize_commit: Duration,
}

pub struct CachedProcessCommitResult {
    pub tree_hash: Vec<u8>,
    pub epoch: u64,
    pub state_version: u64,
}

pub struct CachedExportSecretResult {
    pub secret: Vec<u8>,
    pub epoch: u64,
    pub state_version: u64,
}

pub struct CachedCheckpointResult {
    pub group_state: Vec<u8>,
    pub tree_hash: Vec<u8>,
    pub epoch: u64,
    pub state_version: u64,
    pub state_size_bytes: u64,
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
            "group_state is in legacy format (no crypto data); please recreate the group"
                .to_string()
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
        .map_err(|e| {
            format!(
                "storage error loading group '{}': {e:?}",
                persisted.group_id
            )
        })?
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

impl RuntimeCache {
    pub fn load_group(
        &self,
        group_id: &str,
        group_state: &[u8],
        state_version: u64,
    ) -> Result<CachedGroupMetadata, String> {
        if group_id.trim().is_empty() {
            return Err("group_id is required".into());
        }
        let imp = import_state(group_state)?;
        if imp.group_id != group_id {
            return Err(format!(
                "LoadGroup group_id mismatch: request='{group_id}' state='{}'",
                imp.group_id
            ));
        }
        let epoch = get_epoch(&imp.group);
        let runtime = GroupRuntime {
            group_id: imp.group_id,
            signing_key: imp.signing_key,
            provider: imp.provider,
            signer: imp.signer,
            group: imp.group,
            state_version,
            dirty: false,
        };
        self.groups
            .insert(group_id.to_string(), Arc::new(Mutex::new(runtime)));
        Ok(CachedGroupMetadata {
            group_id: group_id.to_string(),
            epoch,
            state_version,
            tree_hash: compute_tree_hash(group_id, epoch),
            dirty: false,
            state_size_bytes: group_state.len() as u64,
        })
    }

    pub fn unload_group(&self, group_id: &str) -> bool {
        self.groups.remove(group_id).is_some()
    }

    pub fn metadata(&self, group_id: &str) -> Result<CachedGroupMetadata, String> {
        let group_ref = self.group_ref(group_id)?;
        let runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{group_id}' runtime lock poisoned"))?;
        Ok(runtime.metadata())
    }

    pub fn encrypt_message_cached(
        &self,
        ctx: &CachedOperationContext,
        plaintext: &[u8],
    ) -> Result<CachedEncryptResult, String> {
        let group_ref = self.group_ref(&ctx.group_id)?;
        let mut runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{}' runtime lock poisoned", ctx.group_id))?;
        runtime.validate_context(ctx)?;

        let GroupRuntime {
            group,
            provider,
            signer,
            ..
        } = &mut *runtime;
        let mls_out = group
            .create_message(provider, signer, plaintext)
            .map_err(|e| format!("cached create_message: {e:?}"))?;
        let ciphertext = mls_out
            .tls_serialize_detached()
            .map_err(|e| format!("cached serialize MlsMessageOut: {e:?}"))?;

        runtime.mark_mutated();
        Ok(CachedEncryptResult {
            ciphertext,
            epoch: get_epoch(&runtime.group),
            state_version: runtime.state_version,
        })
    }

    pub fn decrypt_message_cached(
        &self,
        ctx: &CachedOperationContext,
        ciphertext: &[u8],
    ) -> Result<CachedDecryptResult, String> {
        let group_ref = self.group_ref(&ctx.group_id)?;
        let mut runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{}' runtime lock poisoned", ctx.group_id))?;
        runtime.validate_context(ctx)?;

        let mls_msg = MlsMessageIn::tls_deserialize_exact(ciphertext)
            .map_err(|e| format!("cached deserialize ciphertext: {e:?}"))?;
        let protocol_msg = mls_msg
            .try_into_protocol_message()
            .map_err(|e| format!("cached extract protocol message: {e:?}"))?;
        let GroupRuntime {
            group, provider, ..
        } = &mut *runtime;
        let processed = group
            .process_message(provider, protocol_msg)
            .map_err(|e| format!("cached process_message: {e:?}"))?;
        let plaintext = match processed.into_content() {
            ProcessedMessageContent::ApplicationMessage(app_msg) => app_msg.into_bytes(),
            ProcessedMessageContent::StagedCommitMessage(_) => {
                return Err("cached decrypt expected application message, got commit".into())
            }
            ProcessedMessageContent::ProposalMessage(_) => {
                return Err("cached decrypt expected application message, got proposal".into())
            }
            ProcessedMessageContent::ExternalJoinProposalMessage(_) => {
                return Err("cached decrypt expected application message, got external join".into())
            }
        };

        runtime.mark_mutated();
        Ok(CachedDecryptResult {
            plaintext,
            epoch: get_epoch(&runtime.group),
            state_version: runtime.state_version,
        })
    }

    pub fn create_update_commit_cached(
        &self,
        ctx: &CachedOperationContext,
    ) -> Result<CachedUpdateCommitResult, String> {
        self.create_update_commit_cached_profiled(ctx)
            .map(|profile| profile.result)
    }

    pub fn create_update_commit_cached_profiled(
        &self,
        ctx: &CachedOperationContext,
    ) -> Result<CachedUpdateCommitProfileResult, String> {
        let group_ref = self.group_ref(&ctx.group_id)?;
        let mut runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{}' runtime lock poisoned", ctx.group_id))?;
        runtime.validate_context(ctx)?;

        let GroupRuntime {
            group,
            provider,
            signer,
            ..
        } = &mut *runtime;
        let started = Instant::now();
        let bundle = group
            .self_update(provider, signer, LeafNodeParameters::default())
            .map_err(|e| format!("cached self_update: {e:?}"))?;
        let self_update = started.elapsed();
        let (commit_out, _welcome_out, _group_info) = bundle.into_contents();
        let started = Instant::now();
        group
            .merge_pending_commit(provider)
            .map_err(|e| format!("cached merge_pending_commit: {e:?}"))?;
        let merge_pending_commit = started.elapsed();
        let started = Instant::now();
        let commit_bytes = commit_out
            .tls_serialize_detached()
            .map_err(|e| format!("cached serialize commit: {e:?}"))?;
        let serialize_commit = started.elapsed();

        runtime.mark_mutated();
        let epoch = get_epoch(&runtime.group);
        Ok(CachedUpdateCommitProfileResult {
            result: CachedUpdateCommitResult {
                commit_bytes,
                tree_hash: compute_tree_hash(&runtime.group_id, epoch),
                epoch,
                state_version: runtime.state_version,
            },
            self_update,
            merge_pending_commit,
            serialize_commit,
        })
    }

    pub fn process_commit_cached(
        &self,
        ctx: &CachedOperationContext,
        commit_bytes: &[u8],
    ) -> Result<CachedProcessCommitResult, String> {
        let group_ref = self.group_ref(&ctx.group_id)?;
        let mut runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{}' runtime lock poisoned", ctx.group_id))?;
        runtime.validate_context(ctx)?;

        let mls_msg = MlsMessageIn::tls_deserialize_exact(commit_bytes)
            .map_err(|e| format!("cached deserialize commit: {e:?}"))?;
        let protocol_msg = mls_msg
            .try_into_protocol_message()
            .map_err(|e| format!("cached extract protocol message: {e:?}"))?;
        let GroupRuntime {
            group, provider, ..
        } = &mut *runtime;
        let processed = group
            .process_message(provider, protocol_msg)
            .map_err(|e| format!("cached process_message (commit): {e:?}"))?;
        match processed.into_content() {
            ProcessedMessageContent::StagedCommitMessage(staged_commit) => {
                group
                    .merge_staged_commit(provider, *staged_commit)
                    .map_err(|e| format!("cached merge_staged_commit: {e:?}"))?;
            }
            other => {
                return Err(format!(
                    "cached process commit expected StagedCommit, got {:?}",
                    std::mem::discriminant(&other)
                ))
            }
        }

        runtime.mark_mutated();
        let epoch = get_epoch(&runtime.group);
        Ok(CachedProcessCommitResult {
            tree_hash: compute_tree_hash(&runtime.group_id, epoch),
            epoch,
            state_version: runtime.state_version,
        })
    }

    pub fn export_secret_cached(
        &self,
        ctx: &CachedOperationContext,
        label: &str,
        context: &[u8],
        length: u32,
    ) -> Result<CachedExportSecretResult, String> {
        let group_ref = self.group_ref(&ctx.group_id)?;
        let runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{}' runtime lock poisoned", ctx.group_id))?;
        runtime.validate_context(ctx)?;
        let secret = runtime
            .group
            .export_secret(runtime.provider.crypto(), label, context, length as usize)
            .map_err(|e| format!("cached export_secret: {e:?}"))?;
        Ok(CachedExportSecretResult {
            secret,
            epoch: get_epoch(&runtime.group),
            state_version: runtime.state_version,
        })
    }

    pub fn export_checkpoint(&self, group_id: &str) -> Result<CachedCheckpointResult, String> {
        let group_ref = self.group_ref(group_id)?;
        let mut runtime = group_ref
            .lock()
            .map_err(|_| format!("group '{group_id}' runtime lock poisoned"))?;
        let epoch = get_epoch(&runtime.group);
        let group_state = export_state(
            &runtime.provider,
            &runtime.group_id,
            epoch,
            &runtime.signing_key,
        );
        runtime.dirty = false;
        Ok(CachedCheckpointResult {
            tree_hash: compute_tree_hash(&runtime.group_id, epoch),
            epoch,
            state_version: runtime.state_version,
            state_size_bytes: group_state.len() as u64,
            group_state,
        })
    }

    fn group_ref(&self, group_id: &str) -> Result<Arc<Mutex<GroupRuntime>>, String> {
        self.groups
            .get(group_id)
            .map(|entry| Arc::clone(entry.value()))
            .ok_or_else(|| format!("group '{group_id}' is not loaded"))
    }
}

impl GroupRuntime {
    fn validate_context(&self, ctx: &CachedOperationContext) -> Result<(), String> {
        if ctx.group_id != self.group_id {
            return Err(format!(
                "operation group_id mismatch: context='{}' runtime='{}'",
                ctx.group_id, self.group_id
            ));
        }
        if ctx.operation_id.trim().is_empty() {
            return Err("operation_id is required for cached hot-path RPCs".into());
        }
        let epoch = get_epoch(&self.group);
        if ctx.expected_epoch != epoch {
            return Err(format!(
                "epoch mismatch for group '{}': expected {}, actual {}",
                self.group_id, ctx.expected_epoch, epoch
            ));
        }
        if ctx.expected_state_version != self.state_version {
            return Err(format!(
                "state_version mismatch for group '{}': expected {}, actual {}",
                self.group_id, ctx.expected_state_version, self.state_version
            ));
        }
        Ok(())
    }

    fn mark_mutated(&mut self) {
        self.state_version = self.state_version.saturating_add(1);
        self.dirty = true;
    }

    fn metadata(&self) -> CachedGroupMetadata {
        let epoch = get_epoch(&self.group);
        let state_size_bytes =
            export_state(&self.provider, &self.group_id, epoch, &self.signing_key).len() as u64;
        CachedGroupMetadata {
            group_id: self.group_id.clone(),
            epoch,
            state_version: self.state_version,
            tree_hash: compute_tree_hash(&self.group_id, epoch),
            dirty: self.dirty,
            state_size_bytes,
        }
    }
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
        .build(CIPHERSUITE, &provider, &signer, credential_with_key)
        .map_err(|e| format!("KeyPackageBuilder::build: {e:?}"))?;

    let key_package_bytes = bundle
        .key_package()
        .tls_serialize_detached()
        .map_err(|e| format!("serialize KeyPackage: {e:?}"))?;

    let key_package_bundle_private =
        serde_json::to_vec(&bundle).map_err(|e| format!("serialize KeyPackageBundle: {e}"))?;

    Ok(GenerateKeyPackageResult {
        key_package_bytes,
        key_package_bundle_private,
    })
}

/// Remove one or more members from the group, identified by their
/// BasicCredential identity bytes. Used by Phase 6 group lifecycle (creator
/// removes member) and reusable from `create_commit` when buffered
/// `ProposalRemove` descriptors are flushed by the Token Holder.
///
/// Identity matching is a tree scan via `MlsGroup::member_leaf_index` — the
/// MLS-canonical resolution per RFC 9420. Welcome is always `None` for pure
/// remove, so the response carries an empty `welcome_bytes`.
///
/// Errors if any target identity cannot be located on the current ratchet
/// tree, or if `target_identities` is empty.
pub fn remove_members(
    group_state: &[u8],
    target_identities: &[Vec<u8>],
) -> Result<CommitResult, String> {
    let mut imp = import_state(group_state)?;

    if target_identities.is_empty() {
        return Err("no target identities".into());
    }

    let mut leaf_indices: Vec<LeafNodeIndex> = Vec::with_capacity(target_identities.len());
    for identity in target_identities {
        let credential: Credential = BasicCredential::new(identity.clone()).into();
        let idx = imp.group.member_leaf_index(&credential).ok_or_else(|| {
            format!(
                "remove_members: identity (len={}) not found in ratchet tree",
                identity.len()
            )
        })?;
        leaf_indices.push(idx);
    }

    let (commit_out, welcome_out, _group_info) = imp
        .group
        .remove_members(&imp.provider, &imp.signer, &leaf_indices)
        .map_err(|e| format!("remove_members: {e:?}"))?;

    imp.group
        .merge_pending_commit(&imp.provider)
        .map_err(|e| format!("merge_pending_commit: {e:?}"))?;

    let commit_bytes = commit_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize commit: {e:?}"))?;

    // Welcome is always None for pure removal; surface an empty Vec to keep
    // the wire format stable with add_members / create_commit.
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

/// Returns true if `identity` is present in the group's current ratchet tree as
/// a BasicCredential identity, false otherwise.
///
/// This is a pure state query used by the Go coordinator to determine whether
/// the local device has been removed after applying a commit.
pub fn has_member(group_state: &[u8], identity: &[u8]) -> Result<bool, String> {
    if identity.is_empty() {
        return Err("identity is required".into());
    }
    let imp = import_state(group_state)?;
    let credential: Credential = BasicCredential::new(identity.to_vec()).into();
    Ok(imp.group.member_leaf_index(&credential).is_some())
}

/// Enumerate the BasicCredential identity bytes for every leaf currently in
/// the MLS group. Used by the Go runtime to reconstruct the local roster
/// directly from MLS state (independent of which node sent the Welcome).
///
/// In this application, BasicCredential identity bytes are the leaf's MLS
/// signing public-key bytes. Returns an empty Vec when the group has no
/// members (should never happen for a valid persisted state but treated
/// defensively).
pub fn list_member_identities(group_state: &[u8]) -> Result<Vec<Vec<u8>>, String> {
    let imp = import_state(group_state)?;
    let identities: Vec<Vec<u8>> = imp
        .group
        .members()
        .map(|m| m.credential.serialized_content().to_vec())
        .collect();
    Ok(identities)
}

/// Add one or more members via their KeyPackages (commit + welcome in one step).
pub fn add_members(
    group_state: &[u8],
    key_packages_bytes: &[Vec<u8>],
) -> Result<CommitResult, String> {
    let started = Instant::now();
    let mut imp = import_state(group_state)?;
    eprintln!(
        "mls::add_members phase=import_state_done group_id={} epoch={} elapsed_ms={}",
        imp.group_id,
        get_epoch(&imp.group),
        started.elapsed().as_millis()
    );

    if key_packages_bytes.is_empty() {
        return Err("no key packages".into());
    }

    let kp_deserialize_started = Instant::now();
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
    eprintln!(
        "mls::add_members phase=keypackages_validated group_id={} count={} elapsed_ms={}",
        imp.group_id,
        key_packages.len(),
        kp_deserialize_started.elapsed().as_millis()
    );

    let add_started = Instant::now();
    let (commit_out, welcome_out, _group_info) = imp
        .group
        .add_members(&imp.provider, &imp.signer, &key_packages)
        .map_err(|e| format!("add_members: {e:?}"))?;
    eprintln!(
        "mls::add_members phase=openmls_add_members_done group_id={} new_epoch={} elapsed_ms={}",
        imp.group_id,
        get_epoch(&imp.group),
        add_started.elapsed().as_millis()
    );

    let merge_started = Instant::now();
    imp.group
        .merge_pending_commit(&imp.provider)
        .map_err(|e| format!("merge_pending_commit: {e:?}"))?;
    eprintln!(
        "mls::add_members phase=merge_pending_commit_done group_id={} epoch={} elapsed_ms={}",
        imp.group_id,
        get_epoch(&imp.group),
        merge_started.elapsed().as_millis()
    );

    let serialize_started = Instant::now();
    let commit_bytes = commit_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize commit: {e:?}"))?;

    let welcome_bytes = welcome_out
        .tls_serialize_detached()
        .map_err(|e| format!("serialize welcome: {e:?}"))?;

    let epoch = get_epoch(&imp.group);
    let new_tree_hash = compute_tree_hash(&imp.group_id, epoch);
    let new_state = export_state(&imp.provider, &imp.group_id, epoch, &imp.signing_key);
    eprintln!(
        "mls::add_members phase=serialized group_id={} epoch={} commit_bytes={} welcome_bytes={} state_bytes={} elapsed_ms={} total_ms={}",
        imp.group_id,
        epoch,
        commit_bytes.len(),
        welcome_bytes.len(),
        new_state.len(),
        serialize_started.elapsed().as_millis(),
        started.elapsed().as_millis()
    );

    Ok(CommitResult {
        commit_bytes,
        welcome_bytes,
        new_group_state: new_state,
        new_tree_hash,
    })
}

pub fn create_group(group_id: &str, signing_key: &[u8]) -> Result<CreateGroupResult, String> {
    let provider = OpenMlsRustCrypto::default();
    let signer = reconstruct_signer(&provider, signing_key)?;

    // Keep identity scheme consistent with key packages/external join:
    // BasicCredential identity is always the MLS signing public key bytes.
    let credential = BasicCredential::new(signer.to_public_vec());
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

pub fn encrypt_message(group_state: &[u8], plaintext: &[u8]) -> Result<EncryptResult, String> {
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

pub fn decrypt_message(group_state: &[u8], ciphertext: &[u8]) -> Result<DecryptResult, String> {
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

pub fn create_commit(group_state: &[u8], proposals: &[Vec<u8>]) -> Result<CommitResult, String> {
    if proposals.is_empty() {
        return Err("no proposals to commit".into());
    }

    // Classify the batch. The Go coordination layer funnels homogeneous batches
    // per epoch (Add / Remove / Update separately) because the Token Holder
    // commits one proposal class at a time; mixed batches are rejected here
    // to surface coordination-layer regressions early.
    let mut update_count = 0u32;
    let mut remove_targets: Vec<Vec<u8>> = Vec::new();
    let mut add_key_packages: Vec<Vec<u8>> = Vec::new();
    for raw in proposals {
        let desc: ProposalDescriptor =
            serde_json::from_slice(raw).map_err(|e| format!("parse proposal descriptor: {e}"))?;
        match desc.proposal_type {
            2 => update_count += 1,                // ProposalUpdate
            1 => remove_targets.push(desc.data),   // ProposalRemove → identity bytes
            0 => add_key_packages.push(desc.data), // ProposalAdd → KeyPackage bytes
            _ => return Err(format!("unknown proposal type: {}", desc.proposal_type)),
        }
    }

    let kinds = (update_count > 0) as u8
        + (!remove_targets.is_empty() as u8)
        + (!add_key_packages.is_empty() as u8);
    if kinds > 1 {
        return Err("create_commit: mixed proposal batches not supported".into());
    }

    if !add_key_packages.is_empty() {
        // Token Holder applying buffered ProposalAdd descriptors. Reuse the
        // dedicated add_members path so commit + welcome generation stays
        // identical to the direct add_members RPC.
        return add_members(group_state, &add_key_packages);
    }

    if !remove_targets.is_empty() {
        // Delegate to the dedicated remove path so persistence/wire shapes
        // stay identical regardless of whether the caller is the Token Holder
        // committing buffered ProposalRemove descriptors or invoking the
        // direct RemoveMembers RPC.
        return remove_members(group_state, &remove_targets);
    }

    if update_count == 0 {
        return Err("no supported proposals to commit".into());
    }

    let mut imp = import_state(group_state)?;

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

/// Exports a verifiable GroupInfo for the current group state, signed by the
/// caller's signing key. Used by the winning branch during fork healing so
/// peers on the losing branch can re-join via [`external_join`] without
/// rebuilding the full ratchet tree out-of-band.
///
/// When `with_ratchet_tree` is true (recommended for fork healing), the
/// returned MlsMessage embeds a `RatchetTreeExtension`, allowing the
/// receiver to construct the public group from the GroupInfo alone. Returns
/// the TLS-serialized `MlsMessageOut` bytes.
pub fn export_group_info(group_state: &[u8], with_ratchet_tree: bool) -> Result<Vec<u8>, String> {
    let imp = import_state(group_state)?;

    let group_info_msg = imp
        .group
        .export_group_info(imp.provider.crypto(), &imp.signer, with_ratchet_tree)
        .map_err(|e| format!("export_group_info: {e:?}"))?;

    let bytes = group_info_msg
        .tls_serialize_detached()
        .map_err(|e| format!("serialize group_info: {e:?}"))?;

    Ok(bytes)
}

/// Performs an MLS External Commit into a target group given its signed
/// `GroupInfo`. Used during fork healing: the losing branch drops its old
/// MlsGroup and re-joins the winning branch by issuing an external commit
/// at the winner's current epoch.
///
/// Forward Secrecy of the joining party's old leaf in the winner's tree is
/// preserved automatically: OpenMLS's `ExternalCommitBuilder` injects a
/// `Remove` proposal for any existing leaf that shares the new joiner's
/// signature key (see openmls 0.8.0 `external_commits.rs:249-255`).
pub fn external_join(group_info: &[u8], signing_key: &[u8]) -> Result<ExternalJoinResult, String> {
    let provider = OpenMlsRustCrypto::default();
    let signer = reconstruct_signer(&provider, signing_key)?;

    let mls_msg = MlsMessageIn::tls_deserialize_exact(group_info)
        .map_err(|e| format!("deserialize group_info: {e:?}"))?;
    let verifiable_group_info = match mls_msg.extract() {
        MlsMessageBodyIn::GroupInfo(gi) => gi,
        _ => return Err("expected MlsMessageBodyIn::GroupInfo body".into()),
    };

    let credential = BasicCredential::new(signer.to_public_vec());
    let credential_with_key = CredentialWithKey {
        credential: credential.into(),
        signature_key: signer.to_public_vec().into(),
    };

    let (group, bundle) = MlsGroup::external_commit_builder()
        .with_config(
            MlsGroupJoinConfig::builder()
                .use_ratchet_tree_extension(true)
                .build(),
        )
        .build_group(&provider, verifiable_group_info, credential_with_key)
        .map_err(|e| format!("external_commit_builder.build_group: {e:?}"))?
        .load_psks(provider.storage())
        .map_err(|e| format!("external_commit_builder.load_psks: {e:?}"))?
        .build(provider.rand(), provider.crypto(), &signer, |_| true)
        .map_err(|e| format!("external_commit_builder.build: {e:?}"))?
        .finalize(&provider)
        .map_err(|e| format!("external_commit_builder.finalize: {e:?}"))?;

    let (commit_msg, _welcome_opt, _group_info_opt) = bundle.into_messages();
    let commit_bytes = commit_msg
        .tls_serialize_detached()
        .map_err(|e| format!("serialize external commit: {e:?}"))?;

    let group_id = String::from_utf8_lossy(group.group_id().as_slice()).to_string();
    let epoch = get_epoch(&group);
    let tree_hash = compute_tree_hash(&group_id, epoch);
    let new_state = export_state(&provider, &group_id, epoch, signing_key);

    Ok(ExternalJoinResult {
        group_state: new_state,
        commit_bytes,
        tree_hash,
    })
}

pub fn export_secret(
    group_state: &[u8],
    label: &str,
    context: &[u8],
    length: u32,
) -> Result<Vec<u8>, String> {
    let imp = import_state(group_state)?;

    let secret = imp
        .group
        .export_secret(imp.provider.crypto(), label, context, length as usize)
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

        let persisted: PersistedGroupState = serde_json::from_slice(&result.group_state).unwrap();
        assert_eq!(persisted.group_id, "test-group-1");
        assert_eq!(persisted.epoch, 0);
        assert_eq!(persisted.version, STATE_VERSION);
        assert!(!persisted.entries.is_empty());
    }

    #[test]
    fn test_create_group_local_identity_is_member() {
        let sk = test_signing_key();
        let group = create_group("test-group-local-member", &sk).expect("create_group");
        let id = invitee_identity_bytes(&sk);
        let is_member = has_member(&group.group_state, &id).expect("has_member");
        assert!(
            is_member,
            "creator signing public key must be present in group"
        );
    }

    #[test]
    fn test_encrypt_message() {
        let sk = test_signing_key();
        let cr = create_group("test-encrypt", &sk).expect("create_group");

        let enc = encrypt_message(&cr.group_state, b"Hello, MLS!").expect("encrypt_message");
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

        let secret = export_secret(&cr.group_state, "test-label", &[], 32).expect("export_secret");
        assert_eq!(secret.len(), 32);

        let ctx_a = b"context-a";
        let ctx_b = b"context-b";
        let sa = export_secret(&cr.group_state, "test-label", ctx_a, 32).expect("export a");
        let sb = export_secret(&cr.group_state, "test-label", ctx_b, 32).expect("export b");
        assert_ne!(
            sa, sb,
            "different exporter context must yield different secrets"
        );
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

        let commit =
            add_members(&cr.group_state, &[kp_b.key_package_bytes.clone()]).expect("add_members");

        let welcome_b = process_welcome(
            &commit.welcome_bytes,
            &sk_b,
            &kp_b.key_package_bundle_private,
        )
        .expect("process_welcome B");

        let enc_a = encrypt_message(&commit.new_group_state, b"hello from A").expect("encrypt A");
        let dec_b = decrypt_message(&welcome_b.group_state, &enc_a.ciphertext).expect("decrypt B");
        assert_eq!(dec_b.plaintext, b"hello from A");
    }

    /// ProposalAdd routed through `create_commit` (i.e. the Token Holder
    /// flushing a buffered ProposalAdd) must produce the same Commit+Welcome
    /// shape as `add_members`, and the invitee must be able to join via the
    /// resulting Welcome. This pins the Single-Writer flow where any member
    /// proposes Add but only the Token Holder issues the commit.
    #[test]
    fn test_create_commit_from_proposal_add() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let cr = create_group("proposal-add-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package for B");

        // Wrap the public KeyPackage in the same ProposalDescriptor envelope
        // any non-holder would broadcast on GossipSub.
        let proposal_add =
            create_proposal(&cr.group_state, 0, &kp_b.key_package_bytes).expect("create_proposal");

        let commit = create_commit(&cr.group_state, &[proposal_add]).expect("create_commit");

        assert!(
            !commit.commit_bytes.is_empty(),
            "ProposalAdd commit must produce commit bytes"
        );
        assert!(
            !commit.welcome_bytes.is_empty(),
            "ProposalAdd commit MUST produce a Welcome for the invitee"
        );

        let welcome_b = process_welcome(
            &commit.welcome_bytes,
            &sk_b,
            &kp_b.key_package_bundle_private,
        )
        .expect("process_welcome B");

        let enc_a =
            encrypt_message(&commit.new_group_state, b"hello via proposal").expect("encrypt A");
        let dec_b = decrypt_message(&welcome_b.group_state, &enc_a.ciphertext).expect("decrypt B");
        assert_eq!(dec_b.plaintext, b"hello via proposal");
    }

    /// Mixing proposal types in a single batch must be rejected — the Go
    /// coordination layer is responsible for splitting by type.
    #[test]
    fn test_create_commit_rejects_mixed_batch() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let cr = create_group("mixed-batch-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("kp B");

        let add_prop = create_proposal(&cr.group_state, 0, &kp_b.key_package_bytes).expect("add");
        let upd_prop = create_proposal(&cr.group_state, 2, &[]).expect("update");

        let err = match create_commit(&cr.group_state, &[add_prop, upd_prop]) {
            Ok(_) => panic!("mixed batch must be rejected"),
            Err(e) => e,
        };
        assert!(
            err.contains("mixed proposal batches"),
            "unexpected error: {err}"
        );
    }

    #[test]
    fn test_export_group_info_roundtrip() {
        let sk = test_signing_key();
        let cr = create_group("export-info-group", &sk).expect("create_group");

        let group_info = export_group_info(&cr.group_state, true).expect("export_group_info");
        assert!(
            !group_info.is_empty(),
            "exported GroupInfo should be non-empty"
        );

        // Re-export must succeed (stateless): the underlying group state is unchanged.
        let group_info_2 = export_group_info(&cr.group_state, true).expect("export_group_info #2");
        assert!(!group_info_2.is_empty());
    }

    /// Sprint 2A happy-path: simulates fork healing where node B (losing branch)
    /// abandons its old group and re-joins node A's branch via External Commit.
    /// After heal, A and B must converge to the same epoch, and B must be able to
    /// encrypt application messages that A can decrypt at the new epoch.
    #[test]
    fn test_external_join_fork_heal_happy_path() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let group_a = create_group("fork-heal-group", &sk_a).expect("A create_group");

        // A exports a GroupInfo at the current epoch (winning branch perspective).
        let group_info =
            export_group_info(&group_a.group_state, true).expect("A export_group_info");
        assert!(!group_info.is_empty());

        // B (losing branch) external-joins A's branch using the GroupInfo.
        let join_b = external_join(&group_info, &sk_b).expect("B external_join");
        assert!(
            !join_b.group_state.is_empty(),
            "B group_state must be non-empty"
        );
        assert!(
            !join_b.commit_bytes.is_empty(),
            "B external commit bytes must be non-empty"
        );
        assert!(!join_b.tree_hash.is_empty());

        // A processes B's external commit to advance its own state to the joined epoch.
        let commit_a = process_commit(&group_a.group_state, &join_b.commit_bytes)
            .expect("A process_commit(external)");
        assert!(!commit_a.new_group_state.is_empty());

        // B encrypts a message at the new epoch.
        let enc_b = encrypt_message(&join_b.group_state, b"hello from rejoined B")
            .expect("B encrypt after external join");
        assert!(!enc_b.ciphertext.is_empty());

        // A (now at the post-external-commit epoch) decrypts B's message.
        let dec_a =
            decrypt_message(&commit_a.new_group_state, &enc_b.ciphertext).expect("A decrypt B");
        assert_eq!(dec_a.plaintext, b"hello from rejoined B");
    }

    #[test]
    fn test_external_join_rejects_invalid_group_info() {
        let sk = test_signing_key();
        // Random bytes are not a valid TLS-encoded MlsMessage.
        let result = external_join(&[0xDE, 0xAD, 0xBE, 0xEF], &sk);
        assert!(
            result.is_err(),
            "external_join must reject malformed group_info"
        );
    }

    /// Helper that mirrors `generate_key_package`: BasicCredential identity is the
    /// signer's public key bytes. Used by remove_members tests to look up
    /// invitee leaf indices.
    fn invitee_identity_bytes(signing_key: &[u8]) -> Vec<u8> {
        let provider = OpenMlsRustCrypto::default();
        let signer = reconstruct_signer(&provider, signing_key).expect("reconstruct_signer");
        signer.to_public_vec()
    }

    /// Sprint 3A happy path: 2-member group → remove the invitee → epoch advances,
    /// remaining state is non-empty, no welcome was emitted.
    #[test]
    fn test_remove_members_happy_path() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let group_a = create_group("rm-happy-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package B");

        let commit_add = add_members(&group_a.group_state, &[kp_b.key_package_bytes.clone()])
            .expect("add_members");

        let target = invitee_identity_bytes(&sk_b);
        let commit_rm =
            remove_members(&commit_add.new_group_state, &[target]).expect("remove_members");

        assert!(
            !commit_rm.commit_bytes.is_empty(),
            "remove commit bytes must be non-empty"
        );
        assert!(
            commit_rm.welcome_bytes.is_empty(),
            "pure remove must not emit a Welcome"
        );
        assert!(
            !commit_rm.new_group_state.is_empty(),
            "remove must produce a new group state"
        );
        // Tree hash must change between the add-epoch and the remove-epoch.
        assert_ne!(
            commit_rm.new_tree_hash, commit_add.new_tree_hash,
            "remove must advance the epoch (different tree hash)"
        );
    }

    /// Forward secrecy: after remove, the removed member cannot decrypt
    /// subsequent messages produced under the new epoch state by the surviving
    /// group. Their pre-remove ciphertext path is irrelevant to this check —
    /// what matters is that the post-remove epoch keys are unreachable to them.
    #[test]
    fn test_remove_members_forward_secrecy() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let group_a = create_group("rm-fs-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package B");

        let commit_add = add_members(&group_a.group_state, &[kp_b.key_package_bytes.clone()])
            .expect("add_members");
        let join_b = process_welcome(
            &commit_add.welcome_bytes,
            &sk_b,
            &kp_b.key_package_bundle_private,
        )
        .expect("process_welcome B");

        // B's view at the add-epoch is captured; we will try to use it after A removes B.
        let b_state_pre_remove = join_b.group_state.clone();

        let target = invitee_identity_bytes(&sk_b);
        let commit_rm =
            remove_members(&commit_add.new_group_state, &[target]).expect("remove_members");

        // A encrypts a message at the post-remove epoch.
        let enc = encrypt_message(&commit_rm.new_group_state, b"after-remove")
            .expect("encrypt post-remove");

        // B's stale state must not decrypt this — forward secrecy on remove.
        let dec = decrypt_message(&b_state_pre_remove, &enc.ciphertext);
        assert!(
            dec.is_err(),
            "removed member must not decrypt post-remove epoch ciphertext"
        );
    }

    /// Identity that is not present in the ratchet tree must be rejected with
    /// a clear error rather than committing a no-op.
    #[test]
    fn test_remove_members_unknown_identity() {
        let sk_a = test_signing_key();
        let group_a = create_group("rm-unknown-group", &sk_a).expect("create_group");

        // 32-byte identity that no member ever advertised.
        let bogus = vec![0xAB; 32];
        let result = remove_members(&group_a.group_state, &[bogus]);
        assert!(result.is_err(), "must reject unknown identity");
        if let Err(msg) = result {
            assert!(
                msg.contains("not found"),
                "expected 'not found' error, got: {msg}"
            );
        }
    }

    /// `create_commit` is used by the Token Holder path in the coordination
    /// layer. Buffered ProposalRemove descriptors must be committable end-to-end
    /// without any out-of-band knowledge of leaf indices.
    #[test]
    fn test_create_commit_with_remove_proposals() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let group_a = create_group("rm-commit-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package B");
        let commit_add = add_members(&group_a.group_state, &[kp_b.key_package_bytes.clone()])
            .expect("add_members");

        let target = invitee_identity_bytes(&sk_b);
        let proposal_descriptor =
            create_proposal(&commit_add.new_group_state, 1, &target).expect("create_proposal");

        let commit_rm = create_commit(&commit_add.new_group_state, &[proposal_descriptor])
            .expect("create_commit with ProposalRemove");

        assert!(commit_rm.welcome_bytes.is_empty());
        assert_ne!(commit_rm.new_tree_hash, commit_add.new_tree_hash);
    }

    #[test]
    fn test_has_member_true_and_false_after_remove() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();

        let group_a = create_group("has-member-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package B");
        let commit_add = add_members(&group_a.group_state, &[kp_b.key_package_bytes.clone()])
            .expect("add_members");

        let id_b = invitee_identity_bytes(&sk_b);
        let before = has_member(&commit_add.new_group_state, &id_b).expect("has_member before");
        assert!(before, "invitee should be present before remove");

        let commit_rm =
            remove_members(&commit_add.new_group_state, &[id_b.clone()]).expect("remove_members");
        let after = has_member(&commit_rm.new_group_state, &id_b).expect("has_member after");
        assert!(!after, "invitee should be absent after remove");
    }

    /// list_member_identities must return one entry per leaf and the entries
    /// must match the BasicCredential identity bytes computed independently
    /// via the signing key (the same scheme has_member queries against).
    #[test]
    fn test_list_member_identities_after_add_members() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();
        let sk_c = test_signing_key();

        let group_a = create_group("list-leaves-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package B");
        let kp_c = generate_key_package(&sk_c).expect("generate_key_package C");
        let commit_ab = add_members(&group_a.group_state, &[kp_b.key_package_bytes.clone()])
            .expect("add_members B");
        let commit_abc = add_members(
            &commit_ab.new_group_state,
            &[kp_c.key_package_bytes.clone()],
        )
        .expect("add_members C");

        let ids =
            list_member_identities(&commit_abc.new_group_state).expect("list_member_identities");
        assert_eq!(ids.len(), 3, "group must have exactly 3 leaves");

        let id_a = invitee_identity_bytes(&sk_a);
        let id_b = invitee_identity_bytes(&sk_b);
        let id_c = invitee_identity_bytes(&sk_c);
        let mut sorted = ids.clone();
        sorted.sort();
        let mut expected = vec![id_a, id_b, id_c];
        expected.sort();
        assert_eq!(sorted, expected, "leaf identities must match signing keys");

        // Each enumerated identity must round-trip through has_member.
        for id in &ids {
            assert!(
                has_member(&commit_abc.new_group_state, id).expect("has_member round-trip"),
                "identity from list must be a member"
            );
        }
    }

    /// list_member_identities reflects removals immediately after the commit.
    #[test]
    fn test_list_member_identities_after_remove() {
        let sk_a = test_signing_key();
        let sk_b = test_signing_key();
        let group_a = create_group("list-leaves-remove-group", &sk_a).expect("create_group");
        let kp_b = generate_key_package(&sk_b).expect("generate_key_package B");
        let commit_add = add_members(&group_a.group_state, &[kp_b.key_package_bytes.clone()])
            .expect("add_members");
        let id_b = invitee_identity_bytes(&sk_b);
        let commit_rm =
            remove_members(&commit_add.new_group_state, &[id_b.clone()]).expect("remove_members");
        let ids = list_member_identities(&commit_rm.new_group_state)
            .expect("list_member_identities after remove");
        assert_eq!(ids.len(), 1, "only creator must remain");
        let id_a = invitee_identity_bytes(&sk_a);
        assert_eq!(ids[0], id_a, "remaining leaf must be creator");
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

    #[test]
    fn test_cached_load_encrypt_checkpoint_roundtrip() {
        let sk = test_signing_key();
        let created = create_group("cached-roundtrip", &sk).expect("create_group");
        let cache = RuntimeCache::default();
        let meta = cache
            .load_group("cached-roundtrip", &created.group_state, 0)
            .expect("load_group");

        let encrypted = cache
            .encrypt_message_cached(
                &CachedOperationContext {
                    group_id: "cached-roundtrip".into(),
                    expected_epoch: meta.epoch,
                    expected_state_version: meta.state_version,
                    operation_id: "encrypt-1".into(),
                },
                b"hello",
            )
            .expect("encrypt cached");
        assert!(!encrypted.ciphertext.is_empty());
        assert_eq!(encrypted.epoch, 0);
        assert_eq!(encrypted.state_version, 1);

        let checkpoint = cache
            .export_checkpoint("cached-roundtrip")
            .expect("checkpoint");
        assert_eq!(checkpoint.epoch, encrypted.epoch);
        assert_eq!(checkpoint.state_version, encrypted.state_version);
        assert!(!checkpoint.group_state.is_empty());

        let reloaded = import_state(&checkpoint.group_state).expect("import checkpoint");
        assert_eq!(reloaded.group_id, "cached-roundtrip");
        assert_eq!(get_epoch(&reloaded.group), encrypted.epoch);
    }

    #[test]
    fn test_cached_rejects_stale_state_version() {
        let sk = test_signing_key();
        let created = create_group("cached-version-fence", &sk).expect("create_group");
        let cache = RuntimeCache::default();
        cache
            .load_group("cached-version-fence", &created.group_state, 7)
            .expect("load_group");

        let err = cache
            .encrypt_message_cached(
                &CachedOperationContext {
                    group_id: "cached-version-fence".into(),
                    expected_epoch: 0,
                    expected_state_version: 6,
                    operation_id: "stale-version".into(),
                },
                b"hello",
            )
            .expect_err("stale version must be rejected");
        assert!(err.contains("state_version mismatch"));
    }

    #[test]
    fn test_cached_unload_group() {
        let sk = test_signing_key();
        let created = create_group("cached-unload", &sk).expect("create_group");
        let cache = RuntimeCache::default();
        cache
            .load_group("cached-unload", &created.group_state, 0)
            .expect("load_group");

        assert!(cache.unload_group("cached-unload"));
        assert!(cache.metadata("cached-unload").is_err());
    }
}
