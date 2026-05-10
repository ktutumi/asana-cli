# TODO: Go implementation plan

## Plan
- [x] Create Go module and CLI entrypoint.
- [x] Implement CLI runtime, global flags, command flags, routing, and rendering.
- [x] Implement config load/save/merge and file permissions.
- [x] Implement OAuth URL/state helpers and localhost callback server.
- [x] Implement Asana OAuth token and read-only API client.
- [x] Implement auth and read-only CLI commands.
- [x] Add Go README and GitHub Actions CI/release workflows.
- [x] Run formatting, tests, build, vet, and smoke checks.
- [x] Complete subagent reviewer pass and address findings.

## Review
- Go implementation files were added under `cmd/asana-cli` and `internal/{cli,config,oauth,asana}`.
- Documentation and workflows were added/updated for Go install, CI, and release assets.
- Reviewer loop ran 2 iterations: iteration 1 BLOCKED, iteration 2 PASS.
- Final validation passed: `go test ./...`, `go vet ./...`, and `go build -o /tmp/asana-cli ./cmd/asana-cli`.
- Safe smoke checks passed before review: `--help`, `--version`, `auth url --client-id dummy --state fixed`, and `auth status --config $(mktemp -d)/credentials.json`.
