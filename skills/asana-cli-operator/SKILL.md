---
name: asana-cli-operator
description: Use when the user wants to inspect, authenticate, fetch, create, update, or delete Asana data through `asana-cli`, or validate the current Go repository checkout by running the CLI instead of only reading source.
version: 1.1.0
author: Hermes Agent
license: MIT
metadata:
  hermes:
    tags: [asana, cli, oauth, productivity, go, terminal]
    related_skills: [asana-cli-go-development]
---

# Asana CLI Operator

> 作成日時: 2026-06-01 23:32
> 更新日時: 2026-08-10 10:24

## Goal

Use `asana-cli` safely and efficiently for real Asana reads, writes, and OAuth maintenance.

This skill is about operating the CLI, not implementing it. Prefer executing the CLI and showing grounded results over paraphrasing README text.

## When to use

Trigger this skill when the user asks for things like:
- wants to inspect Asana workspaces, projects, or tasks
- wants to retrieve Asana comments or attachments
- wants to create, update, relate, duplicate, or delete Asana resources
- wants to check authentication status with `asana-cli`
- wants to refresh a token
- wants to know what this CLI can do
- wants verification using real `asana-cli` commands

Also trigger when the current directory is this Go repository and the user asks to verify UX by running the current source tree.

Do not use this skill to implement the CLI. For implementation and code review,
use `.claude/skills/asana-cli-go-development/SKILL.md` and its focused skills.

## Hard gates

1. Do not call the real Asana API unless the user explicitly asks for that operation.
2. Never print, log, or quote access tokens, refresh tokens, client secrets, authorization codes, or credential-file contents.
3. Before a write or delete, resolve the exact resource and command arguments. If the target or effect is ambiguous, ask the user.
4. Use source-tree commands only from this repository root. Otherwise use the installed binary.

## Command resolution order

1. If `asana-cli` is installed, use it directly.
2. If you are inside this Go repository and need the current working tree, prefer `go run ./cmd/asana-cli <args>`.
3. If both are available, choose based on the task:
   - installed binary for end-user reproduction
   - `go run ./cmd/asana-cli` for validating uninstalled source changes

Before assuming availability, check with terminal commands such as:
- `command -v asana-cli`
- `go run ./cmd/asana-cli --help` (inside the repo)

## First checks

Before reading live Asana data, start with:

```bash
asana-cli auth status
```

or, in the repo:

```bash
go run ./cmd/asana-cli auth status
```

Why:
- it reveals whether credentials exist
- it shows the saved client ID and redirect URI
- it confirms whether access/refresh tokens are present
- token values are redacted, so it is safe to share the output unless the user asks otherwise

If credentials are missing, do not pretend the API calls will work. Move to the authentication flows below.

## Expired-token hard rule

When `asana-cli auth status` shows an expired access token and a refresh token is present, your next command MUST be:

```bash
asana-cli auth refresh --client-secret "$ASANA_CLIENT_SECRET"
```

This is mandatory whenever `ASANA_CLIENT_SECRET` is available. Do this before any other investigation, API read, config-file inspection, `auth url`, `auth exchange`, or `auth login` attempt.

If `ASANA_CLIENT_SECRET` is not available, ask for it explicitly and do not explore alternative authentication methods first. Only switch to `auth login` or the manual `auth url` + `auth exchange` flow after `auth refresh` has actually failed because the refresh token is missing, invalid, revoked, or rejected by Asana.

Common rationalizations are wrong:
- Do not inspect the credentials file to look for another way around refresh.
- Do not retry the original read command before refreshing.
- Do not start OAuth login just because the access token expired.
- Do not ask for `ASANA_CLIENT_ID` when refresh only needs `--client-secret`.

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

Sections in a project:
```bash
asana-cli sections list 120000000000010
asana-cli sections tasks 120000000000020
```

Tasks in a project:
```bash
asana-cli tasks list 120000000000010
asana-cli tasks ls --project 120000000000010
```

Search tasks in a workspace:
```bash
asana-cli tasks search \
  --workspace 120000000000001 \
  --assignee me \
  --projects-any 120000000000010 \
  --sort-by due_date \
  --limit 50
```

Search requires Asana Premium, is eventually consistent, and does not use the
normal offset pagination used by list commands.

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
asana-cli attachments list 120000000000999
asana-cli attachments get 120000000001999
```

### Write APIs

Task create and update:
```bash
asana-cli tasks create --workspace 120000000000001 --name "Prepare launch"
asana-cli tasks create-subtask 120000000000999 --name "Check links"
asana-cli tasks update 120000000000999 --completed true
```

Task relationships:
```bash
asana-cli tasks set-parent 120000000000999 --parent 120000000000888
asana-cli tasks unset-parent 120000000000999
asana-cli tasks add-project 120000000000999 --project 120000000000010 --section 120000000000020
asana-cli tasks remove-project 120000000000999 --project 120000000000010
asana-cli tasks add-tag 120000000000999 --tag 120000000000030
asana-cli tasks add-followers 120000000000999 --follower 120000000000040
```

Comments and attachments:
```bash
asana-cli tasks comment 120000000000999 --text "Ready for review"
asana-cli stories update 120000000000050 --text "Updated comment"
asana-cli attachments upload --parent 120000000000999 --file ./report.pdf
asana-cli attachments upload \
  --parent 120000000000999 \
  --url https://example.com/report \
  --name "Report" \
  --connect-to-app
```

Projects and memberships:
```bash
asana-cli projects create --workspace 120000000000001 --name "Launch"
asana-cli projects update 120000000000010 --privacy-setting private
asana-cli memberships create --parent 120000000000010 --member 120000000000040 --access-level editor
```

Delete only when the user explicitly identifies the target:
```bash
asana-cli tasks delete 120000000000999
asana-cli stories delete 120000000000050
asana-cli attachments delete 120000000001999
asana-cli projects delete 120000000000010
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

### Custom-field strings and typed values are separate

Use `--custom-field` for text and enum option GIDs. Numeric-looking GIDs must
remain strings:

```bash
asana-cli tasks update 120000000000999 \
  --custom-field 120000000000060=120000000000070
```

Use `--custom-field-json` only for explicit JSON values such as numbers,
booleans, arrays, or objects:

```bash
asana-cli tasks update 120000000000999 --custom-field-json 120000000000060=true
```

Do not use `--custom-field-json` for an enum option GID.

### Start dates require due dates in the same request

When setting or clearing `--start-on`, include `--due-on` or `--due-at` in the
same request. When setting or clearing `--start-at`, include `--due-at`.

### Repeated tag operations are not atomic

`add-tag` and `remove-tag` send one request per tag. If one request fails, the
error identifies the failing tag and the number of earlier tags already
applied. Do not describe the operation as rolled back.

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

### When the user asks for a write

1. Run `asana-cli auth status`.
2. Resolve every target GID with the smallest read command when the user did not
   already provide an unambiguous target.
3. State the exact mutation and its target before destructive operations.
4. Run the write once, then report the returned object or job GID.
5. For asynchronous duplicate/template operations, use `jobs get <JOB_GID>`;
   do not poll indefinitely.

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
- config directory uses `0700` and the credentials file uses `0600` on Unix
- stdout token output is redacted
- `auth status` reports token presence without printing secrets

## Repo-aware validation flow

When working inside this Go repository:
1. inspect help and README if the requested behavior is unclear
2. prefer `go run ./cmd/asana-cli <args>` to validate the current source tree
3. if code changed, run:
```bash
gofmt -w <changed-go-files>
go test ./...
go build -o /tmp/asana-cli ./cmd/asana-cli
```
4. report both CLI behavior and verification results

## Response style

- Ground every claim in command output when possible.
- Quote exact commands you ran.
- If you could not authenticate, say so clearly instead of fabricating Asana data.
- Keep summaries concise, but preserve key IDs such as workspace/project/task GIDs when they matter.
