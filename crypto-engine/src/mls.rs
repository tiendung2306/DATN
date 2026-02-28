// openmls::prelude re-exports OpenMlsProvider (and thus brings .crypto() into scope).
use openmls::prelude::*;
use openmls_rust_crypto::OpenMlsRustCrypto;
// Needed to call methods on the value returned by .crypto().
use openmls_traits::crypto::OpenMlsCrypto;

/// Output of a successful identity generation.
pub struct GeneratedIdentity {
    /// Raw Ed25519 private key bytes.
    /// Stored in DB; never transmitted over the network.
    /// In Phase 4, will be passed back to Rust to reconstruct a SignatureKeyPair
    /// for signing MLS Commit/Application messages.
    pub signing_key_private: Vec<u8>,

    /// Raw Ed25519 public key bytes (32 bytes).
    /// Included in `InvitationToken` for identity binding — send to Admin.
    pub public_key: Vec<u8>,

    /// Raw UTF-8 bytes of display_name.
    /// Stored in DB as the MLS identity label.
    /// In Phase 4, will be wrapped in a proper TLS-serialized BasicCredential.
    pub credential: Vec<u8>,
}

/// Generates a new MLS Ed25519 key pair for this node.
///
/// `display_name` is intentionally left empty at setup time — the actual name
/// is assigned by the Admin when they create the InvitationBundle (CSR model).
/// Go updates `mls_identity.credential` in SQLite after the bundle is imported.
///
/// This is a **pure function** — no state is stored inside Rust.
/// The caller (Go) is responsible for persisting all returned bytes in SQLite.
pub fn generate_identity() -> Result<GeneratedIdentity, String> {
    let provider = OpenMlsRustCrypto::default();

    // Use OpenMlsCrypto::signature_key_gen to produce raw Ed25519 key bytes.
    // Returns (private_key_bytes, public_key_bytes).
    // Ed25519 is the signature scheme for the standard MLS ciphersuite
    // MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519.
    let (signing_key_private, public_key) = provider
        .crypto()
        .signature_key_gen(SignatureScheme::ED25519)
        .map_err(|e| format!("Failed to generate Ed25519 key pair: {e:?}"))?;

    // Credential is empty at generation time.
    // Phase 4 will build a proper TLS-serialized BasicCredential(display_name)
    // using the name assigned by Admin in the InvitationToken.
    Ok(GeneratedIdentity {
        signing_key_private,
        public_key,
        credential: Vec::new(),
    })
}
