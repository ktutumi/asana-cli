# TODO: Claude agents to Pi subagents conversion

## Plan
- [x] Inspect existing `.claude/agents/*.md` files.
- [x] Create `.pi/agents/` for project-scoped Pi subagents.
- [x] Convert each Claude agent frontmatter to Pi-compatible fields and preserve role instructions.
- [x] Verify converted agents with `subagent` list/get.
- [x] Record review results.

## Review
- Converted 3 Claude agents into project-scoped Pi agents under `.pi/agents/`.
- Verified `subagent({ action: "list", agentScope: "both" })` shows all converted agents as `(project)`.
- Verified `subagent({ action: "get", agent: "<name>" })` for `go-cli-implementer`, `asana-api-reviewer`, and `test-security-reviewer` shows expected paths, descriptions, tools, and prompts.
- Note: current directory is not a Git repository, so git diff/status verification was not available.
