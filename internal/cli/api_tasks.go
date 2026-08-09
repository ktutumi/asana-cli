package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ktutumi/asana-cli-go/internal/asana"
)

func tasksListCmd(args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token string) error {
	p, err := parseFlags(args, []string{"project", "section", "tag", "user-task-list", "opt-fields"}, nil, nil, 1)
	if err != nil {
		return err
	}
	if len(p.pos) == 1 {
		if p.vals["project"] != "" {
			return fmt.Errorf("duplicate target: positional and --project")
		}
		p.vals["project"] = p.pos[0]
	}
	kind, gid, err := exactlyOne(p.vals, "project", "section", "tag", "user-task-list")
	if err != nil {
		return err
	}
	var path string
	switch kind {
	case "project":
		path = resourcePath("projects", gid, "tasks")
	case "section":
		path = resourcePath("sections", gid, "tasks")
	case "tag":
		path = resourcePath("tags", gid, "tasks")
	case "user-task-list":
		path = resourcePath("user_task_lists", gid, "tasks")
	}
	items, err := c.ListObjects(context.Background(), token, path, optFieldsQuery(p.vals["opt-fields"]))
	if err != nil {
		return err
	}
	return render(io.out(), rt.Output, "tasks", items)
}

func tasksExtendedCmd(sub string, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token string) error {
	ctx := context.Background()
	switch sub {
	case "create", "create-subtask", "update":
		max := 0
		if sub != "create" {
			max = 1
		}
		p, err := parseFlags(args, taskValueFlags, taskRepeatFlags, nil, max)
		if err != nil {
			return err
		}
		if sub == "create-subtask" && (p.vals["workspace"] != "" || p.vals["parent"] != "") {
			return fmt.Errorf("create-subtask uses its positional PARENT_GID; --workspace and --parent are not supported")
		}
		data, err := taskRequestData(p, sub != "update")
		if err != nil {
			return err
		}
		method, path := http.MethodPost, "tasks"
		if sub == "create" {
			if p.vals["name"] == "" {
				return fmt.Errorf("--name is required")
			}
			if p.vals["workspace"] == "" && p.vals["parent"] == "" && len(p.lists["project"]) == 0 && len(p.lists["membership"]) == 0 {
				return fmt.Errorf("one of --workspace, --parent, --project, or --membership is required")
			}
		} else {
			if len(p.pos) == 0 {
				return fmt.Errorf("task gid is required")
			}
			if sub == "create-subtask" {
				if p.vals["name"] == "" {
					return fmt.Errorf("--name is required")
				}
				path = resourcePath("tasks", p.pos[0], "subtasks")
			} else {
				method = http.MethodPut
				path = resourcePath("tasks", p.pos[0])
				if len(data) == 0 {
					return fmt.Errorf("at least one task field is required")
				}
			}
		}
		return writeAndRender(ctx, io, rt, c, token, method, path, "task", optFieldsQuery(p.vals["opt-fields"]), data)

	case "set-parent", "unset-parent":
		valueFlags := []string{"parent", "insert-before", "insert-after"}
		if sub == "unset-parent" {
			valueFlags = nil
		}
		p, err := parseFlags(args, valueFlags, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		if err := exclusive(p, "insert-before", "insert-after"); err != nil {
			return err
		}
		data := asana.Object{"parent": nil}
		if sub == "set-parent" {
			if err := required(p.vals["parent"], "--parent"); err != nil {
				return err
			}
			data["parent"] = p.vals["parent"]
		}
		if p.vals["insert-before"] != "" {
			data["insert_before"] = p.vals["insert-before"]
		}
		if p.vals["insert-after"] != "" {
			data["insert_after"] = p.vals["insert-after"]
		}
		return post(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0], "setParent"), "task", data)

	case "search":
		valueFlags := []string{"workspace", "assignee", "projects-any", "sections-any", "tags-any", "text", "completed", "is-subtask", "modified-at-after", "due-on-before", "due-on-after", "start-on-before", "start-on-after", "sort-by", "sort-ascending", "limit", "opt-fields"}
		p, err := parseFlags(args, valueFlags, nil, nil, 0)
		if err != nil {
			return err
		}
		if err := required(p.vals["workspace"], "--workspace"); err != nil {
			return err
		}
		for _, flag := range []string{"completed", "is-subtask", "sort-ascending"} {
			if p.vals[flag] != "" {
				if _, err := parseBool(p.vals[flag], flag); err != nil {
					return err
				}
			}
		}
		for _, flag := range []string{"due-on-before", "due-on-after", "start-on-before", "start-on-after"} {
			if err := validateDate(p.vals[flag], flag); err != nil {
				return err
			}
		}
		if err := validateTime(p.vals["modified-at-after"], "modified-at-after"); err != nil {
			return err
		}
		if p.vals["limit"] != "" {
			limit, err := strconv.Atoi(p.vals["limit"])
			if err != nil || limit < 1 || limit > 100 {
				return fmt.Errorf("--limit must be an integer from 1 to 100")
			}
		}
		if p.vals["sort-by"] != "" && !set([]string{"due_date", "created_at", "completed_at", "likes", "modified_at", "relevance"})[p.vals["sort-by"]] {
			return fmt.Errorf("invalid --sort-by value: %s", p.vals["sort-by"])
		}
		q := url.Values{}
		mapping := map[string]string{"assignee": "assignee.any", "projects-any": "projects.any", "sections-any": "sections.any", "tags-any": "tags.any", "text": "text", "completed": "completed", "is-subtask": "is_subtask", "modified-at-after": "modified_at.after", "due-on-before": "due_on.before", "due-on-after": "due_on.after", "start-on-before": "start_on.before", "start-on-after": "start_on.after", "sort-by": "sort_by", "sort-ascending": "sort_ascending", "limit": "limit", "opt-fields": "opt_fields"}
		for flag, query := range mapping {
			if p.vals[flag] != "" {
				q.Set(query, p.vals[flag])
			}
		}
		items, err := c.SearchObjects(ctx, token, resourcePath("workspaces", p.vals["workspace"], "tasks", "search"), q)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "tasks", items)

	case "get-custom-id":
		p, err := parseFlags(args, []string{"workspace", "opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["workspace"] == "" {
			return fmt.Errorf("workspace and custom ID are required")
		}
		item, err := c.GetObject(ctx, token, resourcePath("workspaces", p.vals["workspace"], "tasks", "custom_id", p.pos[0]), optFieldsQuery(p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "task", item)

	case "projects", "dependencies", "dependents":
		p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		path := resourcePath("tasks", p.pos[0], sub)
		items, err := c.ListObjects(ctx, token, path, optFieldsQuery(p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		typ := sub
		if sub == "dependencies" || sub == "dependents" {
			typ = "tasks"
		}
		return render(io.out(), rt.Output, typ, items)

	case "add-dependencies", "remove-dependencies", "add-dependents", "remove-dependents":
		flag := "dependency"
		key := "dependencies"
		apiSub := asanaAction(sub)
		if strings.Contains(sub, "dependents") {
			flag, key = "dependent", "dependents"
		}
		p, err := parseFlags(args, nil, []string{flag}, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		data, err := relationshipData(key, p.lists[flag])
		if err != nil {
			return err
		}
		return post(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0], apiSub), "task", data)

	case "add-followers", "remove-followers":
		p, err := parseFlags(args, nil, []string{"follower"}, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		data, err := relationshipData("followers", p.lists["follower"])
		if err != nil {
			return err
		}
		return post(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0], asanaAction(sub)), "task", data)

	case "add-tag", "remove-tag":
		p, err := parseFlags(args, nil, []string{"tag"}, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || len(p.lists["tag"]) == 0 {
			return fmt.Errorf("task gid and at least one --tag are required")
		}
		var result asana.Object
		for i, tag := range p.lists["tag"] {
			result, err = c.WriteObject(ctx, token, http.MethodPost, resourcePath("tasks", p.pos[0], asanaAction(sub)), nil, asana.Object{"tag": tag})
			if err != nil {
				return fmt.Errorf("%s failed for tag %s after applying %d of %d tags: %w", sub, tag, i, len(p.lists["tag"]), err)
			}
		}
		return render(io.out(), rt.Output, "task", result)

	case "add-project", "remove-project":
		valueFlags := []string{"project", "section", "insert-before", "insert-after"}
		if sub == "remove-project" {
			valueFlags = []string{"project"}
		}
		p, err := parseFlags(args, valueFlags, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["project"] == "" {
			return fmt.Errorf("task gid and --project are required")
		}
		if err := exclusive(p, "insert-before", "insert-after"); err != nil {
			return err
		}
		data := asana.Object{"project": p.vals["project"]}
		for flag, field := range map[string]string{"section": "section", "insert-before": "insert_before", "insert-after": "insert_after"} {
			if p.vals[flag] != "" {
				data[field] = p.vals[flag]
			}
		}
		return post(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0], asanaAction(sub)), "task", data)

	case "comment":
		p, err := parseFlags(args, []string{"text", "html-text"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		if err := exclusive(p, "text", "html-text"); err != nil {
			return err
		}
		data := asana.Object{}
		if p.vals["text"] != "" {
			data["text"] = p.vals["text"]
		} else if p.vals["html-text"] != "" {
			data["html_text"] = p.vals["html-text"]
		} else {
			return fmt.Errorf("one of --text or --html-text is required")
		}
		return post(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0], "stories"), "story", data)

	case "duplicate":
		p, err := parseFlags(args, []string{"name", "include"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		data := asana.Object{}
		if p.vals["name"] != "" {
			data["name"] = p.vals["name"]
		}
		if p.vals["include"] != "" {
			data["include"] = p.vals["include"]
		}
		return post(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0], "duplicate"), "job", data)

	case "delete":
		p, err := parseFlags(args, nil, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("task gid is required")
		}
		return deleteAndRender(ctx, io, rt, c, token, resourcePath("tasks", p.pos[0]), p.pos[0], "task")
	}
	return fmt.Errorf("unknown command")
}
