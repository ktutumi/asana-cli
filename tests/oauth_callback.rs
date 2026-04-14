use std::time::Duration;

use asana_cli::oauth_callback::{WaitForOAuthCallbackOptions, wait_for_oauth_callback};
use reqwest::StatusCode;

#[tokio::test]
async fn receives_the_authorization_code_and_state_from_localhost_callback() {
    let listener = wait_for_oauth_callback(WaitForOAuthCallbackOptions {
        hostname: "127.0.0.1".into(),
        port: 0,
        callback_path: "/callback".into(),
        timeout: Duration::from_secs(2),
    })
    .await
    .expect("listener");

    let response = reqwest::get(format!(
        "{}?code=code-123&state=state-abc",
        listener.callback_url()
    ))
    .await
    .expect("callback response");
    let status = response.status();
    let body = response.text().await.expect("body");
    let callback = listener.wait().await.expect("callback result");

    assert_eq!(status, StatusCode::OK);
    assert!(body.contains("Asana OAuth login completed"));
    assert_eq!(callback.code, "code-123");
    assert_eq!(callback.state.as_deref(), Some("state-abc"));
}

#[tokio::test]
async fn returns_an_error_page_when_code_is_missing() {
    let listener = wait_for_oauth_callback(WaitForOAuthCallbackOptions {
        hostname: "127.0.0.1".into(),
        port: 0,
        callback_path: "/callback".into(),
        timeout: Duration::from_secs(2),
    })
    .await
    .expect("listener");

    let response = reqwest::get(format!("{}?state=state-abc", listener.callback_url()))
        .await
        .expect("callback response");
    let status = response.status();
    let body = response.text().await.expect("body");
    let error = listener.wait().await.expect_err("listener should fail");

    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert!(body.contains("Missing `code` query parameter"));
    assert!(error.to_string().contains("Missing `code` query parameter"));
}

#[tokio::test]
async fn does_not_crash_if_an_extra_browser_request_arrives_around_callback_completion() {
    let listener = wait_for_oauth_callback(WaitForOAuthCallbackOptions {
        hostname: "127.0.0.1".into(),
        port: 0,
        callback_path: "/callback".into(),
        timeout: Duration::from_secs(2),
    })
    .await
    .expect("listener");

    let callback_url = listener.callback_url().to_owned();
    let favicon_url = callback_url.replace("/callback", "/favicon.ico");

    let callback_future = reqwest::get(format!("{}?code=code-123&state=state-abc", callback_url));
    let favicon_future = reqwest::get(favicon_url);

    let (callback_result, favicon_result) = tokio::join!(callback_future, favicon_future);
    let callback_response = callback_result.expect("callback response");
    let favicon_response = favicon_result.expect("favicon response");
    let callback = listener.wait().await.expect("callback result");

    assert_eq!(callback_response.status(), StatusCode::OK);
    assert!(matches!(
        favicon_response.status(),
        StatusCode::OK | StatusCode::NOT_FOUND
    ));
    assert_eq!(callback.code, "code-123");
    assert_eq!(callback.state.as_deref(), Some("state-abc"));
}

#[tokio::test]
async fn times_out_cleanly_when_no_callback_arrives() {
    let listener = wait_for_oauth_callback(WaitForOAuthCallbackOptions {
        hostname: "127.0.0.1".into(),
        port: 0,
        callback_path: "/callback".into(),
        timeout: Duration::from_millis(50),
    })
    .await
    .expect("listener");

    let error = listener.wait().await.expect_err("listener should time out");
    assert!(
        error
            .to_string()
            .contains("Timed out waiting for OAuth callback")
    );
}
