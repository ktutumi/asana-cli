---
name: asana-cli-operator
description: Use this whenever the user wants to inspect, authenticate, or fetch Asana data through the `asana-cli` command, especially for workspaces, projects, tasks, comments, attachments, current-user info, or OAuth status/refresh. Also use it when the user is in the `asana-cli` Rust repo and asks to validate real CLI behavior with live commands instead of only reading source.
version: 1.0.0
author: Hermes Agent
license: MIT
metadata:
  hermes:
    tags: [asana, cli, oauth, productivity, rust, terminal]
    related_skills: [asana-rust-cli, oauth-localhost-callback-cli]
---

# Asana CLI Operator

## Goal

Use the `asana-cli` tool correctly and efficiently for real Asana reads and OAuth maintenance.

This skill is about operating the CLI, not implementing it. Prefer executing the CLI and showing grounded results over paraphrasing README text.

## When to use

Trigger this skill when the user asks for things like:
- wants to inspect Asana workspaces, projects, or tasks
- wants to retrieve Asana comments or attachments
- wants to check authentication status with `asana-cli`
- wants to refresh a token
- wants to know what this CLI can do
- wants verification using real `asana-cli` commands

Also trigger when the repository in the current directory is this Rust CLI and the user asks to verify UX by actually running commands.

## Command resolution order

1. If `asana-cli` is installed, use it directly.
2. If you are inside the Rust repo and want to validate the current working tree, prefer `cargo run -- <args>`.
3. If both are available, choose based on the task:
   - installed binary for end-user reproduction
   - `cargo run --` for validating uninstalled source changes

Before assuming availability, check with terminal commands such as:
- `command -v asana-cli`
- `cargo run -- --help` (inside the repo)

## First checks

Before reading live Asana data, start with:

```bash
asana-cli auth status
```

or, in the repo:

```bash
cargo run -- auth status
```

Why:
- it reveals whether credentials exist
- it shows the config path
- it confirms whether access/refresh tokens are present
- token values are redacted, so it is safe to share the output unless the user asks otherwise

If credentials are missing, do not pretend the API calls will work. Move to the authentication flows below.

## Output format policy

Pick output deliberately.

### Prefer `--output json` when:
- you will parse the result further
- another tool/subagent will consume the output
- the user wants raw data or stable structure

Examples:
```bash
asana-cli --output json workspaces list
asana-cli --output json tasks get 123456789
```

### Prefer `--output table` when:
- the user wants a human-readable overview
- you are previewing lists in the terminal

Examples:
```bash
asana-cli --output table workspaces list
asana-cli --output table projects list 120000000000001
asana-cli --output table tasks comments 120000000000999
```

### Prefer `--output compact` when:
- the user wants terse terminal output
- headers would add noise

Example:
```bash
asana-cli --output compact tasks get 120000000000999
```

## Command map

### Authentication

Check saved auth state:
```bash
asana-cli auth status
```

Generate authorization URL for manual/OOB flow:
```bash
asana-cli auth url --client-id "$ASANA_CLIENT_ID"
```

Exchange authorization code for token and save it:
```bash
asana-cli auth exchange \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri urn:ietf:wg:oauth:2.0:oob \
  --code "$ASANA_CODE"
```

Localhost callback login:
```bash
asana-cli auth login \
  --client-id "$ASANA_CLIENT_ID" \
  --client-secret "$ASANA_CLIENT_SECRET" \
  --redirect-uri http://127.0.0.1:18787/callback
```

Refresh access token:
```bash
asana-cli auth refresh --client-secret "$ASANA_CLIENT_SECRET"
```

### Read APIs

Current user:
```bash
asana-cli me
```

Workspaces:
```bash
asana-cli workspaces list
asana-cli workspaces ls
```

Projects in a workspace:
```bash
asana-cli projects list 120000000000001
asana-cli projects ls --workspace 120000000000001
```

Tasks in a project:
```bash
asana-cli tasks list 120000000000010
asana-cli tasks ls --project 120000000000010
```

Single task:
```bash
asana-cli tasks get 120000000000999
```

Subtasks:
```bash
asana-cli tasks subtasks 120000000000999
```

Story history:
```bash
asana-cli tasks stories 120000000000999
```

Comments only:
```bash
asana-cli tasks comments 120000000000999
```

Attachments:
```bash
asana-cli tasks attachments 120000000000999
```

## Important behavior differences

### Use `tasks comments` for comment text

If the user wants actual comment bodies, prefer:
```bash
asana-cli tasks comments <TASK_GID>
```

Reason:
- `tasks stories` is the broader history stream
- `tasks comments` filters down to comment entries and includes text-focused fields such as `text`, `created_at`, and `created_by.name`

### `auth login` is localhost-only

Use `auth login` only with:
- `http://127.0.0.1/...`
- `http://localhost/...`

Do not use `auth login` with:
- `urn:ietf:wg:oauth:2.0:oob`

For OOB/manual copy-paste flows, use:
- `auth url`
- `auth exchange`

### `auth refresh` needs the client secret again

Do not assume the CLI saved `clientSecret`.
This CLI intentionally does not persist it.
If refresh is needed, obtain or ask for `--client-secret` explicitly.

## Argument rules

Some commands accept either a positional ID or a named flag.
Use one or the other, not both.

Good:
```bash
asana-cli projects list 120000000000001
asana-cli projects list --workspace 120000000000001
```

Bad:
```bash
asana-cli projects list 120000000000001 --workspace 120000000000001
```

The same rule applies to task/project/workspace selectors.

## Practical operating flow

### When the user asks for Asana data

1. Run `asana-cli auth status`.
2. If auth is missing or broken, tell the user exactly which auth step is needed.
3. If auth exists, run the smallest read command that answers the question.
4. Choose `json` for machine processing, `table` for quick human inspection.
5. Summarize the result in Japanese after showing or extracting the relevant fields.

### When the user asks what projects or tasks exist

Use a top-down drill-down:
1. `workspaces list`
2. `projects list <workspace>`
3. `tasks list <project>`
4. `tasks get <task>` or `tasks comments <task>` as needed

## Error handling guidance

If a read command fails because the token is missing, explain that the CLI normally suggests:
- `asana-cli auth login`
- or manual flow via `asana-cli auth url` + `asana-cli auth exchange`

If `auth login` fails because the redirect URI is OOB or non-localhost, switch to the correct flow instead of retrying the wrong command.

If browser auto-open is undesirable or unsupported, use:
```bash
asana-cli auth login --no-open ...
```

## Config and security facts

Default config path:
```text
$XDG_CONFIG_HOME/asana-cli/credentials.json
~/.config/asana-cli/credentials.json
```

Persisted data:
- `clientId`
- `redirectUri`
- token fields

Not persisted:
- `clientSecret`

Expected security behavior:
- config file uses owner-only permissions on Unix
- stdout token output is redacted
- `auth status` reports token presence without printing secrets

## Repo-aware validation flow

When working inside the Rust repo:
1. inspect help and README if the requested behavior is unclear
2. prefer `cargo run -- <args>` to validate the current source tree
3. if code changed, run:
```bash
cargo fmt --all
cargo check
cargo test
cargo clippy --all-targets --all-features -- -D warnings
```
4. report both CLI behavior and verification results

## Response style

- Ground every claim in command output when possible.
- Quote exact commands you ran.
- If you could not authenticate, say so clearly instead of fabricating Asana data.
- Keep summaries concise, but preserve key IDs such as workspace/project/task GIDs when they matter.
