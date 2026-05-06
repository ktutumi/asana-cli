---
name: asana-cli-go-development
description: Use when implementing, reviewing, or planning changes in ktutumi/asana-cli-go, a dependency-light Go CLI for Asana OAuth and read-only API access.
version: 1.0.0
author: Project Agents
license: MIT
metadata:
  hermes:
    tags: [go, cli, asana, oauth, testing, agentic-development]
    related_skills: [subagent-driven-development, test-driven-development, systematic-debugging, requesting-code-review]
---

# asana-cli-go Development

## Overview

This skill captures the project-specific workflow for `github.com/ktutumi/asana-cli-go`.

The project is a small Go CLI with no runtime dependencies beyond the standard library. It implements Asana OAuth login/manual flow, local credential storage, and read-only Asana API commands for users, workspaces, projects, tasks, stories, comments, and attachments.

Use this skill to keep agents aligned on architecture, tests, security boundaries, and compatibility expectations.

## When to Use

Use this skill when:

- Adding or changing an `asana-cli` command, flag, output format, or help text.
- Adding an Asana API endpoint wrapper.
- Changing OAuth login, manual exchange, refresh, callback server, or config persistence.
- Reviewing a proposed patch for secret leakage, output compatibility, or API correctness.
- Planning multi-step work that should be delegated to project subagents.

Do not use this skill for unrelated repository housekeeping that does not touch Go, CLI behavior, Asana API behavior, docs, or tests.

## Repository Map

- `cmd/asana-cli/main.go`: binary entrypoint; calls `cli.RunCLI` and exits with the returned code.
- `internal/cli/cli.go`: command router, flag parser, output rendering, help text, auth command orchestration.
- `internal/cli/cli_test.go`: CLI behavior tests using `captureIO`.
- `internal/asana/client.go`: HTTP client for Asana API and OAuth token endpoints.
- `internal/oauth/oauth.go`: authorization URL, default scopes, random state, callback parsing.
- `internal/oauth/callback.go`: localhost callback server for OAuth login.
- `internal/config/config.go`: credential file path, load/save, token merge, file permissions.
- `README.md`: Japanese user documentation and command reference.
- `go.mod`: module declaration; currently standard-library-only.

## Standard Commands

Run from repo root:

```sh
go test ./...
go test ./internal/cli -run TestName -v
go build -o /tmp/asana-cli ./cmd/asana-cli
gofmt -w <changed-go-files>
```

Safe smoke commands:

```sh
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

Avoid commands that hit real Asana endpoints unless the user explicitly requests a live integration check.

## Implementation Playbook

1. Identify the package and behavior boundary.
   - CLI parsing/rendering: `internal/cli`.
   - API request path/query/pagination: `internal/asana`.
   - OAuth URL/callback/state: `internal/oauth`.
   - Credentials: `internal/config`.

2. Start with tests when practical.
   - CLI behavior: add a focused `RunCLI` test in `internal/cli/cli_test.go`.
   - HTTP client behavior: create package tests with `httptest.Server`.
   - Config behavior: use `t.TempDir()` and never touch real user config.

3. Implement the smallest compatible change.
   - Preserve aliases such as singular/plural commands when present.
   - Keep global flags parsed before subcommands.
   - Keep error messages user-readable.

4. Format and test.
   - Run `gofmt` on changed Go files.
   - Run targeted tests, then `go test ./...`.

5. Update docs/help when user-facing behavior changes.
   - README command tables and examples.
   - `printHelp` / `printAuthSubcommandHelp` / subcommand help strings.

6. Perform review gates.
   - Asana API correctness gate.
   - Secret/config security gate.
   - Output compatibility gate.

## Adding a New Read-Only Asana Command

Typical sequence:

1. Add tests for CLI routing and flag validation.
2. Add an API method in `internal/asana/client.go`.
   - Use `joinSegments` for path parameters.
   - Use `getPaginated` for list endpoints with `next_page.offset`.
   - Use `getDataJSON` for single-object endpoints.
   - Set query parameters with `u.Query()` and `u.RawQuery = q.Encode()`.
3. Add rendering profile columns in `internal/cli/cli.go` if table/compact output needs a stable subset.
4. Route the command in the relevant handler or add a new handler if it is a new top-level noun.
5. Add help text and README docs.
6. Run `go test ./...` and non-credential smoke checks.

## OAuth and Config Rules

- Never expose real tokens in stdout/stderr/tests/docs.
- `auth login` is localhost callback only; manual/OOB flow uses `auth url` + `auth exchange`.
- OAuth `state` must be generated when omitted and checked after callback.
- Callback redirect URI must stay `http://localhost` or `http://127.0.0.1` and must not include query or fragment.
- Config files must be written with owner-only permissions (`0600`) and config directory private permissions (`0700`).
- Use temp config files in tests: `--config`, `t.TempDir() + "/credentials.json"`.

## Output Contract

- JSON: pretty-printed with `json.MarshalIndent`.
- Table collections: header row, then tab-separated row values.
- Table objects: `field\tvalue`, then one row per configured column.
- Compact: `field=value` lines.
- Table/compact values must be sanitized to escape `\\`, tab, CR, and LF.

When adding columns, prefer stable scalar fields. Nested fields may use dotted paths such as `created_by.name`.

## Subagent Workflow

For multi-step changes, orchestrate with these project subagents:

- `.claude/agents/go-cli-implementer.md`: writes tests and code for CLI behavior.
- `.claude/agents/asana-api-reviewer.md`: verifies Asana endpoint paths, queries, OAuth assumptions, and pagination.
- `.claude/agents/test-security-reviewer.md`: reviews tests, secret handling, credential file safety, and final readiness.

Recommended order:

1. `go-cli-implementer` implements a focused task with tests.
2. `asana-api-reviewer` reviews endpoint/OAuth/API assumptions if HTTP behavior changed.
3. `test-security-reviewer` performs final quality/security review.
4. Main agent verifies with `go test ./...` and smoke commands.

Do not run two implementation subagents concurrently against `internal/cli/cli.go`.

## Common Pitfalls

1. Live API calls in tests. Use `httptest.Server` and runtime endpoint overrides instead.
2. Secret leakage. Redact tokens and avoid printing config contents.
3. Missing README/help updates for user-facing flags.
4. Adding dependencies for simple parsing. This project intentionally uses small hand-rolled parsing.
5. Forgetting both singular and plural command aliases where the router supports them.
6. Breaking table/compact one-line output by failing to sanitize embedded tabs/newlines.
7. Touching the real default config path in tests.
8. Changing OAuth callback security by allowing non-localhost redirects.

## Verification Checklist

- [ ] Changed Go files are `gofmt` formatted.
- [ ] Targeted tests pass.
- [ ] `go test ./...` passes.
- [ ] User-facing help and README are updated when commands/flags changed.
- [ ] No real credentials or tokens appear in files or test output.
- [ ] API paths and query parameters are tested or reviewed.
- [ ] Config tests use temp paths and preserve permission expectations.
- [ ] Output format compatibility is maintained.
