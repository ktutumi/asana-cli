# AGENTS.md

このリポジトリで AI Agent が安全かつ自律的に作業するためのプロジェクト指示です。

## Project Overview

- Project: `github.com/ktutumi/asana-cli-go`
- Product: Asana OAuth CLI written in Go.
- Goal: Terminal-first CLI for Asana OAuth authentication and read-only Asana API access.
- Main binary: `cmd/asana-cli/main.go`
- Core packages:
  - `internal/cli`: command parsing, subcommands, rendering, config/token orchestration.
  - `internal/asana`: Asana API and OAuth token HTTP client.
  - `internal/oauth`: authorization URL generation and localhost callback server.
  - `internal/config`: credentials file load/save and token merge behavior.

## Hard Rules

1. Never print, log, commit, or snapshot real `client_secret`, `access_token`, `refresh_token`, authorization `code`, or credentials file contents.
2. Do not run commands that call the real Asana API unless the user explicitly asks and provides credentials for that run.
3. Prefer hermetic tests with `httptest` and temp config paths over live network tests.
4. Keep the CLI dependency-light. Do not add a CLI framework unless the user explicitly asks for a larger refactor.
5. Keep user-facing command names and flags backward compatible unless the task is explicitly a breaking change.
6. Preserve Japanese README tone when editing documentation.
7. For any behavior change, add or update tests first when practical.

## Build and Test Commands

Run from the repository root:

```sh
go test ./...
go test ./internal/cli -run TestName -v
go build -o /tmp/asana-cli ./cmd/asana-cli
```

Useful manual smoke checks that do not require credentials:

```sh
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

If formatting changed:

```sh
gofmt -w <changed-go-files>
go test ./...
```

## Architecture Notes

### CLI flow

`cmd/asana-cli/main.go` passes `os.Args[1:]` into `cli.RunCLI` with `cli.NewRuntimeOptionsFromEnv()`.

`internal/cli/cli.go`:

- Parses global flags before the command: `--config`, `--output`, `--help`, `--version`.
- Builds default runtime endpoints when env overrides are absent.
- Routes commands to `handleAuth`, `handleProjects`, `handleTasks`, `handleWorkspaces`, or `me`.
- Uses `CliIO` to make stdout/stderr assertions easy in tests.
- Renders output as `json`, `table`, or `compact`.

### Asana API client

`internal/asana/client.go` wraps an `http.Client` and exposes small methods such as `FetchMe`, `ListWorkspaces`, `ListProjects`, `ListTasks`, `GetTask`, `ListComments`, and OAuth token exchange/refresh.

Pagination is handled in `getPaginated`; new list endpoints should normally reuse it.

### OAuth and config

`internal/oauth` handles URL generation, CSRF state generation, callback parsing, and the localhost callback server.

`internal/config` stores credentials at the platform config path by default and writes files with owner-only permissions. Tests must use temp config paths.

## Implementation Workflow

For non-trivial tasks, use the in-repo skill `.claude/skills/asana-cli-go-development/SKILL.md` and delegate to subagents in `.claude/agents/`:

1. Clarify the smallest safe behavior change.
2. Write or update tests first, usually in `internal/cli/cli_test.go` or a package-specific `_test.go` file.
3. Implement the minimal change.
4. Run `gofmt` on changed Go files.
5. Run `go test ./...`.
6. Run a targeted smoke command when relevant.
7. Review for secret handling, output compatibility, and Asana API URL correctness.

## Recommended Subagent Routing

- Use `go-cli-implementer` for command parsing, rendering, test-first CLI features, and small Go implementation tasks.
- Use `asana-api-reviewer` for endpoint paths, query parameters, pagination, OAuth behavior, and API error handling.
- Use `test-security-reviewer` before finalizing changes that touch auth, config, HTTP requests, output rendering, or filesystem permissions.

Do not dispatch multiple implementers to edit `internal/cli/cli.go` concurrently; it is a single large routing file and merge conflicts are likely.

## Testing Guidance

- CLI tests should instantiate `captureIO` and call `RunCLI(args, io, RuntimeOptions{})`.
- Use `t.TempDir()` for all `--config` paths.
- Prefer `strings.Contains` assertions for help/error text unless exact output is part of the contract.
- For HTTP behavior, prefer `httptest.Server` plus `RuntimeOptions{APIBase: server.URL + "/", TokenEndpoint: server.URL + "/oauth"}`.
- Test both success and user-error paths for new flags/subcommands.

## Output Compatibility

- `json` output must remain valid pretty-printed JSON.
- `table` output uses tab-separated columns and a header row for collections.
- `compact` output uses `field=value` lines.
- Sanitization must preserve one-line table/compact output by escaping tabs, CR, LF, and backslashes.

## Security Checklist

Before finishing auth/config/API changes:

- [ ] No real secrets in code, tests, docs, or terminal output.
- [ ] Token output is redacted where appropriate.
- [ ] Config writes retain `0600` file permissions and private config directory behavior.
- [ ] OAuth state is generated and checked for localhost login.
- [ ] Callback listener remains localhost-only.
- [ ] Tests avoid live Asana network calls.

## Documentation Checklist

When adding or changing commands:

- [ ] Update README command list and examples.
- [ ] Update help text in `internal/cli/cli.go`.
- [ ] Add tests for new help/error behavior.
- [ ] Mention any new environment variable in README.
