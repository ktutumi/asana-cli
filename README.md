# asana-cli

Language: English | [日本語](README.ja.md)

A personal Asana OAuth CLI written in Go, structured for distributing macOS and Linux binaries through GitHub Releases.

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

### go install

```bash
go install github.com/ktutumi/asana-cli-go/cmd/asana-cli@latest
```

### Build from source

```bash
go build -o /tmp/asana-cli ./cmd/asana-cli
/tmp/asana-cli --help
```

### Prebuilt binaries

Prebuilt binaries are available for the following targets:
- `linux-amd64`
- `darwin-amd64`
- `darwin-arm64`

Releases:
- https://github.com/ktutumi/asana-cli-go/releases

Each archive also includes a matching `.sha256` file.

Example filenames:
- `asana-cli-vX.Y.Z-linux-amd64.tar.gz`
- `asana-cli-vX.Y.Z-linux-amd64.tar.gz.sha256`

Download examples:

Linux amd64:
```bash
VERSION=v0.1.0
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-linux-amd64.tar.gz
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-linux-amd64.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-linux-amd64.tar.gz.sha256
```

macOS Intel:
```bash
VERSION=v0.1.0
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-amd64.tar.gz
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-amd64.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-darwin-amd64.tar.gz.sha256
```

macOS Apple Silicon:
```bash
VERSION=v0.1.0
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-arm64.tar.gz
curl -LO https://github.com/ktutumi/asana-cli-go/releases/download/${VERSION}/asana-cli-${VERSION}-darwin-arm64.tar.gz.sha256
shasum -a 256 -c asana-cli-${VERSION}-darwin-arm64.tar.gz.sha256
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
VERSION=v0.1.0
tar -xzf asana-cli-${VERSION}-linux-amd64.tar.gz
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

The default output format is `table`. Use `--output json` or `--output compact` when needed.

```bash
asana-cli --output json workspaces list
asana-cli --output table workspaces list
asana-cli --output compact tasks comments 789
```

When to use each format:
- `json`: Pretty JSON. Easy to process with `jq` and similar tools
- `table`: TSV-like output with headers. Easier for humans to scan in a list
- `compact`: Concise `field=value` output. Collections are rendered as one item per line

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
1. The CLI prints the URL to open in your browser
2. It tries to open the browser automatically if possible, and otherwise tells you to open the URL manually
3. The localhost callback receives `code` and `state`
4. The CLI exchanges the code for tokens and saves them to the config file

### Check saved credentials

```bash
asana-cli auth status
```

This command shows:
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

Override the path with `--config /path/to/credentials.json`.

## Environment variables

- `ASANA_API_BASE`: override the Asana API base URL
- `ASANA_OAUTH_TOKEN_ENDPOINT`: override the OAuth token endpoint
- `BROWSER`: browser command used by `auth login`

## Skills

Skills for AI agents operating this CLI live under `skills/`.

Currently included:
- `skills/asana-cli-operator/`
  - An operational skill for `asana-cli`. It defines how to check authentication status, fetch workspaces / projects / tasks / comments / attachments, refresh tokens, and choose output formats.
  - Main file: `skills/asana-cli-operator/SKILL.md`

See `skills/README.md` for details.

## Development

```bash
gofmt -w cmd/asana-cli/main.go internal/**/*.go
go test ./...
go vet ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```

Safe smoke checks:

```bash
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

### Git hooks (lefthook)

This project uses [lefthook](https://github.com/evilmartians/lefthook) to run formatting and linting before every commit. Install it once with:

```bash
# Install lefthook (requires Go toolchain)
go install github.com/evilmartians/lefthook@latest

# Activate hooks in this repository
lefthook install
```

After activation, the following checks run automatically on every commit:

- `gofmt` — ensures all Go files are formatted
- `go vet` — runs static analysis
- `go test` — runs the test suite

## GitHub Actions

- `ci.yml`: gofmt check / vet / test
- `release.yml`: builds macOS / Linux binaries and creates release assets when a tag is pushed

## Development flow

- Treat `main` as a protected branch and do not push to it directly
- Make changes on a feature branch and merge into `main` through a Pull Request
- Prefer squash merges when possible, and delete branches that are no longer needed
