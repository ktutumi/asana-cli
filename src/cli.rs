use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::{Context, Result, anyhow, bail};
use clap::{Parser, Subcommand};

use crate::asana_api::{AsanaClient, OAuthExchangeInput, OAuthRefreshInput};
use crate::config::{StoredConfigPatch, TokenData, default_config_path, load_config, save_config};
use crate::oauth::{
    AuthorizationUrlOptions, build_authorization_url, default_localhost_redirect_uri,
    generate_state,
};
use crate::oauth_callback::{WaitForOAuthCallbackOptions, wait_for_oauth_callback};

#[derive(Debug, Clone, Default)]
pub struct RuntimeOptions {
    pub api_base: Option<String>,
    pub oauth_token_endpoint: Option<String>,
}

impl RuntimeOptions {
    pub fn from_env() -> Self {
        Self {
            api_base: std::env::var("ASANA_API_BASE").ok(),
            oauth_token_endpoint: std::env::var("ASANA_OAUTH_TOKEN_ENDPOINT").ok(),
        }
    }
}

pub trait CliIo: Send + Sync {
    fn stdout(&self, line: String);
    fn stderr(&self, line: String);
}

#[derive(Debug, Clone, Default)]
pub struct BufferedCliIo {
    stdout: Arc<Mutex<Vec<String>>>,
    stderr: Arc<Mutex<Vec<String>>>,
}

impl BufferedCliIo {
    pub fn stdout_lines(&self) -> Vec<String> {
        self.stdout.lock().expect("stdout lock").clone()
    }

    pub fn stderr_lines(&self) -> Vec<String> {
        self.stderr.lock().expect("stderr lock").clone()
    }
}

impl CliIo for BufferedCliIo {
    fn stdout(&self, line: String) {
        self.stdout.lock().expect("stdout lock").push(line);
    }

    fn stderr(&self, line: String) {
        self.stderr.lock().expect("stderr lock").push(line);
    }
}

pub struct StdCliIo;

impl CliIo for StdCliIo {
    fn stdout(&self, line: String) {
        println!("{line}");
    }

    fn stderr(&self, line: String) {
        eprintln!("{line}");
    }
}

#[derive(Debug, Parser)]
#[command(
    name = "asana-cli",
    about = "Lightweight Asana OAuth CLI",
    disable_help_flag = false
)]
struct Cli {
    #[arg(long, global = true, default_value_os_t = default_config_path())]
    config: PathBuf,
    #[command(subcommand)]
    command: Commands,
}

#[derive(Debug, Subcommand)]
enum Commands {
    Auth(AuthCommands),
    Me,
    Projects(ProjectCommands),
    Project(ProjectCommands),
    Tasks(TaskCommands),
    Workspaces(WorkspaceCommands),
}

#[derive(Debug, Parser)]
struct AuthCommands {
    #[command(subcommand)]
    command: AuthSubcommand,
}

#[derive(Debug, Subcommand)]
enum AuthSubcommand {
    Url {
        #[arg(long)]
        client_id: String,
        #[arg(long, default_value = "urn:ietf:wg:oauth:2.0:oob")]
        redirect_uri: String,
        #[arg(long = "scope")]
        scopes: Vec<String>,
        #[arg(long)]
        state: Option<String>,
    },
    Exchange {
        #[arg(long)]
        client_id: String,
        #[arg(long)]
        client_secret: String,
        #[arg(long)]
        code: String,
        #[arg(long, default_value = "urn:ietf:wg:oauth:2.0:oob")]
        redirect_uri: String,
    },
    Login {
        #[arg(long)]
        client_id: String,
        #[arg(long)]
        client_secret: String,
        #[arg(long, default_value = default_localhost_redirect_uri())]
        redirect_uri: String,
        #[arg(long = "scope")]
        scopes: Vec<String>,
        #[arg(long)]
        state: Option<String>,
        #[arg(long, default_value_t = 120_000)]
        listen_timeout_ms: u64,
    },
    Refresh {
        #[arg(long)]
        client_secret: String,
    },
}

#[derive(Debug, Parser)]
struct ProjectCommands {
    #[command(subcommand)]
    command: ProjectSubcommand,
}

#[derive(Debug, Subcommand)]
enum ProjectSubcommand {
    List {
        #[arg(long)]
        workspace: String,
    },
}

#[derive(Debug, Parser)]
struct TaskCommands {
    #[command(subcommand)]
    command: TaskSubcommand,
}

#[derive(Debug, Subcommand)]
enum TaskSubcommand {
    List {
        #[arg(long)]
        project: String,
    },
    Get {
        #[arg(long)]
        task: String,
    },
    Subtasks {
        #[arg(long)]
        task: String,
    },
    Stories {
        #[arg(long)]
        task: String,
    },
    Comments {
        #[arg(long)]
        task: String,
    },
    Attachments {
        #[arg(long)]
        task: String,
    },
}

#[derive(Debug, Parser)]
struct WorkspaceCommands {
    #[command(subcommand)]
    command: WorkspaceSubcommand,
}

#[derive(Debug, Subcommand)]
enum WorkspaceSubcommand {
    List,
}

pub async fn run_cli_catching<S: AsRef<str>>(
    args: &[S],
    io: &dyn CliIo,
    runtime: RuntimeOptions,
) -> i32 {
    match run_cli(args, io, runtime).await {
        Ok(()) => 0,
        Err(error) => {
            io.stderr(error.to_string());
            1
        }
    }
}

pub async fn run_cli<S: AsRef<str>>(
    args: &[S],
    io: &dyn CliIo,
    runtime: RuntimeOptions,
) -> Result<()> {
    let argv = std::iter::once("asana-cli".to_string())
        .chain(args.iter().map(|item| item.as_ref().to_string()))
        .collect::<Vec<_>>();

    let cli = match Cli::try_parse_from(argv) {
        Ok(cli) => cli,
        Err(error) => {
            let rendered = error.to_string();
            if error.kind() == clap::error::ErrorKind::DisplayHelp
                || error.kind() == clap::error::ErrorKind::DisplayVersion
            {
                io.stdout(rendered.trim_end().to_string());
                return Ok(());
            }
            return Err(anyhow!(rendered.trim_end().to_string()));
        }
    };

    let api_client = AsanaClient::new(
        runtime
            .api_base
            .as_deref()
            .unwrap_or("https://app.asana.com/api/1.0/"),
        runtime
            .oauth_token_endpoint
            .as_deref()
            .unwrap_or("https://app.asana.com/-/oauth_token"),
    )?;

    match cli.command {
        Commands::Auth(auth) => handle_auth(auth, &cli.config, io, &api_client).await,
        Commands::Me => {
            let access_token = require_access_token(&cli.config).await?;
            io.stdout(serde_json::to_string_pretty(
                &api_client.fetch_me(&access_token).await?,
            )?);
            Ok(())
        }
        Commands::Projects(projects) | Commands::Project(projects) => match projects.command {
            ProjectSubcommand::List { workspace } => {
                let access_token = require_access_token(&cli.config).await?;
                let items = api_client.list_projects(&access_token, &workspace).await?;
                io.stdout(serde_json::to_string_pretty(&items)?);
                Ok(())
            }
        },
        Commands::Tasks(tasks) => {
            let access_token = require_access_token(&cli.config).await?;
            match tasks.command {
                TaskSubcommand::List { project } => io.stdout(serde_json::to_string_pretty(
                    &api_client.list_tasks(&access_token, &project).await?,
                )?),
                TaskSubcommand::Get { task } => io.stdout(serde_json::to_string_pretty(
                    &api_client.get_task(&access_token, &task).await?,
                )?),
                TaskSubcommand::Subtasks { task } => io.stdout(serde_json::to_string_pretty(
                    &api_client.list_subtasks(&access_token, &task).await?,
                )?),
                TaskSubcommand::Stories { task } => io.stdout(serde_json::to_string_pretty(
                    &api_client.list_stories(&access_token, &task).await?,
                )?),
                TaskSubcommand::Comments { task } => io.stdout(serde_json::to_string_pretty(
                    &api_client.list_comments(&access_token, &task).await?,
                )?),
                TaskSubcommand::Attachments { task } => io.stdout(serde_json::to_string_pretty(
                    &api_client.list_attachments(&access_token, &task).await?,
                )?),
            }
            Ok(())
        }
        Commands::Workspaces(workspaces) => match workspaces.command {
            WorkspaceSubcommand::List => {
                let access_token = require_access_token(&cli.config).await?;
                io.stdout(serde_json::to_string_pretty(
                    &api_client.list_workspaces(&access_token).await?,
                )?);
                Ok(())
            }
        },
    }
}

async fn handle_auth(
    auth: AuthCommands,
    config_path: &Path,
    io: &dyn CliIo,
    api_client: &AsanaClient,
) -> Result<()> {
    match auth.command {
        AuthSubcommand::Url {
            client_id,
            redirect_uri,
            scopes,
            state,
        } => {
            let url = build_authorization_url(&AuthorizationUrlOptions {
                client_id,
                redirect_uri,
                scopes,
                state: Some(state.unwrap_or_else(generate_state)),
            })?;
            io.stdout(url);
            Ok(())
        }
        AuthSubcommand::Exchange {
            client_id,
            client_secret,
            code,
            redirect_uri,
        } => {
            let token = api_client
                .exchange_code_for_token(OAuthExchangeInput {
                    client_id: client_id.clone(),
                    client_secret,
                    code,
                    redirect_uri: redirect_uri.clone(),
                    now_unix_seconds: None,
                })
                .await?;
            save_config(
                config_path,
                StoredConfigPatch {
                    client_id: Some(client_id),
                    redirect_uri: Some(redirect_uri),
                    token: Some(token.clone()),
                },
            )
            .await?;
            io.stdout(serde_json::to_string_pretty(&redact_token(&token))?);
            Ok(())
        }
        AuthSubcommand::Login {
            client_id,
            client_secret,
            redirect_uri,
            scopes,
            state,
            listen_timeout_ms,
        } => {
            let redirect_url = url::Url::parse(&redirect_uri)?;
            if redirect_url.scheme() != "http"
                || !matches!(redirect_url.host_str(), Some("127.0.0.1" | "localhost"))
            {
                bail!("auth login requires an http://localhost or http://127.0.0.1 redirect URI");
            }

            let expected_state = state.unwrap_or_else(generate_state);
            let listener = wait_for_oauth_callback(WaitForOAuthCallbackOptions {
                hostname: redirect_url.host_str().unwrap_or("127.0.0.1").to_string(),
                port: redirect_url.port().unwrap_or(80),
                callback_path: redirect_url.path().to_string(),
                timeout: Duration::from_millis(listen_timeout_ms),
            })
            .await?;

            let auth_url = build_authorization_url(&AuthorizationUrlOptions {
                client_id: client_id.clone(),
                redirect_uri: listener.callback_url().to_string(),
                scopes,
                state: Some(expected_state.clone()),
            })?;
            io.stdout(format!("Open this URL in your browser: {auth_url}"));

            let callback = listener.wait().await?;
            if callback.state.as_deref() != Some(expected_state.as_str()) {
                bail!("OAuth state mismatch");
            }

            let token = api_client
                .exchange_code_for_token(OAuthExchangeInput {
                    client_id: client_id.clone(),
                    client_secret,
                    code: callback.code,
                    redirect_uri: listener.callback_url().to_string(),
                    now_unix_seconds: None,
                })
                .await?;

            save_config(
                config_path,
                StoredConfigPatch {
                    client_id: Some(client_id),
                    redirect_uri: Some(listener.callback_url().to_string()),
                    token: Some(token.clone()),
                },
            )
            .await?;
            io.stdout(serde_json::to_string_pretty(&redact_token(&token))?);
            Ok(())
        }
        AuthSubcommand::Refresh { client_secret } => {
            let config = load_config(config_path).await?;
            let client_id = config
                .client_id
                .context("Saved clientId/redirectUri/refresh_token are required")?;
            let redirect_uri = config
                .redirect_uri
                .context("Saved clientId/redirectUri/refresh_token are required")?;
            let refresh_token = config
                .token
                .and_then(|token| token.refresh_token)
                .context("Saved clientId/redirectUri/refresh_token are required")?;

            let token = api_client
                .refresh_access_token(OAuthRefreshInput {
                    client_id,
                    client_secret,
                    redirect_uri,
                    refresh_token,
                    now_unix_seconds: None,
                })
                .await?;
            save_config(
                config_path,
                StoredConfigPatch {
                    client_id: None,
                    redirect_uri: None,
                    token: Some(token.clone()),
                },
            )
            .await?;
            io.stdout(serde_json::to_string_pretty(&redact_token(&token))?);
            Ok(())
        }
    }
}

async fn require_access_token(config_path: &Path) -> Result<String> {
    let config = load_config(config_path).await?;
    config
        .token
        .and_then(|token| (!token.access_token.is_empty()).then_some(token.access_token))
        .context("No access token saved. Run `auth exchange` first.")
}

fn redact_token(token: &TokenData) -> TokenData {
    TokenData {
        access_token: "***".to_string(),
        refresh_token: token.refresh_token.as_ref().map(|_| "***".to_string()),
        token_type: token.token_type.clone(),
        expires_in: token.expires_in,
        expires_at: token.expires_at.clone(),
    }
}
