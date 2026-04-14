use anyhow::{Context, Result, anyhow, bail};
use reqwest::Client;
use serde::de::DeserializeOwned;
use serde_json::Value;
use time::{Duration, OffsetDateTime, format_description::well_known::Rfc3339};
use url::Url;

use crate::config::TokenData;

#[derive(Debug, Clone)]
pub struct AsanaClient {
    http: Client,
    api_base: Url,
    api_base_segments: Vec<String>,
    oauth_token_endpoint: Url,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OAuthExchangeInput {
    pub client_id: String,
    pub client_secret: String,
    pub redirect_uri: String,
    pub code: String,
    pub now_unix_seconds: Option<i64>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OAuthRefreshInput {
    pub client_id: String,
    pub client_secret: String,
    pub redirect_uri: String,
    pub refresh_token: String,
    pub now_unix_seconds: Option<i64>,
}

#[derive(Debug, serde::Deserialize)]
struct DataEnvelope<T> {
    data: T,
}

#[derive(Debug, serde::Deserialize)]
struct PagedEnvelope<T> {
    data: Vec<T>,
    next_page: Option<NextPage>,
}

#[derive(Debug, serde::Deserialize)]
struct NextPage {
    offset: Option<String>,
}

#[derive(Debug, serde::Deserialize)]
struct ErrorEnvelope {
    errors: Option<Vec<AsanaErrorMessage>>,
}

#[derive(Debug, serde::Deserialize)]
struct AsanaErrorMessage {
    message: Option<String>,
}

impl AsanaClient {
    pub fn new(api_base: impl AsRef<str>, oauth_token_endpoint: impl AsRef<str>) -> Result<Self> {
        let api_base = Url::parse(api_base.as_ref())?;
        let api_base_segments = api_base
            .path_segments()
            .map(|segments| {
                segments
                    .filter(|segment| !segment.is_empty())
                    .map(ToOwned::to_owned)
                    .collect()
            })
            .unwrap_or_else(Vec::new);

        Ok(Self {
            http: Client::builder().build()?,
            api_base,
            api_base_segments,
            oauth_token_endpoint: Url::parse(oauth_token_endpoint.as_ref())?,
        })
    }

    pub async fn exchange_code_for_token(&self, input: OAuthExchangeInput) -> Result<TokenData> {
        let body = vec![
            ("grant_type", "authorization_code".to_string()),
            ("client_id", input.client_id),
            ("client_secret", input.client_secret),
            ("redirect_uri", input.redirect_uri),
            ("code", input.code),
        ];
        self.post_token(body, input.now_unix_seconds).await
    }

    pub async fn refresh_access_token(&self, input: OAuthRefreshInput) -> Result<TokenData> {
        let body = vec![
            ("grant_type", "refresh_token".to_string()),
            ("client_id", input.client_id),
            ("client_secret", input.client_secret),
            ("redirect_uri", input.redirect_uri),
            ("refresh_token", input.refresh_token),
        ];
        self.post_token(body, input.now_unix_seconds).await
    }

    pub async fn fetch_me(&self, access_token: &str) -> Result<Value> {
        self.get_data_json(access_token, self.join_path("users/me")?)
            .await
    }

    pub async fn list_workspaces(&self, access_token: &str) -> Result<Vec<Value>> {
        self.get_data_vec(access_token, self.join_path("workspaces")?)
            .await
    }

    pub async fn list_projects(&self, access_token: &str, workspace: &str) -> Result<Vec<Value>> {
        let mut url = self.join_path("projects")?;
        url.query_pairs_mut().append_pair("workspace", workspace);
        self.get_paginated(access_token, url).await
    }

    pub async fn list_tasks(&self, access_token: &str, project_gid: &str) -> Result<Vec<Value>> {
        self.get_paginated(
            access_token,
            self.join_segments(&["projects", project_gid, "tasks"])?,
        )
        .await
    }

    pub async fn get_task(&self, access_token: &str, task_gid: &str) -> Result<Value> {
        self.get_data_json(access_token, self.join_segments(&["tasks", task_gid])?)
            .await
    }

    pub async fn list_subtasks(&self, access_token: &str, task_gid: &str) -> Result<Vec<Value>> {
        self.get_paginated(
            access_token,
            self.join_segments(&["tasks", task_gid, "subtasks"])?,
        )
        .await
    }

    pub async fn list_stories(&self, access_token: &str, task_gid: &str) -> Result<Vec<Value>> {
        self.get_paginated(
            access_token,
            self.join_segments(&["tasks", task_gid, "stories"])?,
        )
        .await
    }

    pub async fn list_attachments(&self, access_token: &str, task_gid: &str) -> Result<Vec<Value>> {
        self.get_paginated(
            access_token,
            self.join_segments(&["tasks", task_gid, "attachments"])?,
        )
        .await
    }

    async fn post_token(
        &self,
        body: Vec<(&'static str, String)>,
        now_unix_seconds: Option<i64>,
    ) -> Result<TokenData> {
        let response = self
            .http
            .post(self.oauth_token_endpoint.clone())
            .header(
                reqwest::header::CONTENT_TYPE,
                "application/x-www-form-urlencoded",
            )
            .form(&body)
            .send()
            .await
            .context("failed to call Asana OAuth token endpoint")?;

        let token: TokenData = parse_json(response).await?;
        let issued_at = OffsetDateTime::from_unix_timestamp(
            now_unix_seconds.unwrap_or_else(|| OffsetDateTime::now_utc().unix_timestamp()),
        )?;
        let expires_at = token
            .expires_in
            .map(|seconds| issued_at + Duration::seconds(seconds))
            .map(|value| value.format(&Rfc3339))
            .transpose()?;

        Ok(TokenData {
            expires_at: expires_at.or(token.expires_at),
            ..token
        })
    }

    async fn get_paginated(&self, access_token: &str, mut url: Url) -> Result<Vec<Value>> {
        let mut items = Vec::new();
        loop {
            let response = self
                .http
                .get(url.clone())
                .bearer_auth(access_token)
                .header(reqwest::header::ACCEPT, "application/json")
                .send()
                .await
                .context("failed to call Asana API")?;

            let envelope: PagedEnvelope<Value> = parse_json(response).await?;
            items.extend(envelope.data);

            if let Some(offset) = envelope.next_page.and_then(|page| page.offset) {
                let preserved_pairs = url
                    .query_pairs()
                    .filter(|(key, _)| key != "offset")
                    .map(|(key, value)| (key.into_owned(), value.into_owned()))
                    .collect::<Vec<_>>();
                {
                    let mut pairs = url.query_pairs_mut();
                    pairs.clear();
                    for (key, value) in preserved_pairs {
                        pairs.append_pair(&key, &value);
                    }
                    pairs.append_pair("offset", &offset);
                }
            } else {
                break;
            }
        }

        Ok(items)
    }

    async fn get_data_json(&self, access_token: &str, url: Url) -> Result<Value> {
        let response = self
            .http
            .get(url)
            .bearer_auth(access_token)
            .header(reqwest::header::ACCEPT, "application/json")
            .send()
            .await
            .context("failed to call Asana API")?;
        let envelope: DataEnvelope<Value> = parse_json(response).await?;
        Ok(envelope.data)
    }

    async fn get_data_vec(&self, access_token: &str, url: Url) -> Result<Vec<Value>> {
        let response = self
            .http
            .get(url)
            .bearer_auth(access_token)
            .header(reqwest::header::ACCEPT, "application/json")
            .send()
            .await
            .context("failed to call Asana API")?;
        let envelope: DataEnvelope<Vec<Value>> = parse_json(response).await?;
        Ok(envelope.data)
    }

    fn join_path(&self, path: &str) -> Result<Url> {
        self.api_base.join(path).map_err(Into::into)
    }

    fn join_segments(&self, segments: &[&str]) -> Result<Url> {
        let mut url = self.api_base.clone();
        {
            let mut path_segments = url
                .path_segments_mut()
                .map_err(|()| anyhow!("API base URL cannot be a base for path segments"))?;
            path_segments.clear();
            for segment in &self.api_base_segments {
                path_segments.push(segment);
            }
            for segment in segments {
                path_segments.push(segment);
            }
        }
        Ok(url)
    }
}

async fn parse_json<T: DeserializeOwned>(response: reqwest::Response) -> Result<T> {
    let status = response.status();
    let body = response
        .text()
        .await
        .context("failed to read Asana response body")?;
    if !status.is_success() {
        let message = serde_json::from_str::<ErrorEnvelope>(&body)
            .ok()
            .and_then(|payload| payload.errors)
            .map(|errors| {
                errors
                    .into_iter()
                    .filter_map(|item| item.message)
                    .collect::<Vec<_>>()
                    .join("; ")
            })
            .filter(|joined| !joined.is_empty())
            .unwrap_or_else(|| status.to_string());
        bail!(message);
    }

    serde_json::from_str(&body).map_err(|error| anyhow!(error))
}
