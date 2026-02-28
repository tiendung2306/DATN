use tonic::{transport::Server, Request, Response, Status};
use clap::Parser;
use chrono::Utc;

mod mls;

pub mod mls_service {
    tonic::include_proto!("mls_service");
}

use mls_service::mls_crypto_service_server::{MlsCryptoService, MlsCryptoServiceServer};
use mls_service::{
    PingRequest, PingResponse,
    GenerateIdentityRequest, GenerateIdentityResponse,
    ExportIdentityRequest, ExportIdentityResponse,
    ImportIdentityRequest, ImportIdentityResponse,
};

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    #[arg(short, long, default_value_t = 50051)]
    port: u16,
}

#[derive(Debug, Default)]
pub struct MyMlsService {}

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
        // display_name is ignored at key generation time.
        // The name is assigned by Admin and stored in Go after bundle import.
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
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let addr = format!("127.0.0.1:{}", args.port).parse()?;
    let service = MyMlsService::default();

    println!("Crypto Engine listening on {addr}");

    Server::builder()
        .add_service(MlsCryptoServiceServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
