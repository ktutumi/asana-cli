# asana-cli

Language: English | [日本語](README.ja.md)

A personal Asana OAuth and API CLI written in Go, structured for distributing macOS and Linux binaries through GitHub Releases.

Key features:
- Generate an authorization URL with `auth url`
- Exchange an authorization code for a token with `auth exchange`
- Complete automatic login via a localhost callback with `auth login`
- Check the status of saved credentials with `auth status`
- Refresh the access token with a refresh token via `auth refresh`
- `me`
- `workspaces list`
- Read and write tasks, subtasks, projects, sections, comments, memberships, and relationships
- Search tasks and resolve custom IDs
- Upload files or external URL attachments
- Duplicate and delete resources, save project templates, and inspect asynchronous jobs

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
asana-cli sections list 456
asana-cli --output table sections list 456
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

### Manage tasks and subtasks

Create a task from a workspace, project, parent task, or project/section membership. `--membership` alone is enough; the CLI also sends the membership project so Asana accepts the request:

```bash
asana-cli tasks create --workspace 123 --name "Prepare launch" --due-on 2026-08-31
asana-cli tasks create --project 456 --name "Review copy" --follower 111 --tag 222
asana-cli tasks create --name "In progress item" --membership 456=123
asana-cli tasks create-subtask 789 --name "Check links"
asana-cli tasks update 789 --completed true
```

Task fields include `--notes` / `--html-notes`, `--assignee`, `--completed`,
`--approval-status`, `--resource-subtype`, `--start-on` / `--start-at`,
`--due-on` / `--due-at`, repeatable `--follower`, `--project`, `--tag`,
`--custom-field GID=STRING`, `--custom-field-json GID=JSON`, and
`--membership PROJECT_GID=SECTION_GID`. Use `--custom-field` for text and enum
option GIDs, including numeric-looking GIDs. Use `--custom-field-json` only for
typed values such as numbers, booleans, arrays, or objects.
Mutually exclusive body and date forms are validated before an API request.
Updates send only fields explicitly provided on the command line.

Change or remove a parent:

```bash
asana-cli tasks set-parent 789 --parent 123 --insert-after 456
asana-cli tasks unset-parent 789
```

### List and search tasks

Exactly one list context is required:

```bash
asana-cli tasks list --project 123
asana-cli tasks list --section 234
asana-cli tasks list --tag 345
asana-cli tasks list --user-task-list 456
asana-cli tasks search --workspace 123 --assignee me --projects-any 456,789 --sort-by due_date --limit 50
asana-cli tasks get-custom-id OPS-42 --workspace 123
```

Use `--opt-fields` on list, search, and get-custom-id commands to request
additional fields. For `table` and `compact` output, list commands also request
the default display columns so those fields are not blank. JSON output does not
add those columns unless you pass `--opt-fields`. Workspace task search requires Asana Premium, is eventually
consistent, and does not support the normal offset pagination used by list
commands. Filters include `--assignee`, `--projects-any`, `--sections-any`,
`--tags-any`, `--text`, `--completed`, `--is-subtask`, `--modified-at-after`,
`--due-on-before` / `--due-on-after`, `--start-on-before` / `--start-on-after`,
`--sort-by`, and `--sort-ascending`. `--limit` accepts at most 100 items. To page
manually, sort by creation time and narrow each subsequent query so it excludes
items already processed.

### Manage sections and project placement

```bash
asana-cli sections get 123
asana-cli sections create --project 456 --name "In progress"
asana-cli sections update 123 --name "Doing"
asana-cli sections tasks 123
asana-cli sections add-task 123 --task 789
asana-cli sections move 123 --project 456 --after-section 111
asana-cli tasks add-project 789 --project 456 --section 123
asana-cli tasks remove-project 789 --project 456
```

`--insert-before` and `--insert-after` (and the equivalent section move flags)
cannot be combined. Asana only permits deleting an empty section and does not
permit deleting the last section in a project.

### Manage comments and task relationships

```bash
asana-cli tasks comment 789 --text "Ready for review"
asana-cli stories get 123
asana-cli stories update 123 --text "Updated comment"
asana-cli stories delete 123
asana-cli tasks dependencies 789
asana-cli tasks add-dependencies 789 --dependency 111 --dependency 222
asana-cli tasks remove-dependencies 789 --dependency 111
asana-cli tasks add-dependents 789 --dependent 333
asana-cli tasks add-tag 789 --tag 444
asana-cli tasks add-followers 789 --follower 555 --follower 666
```

Only comment stories can be edited. `--text` and `--html-text` are mutually
exclusive. Relationship GIDs are repeatable where Asana supports a batch body;
API limit errors, including the combined dependency/dependent limit, are
reported using Asana's error message. Repeated `add-tag` and `remove-tag`
operations use one request per tag and are not atomic. On failure, the error
reports the failing tag and how many preceding tags were already applied.

### Manage projects and memberships

```bash
asana-cli projects get 123
asana-cli projects create --workspace 456 --name "Launch" --icon rocket --default-view list
asana-cli projects update 123 --privacy-setting private --default-access-level editor
asana-cli projects tasks 123
asana-cli tasks projects 789
asana-cli workspaces projects 456
asana-cli workspaces create-project 456 --name "Workspace project"
asana-cli teams projects 789
asana-cli teams create-project 789 --name "Team project"
asana-cli memberships create --parent 123 --member 456 --access-level editor
asana-cli memberships list --parent 123
asana-cli memberships update 789 --access-level commenter
asana-cli memberships delete 789
asana-cli projects add-followers 123 --follower 456
asana-cli projects task-counts 123
asana-cli projects duplicate 123 --name "Launch copy" --start-on 2026-09-01 --skip-weekends true
```

New project sharing should use memberships instead of the deprecated `team`
field. Membership members may be users or teams. Project task-count requests
have an additional Asana rate/cost limit, so avoid polling them frequently.
Project create/update also support repeatable `--custom-field GID=STRING` and
`--custom-field-json GID=JSON` values. When duplicate shifts `--start-on` or
`--due-on`, `--skip-weekends true|false` is required.

### Attachments

The existing `tasks attachments TASK_GID` command is retained and now uses the
official `GET /attachments?parent=...` route. The parent-oriented commands also
support projects and project briefs:

```bash
asana-cli attachments list 789
asana-cli attachments get 123
asana-cli attachments upload --parent 789 --file ./report.pdf
asana-cli attachments upload --parent 789 --url https://example.com/report --name "Report" --connect-to-app
asana-cli attachments delete 123
```

File uploads are limited to 100MB. Non-ASCII filenames are sent as UTF-8
multipart filenames. Local file content and access tokens are never included in
CLI output or API error text.

### Duplicate, template, delete, and jobs

```bash
asana-cli tasks duplicate 789 --name "Copy of task"
asana-cli projects duplicate 123 --name "Copy of project"
asana-cli projects save-as-template 123 --name "Launch template" --public false --workspace 456
asana-cli jobs get JOB_GID
asana-cli tasks delete 789
asana-cli sections delete 123
asana-cli projects delete 456
```

Duplicate and save-as-template commands return a job GID. Use `jobs get` to
inspect success or failure; the CLI does not poll indefinitely. Deleted tasks
remain in the deleting user's Asana trash and can be recovered for 30 days;
afterward Asana removes them permanently.

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
- `ASANA_CLIENT_SECRET`: enables automatic token refresh before API calls when the saved access token is expired or near expiration. The value is never persisted.

### Automatic token refresh

If `ASANA_CLIENT_SECRET` is set, all API commands automatically refresh the saved access token before making requests when it is expired or within 5 minutes of expiration. The refreshed token is saved back to the config file. If the token cannot be refreshed, the command exits with an error instead of making the API call.

## Skills

Skills for AI agents operating this CLI live under `skills/`.

Currently included:
- `skills/asana-cli-operator/`
  - An operational skill for `asana-cli`. It defines how to authenticate, safely read or manage Asana resources, handle custom-field values, refresh tokens, and choose output formats.
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
