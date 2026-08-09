package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/asana"
)

var taskValueFlags = []string{
	"workspace", "parent", "name", "notes", "html-notes", "assignee", "completed",
	"approval-status", "resource-subtype", "start-on", "start-at", "due-on", "due-at",
	"opt-fields", "insert-before", "insert-after",
}

var taskRepeatFlags = []string{"follower", "project", "tag", "custom-field", "membership"}

func extendedAPICmd(kind, sub string, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token string) error {
	switch kind {
	case "tasks":
		return tasksExtendedCmd(sub, args, io, rt, c, token)
	case "projects":
		return projectsExtendedCmd(sub, args, io, rt, c, token)
	case "sections":
		return sectionsExtendedCmd(sub, args, io, rt, c, token)
	case "stories", "attachments", "memberships", "jobs", "workspaces", "teams":
		return miscExtendedCmd(kind, sub, args, io, rt, c, token)
	default:
		return fmt.Errorf("unknown command")
	}
}

func resourcePath(parts ...string) string {
	escaped := make([]string, len(parts))
	for i, part := range parts {
		escaped[i] = url.PathEscape(part)
	}
	return strings.Join(escaped, "/")
}

func asanaAction(command string) string {
	parts := strings.Split(command, "-")
	if len(parts) == 0 {
		return command
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

func required(value, label string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", label)
	}
	return nil
}

func exclusive(p parsed, names ...string) error {
	count := 0
	for _, name := range names {
		if p.vals[name] != "" {
			count++
		}
	}
	if count > 1 {
		flags := make([]string, len(names))
		for i, name := range names {
			flags[i] = "--" + name
		}
		return fmt.Errorf("%s are mutually exclusive", strings.Join(flags, " and "))
	}
	return nil
}

func exactlyOne(values map[string]string, names ...string) (string, string, error) {
	foundName, foundValue := "", ""
	for _, name := range names {
		if values[name] == "" {
			continue
		}
		if foundName != "" {
			return "", "", fmt.Errorf("exactly one of --%s is required", strings.Join(names, ", --"))
		}
		foundName, foundValue = name, values[name]
	}
	if foundName == "" {
		return "", "", fmt.Errorf("exactly one of --%s is required", strings.Join(names, ", --"))
	}
	return foundName, foundValue, nil
}

func parseBool(raw, flag string) (bool, error) {
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("--%s must be true or false", flag)
	}
	return v, nil
}

func validateDate(raw, flag string) error {
	if raw == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return fmt.Errorf("--%s must use YYYY-MM-DD", flag)
	}
	return nil
}

func validateTime(raw, flag string) error {
	if raw == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, raw); err != nil {
		return fmt.Errorf("--%s must use RFC3339", flag)
	}
	return nil
}

func parsePairs(values []string, flag string) (asana.Object, error) {
	out := asana.Object{}
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("--%s must use GID=VALUE", flag)
		}
		out[key] = scalar(value)
	}
	return out, nil
}

func scalar(raw string) any {
	var value any
	if json.Unmarshal([]byte(raw), &value) == nil {
		switch value.(type) {
		case nil, bool, float64, string, []any, map[string]any:
			return value
		}
	}
	return raw
}

func taskRequestData(p parsed, create bool) (asana.Object, error) {
	if err := exclusive(p, "notes", "html-notes"); err != nil {
		return nil, err
	}
	if err := exclusive(p, "start-on", "start-at"); err != nil {
		return nil, err
	}
	if err := exclusive(p, "due-on", "due-at"); err != nil {
		return nil, err
	}
	for _, name := range []string{"start-on", "due-on"} {
		if err := validateDate(p.vals[name], name); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"start-at", "due-at"} {
		if err := validateTime(p.vals[name], name); err != nil {
			return nil, err
		}
	}
	if p.vals["start-on"] != "" && p.vals["due-on"] == "" && p.vals["due-at"] == "" {
		return nil, fmt.Errorf("--start-on requires --due-on or --due-at in the same request")
	}
	if p.vals["start-at"] != "" && p.vals["due-at"] == "" {
		return nil, fmt.Errorf("--start-at requires --due-at in the same request")
	}
	data := asana.Object{}
	copyStrings := map[string]string{
		"workspace": "workspace", "parent": "parent", "name": "name", "notes": "notes",
		"html-notes": "html_notes", "assignee": "assignee", "approval-status": "approval_status",
		"resource-subtype": "resource_subtype", "start-on": "start_on", "start-at": "start_at",
		"due-on": "due_on", "due-at": "due_at",
	}
	for flag, field := range copyStrings {
		if p.vals[flag] != "" {
			data[field] = p.vals[flag]
		}
	}
	if p.vals["completed"] != "" {
		completed, err := parseBool(p.vals["completed"], "completed")
		if err != nil {
			return nil, err
		}
		data["completed"] = completed
	}
	for flag, field := range map[string]string{"follower": "followers", "project": "projects", "tag": "tags"} {
		if len(p.lists[flag]) > 0 {
			if !create {
				return nil, fmt.Errorf("--%s is create-only; use the relationship command for an existing task", flag)
			}
			data[field] = p.lists[flag]
		}
	}
	if len(p.lists["custom-field"]) > 0 {
		fields, err := parsePairs(p.lists["custom-field"], "custom-field")
		if err != nil {
			return nil, err
		}
		data["custom_fields"] = fields
	}
	if len(p.lists["membership"]) > 0 {
		if !create {
			return nil, fmt.Errorf("--membership is create-only; use tasks add-project for an existing task")
		}
		memberships := make([]asana.Object, 0, len(p.lists["membership"]))
		for _, raw := range p.lists["membership"] {
			project, section, ok := strings.Cut(raw, "=")
			if !ok || project == "" || section == "" {
				return nil, fmt.Errorf("--membership must use PROJECT_GID=SECTION_GID")
			}
			memberships = append(memberships, asana.Object{"project": project, "section": section})
		}
		data["memberships"] = memberships
	}
	return data, nil
}

func optFieldsQuery(raw string) url.Values {
	q := url.Values{}
	if raw != "" {
		q.Set("opt_fields", raw)
	}
	return q
}

func writeAndRender(ctx context.Context, io *CliIO, rt RuntimeOptions, c *asana.Client, token, method, path, typ string, q url.Values, data asana.Object) error {
	result, err := c.WriteObject(ctx, token, method, path, q, data)
	if err != nil {
		return err
	}
	return render(io.out(), rt.Output, typ, result)
}

func deleteAndRender(ctx context.Context, io *CliIO, rt RuntimeOptions, c *asana.Client, token, path, gid, resourceType string) error {
	if err := c.DeleteObject(ctx, token, path); err != nil {
		return err
	}
	return render(io.out(), rt.Output, "result", asana.Object{"deleted": true, "gid": gid, "resource_type": resourceType})
}

func relationshipData(key string, values []string) (asana.Object, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one --%s is required", strings.TrimSuffix(key, "s"))
	}
	return asana.Object{key: values}, nil
}

func post(ctx context.Context, io *CliIO, rt RuntimeOptions, c *asana.Client, token, path, typ string, data asana.Object) error {
	return writeAndRender(ctx, io, rt, c, token, http.MethodPost, path, typ, nil, data)
}
