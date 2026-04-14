use std::path::{Path, PathBuf};

use anyhow::Result;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TokenData {
    pub access_token: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub refresh_token: Option<String>,
    pub token_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_in: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<String>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct StoredConfig {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub client_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub redirect_uri: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub token: Option<TokenData>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct StoredConfigPatch {
    pub client_id: Option<String>,
    pub redirect_uri: Option<String>,
    pub token: Option<TokenData>,
}

pub fn default_config_path() -> PathBuf {
    let config_home = dirs::config_dir().unwrap_or_else(|| PathBuf::from(".config"));
    config_home.join("asana-cli").join("credentials.json")
}

pub async fn load_config(path: &Path) -> Result<StoredConfig> {
    match tokio::fs::read_to_string(path).await {
        Ok(content) => Ok(serde_json::from_str(&content)?),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(StoredConfig::default()),
        Err(error) => Err(error.into()),
    }
}

pub async fn save_config(path: &Path, patch: StoredConfigPatch) -> Result<StoredConfig> {
    let current = load_config(path).await?;
    let next = StoredConfig {
        client_id: patch.client_id.or(current.client_id),
        redirect_uri: patch.redirect_uri.or(current.redirect_uri),
        token: merge_token(current.token, patch.token),
    };

    if let Some(parent) = path.parent() {
        tokio::fs::create_dir_all(parent).await?;
    }

    write_config_securely(path, format!("{}\n", serde_json::to_string_pretty(&next)?))?;
    set_owner_only_permissions(path).await?;
    Ok(next)
}

fn merge_token(current: Option<TokenData>, patch: Option<TokenData>) -> Option<TokenData> {
    match (current, patch) {
        (Some(current), Some(patch)) => Some(TokenData {
            access_token: patch.access_token,
            refresh_token: patch.refresh_token.or(current.refresh_token),
            token_type: patch.token_type,
            expires_in: patch.expires_in.or(current.expires_in),
            expires_at: patch.expires_at.or(current.expires_at),
        }),
        (None, Some(patch)) => Some(patch),
        (current, None) => current,
    }
}

fn write_config_securely(path: &Path, content: String) -> Result<()> {
    #[cfg(unix)]
    {
        use std::fs::OpenOptions;
        use std::io::Write;
        use std::os::unix::fs::OpenOptionsExt;

        let mut file = OpenOptions::new()
            .create(true)
            .truncate(true)
            .write(true)
            .mode(0o600)
            .open(path)?;
        file.write_all(content.as_bytes())?;
        file.sync_all()?;
        Ok(())
    }

    #[cfg(not(unix))]
    {
        std::fs::write(path, content)?;
        Ok(())
    }
}

async fn set_owner_only_permissions(path: &Path) -> Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        tokio::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600)).await?;
    }

    Ok(())
}
