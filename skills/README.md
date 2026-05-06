# skills

Language: English | [日本語](README.ja.md)

This directory contains skills that let AI agents operate the CLI in this repository safely and consistently.

Goals:
- Standardize the procedure for running real commands
- Keep authentication and output-format handling consistent
- Separate implementation knowledge from operational knowledge

Included skills:
- `asana-cli-operator/`
  - An operational skill for using `asana-cli` to check authentication status, fetch workspaces / projects / tasks / comments / attachments, and refresh tokens

How to use them:
- If you are implementing or modifying the CLI itself, read the code and tests
- If you are actually using the CLI to inspect or fetch data, use the skills in this directory

Current structure:
```text
skills/
  README.md
  asana-cli-operator/
    SKILL.md
```

Notes:
- `asana-cli-operator` assumes you check `auth status` before reading from the API
- If you need the comment body, prefer `tasks comments` over `tasks stories`
- Treat localhost OAuth login and the OOB/manual flow as separate flows
