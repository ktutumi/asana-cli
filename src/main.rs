use asana_cli::cli::{RuntimeOptions, StdCliIo, run_cli_catching};

#[tokio::main]
async fn main() {
    let io = StdCliIo;
    let exit_code = run_cli_catching(
        &std::env::args().skip(1).collect::<Vec<_>>(),
        &io,
        RuntimeOptions::from_env(),
    )
    .await;

    std::process::exit(exit_code);
}
