use anyhow::Result;
use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use url::Url;

pub const AUTHORIZATION_ENDPOINT: &str = "https://app.asana.com/-/oauth_authorize";
pub const DEFAULT_LOCALHOST_REDIRECT_URI: &str = "http://127.0.0.1:18787/callback";
const DEFAULT_SCOPES: &[&str] = &[
    "users:read",
    "workspaces:read",
    "projects:read",
    "tasks:read",
    "stories:read",
    "attachments:read",
];

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AuthorizationUrlOptions {
    pub client_id: String,
    pub redirect_uri: String,
    pub scopes: Vec<String>,
    pub state: Option<String>,
}

pub fn default_scopes() -> Vec<&'static str> {
    DEFAULT_SCOPES.to_vec()
}

pub fn default_localhost_redirect_uri() -> &'static str {
    DEFAULT_LOCALHOST_REDIRECT_URI
}

pub fn generate_state() -> String {
    let bytes: [u8; 32] = rand::random();
    URL_SAFE_NO_PAD.encode(bytes)
}

pub fn build_authorization_url(options: &AuthorizationUrlOptions) -> Result<String> {
    let mut url = Url::parse(AUTHORIZATION_ENDPOINT)?;
    let scope = if options.scopes.is_empty() {
        DEFAULT_SCOPES.join(" ")
    } else {
        options.scopes.join(" ")
    };

    {
        let mut pairs = url.query_pairs_mut();
        pairs.append_pair("client_id", &options.client_id);
        pairs.append_pair("redirect_uri", &options.redirect_uri);
        pairs.append_pair("response_type", "code");
        pairs.append_pair("scope", &scope);
        if let Some(state) = &options.state {
            pairs.append_pair("state", state);
        }
    }

    Ok(url.to_string().replace('+', "%20"))
}
