---
name: review-asana-cli-security
description: Use when performing final review of asana-cli-go changes that affect tests, OAuth, credential storage, filesystem permissions, secret handling, HTTP behavior, output compatibility, help, documentation, or release readiness.
---

# Review asana-cli-go Test and Security Readiness

## Overview

Make the final approval decision from observable test, credential-safety,
OAuth, compatibility, and documentation evidence. Never approve a patch with
failing required tests, exposed secrets, or untested auth/config behavior.

Announce that this skill is being used. Read `AGENTS.md` and
`../asana-cli-go-development/SKILL.md` completely before starting the review.
Review only; do not modify code unless the user explicitly asks for fixes.

## Final Review Procedure

1. Establish the requested patch range. If none is supplied, inspect the current
   worktree diff.
2. Identify every changed behavior and map it to a focused test.
3. Inspect changed files and surrounding code for credential exposure without
   printing suspected secret values.
4. Verify auth, config, HTTP, output, help, and documentation contracts that the
   patch touches.
5. Run the relevant focused tests followed by full verification.
6. Inspect the final diff and classify findings by whether they block release.
7. Produce the required verdict with commands and residual risks.

## Release Gates

### Tests

- New behavior has focused success and error-path coverage.
- CLI tests use buffered `CliIO` and
  `RunCLI(args, io, RuntimeOptions{})`.
- HTTP tests use `httptest.Server` and runtime endpoint overrides.
- Config tests use `t.TempDir()` or an explicit temporary `--config` path.
- Tests do not read or write the user's real config.
- `go test ./...` and the required build complete successfully.

### Secrets and Filesystem Safety

- No real `client_secret`, `access_token`, `refresh_token`, authorization code,
  or credential JSON content appears in the patch, tests, logs, or snapshots.
- User-facing output and errors do not reveal token values.
- The client secret is not persisted.
- Config directories retain mode `0700`.
- Credential files retain mode `0600`.

When inspecting suspected secrets, report the file and category only. Never
repeat the sensitive value in review output.

### OAuth and HTTP Safety

- Callback listeners accept only `localhost` or `127.0.0.1`.
- Redirect URI validation rejects unsupported scheme, query, or fragment forms.
- OAuth state is generated when omitted and compared on callback.
- HTTP tests cannot reach production Asana or token endpoints.
- Non-2xx and network failures remain actionable without leaking response data
  that may contain secrets.

### Output and Documentation Compatibility

- JSON remains valid and pretty-printed.
- Table output remains tab-separated with one logical record per line.
- Compact output remains `field=value` lines.
- Backslash, tab, carriage return, and line feed remain escaped in table and
  compact modes.
- Existing command names, aliases, and flags remain compatible unless the user
  explicitly requested a breaking change.
- Help text, `README.md`, and `README.ja.md` match user-visible behavior.

<Good>

`REQUEST_CHANGES — internal/config/config.go:74 writes credentials with a mode
broader than 0600. This can expose tokens to other local users. Restore 0600 and
add a permission assertion using t.TempDir().`

This states the release impact and a hermetic way to prove the correction.

</Good>

<Bad>

`Looks risky; consider more tests.`

This supplies neither an observable failure nor a release decision.

</Bad>

## Verification

Run from the repository root:

```sh
git diff --check
go test ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
go run ./cmd/asana-cli --help
go run ./cmd/asana-cli --version
go run ./cmd/asana-cli auth url --client-id dummy --state fixed
go run ./cmd/asana-cli auth status --config "$(mktemp -d)/credentials.json"
```

Run only smoke checks relevant to the changed surface, but always report which
checks were omitted and why. Do not claim a command passed unless it was run in
the current review and its exit status was observed.

## Verdict and Output Format

Return `REQUEST_CHANGES` if a required test fails, a secret is exposed, an
auth/config change lacks focused coverage, a credential permission regresses,
or a changed public interface is undocumented.

Return `APPROVED` only when no blocking issue remains and current verification
evidence supports release readiness.

Use this exact section order:

1. `Verdict: APPROVED` or `Verdict: REQUEST_CHANGES`
2. `Blocking issues`
3. `Non-blocking issues`
4. `Commands run and results`
5. `Residual risks or assumptions`

Write `None` for an empty issue section.
