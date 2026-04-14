use asana_cli::asana_api::{AsanaClient, OAuthExchangeInput, OAuthRefreshInput};
use serde_json::json;
use wiremock::matchers::{body_string_contains, header, method, path, query_param};
use wiremock::{Mock, MockServer, ResponseTemplate};

#[tokio::test]
async fn exchanges_an_authorization_code_for_a_token_and_computes_expires_at() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/-/oauth_token"))
        .and(header("content-type", "application/x-www-form-urlencoded"))
        .and(body_string_contains("grant_type=authorization_code"))
        .and(body_string_contains("client_id=client-1"))
        .and(body_string_contains("client_secret=secret-1"))
        .and(body_string_contains(
            "redirect_uri=urn%3Aietf%3Awg%3Aoauth%3A2.0%3Aoob",
        ))
        .and(body_string_contains("code=code-1"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "access_token": "access-1",
            "refresh_token": "refresh-1",
            "token_type": "bearer",
            "expires_in": 3600
        })))
        .mount(&server)
        .await;

    let client =
        AsanaClient::new(server.uri(), format!("{}/-/oauth_token", server.uri())).expect("client");
    let token = client
        .exchange_code_for_token(OAuthExchangeInput {
            client_id: "client-1".into(),
            client_secret: "secret-1".into(),
            code: "code-1".into(),
            redirect_uri: "urn:ietf:wg:oauth:2.0:oob".into(),
            now_unix_seconds: Some(1_776_135_600),
        })
        .await
        .expect("exchange token");

    assert_eq!(token.access_token, "access-1");
    assert_eq!(token.refresh_token.as_deref(), Some("refresh-1"));
    assert_eq!(token.token_type, "bearer");
    assert_eq!(token.expires_in, Some(3600));
    assert_eq!(token.expires_at.as_deref(), Some("2026-04-14T04:00:00Z"));
}

#[tokio::test]
async fn refreshes_an_access_token_with_a_refresh_token() {
    let server = MockServer::start().await;
    Mock::given(method("POST"))
        .and(path("/-/oauth_token"))
        .and(body_string_contains("grant_type=refresh_token"))
        .and(body_string_contains("refresh_token=refresh-1"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "access_token": "access-2",
            "refresh_token": "refresh-2",
            "token_type": "bearer",
            "expires_in": 7200
        })))
        .mount(&server)
        .await;

    let client =
        AsanaClient::new(server.uri(), format!("{}/-/oauth_token", server.uri())).expect("client");
    let token = client
        .refresh_access_token(OAuthRefreshInput {
            client_id: "client-1".into(),
            client_secret: "secret-1".into(),
            refresh_token: "refresh-1".into(),
            redirect_uri: "urn:ietf:wg:oauth:2.0:oob".into(),
            now_unix_seconds: Some(1_776_135_600),
        })
        .await
        .expect("refresh token");

    assert_eq!(token.access_token, "access-2");
    assert_eq!(token.refresh_token.as_deref(), Some("refresh-2"));
    assert_eq!(token.expires_at.as_deref(), Some("2026-04-14T05:00:00Z"));
}

#[tokio::test]
async fn fetches_me_workspaces_and_paginated_resources() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/users/me"))
        .and(header("authorization", "Bearer access-1"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "data": {"gid": "123", "name": "Alice"}
        })))
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/workspaces"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "data": [{"gid": "1", "name": "Personal"}]
        })))
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/projects"))
        .and(query_param("workspace", "workspace-1"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "data": [{"gid": "10", "name": "Roadmap"}],
            "next_page": {"offset": "next-1"}
        })))
        .up_to_n_times(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/projects"))
        .and(query_param("workspace", "workspace-1"))
        .and(query_param("offset", "next-1"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "data": [{"gid": "11", "name": "Backlog"}],
            "next_page": {"offset": "next-2"}
        })))
        .up_to_n_times(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/projects"))
        .and(query_param("workspace", "workspace-1"))
        .and(query_param("offset", "next-2"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "data": [{"gid": "12", "name": "Archive"}],
            "next_page": null
        })))
        .up_to_n_times(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/projects/project%2Fwith%3Fspecial/tasks"))
        .respond_with(ResponseTemplate::new(200).set_body_json(json!({
            "data": [],
            "next_page": null
        })))
        .mount(&server)
        .await;

    let client =
        AsanaClient::new(server.uri(), format!("{}/-/oauth_token", server.uri())).expect("client");

    let me = client.fetch_me("access-1").await.expect("me");
    let workspaces = client
        .list_workspaces("access-1")
        .await
        .expect("workspaces");
    let projects = client
        .list_projects("access-1", "workspace-1")
        .await
        .expect("projects");
    let tasks = client
        .list_tasks("access-1", "project/with?special")
        .await
        .expect("tasks");

    assert_eq!(me, json!({"gid": "123", "name": "Alice"}));
    assert_eq!(workspaces, vec![json!({"gid": "1", "name": "Personal"})]);
    assert_eq!(
        projects,
        vec![
            json!({"gid": "10", "name": "Roadmap"}),
            json!({"gid": "11", "name": "Backlog"}),
            json!({"gid": "12", "name": "Archive"})
        ]
    );
    assert!(tasks.is_empty());
}

#[tokio::test]
async fn joins_asana_error_messages_when_a_request_fails() {
    let server = MockServer::start().await;
    Mock::given(method("GET"))
        .and(path("/tasks/12345"))
        .respond_with(ResponseTemplate::new(403).set_body_json(json!({
            "errors": [
                {"message": "not authorized"},
                {"message": "workspace mismatch"}
            ]
        })))
        .mount(&server)
        .await;

    let client =
        AsanaClient::new(server.uri(), format!("{}/-/oauth_token", server.uri())).expect("client");
    let error = client
        .get_task("access-1", "12345")
        .await
        .expect_err("request should fail");

    assert!(
        error
            .to_string()
            .contains("not authorized; workspace mismatch")
    );
}
