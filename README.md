# asana-cli

Rust で書いた個人利用向け Asana OAuth CLI です。既存の `asana-oauth-cli` の主要機能を Rust に移植し、GitHub Releases から macOS / Linux 向けバイナリを配布できる前提で構成しています。

主な機能:
- `auth url` で認可 URL を生成
- `auth exchange` で authorization code を token に交換
- `auth login` で localhost callback による自動ログイン
- `auth refresh` で refresh token を使って access token を更新
- `me`
- `workspaces list`
- `projects list` / `project list`
- `tasks list|get|subtasks|stories|comments|attachments`

セキュリティ/UX 方針:
- 設定ファイルは XDG Base Directory (`$XDG_CONFIG_HOME/asana-cli/credentials.json`) を優先
- 設定ファイル権限は `0600` を維持
- `clientSecret` は保存しない
- 標準出力に token を出すときは `access_token` / `refresh_token` を redact
- `auth login` は `http://127.0.0.1/...` または `http://localhost/...` の redirect URI のみ許可

## インストール

### cargo install

```bash
cargo install --path .
```

### リリースバイナリ

GitHub Releases から以下を配布します。
- `x86_64-unknown-linux-gnu`
- `x86_64-apple-darwin`
- `aarch64-apple-darwin`

最新 release:
- `v0.1.3`: https://github.com/ktutumi/asana-cli/releases/tag/v0.1.3

各 archive には対応する `.sha256` ファイルも添付されます。

例:
- `asana-cli-v0.1.3-x86_64-unknown-linux-gnu.tar.gz`
- `asana-cli-v0.1.3-x86_64-unknown-linux-gnu.tar.gz.sha256`

ダウンロード例:

Linux x86_64:
```bash
curl -LO https://github.com/ktutumi/asana-cli/releases/download/v0.1.3/asana-cli-v0.1.3-x86_64-unknown-linux-gnu.tar.gz
curl -LO https://github.com/ktutumi/asana-cli/releases/download/v0.1.3/asana-cli-v0.1.3-x86_64-unknown-linux-gnu.tar.gz.sha256
shasum -a 256 -c asana-cli-v0.1.3-x86_64-unknown-linux-gnu.tar.gz.sha256
```

macOS Intel:
```bash
curl -LO https://github.com/ktutumi/asana-cli/releases/download/v0.1.3/asana-cli-v0.1.3-x86_64-apple-darwin.tar.gz
curl -LO https://github.com/ktutumi/asana-cli/releases/download/v0.1.3/asana-cli-v0.1.3-x86_64-apple-darwin.tar.gz.sha256
shasum -a 256 -c asana-cli-v0.1.3-x86_64-apple-darwin.tar.gz.sha256
```

macOS Apple Silicon:
```bash
curl -LO https://github.com/ktutumi/asana-cli/releases/download/v0.1.3/asana-cli-v0.1.3-aarch64-apple-darwin.tar.gz
curl -LO https://github.com/ktutumi/asana-cli/releases/download/v0.1.3/asana-cli-v0.1.3-aarch64-apple-darwin.tar.gz.sha256
shasum -a 256 -c asana-cli-v0.1.3-aarch64-apple-darwin.tar.gz.sha256
```

macOS で "Apple はマルウェアが含まれていないことを検証できませんでした" と表示される場合:
```bash
xattr -dr com.apple.quarantine ./asana-cli
./asana-cli --help
```

別の回避方法:
- Finder で `asana-cli` を右クリックして「開く」
- もしくは「システム設定 → プライバシーとセキュリティ」から `このまま開く`

補足:
- 現在の配布バイナリは notarization されていないため、macOS では Gatekeeper による確認ダイアログが出ることがあります。
- 上記の `xattr` 解除は、ダウンロード済みバイナリをローカルで使うための回避策です。

展開例:
```bash
tar -xzf asana-cli-v0.1.3-x86_64-unknown-linux-gnu.tar.gz
./asana-cli --help
```

## Asana OAuth アプリ設定

Asana Developer Console で OAuth アプリを作成し、redirect URI を正確に登録してください。

例:
- `urn:ietf:wg:oauth:2.0:oob`
- `http://127.0.0.1:18787/callback`

注意:
- `auth login` は localhost callback 専用です
- OOB/manual copy-paste を使うときは `auth url` + `auth exchange` を使ってください
- localhost callback で `:0` はテスト用です。本番運用では固定ポートを登録してください

## 使い方

### 認可 URL を出す

```bash
asana-cli auth url \
  --client-id "$ASANA_CLIENT_ID" \
  --state demo-state
```

### manual flow で code を交換する

```bash
asana-cli auth exchange \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri urn:ietf:wg:oauth:2.0:oob \
  --code "$ASANA_CODE"
```

### localhost callback で自動ログインする

```bash
asana-cli auth login \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri http://127.0.0.1:18787/callback
```

期待される挙動:
1. CLI が `Open this URL in your browser: ...` を出力
2. ブラウザで Asana 認可画面を開く
3. localhost callback が `code` と `state` を受信
4. token を交換して設定ファイルへ保存

### token を refresh する

```bash
asana-cli auth refresh --client-secret "$ASANA_CLIENT_SECRET"
```

### API を読む

```bash
asana-cli me
asana-cli workspaces list
asana-cli projects list --workspace 123
asana-cli tasks list --project 456
asana-cli tasks get --task 789
asana-cli tasks subtasks --task 789
asana-cli tasks stories --task 789
asana-cli tasks comments --task 789
asana-cli tasks attachments --task 789
```

補足:
- `tasks stories` は task の story 履歴全体を返しますが、Asana API の compact record が中心です。
- `tasks comments` は `comment_added` の story だけを抽出し、本文表示に必要な `text` / `html_text` / `created_at` / `created_by.name` を含めて返します。
- コメント本文を確認したい場合は `tasks comments` を使ってください。

## 設定ファイル

既定パス:

```text
$XDG_CONFIG_HOME/asana-cli/credentials.json
~/.config/asana-cli/credentials.json
```

保存される内容:
- `clientId`
- `redirectUri`
- `token.access_token`
- `token.refresh_token`
- `token.token_type`
- `token.expires_in`
- `token.expires_at`

保存しない内容:
- `clientSecret`

## 開発

```bash
cargo fmt --all
cargo check
cargo test
cargo clippy --all-targets --all-features -- -D warnings
```

## GitHub Actions

- `ci.yml`: fmt / check / test / clippy
- `release.yml`: タグ push で macOS / Linux バイナリをビルドして release asset を作成
