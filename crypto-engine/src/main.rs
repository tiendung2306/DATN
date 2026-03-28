use std::sync::Arc;

use chrono::Utc;
use clap::Parser;
use tonic::{transport::Server, Request, Response, Status};

mod mls;

pub mod mls_service {
    tonic::include_proto!("mls_service");
}

use mls_service::mls_crypto_service_server::{MlsCryptoService, MlsCryptoServiceServer};
use mls_service::{
    CreateCommitRequest, CreateCommitResponse, CreateGroupRequest, CreateGroupResponse,
    CreateProposalRequest, CreateProposalResponse, DecryptMessageRequest, DecryptMessageResponse,
    EncryptMessageRequest, EncryptMessageResponse, ExportIdentityRequest, ExportIdentityResponse,
    ExportSecretRequest, ExportSecretResponse, ExternalJoinRequest, ExternalJoinResponse,
    GenerateIdentityRequest, GenerateIdentityResponse, ImportIdentityRequest,
    ImportIdentityResponse, PingRequest, PingResponse, ProcessCommitRequest,
    ProcessCommitResponse, ProcessWelcomeRequest, ProcessWelcomeResponse,
};

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    #[arg(short, long, default_value_t = 50051)]
    port: u16,
}

pub struct MyMlsService {
    store: Arc<mls::MlsGroupStore>,
}

#[tonic::async_trait]
impl MlsCryptoService for MyMlsService {
    async fn ping(
        &self,
        _request: Request<PingRequest>,
    ) -> Result<Response<PingResponse>, Status> {
        let reply = PingResponse {
            message: "Pong from Rust Crypto Engine!".to_string(),
            timestamp: Utc::now().timestamp(),
        };
        Ok(Response::new(reply))
    }

    async fn generate_identity(
        &self,
        request: Request<GenerateIdentityRequest>,
    ) -> Result<Response<GenerateIdentityResponse>, Status> {
        let _ = request.into_inner().display_name;

        match mls::generate_identity() {
            Ok(identity) => Ok(Response::new(GenerateIdentityResponse {
                signing_key_private: identity.signing_key_private,
                public_key: identity.public_key,
                credential: identity.credential,
            })),
            Err(e) => {
                eprintln!("generate_identity error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn export_identity(
        &self,
        _request: Request<ExportIdentityRequest>,
    ) -> Result<Response<ExportIdentityResponse>, Status> {
        Err(Status::unimplemented("ExportIdentity — planned for Phase 5"))
    }

    async fn import_identity(
        &self,
        _request: Request<ImportIdentityRequest>,
    ) -> Result<Response<ImportIdentityResponse>, Status> {
        Err(Status::unimplemented("ImportIdentity — planned for Phase 5"))
    }

    // ── Phase 4: Group Operations (Real OpenMLS) ─────────────────────────

    async fn create_group(
        &self,
        request: Request<CreateGroupRequest>,
    ) -> Result<Response<CreateGroupResponse>, Status> {
        let req = request.into_inner();
        match mls::create_group(&self.store, &req.group_id, &req.signing_key) {
            Ok(result) => Ok(Response::new(CreateGroupResponse {
                group_state: result.group_state,
                tree_hash: result.tree_hash,
            })),
            Err(e) => {
                eprintln!("create_group error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn create_proposal(
        &self,
        request: Request<CreateProposalRequest>,
    ) -> Result<Response<CreateProposalResponse>, Status> {
        let req = request.into_inner();
        match mls::create_proposal(&self.store, &req.group_state, req.proposal_type, &req.data) {
            Ok(proposal_bytes) => Ok(Response::new(CreateProposalResponse { proposal_bytes })),
            Err(e) => {
                eprintln!("create_proposal error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn create_commit(
        &self,
        request: Request<CreateCommitRequest>,
    ) -> Result<Response<CreateCommitResponse>, Status> {
        let req = request.into_inner();
        match mls::create_commit(&self.store, &req.group_state, &req.proposals) {
            Ok(result) => Ok(Response::new(CreateCommitResponse {
                commit_bytes: result.commit_bytes,
                welcome_bytes: result.welcome_bytes,
                new_group_state: result.new_group_state,
                new_tree_hash: result.new_tree_hash,
            })),
            Err(e) => {
                eprintln!("create_commit error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn process_commit(
        &self,
        request: Request<ProcessCommitRequest>,
    ) -> Result<Response<ProcessCommitResponse>, Status> {
        let req = request.into_inner();
        match mls::process_commit(&self.store, &req.group_state, &req.commit_bytes) {
            Ok(result) => Ok(Response::new(ProcessCommitResponse {
                new_group_state: result.new_group_state,
                new_tree_hash: result.new_tree_hash,
            })),
            Err(e) => {
                eprintln!("process_commit error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn process_welcome(
        &self,
        request: Request<ProcessWelcomeRequest>,
    ) -> Result<Response<ProcessWelcomeResponse>, Status> {
        let req = request.into_inner();
        match mls::process_welcome(&self.store, &req.welcome_bytes, &req.signing_key) {
            Ok(result) => Ok(Response::new(ProcessWelcomeResponse {
                group_state: result.group_state,
                tree_hash: result.tree_hash,
            })),
            Err(e) => {
                eprintln!("process_welcome error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn encrypt_message(
        &self,
        request: Request<EncryptMessageRequest>,
    ) -> Result<Response<EncryptMessageResponse>, Status> {
        let req = request.into_inner();
        match mls::encrypt_message(&self.store, &req.group_state, &req.plaintext) {
            Ok(result) => Ok(Response::new(EncryptMessageResponse {
                ciphertext: result.ciphertext,
                new_group_state: result.new_group_state,
            })),
            Err(e) => {
                eprintln!("encrypt_message error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn decrypt_message(
        &self,
        request: Request<DecryptMessageRequest>,
    ) -> Result<Response<DecryptMessageResponse>, Status> {
        let req = request.into_inner();
        match mls::decrypt_message(&self.store, &req.group_state, &req.ciphertext) {
            Ok(result) => Ok(Response::new(DecryptMessageResponse {
                plaintext: result.plaintext,
                new_group_state: result.new_group_state,
            })),
            Err(e) => {
                eprintln!("decrypt_message error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn external_join(
        &self,
        request: Request<ExternalJoinRequest>,
    ) -> Result<Response<ExternalJoinResponse>, Status> {
        let req = request.into_inner();
        match mls::external_join(&self.store, &req.group_info, &req.signing_key) {
            Ok(result) => Ok(Response::new(ExternalJoinResponse {
                group_state: result.group_state,
                commit_bytes: result.commit_bytes,
                tree_hash: result.tree_hash,
            })),
            Err(e) => {
                eprintln!("external_join error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn export_secret(
        &self,
        request: Request<ExportSecretRequest>,
    ) -> Result<Response<ExportSecretResponse>, Status> {
        let req = request.into_inner();
        match mls::export_secret(&self.store, &req.group_state, &req.label, req.length) {
            Ok(secret) => Ok(Response::new(ExportSecretResponse { secret })),
            Err(e) => {
                eprintln!("export_secret error: {e}");
                Err(Status::internal(e))
            }
        }
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let addr = format!("127.0.0.1:{}", args.port).parse()?;

    let store = Arc::new(mls::MlsGroupStore::new());
    let service = MyMlsService { store };

    println!("Crypto Engine listening on {addr}");

    Server::builder()
        .add_service(MlsCryptoServiceServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
