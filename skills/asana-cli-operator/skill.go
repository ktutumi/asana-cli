// Package skill embeds the asana-cli-operator Agent Skill markdown so that
// `asana-cli --skill` can print it from the installed binary.
package skill

import _ "embed"

//go:embed SKILL.md
var Markdown string
