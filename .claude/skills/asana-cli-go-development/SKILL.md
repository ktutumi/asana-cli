---
name: asana-cli-go-development
description: Use when implementing, reviewing, or planning changes to commands, flags, output, OAuth, credential storage, or Asana HTTP behavior in this Go repository.
version: 1.1.0
author: Project Agents
license: MIT
metadata:
  hermes:
    tags: [go, cli, asana, oauth, testing, agentic-development]
    related_skills: [test-driven-development, systematic-debugging, requesting-code-review]
---

# asana-cli-go Development

## Overview

Use the repository's existing, dependency-light patterns instead of inventing new
abstractions. Protect authentication data and public CLI behavior with focused,
hermetic tests.

The Go module is `github.com/ktutumi/asana-cli-go`. It provides Asana OAuth,
local credential storage, and read-only API commands.

## When to Use

Use this skill when:

- Adding or changing a command, flag, alias, output format, error, or help text.
- Adding or reviewing Asana API or OAuth HTTP behavior.
- Changing OAuth login, manual exchange, refresh, callback handling, or config
  persistence.
- Reviewing changes for secret leakage, output compatibility, or API correctness.

Do not use it for repository housekeeping that does not affect Go code, CLI
behavior, API behavior, authentication, tests, or user documentation.

## Hard Gates

1. Never expose real credentials, secrets, authorization codes, or token values.
2. Never call the real Asana API unless the user explicitly requests that run and
   supplies the required credentials.
3. Preserve existing commands, aliases, flags, output contracts, and credential
   protections unless the user explicitly requests a breaking change.
4. Use `httptest.Server` and temporary config paths for tests.

## Repository Map

- `cmd/asana-cli/main.go`: binary entrypoint.
- `internal/cli/cli.go`: `RunCLI`, global flag parsing, `authCmd`, `apiCmd`,
  rendering, and help text.
- `internal/cli/cli_test.go`: CLI tests with captured `CliIO`.
- `internal/asana/client.go`: OAuth token requests and Asana API client.
- `internal/asana/client_test.go`: hermetic HTTP request and response tests.
- `internal/oauth/oauth.go`: authorization URL, state, and redirect validation.
- `internal/oauth/callback.go`: localhost callback server.
- `internal/config/config.go`: credential load, merge, save, and permissions.
- `README.md`: English user documentation.
- `README.ja.md`: Japanese user documentation.

## Implementation Playbook

1. Identify the behavior boundary.
   - Parsing, routing, rendering, help: `internal/cli`
   - API paths, queries, pagination, token HTTP: `internal/asana`
   - Authorization URL, redirect validation, callback, state: `internal/oauth`
   - Credential persistence and permissions: `internal/config`

2. Add or update focused tests before implementation when practical.
   - CLI: call `RunCLI(args, io, RuntimeOptions{})` with captured output.
   - HTTP: use `httptest.Server`; point `APIBase` or `TokenEndpoint` at it.
   - Config: use `t.TempDir()` and an explicit config path.
   - Cover both success and user-error behavior for new flags or subcommands.

3. Implement the smallest compatible change.
   - Keep global flags before the subcommand.
   - Preserve existing singular/plural aliases where supported.
   - Keep errors actionable without including secrets or response bodies that
     may contain secrets.

4. Update user-facing surfaces together.
   - Command routing and parsing in `internal/cli/cli.go`.
   - `rootHelp` or `commandHelp`.
   - Both `README.md` and `README.ja.md`.
   - Output columns when table or compact output needs a stable field set.

5. Format, test, build, and review the final diff.

## Adding a Read-Only Asana Command

1. Add CLI routing and validation tests.
2. Add the client method in `internal/asana/client.go`.
   - Escape each path parameter with `url.PathEscape`.
   - Use `getOne` for a single `data` object.
   - Use `getList` for a `data` collection; it follows `next_page.offset`.
   - Pass query parameters as `url.Values`; `doJSON` encodes them.
3. Route the command through `apiCmd`.
4. Add or update the entry in `columns` for stable table/compact output.
5. Update help and both README files.
6. Run targeted tests, then the full verification commands.

<Good>

```go
return c.getList(token, "tasks/"+url.PathEscape(gid)+"/subtasks", nil)
```

This follows the existing pagination path and escapes the user-supplied GID.

</Good>

<Bad>

```go
return c.getList(token, "tasks/"+gid+"/subtasks", nil)
```

This bypasses path escaping and can produce an incorrect request path.

</Bad>

Do not name a helper from memory. Inspect `internal/asana/client.go` and reuse the
helper that exists in the current checkout.

## OAuth and Config Changes

- `auth login` accepts only `http://localhost/...` or
  `http://127.0.0.1/...` redirect URIs with a path and without query or fragment.
- Generate OAuth `state` when omitted and compare it with the callback value.
- The manual flow is `auth url` followed by `auth exchange`; OOB redirect URIs
  are not supported.
- Do not persist `clientSecret`.
- Redact access and refresh tokens from all user-facing output.
- Preserve config directory mode `0700` and credentials file mode `0600`.
- Test token endpoints with `TokenEndpoint`; never use the production endpoint
  in tests.

## Output Contract

- `json`: pretty-printed JSON produced by `json.MarshalIndent`.
- `table` collection: header row followed by tab-separated rows.
- `table` object: `field<TAB>value` per line.
- `compact`: `field=value` lines; collection fields share one logical line.
- `table` and `compact`: escape backslash, tab, CR, and LF.

Prefer stable scalar columns. Use dotted paths such as `created_by.name` for
nested fields already supported by `value`.

## Focused Skill Routing

Read only the focused skill needed for the current task:

- `.claude/skills/implement-asana-cli-go/SKILL.md`: focused CLI implementation
  and tests.
- `.claude/skills/review-asana-api/SKILL.md`: read-only review of endpoint,
  query, pagination, OAuth, and error behavior.
- `.claude/skills/review-asana-cli-security/SKILL.md`: final review when auth,
  config, HTTP, filesystem permissions, secrets, or output contracts changed.

The active agent remains responsible for inspecting the final diff and running
the verification commands.

## Verification

Run from the repository root. Every executed command must exit successfully.

```sh
gofmt -w <changed-go-files>
go test ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```

Run `gofmt` only when Go files changed. Add a targeted test command such as
`go test ./internal/cli -run TestName -v` while iterating.

Use relevant credential-free smoke checks:

```sh
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

Before completion, confirm:

- The final diff contains no real credentials, token values, or authorization
  codes.
- HTTP tests use local test servers and config tests use temporary paths.
- API paths, query parameters, pagination, and error paths are tested or reviewed.
- Help and both README files match any user-facing change.
- Output and credential-file contracts remain intact.

If an endpoint or OAuth assumption cannot be established from the current code,
tests, or authoritative Asana documentation, stop and report the uncertainty
instead of inventing behavior.
