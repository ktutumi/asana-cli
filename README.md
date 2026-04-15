# asana-cli

Language: English | [日本語](README.ja.md)

A personal Asana OAuth CLI written in Rust, structured for distributing macOS and Linux binaries through GitHub Releases.

Key features:
- Generate an authorization URL with `auth url`
- Exchange an authorization code for a token with `auth exchange`
- Complete automatic login via a localhost callback with `auth login`
- Check the status of saved credentials with `auth status`
- Refresh the access token with a refresh token via `auth refresh`
- `me`
- `workspaces list`
- `projects list` / `project list`
- `tasks list|get|subtasks|stories|comments|attachments`

Security and UX policy:
- Prefer the XDG Base Directory for the config file (`$XDG_CONFIG_HOME/asana-cli/credentials.json`)
- Keep config file permissions at `0600`
- Do not persist `clientSecret`
- Redact `access_token` / `refresh_token` when printing tokens to stdout
- `auth login` only accepts redirect URIs under `http://127.0.0.1/...` or `http://localhost/...`

## Installation

### cargo install

```bash
cargo install --path .
```

### Prebuilt binaries

Prebuilt binaries are available for the following targets:
- `x86_64-unknown-linux-gnu`
- `x86_64-apple-darwin`
- `aarch64-apple-darwin`

Releases:
- https://github.com/ktutumi/asana-cli/releases

Each archive also includes a matching `.sha256` file.

Example filenames:
- `asana-cli-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz`
- `asana-cli-vX.Y.Z-x86_64-unknown-linux-gnu.tar.gz.sha256`

Download examples:

Linux x86_64:
```bash
VERSION=v0.1.6
curl -LO https://github.com/ktutumi/asana-cli/releases/download/${VERSION}/asana-cli-${VERSION}-x86_64-unknown-linux-gnu.tar.gz
curl -LO https://github.com/ktutumi/asana-cli/releases/download/${VERSION}/asana-cli-${VERSION}-x86_64-unknown-linux-gnu.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-x86_64-unknown-linux-gnu.tar.gz.sha256
```

macOS Intel:
```bash
VERSION=v0.1.6
curl -LO https://github.com/ktutumi/asana-cli/releases/download/${VERSION}/asana-cli-${VERSION}-x86_64-apple-darwin.tar.gz
curl -LO https://github.com/ktutumi/asana-cli/releases/download/${VERSION}/asana-cli-${VERSION}-x86_64-apple-darwin.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-x86_64-apple-darwin.tar.gz.sha256
```

macOS Apple Silicon:
```bash
VERSION=v0.1.6
curl -LO https://github.com/ktutumi/asana-cli/releases/download/${VERSION}/asana-cli-${VERSION}-aarch64-apple-darwin.tar.gz
curl -LO https://github.com/ktutumi/asana-cli/releases/download/${VERSION}/asana-cli-${VERSION}-aarch64-apple-darwin.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-aarch64-apple-darwin.tar.gz.sha256
```

If macOS shows "Apple could not verify this app is free of malware":
```bash
xattr -dr com.apple.quarantine ./asana-cli
./asana-cli --help
```

Alternative workarounds:
- Right-click `asana-cli` in Finder and choose Open
- Or use System Settings → Privacy & Security → Open Anyway

Notes:
- The current distributed binaries are not notarized, so macOS may show a Gatekeeper warning dialog.
- Removing the quarantine attribute with `xattr` is a local workaround for already-downloaded binaries.

Extraction example:
```bash
VERSION=v0.1.6
tar -xzf asana-cli-${VERSION}-x86_64-unknown-linux-gnu.tar.gz
./asana-cli --help
```

## Asana OAuth app setup

Create an OAuth app in the Asana Developer Console and register the redirect URI exactly.

Examples:
- `urn:ietf:wg:oauth:2.0:oob`
- `http://127.0.0.1:18787/callback`

Notes:
- `auth login` is only for the localhost callback flow
- For the OOB/manual copy-paste flow, use `auth url` + `auth exchange`
- `:0` on a localhost callback is only for testing. Register a fixed port for real use

## Usage

### Choose an output format

The default output format is `json`. For more human-friendly output, use `--output table` or `--output compact`.

```bash
asana-cli --output json workspaces list
asana-cli --output table workspaces list
asana-cli --output compact tasks comments 789
```

When to use each format:
- `json`: Pretty JSON with backward compatibility in mind. Easy to process with `jq` and similar tools
- `table`: TSV-like output with headers. Easier for humans to scan in a list
- `compact`: Concise output without headers. Good for quick terminal checks

### Print an authorization URL

```bash
asana-cli auth url \
  --client-id "$ASANA_CLIENT_ID" \
  --state demo-state
```

### Exchange a code in the manual flow

```bash
asana-cli auth exchange \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri urn:ietf:wg:oauth:2.0:oob \
  --code "$ASANA_CODE"
```

### Complete automatic login via localhost callback

```bash
asana-cli auth login \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri http://127.0.0.1:18787/callback
```

If you do not want the browser to open automatically:

```bash
asana-cli auth login \
  --no-open \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri http://127.0.0.1:18787/callback
```

Expected behavior:
1. The CLI prints `Open this URL in your browser: ...`
2. It tries to open the browser automatically if possible, and otherwise tells you to open the URL manually
3. The localhost callback receives `code` and `state`
4. The CLI exchanges the code for tokens and saves them to the config file
5. It reports the config path and the actual redirect URI that was used

### Check saved credentials

```bash
asana-cli auth status
```

This command shows:
- config path
- whether the config file exists
- `clientId` / `redirectUri`
- whether an access token / refresh token exists (the values themselves are redacted)
- `expires_at`

### Refresh a token

```bash
asana-cli auth refresh --client-secret "$ASANA_CLIENT_SECRET"
```

### Query the API

```bash
asana-cli me
asana-cli --output table me
asana-cli workspaces list
asana-cli --output table workspaces list
asana-cli workspaces ls
asana-cli projects list 123
asana-cli --output table projects list 123
asana-cli projects ls --workspace 123
asana-cli tasks list 456
asana-cli --output table tasks list 456
asana-cli tasks ls --project 456
asana-cli tasks get 789
asana-cli --output compact tasks get 789
asana-cli tasks subtasks 789
asana-cli tasks stories 789
asana-cli --output table tasks comments 789
asana-cli tasks comments 789
asana-cli tasks attachments 789
```

Notes:
- `tasks stories` returns the full story history for a task, but it is centered on Asana API compact records.
- `tasks comments` extracts only `comment_added` stories and includes `text` / `html_text` / `created_at` / `created_by.name`, which are needed to display the comment body.
- If you need the actual comment text, prefer `tasks comments`.

## Config file

Default paths:

```text
$XDG_CONFIG_HOME/asana-cli/credentials.json
~/.config/asana-cli/credentials.json
```

Persisted fields:
- `clientId`
- `redirectUri`
- `token.access_token`
- `token.refresh_token`
- `token.token_type`
- `token.expires_in`
- `token.expires_at`

Not persisted:
- `clientSecret`

## Skills

Skills for AI agents operating this CLI live under `skills/`.

Currently included:
- `skills/asana-cli-operator/`
  - An operational skill for `asana-cli`. It defines how to check authentication status, fetch workspaces / projects / tasks / comments / attachments, refresh tokens, and choose output formats.
  - Main file: `skills/asana-cli-operator/SKILL.md`

See `skills/README.md` for details.

## Development

```bash
cargo fmt --all
cargo check
cargo test
cargo clippy --all-targets --all-features -- -D warnings
```

## GitHub Actions

- `ci.yml`: fmt / check / test / clippy
- `release.yml`: builds macOS / Linux binaries and creates release assets when a tag is pushed

## Development flow

- Treat `main` as a protected branch and do not push to it directly
- Make changes on a feature branch and merge into `main` through a Pull Request
- Prefer squash merges when possible, and delete branches that are no longer needed
