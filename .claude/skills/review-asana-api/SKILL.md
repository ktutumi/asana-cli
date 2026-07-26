---
name: review-asana-api
description: Use when reviewing changes to internal/asana, internal/oauth, authentication commands, endpoint paths, query parameters, pagination, token exchange, or Asana HTTP error handling in the asana-cli-go repository.
---

# Review Asana API and OAuth Changes

## Overview

Review API and OAuth changes against the current implementation, hermetic tests,
and authoritative behavior. Report evidence-backed defects without changing
code unless the user explicitly asks for fixes.

Announce that this skill is being used. Read `AGENTS.md` and
`../asana-cli-go-development/SKILL.md` completely before starting the review.

## Review Procedure

1. Establish the requested review scope. If no range is specified, inspect the
   current worktree diff without modifying it.
2. Read each changed API, OAuth, auth CLI, and related test path in context.
3. Trace request construction, response handling, pagination, and error
   propagation end to end.
4. Run focused package tests, then `go test ./...` when the repository state
   permits it.
5. Classify only reproducible or concretely reasoned findings. Include a tight
   file and line reference, impact, triggering condition, and recommended
   correction.
6. Produce the required verdict and list the commands actually run.

## Review Checklist

### Endpoint and Query Correctness

- Escape every user-provided path segment with `url.PathEscape`.
- Reuse the current checkout's `getOne`, `getList`, and `doJSON` behavior
  instead of duplicating request logic.
- Build query parameters with `url.Values`; verify `doJSON` encodes them.
- Confirm default endpoints remain:
  - Authorization: `https://app.asana.com/-/oauth_authorize`
  - Token: `https://app.asana.com/-/oauth_token`
- Confirm runtime endpoint overrides remain available for hermetic tests.

### Pagination and Response Lifecycle

- Use or mirror `getList` for list endpoints.
- Follow `next_page.offset` until absent or empty.
- Preserve the original query parameters on subsequent pages.
- Close every HTTP response body.
- Decode successful responses into the expected `data` shape.

### OAuth and Credential Safety

- Bind login callbacks only to `localhost` or `127.0.0.1`.
- Generate OAuth `state` when omitted and compare callback state before token
  exchange.
- Keep the manual `auth url` plus `auth exchange` flow working.
- Do not expose or persist values that the repository treats as secrets.
- Reject redirect URI query and fragment components where current validation
  requires it.

### Error Behavior and Testability

- Surface meaningful `errors[].message` content for non-2xx Asana responses
  without leaking secret-bearing bodies.
- Add context that distinguishes token endpoint failures from Asana API
  failures.
- Use `httptest.Server`, `APIBase`, and `TokenEndpoint` overrides in tests.
- Never make a live Asana request during review unless the user explicitly
  requests it.
- Never place real secrets, tokens, or authorization codes in tests.

<Good>

`internal/asana/client.go:141 — REQUEST_CHANGES: the GID is appended without
url.PathEscape, so a value containing "/" changes the endpoint path. Escape the
segment and add a request-path test with httptest.Server.`

This identifies the location, trigger, impact, and verifiable correction.

</Good>

<Bad>

`API handling could be safer.`

This is not actionable and provides no evidence or affected location.

</Bad>

## Verdict Gate

Return `REQUEST_CHANGES` when any correctness or security blocker remains,
including an unescaped path segment, broken pagination, unchecked OAuth state,
non-local callback binding, secret exposure, live-network-dependent test, or
missing focused coverage for changed auth/API behavior.

Return `APPROVED` only when no blocking finding remains and the executed tests
support the verdict. A pre-existing failure outside the patch must be reported
separately and must not be presented as a passing verification.

## Output Format

Use this exact section order:

1. `Verdict: APPROVED` or `Verdict: REQUEST_CHANGES`
2. `Critical issues` — correctness or security blockers
3. `Important issues` — defects that should be fixed before merge
4. `Minor notes` — optional improvements
5. `Commands run and results`
6. `Residual risks or assumptions`

Write `None` for an empty issue section. Do not modify code as part of the
review unless the user explicitly requests fixes.
