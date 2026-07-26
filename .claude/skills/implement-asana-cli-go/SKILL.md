---
name: implement-asana-cli-go
description: Use when implementing or fixing commands, flags, rendering, help text, Asana HTTP methods, OAuth flow, credential persistence, or related tests in the asana-cli-go repository before writing production changes.
---

# Implement asana-cli-go Changes

## Overview

Make the smallest compatible change that follows the repository's existing,
dependency-light patterns. Protect CLI behavior and authentication data with
focused, hermetic tests.

Announce that this skill is being used. Read `AGENTS.md` and
`../asana-cli-go-development/SKILL.md` completely before editing repository
files.

## Hard Gates

1. Never expose real client secrets, tokens, authorization codes, or credential
   file contents in output, logs, fixtures, snapshots, or commits.
2. Never call the real Asana API unless the user explicitly requests the run
   and provides the required credentials.
3. Preserve commands, aliases, flags, output contracts, and credential
   protections unless the user explicitly requests a breaking change.
4. Use `httptest.Server` for HTTP tests and `t.TempDir()` or a temporary
   `--config` path for config tests.
5. Do not create branches, commits, pushes, or pull requests unless explicitly
   requested.

## Workflow

1. Inspect the relevant code, tests, help text, and both README files.
2. Define the smallest behavior boundary:
   - Parsing, routing, rendering, help: `internal/cli`
   - API paths, queries, pagination, token HTTP: `internal/asana`
   - Authorization URL, redirect validation, callback, state: `internal/oauth`
   - Credential persistence and permissions: `internal/config`
3. Add or update a focused failing test before implementation when the behavior
   can be tested practically.
4. Run the focused test and confirm it fails for the intended reason.
5. Implement the minimum compatible change.
6. Re-run the focused test, then refactor without changing behavior.
7. Update help, `README.md`, and `README.ja.md` together for user-visible
   commands, flags, environment variables, or output changes.
8. Format changed Go files, run full verification, and inspect the final diff.

For CLI tests, use buffered `CliIO` and
`RunCLI(args, io, RuntimeOptions{})`. For HTTP behavior, override `APIBase` or
`TokenEndpoint` at runtime instead of contacting production endpoints.

## Compatibility Rules

- Keep JSON output valid and pretty-printed.
- Keep table collections as a header row followed by tab-separated rows.
- Keep compact output as `field=value` lines.
- Escape backslash, tab, carriage return, and line feed in table and compact
  output so each logical record remains on one line.
- Escape user-provided path segments with `url.PathEscape`.
- Encode queries through `url.Values`.
- Follow `next_page.offset` until it is absent or empty.
- Keep OAuth callbacks bound to `localhost` or `127.0.0.1`.
- Generate and compare OAuth `state`.
- Keep config directories at `0700` and credential files at `0600`.

<Good>

```go
return c.getList(token, "tasks/"+url.PathEscape(gid)+"/subtasks", nil)
```

This reuses the repository's pagination path and escapes user input.

</Good>

<Bad>

```go
return c.getList(token, "tasks/"+gid+"/subtasks", nil)
```

This can produce an incorrect request path for a crafted GID.

</Bad>

Inspect the current checkout before naming or adding helpers. Do not implement a
helper from memory when an existing repository helper already owns the behavior.

## Verification

Run from the repository root. Every command executed as verification must exit
successfully:

```sh
gofmt -w <changed-go-files>
go test ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```

Run `gofmt` only when Go files changed. During iteration, run the narrowest
relevant test first. Use applicable credential-free smoke checks:

```sh
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

Before claiming completion, inspect the diff and report changed files, commands
run, results, compatibility impact, and any residual risk.

## Integration

- Use `../review-asana-api/SKILL.md` after changing API paths, queries,
  pagination, OAuth, token exchange, or HTTP error handling.
- Use `../review-asana-cli-security/SKILL.md` for final review when changes touch
  auth, config, filesystem permissions, secrets, HTTP behavior, or output
  contracts.

If Asana or OAuth behavior cannot be established from current code, tests, or
authoritative documentation, stop and report the uncertainty instead of
inventing behavior.
