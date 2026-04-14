use std::time::Duration;

use anyhow::Result;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use tokio::sync::watch;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OAuthCallbackResult {
    pub code: String,
    pub state: Option<String>,
}

#[derive(Debug, Clone)]
pub struct WaitForOAuthCallbackOptions {
    pub hostname: String,
    pub port: u16,
    pub callback_path: String,
    pub timeout: Duration,
}

#[derive(Debug, Clone)]
pub struct OAuthCallbackListener {
    callback_url: String,
    result_rx: watch::Receiver<Option<std::result::Result<OAuthCallbackResult, String>>>,
    shutdown_tx: watch::Sender<bool>,
}

pub async fn wait_for_oauth_callback(
    options: WaitForOAuthCallbackOptions,
) -> Result<OAuthCallbackListener> {
    let listener = TcpListener::bind((options.hostname.as_str(), options.port)).await?;
    let local_addr = listener.local_addr()?;
    let callback_base_url = format!("http://{}:{}", options.hostname, local_addr.port());
    let callback_url = format!("{}{}", callback_base_url, options.callback_path);
    let (result_tx, result_rx) = watch::channel(None);
    let (shutdown_tx, mut shutdown_rx) = watch::channel(false);
    let callback_path = options.callback_path.clone();
    let timeout = options.timeout;

    tokio::spawn(async move {
        let timeout_sleep = tokio::time::sleep(timeout);
        tokio::pin!(timeout_sleep);

        loop {
            tokio::select! {
                _ = shutdown_rx.changed() => {
                    if *shutdown_rx.borrow() {
                        break;
                    }
                }
                _ = &mut timeout_sleep => {
                    let _ = send_result_once(&result_tx, Err("Timed out waiting for OAuth callback".to_string()));
                    break;
                }
                accept_result = listener.accept() => {
                    let (mut stream, _) = match accept_result {
                        Ok(value) => value,
                        Err(error) => {
                            let _ = send_result_once(&result_tx, Err(error.to_string()));
                            continue;
                        }
                    };

                    let mut buffer = vec![0_u8; 4096];
                    let size = match stream.read(&mut buffer).await {
                        Ok(size) => size,
                        Err(error) => {
                            let _ = write_response(&mut stream, 500, "Internal Server Error", &error.to_string()).await;
                            continue;
                        }
                    };

                    let request_text = String::from_utf8_lossy(&buffer[..size]);
                    let request_target = request_text
                        .lines()
                        .next()
                        .and_then(|line| line.split_whitespace().nth(1))
                        .unwrap_or("/");

                    let request_url = match url::Url::parse(&format!("{}{}", callback_base_url, request_target)) {
                        Ok(url) => url,
                        Err(_) => {
                            let _ = write_response(&mut stream, 400, "Bad Request", "Invalid callback URL").await;
                            continue;
                        }
                    };

                    if request_url.path() != callback_path {
                        let _ = write_response(&mut stream, 404, "Not Found", "Not found").await;
                        continue;
                    }

                    let code = request_url
                        .query_pairs()
                        .find(|(key, _)| key == "code")
                        .map(|(_, value)| value.into_owned());
                    let state = request_url
                        .query_pairs()
                        .find(|(key, _)| key == "state")
                        .map(|(_, value)| value.into_owned());

                    match code {
                        Some(code) => {
                            let _ = write_response(&mut stream, 200, "OK", "Asana OAuth login completed. You can close this tab.").await;
                            let _ = send_result_once(&result_tx, Ok(OAuthCallbackResult { code, state }));
                        }
                        None => {
                            let _ = write_response(&mut stream, 400, "Bad Request", "Missing `code` query parameter").await;
                            let _ = send_result_once(&result_tx, Err("Missing `code` query parameter".to_string()));
                        }
                    }
                }
            }
        }
    });

    Ok(OAuthCallbackListener {
        callback_url,
        result_rx,
        shutdown_tx,
    })
}

impl OAuthCallbackListener {
    pub fn callback_url(&self) -> &str {
        &self.callback_url
    }

    pub async fn wait(&self) -> Result<OAuthCallbackResult> {
        let mut rx = self.result_rx.clone();
        loop {
            if let Some(result) = rx.borrow().clone() {
                let _ = self.shutdown_tx.send(true);
                return result.map_err(anyhow::Error::msg);
            }

            if rx.changed().await.is_err() {
                break;
            }
        }

        let _ = self.shutdown_tx.send(true);
        Err(anyhow::anyhow!(
            "OAuth callback listener closed before producing a result"
        ))
    }

    pub async fn close(&self) {
        let _ = self.shutdown_tx.send(true);
    }
}

fn send_result_once(
    sender: &watch::Sender<Option<std::result::Result<OAuthCallbackResult, String>>>,
    result: std::result::Result<OAuthCallbackResult, String>,
) -> std::result::Result<
    (),
    watch::error::SendError<Option<std::result::Result<OAuthCallbackResult, String>>>,
> {
    if sender.borrow().is_none() {
        sender.send(Some(result))
    } else {
        Ok(())
    }
}

async fn write_response(
    stream: &mut tokio::net::TcpStream,
    status: u16,
    reason: &str,
    body: &str,
) -> Result<()> {
    let response = format!(
        "HTTP/1.1 {} {}\r\ncontent-type: text/plain; charset=utf-8\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{}",
        status,
        reason,
        body.len(),
        body
    );
    stream.write_all(response.as_bytes()).await?;
    stream.shutdown().await?;
    Ok(())
}
