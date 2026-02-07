fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .build_server(true) // Generate server code
        .build_client(false) // Not generating client code for the engine itself
        .compile(
            &["../proto/mls_service.proto"], // Path to the proto file
            &["../proto"], // Directory containing the proto file
        )?;
    Ok(())
}
