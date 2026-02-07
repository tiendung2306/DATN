use tonic::{transport::Server, Request, Response, Status};
use clap::Parser;
use chrono::Utc;

// Include the generated proto code
pub mod mls_service {
    tonic::include_proto!("mls_service");
}

use mls_service::mls_crypto_service_server::{MlsCryptoService, MlsCryptoServiceServer};
use mls_service::{
    PingRequest, PingResponse, 
    GenerateIdentityRequest, GenerateIdentityResponse,
    ExportIdentityRequest, ExportIdentityResponse,
    ImportIdentityRequest, ImportIdentityResponse
};

/// CLI arguments for the crypto engine
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    /// Port to listen on
    #[arg(short, long, default_value_t = 50051)]
    port: u16,
}

/// Implementation of the MLSCryptoService
#[derive(Debug, Default)]
pub struct MyMlsService {}

#[tonic::async_trait]
impl MlsCryptoService for MyMlsService {
    async fn ping(&self, _request: Request<PingRequest>) -> Result<Response<PingResponse>, Status> {
        println!("Received Ping request");
        let reply = PingResponse {
            message: "Pong from Rust Crypto Engine!".to_string(),
            timestamp: Utc::now().timestamp(),
        };
        Ok(Response::new(reply))
    }

    async fn generate_identity(
        &self,
        _request: Request<GenerateIdentityRequest>,
    ) -> Result<Response<GenerateIdentityResponse>, Status> {
        Err(Status::unimplemented("GenerateIdentity is not yet implemented"))
    }

    async fn export_identity(
        &self,
        _request: Request<ExportIdentityRequest>,
    ) -> Result<Response<ExportIdentityResponse>, Status> {
        Err(Status::unimplemented("ExportIdentity is not yet implemented"))
    }

    async fn import_identity(
        &self,
        _request: Request<ImportIdentityRequest>,
    ) -> Result<Response<ImportIdentityResponse>, Status> {
        Err(Status::unimplemented("ImportIdentity is not yet implemented"))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let addr = format!("127.0.0.1:{}", args.port).parse()?;
    let mls_service = MyMlsService::default();

    println!("Crypto Engine listening on {}", addr);

    Server::builder()
        .add_service(MlsCryptoServiceServer::new(mls_service))
        .serve(addr)
        .await?;

    Ok(())
}
