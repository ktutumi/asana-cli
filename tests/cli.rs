use std::fs;
use std::path::PathBuf;
use std::time::Duration;

use asana_cli::cli::{BufferedCliIo, RuntimeOptions, run_cli_catching};
use serde_json::Value;
use tempfile::tempdir;
use wiremock::matchers::{body_string_contains, header, method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

#[tokio::test]
async fn prints_the_authorization_url_for_auth_url() {
    let io = BufferedCliIo::default();

    let exit_code = run_cli_catching(
        &[
            "auth",
            "url",
            "--client-id",
            "client-1",
            "--state",
            "state-1",
        ],
        &io,
        RuntimeOptions::default(),
    )
    .await;

    assert_eq!(exit_code, 0);
    let stdout = io.stdout_lines();
    assert_eq!(stdout.len(), 1);
    assert!(stdout[0].contains("https://app.asana.com/-/oauth_authorize?"));
    assert!(stdout[0].contains("client_id=client-1"));
    assert!(stdout[0].contains("state=state-1"));
}

#[tokio::test]
async fn completes_auth_login_through_localhost_callback_and_saves_the_token() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/-/oauth_token"))
        .and(body_string_contains("grant_type=authorization_code"))
        .and(body_string_contains("code=code-1"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            r#"{"access_token":"access-1","refresh_token":"refresh-1","token_type":"bearer","expires_in":3600}"#,
            "application/json",
        ))
        .mount(&server)
        .await;

    let temp = tempdir().expect("tempdir");
    let config_path = temp.path().join("credentials.json");
    let io = BufferedCliIo::default();
    let runtime = RuntimeOptions {
        api_base: Some(server.uri()),
        oauth_token_endpoint: Some(format!("{}/-/oauth_token", server.uri())),
    };

    let io_for_task = io.clone();
    let config_path_for_task = config_path.clone();
    let cli_task = tokio::spawn(async move {
        run_cli_catching(
            &[
                "--config",
                config_path_for_task.to_str().expect("config path"),
                "auth",
                "login",
                "--client-id",
                "client-1",
                "--client-secret",
                "secret-1",
                "--redirect-uri",
                "http://127.0.0.1:0/callback",
                "--state",
                "state-1",
                "--listen-timeout-ms",
                "2000",
            ],
            &io_for_task,
            runtime,
        )
        .await
    });

    let auth_url = tokio::time::timeout(Duration::from_secs(3), async {
        loop {
            if let Some(url) = io.stdout_lines().iter().find_map(|line| {
                line.strip_prefix("Open this URL in your browser: ")
                    .map(ToOwned::to_owned)
            }) {
                break url;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .expect("auth url timeout");

    let auth_url = reqwest::Url::parse(&auth_url).expect("auth url parse");
    let mut callback = reqwest::Url::parse(
        auth_url
            .query_pairs()
            .find(|(key, _)| key == "redirect_uri")
            .expect("redirect uri")
            .1
            .as_ref(),
    )
    .expect("callback parse");
    callback.query_pairs_mut().append_pair("code", "code-1");
    callback.query_pairs_mut().append_pair("state", "state-1");

    let callback_response = reqwest::get(callback).await.expect("callback request");
    assert_eq!(callback_response.status(), reqwest::StatusCode::OK);

    let exit_code = cli_task.await.expect("cli join");
    assert_eq!(exit_code, 0);

    let saved: Value =
        serde_json::from_str(&fs::read_to_string(&config_path).expect("saved config"))
            .expect("saved json");
    assert_eq!(saved["clientId"], "client-1");
    assert!(saved.get("clientSecret").is_none());
    assert!(
        saved["redirectUri"]
            .as_str()
            .unwrap_or_default()
            .starts_with("http://127.0.0.1:")
    );
    assert_eq!(saved["token"]["access_token"], "access-1");
    assert_eq!(saved["token"]["refresh_token"], "refresh-1");
    assert!(
        io.stdout_lines()
            .iter()
            .any(|line| line.contains("\"access_token\": \"***\""))
    );
}

#[tokio::test]
async fn fails_auth_login_when_the_callback_state_is_missing() {
    let temp = tempdir().expect("tempdir");
    let config_path = temp.path().join("credentials.json");
    let io = BufferedCliIo::default();

    let io_for_task = io.clone();
    let config_path_for_task = config_path.clone();
    let cli_task = tokio::spawn(async move {
        run_cli_catching(
            &[
                "--config",
                config_path_for_task.to_str().expect("config path"),
                "auth",
                "login",
                "--client-id",
                "client-1",
                "--client-secret",
                "secret-1",
                "--redirect-uri",
                "http://127.0.0.1:0/callback",
                "--state",
                "state-1",
                "--listen-timeout-ms",
                "2000",
            ],
            &io_for_task,
            RuntimeOptions::default(),
        )
        .await
    });

    let auth_url = tokio::time::timeout(Duration::from_secs(3), async {
        loop {
            if let Some(url) = io.stdout_lines().iter().find_map(|line| {
                line.strip_prefix("Open this URL in your browser: ")
                    .map(ToOwned::to_owned)
            }) {
                break url;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .expect("auth url timeout");

    let auth_url = reqwest::Url::parse(&auth_url).expect("auth url parse");
    let mut callback = reqwest::Url::parse(
        auth_url
            .query_pairs()
            .find(|(key, _)| key == "redirect_uri")
            .expect("redirect uri")
            .1
            .as_ref(),
    )
    .expect("callback parse");
    callback.query_pairs_mut().append_pair("code", "code-1");

    let callback_response = reqwest::get(callback).await.expect("callback request");
    assert_eq!(callback_response.status(), reqwest::StatusCode::OK);

    let exit_code = cli_task.await.expect("cli join");
    assert_eq!(exit_code, 1);
    assert!(
        io.stderr_lines()
            .iter()
            .any(|line| line.contains("OAuth state mismatch"))
    );
}

#[tokio::test]
async fn lists_projects_for_a_workspace_with_projects_and_project_alias() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/projects"))
        .and(query_param("workspace", "workspace-1"))
        .and(header("authorization", "Bearer access-1"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            r#"{"data":[{"gid":"10","name":"Roadmap"}],"next_page":null}"#,
            "application/json",
        ))
        .expect(2)
        .mount(&server)
        .await;

    let temp = tempdir().expect("tempdir");
    let config_path = write_config(temp.path().join("credentials.json"));

    let io = BufferedCliIo::default();
    let runtime = RuntimeOptions {
        api_base: Some(server.uri()),
        oauth_token_endpoint: Some(format!("{}/-/oauth_token", server.uri())),
    };

    let exit_code = run_cli_catching(
        &[
            "--config",
            config_path.to_str().expect("config path"),
            "projects",
            "list",
            "--workspace",
            "workspace-1",
        ],
        &io,
        runtime.clone(),
    )
    .await;
    assert_eq!(exit_code, 0);
    assert!(io.stdout_lines()[0].contains("Roadmap"));

    let alias_io = BufferedCliIo::default();
    let exit_code = run_cli_catching(
        &[
            "--config",
            config_path.to_str().expect("config path"),
            "project",
            "list",
            "--workspace",
            "workspace-1",
        ],
        &alias_io,
        runtime,
    )
    .await;
    assert_eq!(exit_code, 0);
    assert!(alias_io.stdout_lines()[0].contains("Roadmap"));
}

#[tokio::test]
async fn help_shows_release_relevant_commands() {
    let io = BufferedCliIo::default();
    let exit_code = run_cli_catching(&["--help"], &io, RuntimeOptions::default()).await;

    assert_eq!(exit_code, 0);
    let output = io.stdout_lines().join("\n");
    assert!(output.contains("auth"));
    assert!(output.contains("projects"));
    assert!(output.contains("tasks"));
    assert!(output.contains("workspaces"));
    assert!(output.contains("me"));
}

#[tokio::test]
async fn routes_me_workspaces_and_task_commands_through_the_saved_access_token() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/api/1.0/users/me"))
        .and(header("authorization", "Bearer access-1"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_raw(r#"{"data":{"gid":"1","name":"Alice"}}"#, "application/json"),
        )
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/api/1.0/workspaces"))
        .and(header("authorization", "Bearer access-1"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            r#"{"data":[{"gid":"w1","name":"Personal"}]}"#,
            "application/json",
        ))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/api/1.0/tasks/task-1"))
        .and(header("authorization", "Bearer access-1"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            r#"{"data":{"gid":"task-1","name":"Buy groceries"}}"#,
            "application/json",
        ))
        .mount(&server)
        .await;
    Mock::given(method("GET"))
        .and(path("/api/1.0/tasks/task-1/stories"))
        .and(query_param(
            "opt_fields",
            "gid,resource_subtype,resource_type,text,html_text,created_at,created_by.name",
        ))
        .and(header("authorization", "Bearer access-1"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            r#"{"data":[{"gid":"story-1","resource_subtype":"comment_added","resource_type":"story","text":"Looks good","html_text":"<body>Looks good</body>","created_at":"2026-04-14T03:00:00.000Z","created_by":{"name":"Alice"}},{"gid":"story-2","resource_subtype":"assigned","resource_type":"story","text":"assigned this task to Alice"}],"next_page":null}"#,
            "application/json",
        ))
        .mount(&server)
        .await;

    let temp = tempdir().expect("tempdir");
    let config_path = write_config(temp.path().join("credentials.json"));
    let runtime = RuntimeOptions {
        api_base: Some(format!("{}/api/1.0/", server.uri())),
        oauth_token_endpoint: Some(format!("{}/-/oauth_token", server.uri())),
    };

    let me_io = BufferedCliIo::default();
    assert_eq!(
        run_cli_catching(
            &["--config", config_path.to_str().expect("config path"), "me"],
            &me_io,
            runtime.clone(),
        )
        .await,
        0
    );
    assert!(me_io.stdout_lines()[0].contains("Alice"));

    let workspaces_io = BufferedCliIo::default();
    assert_eq!(
        run_cli_catching(
            &[
                "--config",
                config_path.to_str().expect("config path"),
                "workspaces",
                "list"
            ],
            &workspaces_io,
            runtime.clone(),
        )
        .await,
        0
    );
    assert!(workspaces_io.stdout_lines()[0].contains("Personal"));

    let task_io = BufferedCliIo::default();
    assert_eq!(
        run_cli_catching(
            &[
                "--config",
                config_path.to_str().expect("config path"),
                "tasks",
                "get",
                "--task",
                "task-1",
            ],
            &task_io,
            runtime.clone(),
        )
        .await,
        0
    );
    assert!(task_io.stdout_lines()[0].contains("Buy groceries"));

    let comments_io = BufferedCliIo::default();
    assert_eq!(
        run_cli_catching(
            &[
                "--config",
                config_path.to_str().expect("config path"),
                "tasks",
                "comments",
                "--task",
                "task-1",
            ],
            &comments_io,
            runtime,
        )
        .await,
        0
    );
    let comments_output = comments_io.stdout_lines().join("\n");
    assert!(comments_output.contains("Looks good"));
    assert!(!comments_output.contains("assigned this task to Alice"));
}

fn write_config(path: PathBuf) -> PathBuf {
    fs::write(
        &path,
        r#"{
  "clientId": "client-1",
  "redirectUri": "http://127.0.0.1:18787/callback",
  "token": {
    "access_token": "access-1",
    "refresh_token": "refresh-1",
    "token_type": "bearer"
  }
}
"#,
    )
    .expect("write config");
    path
}
