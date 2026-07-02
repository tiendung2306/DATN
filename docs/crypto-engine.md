# Crypto Engine (Rust) & gRPC Protocol

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md) · [Adapter Layer → Sidecar](adapter-layer.md#sidecar-adapter-adaptersidecar) · [Security & Protocol](security-protocol.md)

## Crypto Engine

**Vị trí:** `crypto-engine/`  
**Vai trò:** Pure MLS computation — stateless black box. Nhận group state bytes → thực hiện MLS operation → trả về new state bytes. Không biết về coordination, epochs, Single-Writer, hay fork healing.

### Dependencies (`Cargo.toml`)

```toml
[dependencies]
openmls = "0.8.0"              # MLS implementation (RFC 9420)
openmls_rust_crypto = "0.5"    # Default crypto provider (RustCrypto)
openmls_basic_credential = "0.5"  # Basic credential type
ed25519-dalek = "2"            # Ed25519 signatures
tonic = "0.14.3"               # gRPC framework
prost = "0.13"                 # Protocol buffers
tokio = "1"                    # Async runtime
clap = "4.4"                   # CLI args (--port)
dashmap = "6"                  # Concurrent HashMap (cached path only)
serde = { version = "1", features = ["derive"] }
serde_json = "1"               # JSON serialization for PersistedGroupState
tls_codec = "0.4"              # TLS encoding for MLS types
```

### gRPC Server (`main.rs`)

`MyMlsService` implements `MlsCryptoService` trait (generated from proto):

```rust
struct MyMlsService {
    cache: Arc<RuntimeCache>,  // For cached RPCs (benchmark only)
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Cli::parse();
    let addr = format!("127.0.0.1:{}", args.port);
    let svc = MyMlsService { cache: Arc::new(RuntimeCache::new()) };
    Server::builder()
        .max_decoding_message_size(64 * 1024 * 1024)  // 64 MiB
        .add_service(MlsCryptoServiceServer::new(svc))
        .serve(addr.parse()?)
        .await
}
```

**Key design points:**
- Bind `127.0.0.1` only — không expose ra network, chỉ Go local process connect
- Max message size 64 MiB — accommodate large group states (ratchet tree)
- `clap::Parser` cho `--port` flag (default 50051)

### MLS Operations (`mls.rs`)

Core MLS logic — 2173 lines, 30+ unit tests:

**Ciphersuite:** `MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519`
- **KEM:** X25519 (Diffie-Hellman key exchange)
- **AEAD:** AES-128-GCM (authenticated encryption)
- **Hash:** SHA-256
- **Signature:** Ed25519

**PersistedGroupState** — serializable snapshot của MLS group:

```rust
#[derive(Serialize, Deserialize)]
struct PersistedGroupState {
    group: Vec<u8>,           // TLS-encoded MLS group
    signing_key: Vec<u8>,     // Ed25519 private key (seed)
    credential: Vec<u8>,      // BasicCredential
    max_past_epochs: u32,     // Key retention config
}
```

**Stateless pattern** — mỗi RPC tuân theo:
```
1. import_state(group_state_bytes) → PersistedGroupState
2. Deserialize → rebuild MLS group + signer + provider
3. Perform MLS operation (CreateProposal, EncryptMessage, etc.)
4. export_state() → serialize new PersistedGroupState
5. Return new_group_state bytes
```

| Function | Mục đích |
|----------|----------|
| `import_state(bytes)` | Deserialize PersistedGroupState from JSON |
| `export_state(group, signer, ...)` | Serialize PersistedGroupState to JSON |
| `create_group(signing_key, max_past_epochs)` | Tạo MLS group mới |
| `create_proposal(state, proposal_type, target_kp)` | Tạo Add/Remove/Update proposal |
| `process_proposal(state, proposal_bytes)` | Process proposal vào group state |
| `create_commit(state, proposals)` | Tạo commit từ queued proposals |
| `stage_commit(state, commit, proposals)` | Stage commit — verify proposal refs |
| `process_commit(state, commit, proposals)` | Áp dụng commit → new state + tree hash |
| `process_welcome(welcome, signing_key, kp_private)` | Join group từ Welcome |
| `encrypt_message(state, plaintext)` | MLS application encryption |
| `decrypt_message(state, ciphertext)` | MLS application decryption |
| `external_join(group_info, signing_key)` | Join group qua ExternalJoin (fork healing) |
| `export_group_info(state, with_ratchet_tree)` | Export GroupInfo cho fork healing |
| `export_secret(state, label, context, length)` | MLS key exporter (file transfer encryption) |
| `generate_key_package(signing_key)` | Tạo KeyPackage cho invite |
| `add_members(state, key_packages)` | Add members → commit + welcome |
| `remove_members(state, identities)` | Remove members → commit |
| `has_member(state, identity)` | Check membership |
| `list_member_identities(state)` | List all member identities |

**Cached path (benchmark only):**

```rust
struct RuntimeCache {
    groups: DashMap<String, GroupRuntime>,
}

struct GroupRuntime {
    provider: OpenMlsRustCrypto,
    signer: Signer,
    group: MlsGroup,
    state_version: u64,
    dirty: bool,
}
```

- `LoadGroup` — import state + cache in DashMap
- Cached RPCs operate on in-memory `GroupRuntime` — skip serialization
- `UnloadGroup` — export state + remove from cache
- OCC validation: `state_version` check — reject if caller's version stale

### Build (`build.rs`)

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_prost_build::configure()
        .build_server(true)
        .build_client(false)  // Rust chỉ là server, Go là client
        .compile_protos(&["../proto/mls_service.proto"], &["../proto"])?;
    Ok(())
}
```

Generate gRPC server code từ `proto/mls_service.proto` → `OUT_DIR/mls_service.rs`.

### Benchmark Binary (`src/bin/mls_bench.rs`)

Standalone benchmark binary cho MLS optimization research:
- Measure stateless vs cached performance
- Group sizes: 16, 64, 256, 1024, 4096 members
- Operations: CreateCommit, ProcessCommit, EncryptMessage, DecryptMessage
- Output: CSV metrics cho evaluation scripts

---

## gRPC Protocol

**Vị trí:** `proto/mls_service.proto`

Service `MLSCryptoService` với 30 RPCs, chia thành 3 nhóm:

### Phase 2: Identity (4 RPCs)

| RPC | Request | Response | Mục đích |
|-----|---------|----------|----------|
| `Ping` | `PingRequest{}` | `PingResponse{message, timestamp}` | Health check |
| `GenerateIdentity` | `GenerateIdentityRequest{display_name}` | `GenerateIdentityResponse{signing_key_private, public_key, credential}` | Tạo Ed25519 key pair |
| `ExportIdentity` | `ExportIdentityRequest{key_package, passphrase}` | `ExportIdentityResponse{encrypted_backup_data}` | Export encrypted identity backup |
| `ImportIdentity` | `ImportIdentityRequest{encrypted_backup_data, passphrase}` | `ImportIdentityResponse{key_package}` | Import identity từ encrypted backup |

### Phase 4: Group Operations — Stateless (17 RPCs)

| RPC | Mục đích |
|-----|----------|
| `CreateGroup` | Tạo MLS group với signing key + max_past_epochs |
| `CreateProposal` | Tạo proposal (Add/Remove/Update) từ group state |
| `ProcessProposal` | Process proposal vào group state (stage) |
| `CreateCommit` | Tạo commit từ expected proposal refs |
| `StageCommit` | Stage commit — verify proposal refs match |
| `ProcessCommit` | Áp dụng commit → new state + tree hash |
| `ProcessWelcome` | Join group từ Welcome message |
| `EncryptMessage` | MLS application encryption (ciphertext) |
| `DecryptMessage` | MLS application decryption (plaintext) |
| `ExternalJoin` | Join group qua GroupInfo (fork healing) |
| `ExportSecret` | MLS key exporter (file transfer, custom keys) |
| `GenerateKeyPackage` | Tạo KeyPackage cho invite flow |
| `AddMembers` | Add members → commit + welcome + new state |
| `RemoveMembers` | Remove members → commit + new state |
| `HasMember` | Check membership |
| `ListMemberIdentities` | List all member identities |
| `ExportGroupInfo` | Export GroupInfo (with optional ratchet tree) |

**Stateless pattern:** Mọi RPC nhận `group_state: bytes` và trả về `new_group_state: bytes`. Rust không lưu state giữa các RPC calls.

### Phase A: Optimization Research — Cached (9 RPCs)

| RPC | Mục đích |
|-----|----------|
| `LoadGroup` | Load group vào in-memory cache (DashMap) |
| `UnloadGroup` | Unload group từ cache → export state |
| `GetGroupMetadata` | Query cached group metadata (epoch, member count, tree hash) |
| `EncryptMessageCached` | Encrypt using cached group (skip serialization) |
| `DecryptMessageCached` | Decrypt using cached group |
| `CreateUpdateCommitCached` | Create update commit (key rotation) using cached group |
| `ProcessCommitCached` | Process commit using cached group |
| `ExportSecretCached` | Export secret using cached group |
| `ExportGroupStateCheckpoint` | Export checkpoint of cached group state |

**Cached path design:**
- `RuntimeCache` (DashMap) giữ `GroupRuntime` per group ID
- OCC (Optimistic Concurrency Control): `state_version` check — reject if caller's version stale
- `dirty` flag: mark when group modified, require export before unload
- Benchmark only — production uses stateless path
