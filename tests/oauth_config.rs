use std::fs;

use asana_cli::config::{TokenData, load_config, save_config};
use asana_cli::oauth::{
    AuthorizationUrlOptions, build_authorization_url, default_localhost_redirect_uri,
    default_scopes, generate_state,
};
use tempfile::tempdir;

#[test]
fn builds_an_asana_authorization_url_with_required_parameters() {
    let url = build_authorization_url(&AuthorizationUrlOptions {
        client_id: "client-123".into(),
        redirect_uri: "urn:ietf:wg:oauth:2.0:oob".into(),
        scopes: vec!["default".into(), "tasks:read".into()],
        state: Some("state-abc".into()),
    })
    .expect("authorization url");

    assert_eq!(
        url,
        "https://app.asana.com/-/oauth_authorize?client_id=client-123&redirect_uri=urn%3Aietf%3Awg%3Aoauth%3A2.0%3Aoob&response_type=code&scope=default%20tasks%3Aread&state=state-abc"
    );
}

#[test]
fn returns_a_secure_random_state_token() {
    let state = generate_state();
    assert_eq!(state.len(), 43);
    assert!(
        state
            .chars()
            .all(|ch| ch.is_ascii_alphanumeric() || ch == '-' || ch == '_')
    );
}

#[test]
fn exposes_conservative_default_scopes_for_current_commands() {
    assert_eq!(
        default_scopes(),
        vec![
            "users:read",
            "workspaces:read",
            "projects:read",
            "tasks:read",
            "stories:read",
            "attachments:read",
        ]
    );
}

#[test]
fn uses_a_non_8787_localhost_redirect_uri_default_for_auto_login() {
    assert_eq!(
        default_localhost_redirect_uri(),
        "http://127.0.0.1:18787/callback"
    );
}

#[tokio::test]
async fn persists_and_reloads_merged_config_values_with_0600_permissions() {
    let temp = tempdir().expect("tempdir");
    let config_path = temp.path().join("credentials.json");

    save_config(
        &config_path,
        asana_cli::config::StoredConfigPatch {
            client_id: Some("client-1".into()),
            redirect_uri: Some("urn:ietf:wg:oauth:2.0:oob".into()),
            token: None,
        },
    )
    .await
    .expect("save config header");

    save_config(
        &config_path,
        asana_cli::config::StoredConfigPatch {
            client_id: None,
            redirect_uri: None,
            token: Some(TokenData {
                access_token: "access-1".into(),
                refresh_token: Some("refresh-1".into()),
                token_type: "bearer".into(),
                expires_in: Some(3600),
                expires_at: Some("2026-04-14T03:00:00.000Z".into()),
            }),
        },
    )
    .await
    .expect("save config token");

    let config = load_config(&config_path).await.expect("load config");
    let mode = fs::metadata(&config_path)
        .expect("metadata")
        .permissions()
        .mode()
        & 0o777;

    assert_eq!(config.client_id.as_deref(), Some("client-1"));
    assert_eq!(
        config.redirect_uri.as_deref(),
        Some("urn:ietf:wg:oauth:2.0:oob")
    );
    assert_eq!(
        config.token,
        Some(TokenData {
            access_token: "access-1".into(),
            refresh_token: Some("refresh-1".into()),
            token_type: "bearer".into(),
            expires_in: Some(3600),
            expires_at: Some("2026-04-14T03:00:00.000Z".into()),
        })
    );
    assert_eq!(mode, 0o600);
}

trait PermissionsModeExt {
    fn mode(&self) -> u32;
}

impl PermissionsModeExt for std::fs::Permissions {
    fn mode(&self) -> u32 {
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            PermissionsExt::mode(self)
        }
        #[cfg(not(unix))]
        {
            0
        }
    }
}
