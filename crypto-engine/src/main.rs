use chrono::Utc;
use clap::Parser;
use std::sync::Arc;
use tonic::{transport::Server, Request, Response, Status};

mod mls;

pub mod mls_service {
    tonic::include_proto!("mls_service");
}

use mls_service::mls_crypto_service_server::{MlsCryptoService, MlsCryptoServiceServer};
use mls_service::{
    AddMembersRequest, AddMembersResponse, CreateCommitRequest, CreateCommitResponse,
    CreateGroupRequest, CreateGroupResponse, CreateProposalRequest, CreateProposalResponse,
    CreateUpdateCommitCachedRequest, CreateUpdateCommitCachedResponse, DecryptMessageCachedRequest,
    DecryptMessageCachedResponse, DecryptMessageRequest, DecryptMessageResponse,
    EncryptMessageCachedRequest, EncryptMessageCachedResponse, EncryptMessageRequest,
    EncryptMessageResponse, ExportGroupInfoRequest, ExportGroupInfoResponse,
    ExportGroupStateCheckpointRequest, ExportGroupStateCheckpointResponse, ExportIdentityRequest,
    ExportIdentityResponse, ExportSecretCachedRequest, ExportSecretCachedResponse,
    ExportSecretRequest, ExportSecretResponse, ExternalJoinRequest, ExternalJoinResponse,
    GenerateIdentityRequest, GenerateIdentityResponse, GenerateKeyPackageRequest,
    GenerateKeyPackageResponse, GetGroupMetadataRequest, GetGroupMetadataResponse,
    HasMemberRequest, HasMemberResponse, ImportIdentityRequest, ImportIdentityResponse,
    ListMemberIdentitiesRequest, ListMemberIdentitiesResponse, LoadGroupRequest, LoadGroupResponse,
    OperationContext, PingRequest, PingResponse, ProcessCommitCachedRequest,
    ProcessCommitCachedResponse, ProcessCommitRequest, ProcessCommitResponse,
    ProcessProposalRequest, ProcessProposalResponse, ProcessWelcomeRequest, ProcessWelcomeResponse,
    RemoveMembersRequest, RemoveMembersResponse, StageCommitRequest, StageCommitResponse,
    UnloadGroupRequest, UnloadGroupResponse,
};

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    #[arg(short, long, default_value_t = 50051)]
    port: u16,
}

#[derive(Clone)]
pub struct MyMlsService {
    cache: Arc<mls::RuntimeCache>,
}

fn cached_context(ctx: Option<OperationContext>) -> Result<mls::CachedOperationContext, Status> {
    let ctx = ctx.ok_or_else(|| Status::invalid_argument("operation context is required"))?;
    Ok(mls::CachedOperationContext {
        group_id: ctx.group_id,
        expected_epoch: ctx.expected_epoch,
        expected_state_version: ctx.expected_state_version,
        operation_id: ctx.operation_id,
    })
}

#[tonic::async_trait]
impl MlsCryptoService for MyMlsService {
    async fn ping(&self, _request: Request<PingRequest>) -> Result<Response<PingResponse>, Status> {
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
        Err(Status::unimplemented(
            "ExportIdentity — planned for Phase 5",
        ))
    }

    async fn import_identity(
        &self,
        _request: Request<ImportIdentityRequest>,
    ) -> Result<Response<ImportIdentityResponse>, Status> {
        Err(Status::unimplemented(
            "ImportIdentity — planned for Phase 5",
        ))
    }

    // ── Phase 4: Group Operations (Stateless — Real OpenMLS) ─────────────

    async fn create_group(
        &self,
        request: Request<CreateGroupRequest>,
    ) -> Result<Response<CreateGroupResponse>, Status> {
        let req = request.into_inner();
        match mls::create_group(&req.group_id, &req.signing_key, req.max_past_epochs) {
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
        match mls::create_proposal(&req.group_state, req.proposal_type, &req.data) {
            Ok(result) => Ok(Response::new(CreateProposalResponse {
                proposal_bytes: result.proposal_bytes,
                proposal_ref: result.proposal_ref,
                new_group_state: result.new_group_state,
            })),
            Err(e) => {
                eprintln!("create_proposal error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn process_proposal(
        &self,
        request: Request<ProcessProposalRequest>,
    ) -> Result<Response<ProcessProposalResponse>, Status> {
        let req = request.into_inner();
        match mls::process_proposal(&req.group_state, &req.proposal_bytes) {
            Ok(result) => Ok(Response::new(ProcessProposalResponse {
                proposal_ref: result.proposal_ref,
                proposal_type: result.proposal_type,
                new_group_state: result.new_group_state,
            })),
            Err(e) => {
                eprintln!("process_proposal error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn create_commit(
        &self,
        request: Request<CreateCommitRequest>,
    ) -> Result<Response<CreateCommitResponse>, Status> {
        let req = request.into_inner();
        #[allow(deprecated)]
        match mls::create_commit(
            &req.group_state,
            &req.proposals,
            &req.expected_proposal_refs,
        ) {
            Ok(result) => Ok(Response::new(CreateCommitResponse {
                commit_bytes: result.commit_bytes,
                welcome_bytes: result.welcome_bytes,
                new_group_state: result.new_group_state,
                new_tree_hash: result.new_tree_hash,
                group_info: result.group_info,
                committed_proposal_refs: result.committed_proposal_refs,
            })),
            Err(e) => {
                eprintln!("create_commit error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn stage_commit(
        &self,
        request: Request<StageCommitRequest>,
    ) -> Result<Response<StageCommitResponse>, Status> {
        let req = request.into_inner();
        match mls::stage_commit(&req.group_state, &req.commit_bytes, &req.included_proposals) {
            Ok(result) => Ok(Response::new(StageCommitResponse {
                epoch: result.epoch,
                proposal_refs: result.proposal_refs,
                proposal_types: result.proposal_types,
            })),
            Err(e) => {
                eprintln!("stage_commit error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn process_commit(
        &self,
        request: Request<ProcessCommitRequest>,
    ) -> Result<Response<ProcessCommitResponse>, Status> {
        let req = request.into_inner();
        match mls::process_commit(&req.group_state, &req.commit_bytes, &req.included_proposals) {
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
        match mls::process_welcome(
            &req.welcome_bytes,
            &req.signing_key,
            &req.key_package_bundle_private,
            req.max_past_epochs,
        ) {
            Ok(result) => Ok(Response::new(ProcessWelcomeResponse {
                group_state: result.group_state,
                tree_hash: result.tree_hash,
                epoch: result.epoch,
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
        match mls::encrypt_message(&req.group_state, &req.plaintext) {
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
        match mls::decrypt_message(&req.group_state, &req.ciphertext) {
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
        match mls::external_join(&req.group_info, &req.signing_key, req.max_past_epochs) {
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
        match mls::export_secret(&req.group_state, &req.label, &req.context, req.length) {
            Ok(secret) => Ok(Response::new(ExportSecretResponse { secret })),
            Err(e) => {
                eprintln!("export_secret error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn generate_key_package(
        &self,
        request: Request<GenerateKeyPackageRequest>,
    ) -> Result<Response<GenerateKeyPackageResponse>, Status> {
        let req = request.into_inner();
        match mls::generate_key_package(&req.signing_key) {
            Ok(result) => Ok(Response::new(GenerateKeyPackageResponse {
                key_package_bytes: result.key_package_bytes,
                key_package_bundle_private: result.key_package_bundle_private,
            })),
            Err(e) => {
                eprintln!("generate_key_package error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn add_members(
        &self,
        request: Request<AddMembersRequest>,
    ) -> Result<Response<AddMembersResponse>, Status> {
        let req = request.into_inner();
        match mls::add_members(&req.group_state, &req.key_packages) {
            Ok(result) => Ok(Response::new(AddMembersResponse {
                commit_bytes: result.commit_bytes,
                welcome_bytes: result.welcome_bytes,
                new_group_state: result.new_group_state,
                new_tree_hash: result.new_tree_hash,
            })),
            Err(e) => {
                eprintln!("add_members error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn export_group_info(
        &self,
        request: Request<ExportGroupInfoRequest>,
    ) -> Result<Response<ExportGroupInfoResponse>, Status> {
        let req = request.into_inner();
        match mls::export_group_info(&req.group_state, req.with_ratchet_tree) {
            Ok(group_info) => Ok(Response::new(ExportGroupInfoResponse { group_info })),
            Err(e) => {
                eprintln!("export_group_info error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn remove_members(
        &self,
        request: Request<RemoveMembersRequest>,
    ) -> Result<Response<RemoveMembersResponse>, Status> {
        let req = request.into_inner();
        match mls::remove_members(&req.group_state, &req.target_identities) {
            Ok(result) => Ok(Response::new(RemoveMembersResponse {
                commit_bytes: result.commit_bytes,
                new_group_state: result.new_group_state,
                new_tree_hash: result.new_tree_hash,
            })),
            Err(e) => {
                eprintln!("remove_members error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn has_member(
        &self,
        request: Request<HasMemberRequest>,
    ) -> Result<Response<HasMemberResponse>, Status> {
        let req = request.into_inner();
        match mls::has_member(&req.group_state, &req.identity) {
            Ok(is_member) => Ok(Response::new(HasMemberResponse { is_member })),
            Err(e) => {
                eprintln!("has_member error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn list_member_identities(
        &self,
        request: Request<ListMemberIdentitiesRequest>,
    ) -> Result<Response<ListMemberIdentitiesResponse>, Status> {
        let req = request.into_inner();
        match mls::list_member_identities(&req.group_state) {
            Ok(identities) => Ok(Response::new(ListMemberIdentitiesResponse { identities })),
            Err(e) => {
                eprintln!("list_member_identities error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn load_group(
        &self,
        request: Request<LoadGroupRequest>,
    ) -> Result<Response<LoadGroupResponse>, Status> {
        let req = request.into_inner();
        match self
            .cache
            .load_group(&req.group_id, &req.group_state, req.state_version)
        {
            Ok(meta) => Ok(Response::new(LoadGroupResponse {
                group_id: meta.group_id,
                epoch: meta.epoch,
                state_version: meta.state_version,
                tree_hash: meta.tree_hash,
                state_size_bytes: meta.state_size_bytes,
            })),
            Err(e) => {
                eprintln!("load_group error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn get_group_metadata(
        &self,
        request: Request<GetGroupMetadataRequest>,
    ) -> Result<Response<GetGroupMetadataResponse>, Status> {
        let req = request.into_inner();
        match self.cache.metadata(&req.group_id) {
            Ok(meta) => Ok(Response::new(GetGroupMetadataResponse {
                group_id: meta.group_id,
                epoch: meta.epoch,
                state_version: meta.state_version,
                tree_hash: meta.tree_hash,
                dirty: meta.dirty,
                state_size_bytes: meta.state_size_bytes,
            })),
            Err(e) => {
                eprintln!("get_group_metadata error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn encrypt_message_cached(
        &self,
        request: Request<EncryptMessageCachedRequest>,
    ) -> Result<Response<EncryptMessageCachedResponse>, Status> {
        let req = request.into_inner();
        let ctx = cached_context(req.context)?;
        match self.cache.encrypt_message_cached(&ctx, &req.plaintext) {
            Ok(result) => Ok(Response::new(EncryptMessageCachedResponse {
                ciphertext: result.ciphertext,
                epoch: result.epoch,
                state_version: result.state_version,
            })),
            Err(e) => {
                eprintln!("encrypt_message_cached error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn decrypt_message_cached(
        &self,
        request: Request<DecryptMessageCachedRequest>,
    ) -> Result<Response<DecryptMessageCachedResponse>, Status> {
        let req = request.into_inner();
        let ctx = cached_context(req.context)?;
        match self.cache.decrypt_message_cached(&ctx, &req.ciphertext) {
            Ok(result) => Ok(Response::new(DecryptMessageCachedResponse {
                plaintext: result.plaintext,
                epoch: result.epoch,
                state_version: result.state_version,
            })),
            Err(e) => {
                eprintln!("decrypt_message_cached error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn create_update_commit_cached(
        &self,
        request: Request<CreateUpdateCommitCachedRequest>,
    ) -> Result<Response<CreateUpdateCommitCachedResponse>, Status> {
        let req = request.into_inner();
        let ctx = cached_context(req.context)?;
        match self.cache.create_update_commit_cached(&ctx) {
            Ok(result) => Ok(Response::new(CreateUpdateCommitCachedResponse {
                commit_bytes: result.commit_bytes,
                tree_hash: result.tree_hash,
                epoch: result.epoch,
                state_version: result.state_version,
            })),
            Err(e) => {
                eprintln!("create_update_commit_cached error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn process_commit_cached(
        &self,
        request: Request<ProcessCommitCachedRequest>,
    ) -> Result<Response<ProcessCommitCachedResponse>, Status> {
        let req = request.into_inner();
        let ctx = cached_context(req.context)?;
        match self.cache.process_commit_cached(&ctx, &req.commit_bytes) {
            Ok(result) => Ok(Response::new(ProcessCommitCachedResponse {
                tree_hash: result.tree_hash,
                epoch: result.epoch,
                state_version: result.state_version,
            })),
            Err(e) => {
                eprintln!("process_commit_cached error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn export_secret_cached(
        &self,
        request: Request<ExportSecretCachedRequest>,
    ) -> Result<Response<ExportSecretCachedResponse>, Status> {
        let req = request.into_inner();
        let ctx = cached_context(req.context)?;
        match self
            .cache
            .export_secret_cached(&ctx, &req.label, &req.exporter_context, req.length)
        {
            Ok(result) => Ok(Response::new(ExportSecretCachedResponse {
                secret: result.secret,
                epoch: result.epoch,
                state_version: result.state_version,
            })),
            Err(e) => {
                eprintln!("export_secret_cached error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn export_group_state_checkpoint(
        &self,
        request: Request<ExportGroupStateCheckpointRequest>,
    ) -> Result<Response<ExportGroupStateCheckpointResponse>, Status> {
        let req = request.into_inner();
        match self.cache.export_checkpoint(&req.group_id) {
            Ok(result) => Ok(Response::new(ExportGroupStateCheckpointResponse {
                group_state: result.group_state,
                tree_hash: result.tree_hash,
                epoch: result.epoch,
                state_version: result.state_version,
                state_size_bytes: result.state_size_bytes,
            })),
            Err(e) => {
                eprintln!("export_group_state_checkpoint error: {e}");
                Err(Status::internal(e))
            }
        }
    }

    async fn unload_group(
        &self,
        request: Request<UnloadGroupRequest>,
    ) -> Result<Response<UnloadGroupResponse>, Status> {
        let req = request.into_inner();
        Ok(Response::new(UnloadGroupResponse {
            unloaded: self.cache.unload_group(&req.group_id),
        }))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let addr = format!("127.0.0.1:{}", args.port).parse()?;

    let service = MyMlsService {
        cache: Arc::new(mls::RuntimeCache::default()),
    };

    println!("Crypto Engine listening on {addr}");

    Server::builder()
        .add_service(MlsCryptoServiceServer::new(service)
            .max_decoding_message_size(64 * 1024 * 1024)
            .max_encoding_message_size(64 * 1024 * 1024))
        .serve(addr)
        .await?;

    Ok(())
}
