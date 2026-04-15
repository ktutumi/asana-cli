use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::{Context, Result, anyhow, bail};
use clap::{ArgAction, Parser, Subcommand};

use crate::asana_api::{AsanaClient, OAuthExchangeInput, OAuthRefreshInput};
use crate::config::{StoredConfigPatch, TokenData, default_config_path, load_config, save_config};
use crate::oauth::{
    AuthorizationUrlOptions, build_authorization_url, default_localhost_redirect_uri,
    generate_state,
};
use crate::oauth_callback::{WaitForOAuthCallbackOptions, wait_for_oauth_callback};

const AUTH_LOGIN_EXAMPLES: &str = "Examples:
  asana-cli auth login --client-id \"$ASANA_CLIENT_ID\" --client-secret \"$ASANA_CLIENT_SECRET\"
  asana-cli auth login --no-open --client-id \"$ASANA_CLIENT_ID\" --client-secret \"$ASANA_CLIENT_SECRET\" --redirect-uri http://127.0.0.1:18787/callback";

#[derive(Debug, Clone, Default)]
pub struct RuntimeOptions {
    pub api_base: Option<String>,
    pub oauth_token_endpoint: Option<String>,
    pub browser: Option<String>,
}

impl RuntimeOptions {
    pub fn from_env() -> Self {
        Self {
            api_base: std::env::var("ASANA_API_BASE").ok(),
            oauth_token_endpoint: std::env::var("ASANA_OAUTH_TOKEN_ENDPOINT").ok(),
            browser: std::env::var("BROWSER").ok(),
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
    about = "Asana OAuth 認証と読み取り系 API を扱う CLI",
    long_about = "Asana OAuth 認証と読み取り系 API を扱う CLI。help から初回セットアップ、localhost callback ログイン、主要な read-only API 呼び出しまで辿れるようにしています。",
    disable_help_flag = false,
    version = env!("CARGO_PKG_VERSION")
)]
struct Cli {
    #[arg(
        long,
        global = true,
        default_value_os_t = default_config_path(),
        help = "認証情報を保存する設定ファイルのパス"
    )]
    config: PathBuf,
    #[command(subcommand)]
    command: Commands,
}

#[derive(Debug, Subcommand)]
enum Commands {
    #[command(about = "認証 URL の生成、ログイン、token 更新を行います")]
    Auth(AuthCommands),
    #[command(about = "現在の認証ユーザー情報を取得します")]
    Me,
    #[command(
        alias = "project",
        about = "ワークスペース配下のプロジェクトを取得します"
    )]
    Projects(ProjectCommands),
    #[command(about = "タスク、コメント、添付ファイルを取得します")]
    Tasks(TaskCommands),
    #[command(about = "アクセス可能なワークスペースを取得します")]
    Workspaces(WorkspaceCommands),
}

#[derive(Debug, Parser)]
struct AuthCommands {
    #[command(subcommand)]
    command: AuthSubcommand,
}

#[derive(Debug, Subcommand)]
enum AuthSubcommand {
    #[command(about = "認可 URL を生成します")]
    Url {
        #[arg(long, help = "Asana OAuth app の client ID")]
        client_id: String,
        #[arg(
            long,
            default_value = "urn:ietf:wg:oauth:2.0:oob",
            help = "認可後に code を返す redirect URI"
        )]
        redirect_uri: String,
        #[arg(long = "scope", help = "追加で要求する OAuth scope")]
        scopes: Vec<String>,
        #[arg(long, help = "CSRF 対策用の state。未指定時は自動生成")]
        state: Option<String>,
    },
    #[command(about = "authorization code を token に交換して保存します")]
    Exchange {
        #[arg(long, help = "Asana OAuth app の client ID")]
        client_id: String,
        #[arg(long, help = "token exchange 用 client secret")]
        client_secret: String,
        #[arg(long, help = "認可画面から取得した authorization code")]
        code: String,
        #[arg(
            long,
            default_value = "urn:ietf:wg:oauth:2.0:oob",
            help = "認可時に使った redirect URI"
        )]
        redirect_uri: String,
    },
    #[command(
        about = "localhost callback を使ってログインします",
        long_about = "localhost callback を使ってログインします。redirect URI には http://127.0.0.1/... または http://localhost/... を指定してください。OOB/manual flow を使いたい場合は `auth url` と `auth exchange` を使います。",
        after_help = AUTH_LOGIN_EXAMPLES
    )]
    Login {
        #[arg(long, help = "Asana OAuth app の client ID")]
        client_id: String,
        #[arg(long, help = "token exchange 用 client secret")]
        client_secret: String,
        #[arg(
            long,
            default_value_t = default_localhost_redirect_uri().to_string(),
            help = "localhost callback URL。http://127.0.0.1/... または http://localhost/... のみ対応"
        )]
        redirect_uri: String,
        #[arg(long = "scope", help = "追加で要求する OAuth scope")]
        scopes: Vec<String>,
        #[arg(long, help = "CSRF 対策用の state。未指定時は自動生成")]
        state: Option<String>,
        #[arg(long, default_value_t = 120_000, help = "callback を待つ最大時間 (ms)")]
        listen_timeout_ms: u64,
        #[arg(
            long,
            action = ArgAction::SetTrue,
            help = "ブラウザを自動起動せず、URL を表示するだけにします"
        )]
        no_open: bool,
    },
    #[command(about = "保存済み refresh token で access token を更新します")]
    Refresh {
        #[arg(long, help = "refresh 用に再入力する client secret")]
        client_secret: String,
    },
    #[command(about = "保存済み認証情報の状態を表示します")]
    Status,
}

#[derive(Debug, Parser)]
struct ProjectCommands {
    #[command(subcommand)]
    command: ProjectSubcommand,
}

#[derive(Debug, Subcommand)]
enum ProjectSubcommand {
    #[command(
        about = "ワークスペース配下のプロジェクト一覧を取得します",
        visible_alias = "ls"
    )]
    List {
        #[arg(help = "対象 workspace GID", value_name = "WORKSPACE")]
        workspace_arg: Option<String>,
        #[arg(long, help = "対象 workspace GID")]
        workspace: Option<String>,
    },
}

#[derive(Debug, Parser)]
struct TaskCommands {
    #[command(subcommand)]
    command: TaskSubcommand,
}

#[derive(Debug, Subcommand)]
enum TaskSubcommand {
    #[command(
        about = "プロジェクト配下のタスク一覧を取得します",
        visible_alias = "ls"
    )]
    List {
        #[arg(help = "対象 project GID", value_name = "PROJECT")]
        project_arg: Option<String>,
        #[arg(long, help = "対象 project GID")]
        project: Option<String>,
    },
    #[command(about = "単一タスクを取得します")]
    Get {
        #[arg(help = "対象 task GID", value_name = "TASK")]
        task_arg: Option<String>,
        #[arg(long, help = "対象 task GID")]
        task: Option<String>,
    },
    #[command(about = "タスクの subtasks を取得します")]
    Subtasks {
        #[arg(help = "対象 task GID", value_name = "TASK")]
        task_arg: Option<String>,
        #[arg(long, help = "対象 task GID")]
        task: Option<String>,
    },
    #[command(about = "タスクの story 履歴を取得します")]
    Stories {
        #[arg(help = "対象 task GID", value_name = "TASK")]
        task_arg: Option<String>,
        #[arg(long, help = "対象 task GID")]
        task: Option<String>,
    },
    #[command(about = "タスクのコメントだけを抽出して取得します")]
    Comments {
        #[arg(help = "対象 task GID", value_name = "TASK")]
        task_arg: Option<String>,
        #[arg(long, help = "対象 task GID")]
        task: Option<String>,
    },
    #[command(about = "タスクの添付ファイル一覧を取得します")]
    Attachments {
        #[arg(help = "対象 task GID", value_name = "TASK")]
        task_arg: Option<String>,
        #[arg(long, help = "対象 task GID")]
        task: Option<String>,
    },
}

#[derive(Debug, Parser)]
struct WorkspaceCommands {
    #[command(subcommand)]
    command: WorkspaceSubcommand,
}

#[derive(Debug, Subcommand)]
enum WorkspaceSubcommand {
    #[command(
        about = "アクセス可能なワークスペース一覧を取得します",
        visible_alias = "ls"
    )]
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
        Commands::Auth(auth) => handle_auth(auth, &cli.config, io, &api_client, &runtime).await,
        Commands::Me => {
            let access_token = require_access_token(&cli.config).await?;
            io.stdout(serde_json::to_string_pretty(
                &api_client.fetch_me(&access_token).await?,
            )?);
            Ok(())
        }
        Commands::Projects(projects) => match projects.command {
            ProjectSubcommand::List {
                workspace_arg,
                workspace,
            } => {
                let workspace = required_value("workspace", workspace_arg, workspace)?;
                let access_token = require_access_token(&cli.config).await?;
                let items = api_client.list_projects(&access_token, &workspace).await?;
                io.stdout(serde_json::to_string_pretty(&items)?);
                Ok(())
            }
        },
        Commands::Tasks(tasks) => {
            let access_token = require_access_token(&cli.config).await?;
            match tasks.command {
                TaskSubcommand::List {
                    project_arg,
                    project,
                } => {
                    let project = required_value("project", project_arg, project)?;
                    io.stdout(serde_json::to_string_pretty(
                        &api_client.list_tasks(&access_token, &project).await?,
                    )?)
                }
                TaskSubcommand::Get { task_arg, task } => {
                    let task = required_value("task", task_arg, task)?;
                    io.stdout(serde_json::to_string_pretty(
                        &api_client.get_task(&access_token, &task).await?,
                    )?)
                }
                TaskSubcommand::Subtasks { task_arg, task } => {
                    let task = required_value("task", task_arg, task)?;
                    io.stdout(serde_json::to_string_pretty(
                        &api_client.list_subtasks(&access_token, &task).await?,
                    )?)
                }
                TaskSubcommand::Stories { task_arg, task } => {
                    let task = required_value("task", task_arg, task)?;
                    io.stdout(serde_json::to_string_pretty(
                        &api_client.list_stories(&access_token, &task).await?,
                    )?)
                }
                TaskSubcommand::Comments { task_arg, task } => {
                    let task = required_value("task", task_arg, task)?;
                    io.stdout(serde_json::to_string_pretty(
                        &api_client.list_comments(&access_token, &task).await?,
                    )?)
                }
                TaskSubcommand::Attachments { task_arg, task } => {
                    let task = required_value("task", task_arg, task)?;
                    io.stdout(serde_json::to_string_pretty(
                        &api_client.list_attachments(&access_token, &task).await?,
                    )?)
                }
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
    runtime: &RuntimeOptions,
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
            no_open,
        } => {
            let redirect_url = url::Url::parse(&redirect_uri)?;
            if redirect_uri == "urn:ietf:wg:oauth:2.0:oob" {
                bail!(
                    "auth login は localhost callback 専用です。OOB/manual flow を使う場合は `asana-cli auth url` と `asana-cli auth exchange` を使ってください。"
                );
            }
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

            if !no_open {
                io.stderr(
                    "Attempting to open the authorization URL in your browser...".to_string(),
                );
                if let Err(error) = open_authorization_url(&auth_url, runtime) {
                    io.stderr(format!(
                        "Could not open a browser automatically: {error}. Open the printed URL manually."
                    ));
                }
            }

            let callback = listener.wait().await?;
            if callback.state.as_deref() != Some(expected_state.as_str()) {
                bail!("OAuth state mismatch");
            }

            let resolved_redirect_uri = listener.callback_url().to_string();
            let token = api_client
                .exchange_code_for_token(OAuthExchangeInput {
                    client_id: client_id.clone(),
                    client_secret,
                    code: callback.code,
                    redirect_uri: resolved_redirect_uri.clone(),
                    now_unix_seconds: None,
                })
                .await?;

            save_config(
                config_path,
                StoredConfigPatch {
                    client_id: Some(client_id),
                    redirect_uri: Some(resolved_redirect_uri.clone()),
                    token: Some(token.clone()),
                },
            )
            .await?;
            io.stderr("Login succeeded.".to_string());
            io.stderr(format!("Config saved to {}", config_path.display()));
            io.stderr(format!("Redirect URI: {resolved_redirect_uri}"));
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
        AuthSubcommand::Status => {
            print_auth_status(config_path, io).await?;
            Ok(())
        }
    }
}

async fn print_auth_status(config_path: &Path, io: &dyn CliIo) -> Result<()> {
    let config_exists = config_path.exists();
    let config = load_config(config_path).await?;
    let token = config.token.as_ref();

    io.stdout(format!("Config path: {}", config_path.display()));
    io.stdout(format!(
        "Config file: {}",
        if config_exists { "found" } else { "not found" }
    ));
    io.stdout(format!(
        "clientId: {}",
        config.client_id.as_deref().unwrap_or("missing")
    ));
    io.stdout(format!(
        "redirectUri: {}",
        config.redirect_uri.as_deref().unwrap_or("missing")
    ));
    io.stdout(format!(
        "access_token: {}",
        present_secret(token.and_then(|item| non_empty(&item.access_token)))
    ));
    io.stdout(format!(
        "refresh_token: {}",
        present_secret(token.and_then(|item| item.refresh_token.as_deref().and_then(non_empty)))
    ));
    io.stdout(format!(
        "expires_at: {}",
        token
            .and_then(|item| item.expires_at.as_deref())
            .unwrap_or("missing")
    ));

    if !config_exists {
        io.stdout("Run `asana-cli auth login` to create credentials.".to_string());
    }

    Ok(())
}

async fn require_access_token(config_path: &Path) -> Result<String> {
    let config = load_config(config_path).await?;
    config
        .token
        .and_then(|token| (!token.access_token.is_empty()).then_some(token.access_token))
        .context(
            "アクセストークンが保存されていません。まず `asana-cli auth login` を実行してください。manual flow を使う場合は `asana-cli auth url` と `asana-cli auth exchange` を使ってください。",
        )
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

fn required_value(
    name: &str,
    positional: Option<String>,
    flagged: Option<String>,
) -> Result<String> {
    match (positional, flagged) {
        (Some(value), None) | (None, Some(value)) => Ok(value),
        (Some(_), Some(_)) => {
            bail!("`{name}` は位置引数か --{name} のどちらか一方だけを指定してください")
        }
        (None, None) => bail!("`{name}` を指定してください"),
    }
}

fn non_empty(value: &str) -> Option<&str> {
    (!value.is_empty()).then_some(value)
}

fn present_secret(value: Option<&str>) -> &'static str {
    if value.is_some() {
        "present (***)"
    } else {
        "missing"
    }
}

fn open_authorization_url(url: &str, runtime: &RuntimeOptions) -> Result<()> {
    if let Some(browser) = runtime.browser.as_deref() {
        run_browser_command(browser, url)?;
        return Ok(());
    }

    #[cfg(target_os = "macos")]
    {
        run_browser_command("open", url)
    }

    #[cfg(target_os = "windows")]
    {
        let status = Command::new("cmd")
            .args(["/C", "start", "", url])
            .status()
            .context("failed to launch cmd /C start")?;
        if status.success() {
            Ok(())
        } else {
            bail!("browser command exited with status {status}")
        }
    }

    #[cfg(all(unix, not(target_os = "macos")))]
    {
        run_browser_command("xdg-open", url)
    }

    #[cfg(not(any(target_os = "macos", target_os = "windows", unix)))]
    {
        bail!("no supported browser opener was found for this OS")
    }
}

fn run_browser_command(program: &str, url: &str) -> Result<()> {
    let status = Command::new(program)
        .arg(url)
        .status()
        .with_context(|| format!("failed to launch browser command `{program}`"))?;

    if status.success() {
        Ok(())
    } else {
        bail!("browser command `{program}` exited with status {status}")
    }
}
