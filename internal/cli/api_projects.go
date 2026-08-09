package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ktutumi/asana-cli-go/internal/asana"
)

var projectValueFlags = []string{"name", "workspace", "notes", "html-notes", "color", "icon", "default-view", "privacy-setting", "archived", "owner", "start-on", "due-on", "default-access-level", "opt-fields"}

func projectRequestData(p parsed) (asana.Object, error) {
	if err := exclusive(p, "notes", "html-notes"); err != nil {
		return nil, err
	}
	for _, flag := range []string{"start-on", "due-on"} {
		if err := validateDate(p.vals[flag], flag); err != nil {
			return nil, err
		}
	}
	if p.vals["start-on"] != "" && p.vals["due-on"] == "" {
		return nil, fmt.Errorf("--start-on requires --due-on in the same request")
	}
	if p.vals["start-on"] != "" && p.vals["start-on"] == p.vals["due-on"] {
		return nil, fmt.Errorf("--start-on and --due-on must use different dates")
	}
	data := asana.Object{}
	mapping := map[string]string{"name": "name", "workspace": "workspace", "notes": "notes", "html-notes": "html_notes", "color": "color", "icon": "icon", "default-view": "default_view", "privacy-setting": "privacy_setting", "owner": "owner", "start-on": "start_on", "due-on": "due_on", "default-access-level": "default_access_level"}
	for flag, field := range mapping {
		if p.vals[flag] != "" {
			data[field] = p.vals[flag]
		}
	}
	if p.vals["archived"] != "" {
		value, err := parseBool(p.vals["archived"], "archived")
		if err != nil {
			return nil, err
		}
		data["archived"] = value
	}
	if len(p.lists["follower"]) > 0 {
		data["followers"] = strings.Join(p.lists["follower"], ",")
	}
	if len(p.lists["custom-field"]) > 0 || len(p.lists["custom-field-json"]) > 0 {
		fields, err := mergeCustomFields(p)
		if err != nil {
			return nil, err
		}
		data["custom_fields"] = fields
	}
	return data, nil
}

func projectsExtendedCmd(sub string, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token string) error {
	ctx := context.Background()
	switch sub {
	case "get":
		p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("project gid is required")
		}
		item, err := c.GetObject(ctx, token, resourcePath("projects", p.pos[0]), optFieldsQuery(p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "project", item)

	case "create", "update":
		max := 0
		if sub == "update" {
			max = 1
		}
		p, err := parseFlags(args, projectValueFlags, []string{"follower", "custom-field", "custom-field-json"}, nil, max)
		if err != nil {
			return err
		}
		data, err := projectRequestData(p)
		if err != nil {
			return err
		}
		method, path := http.MethodPost, "projects"
		if sub == "create" {
			if p.vals["name"] == "" || p.vals["workspace"] == "" {
				return fmt.Errorf("--name and --workspace are required")
			}
		} else {
			if len(p.pos) == 0 {
				return fmt.Errorf("project gid is required")
			}
			delete(data, "workspace")
			if len(data) == 0 {
				return fmt.Errorf("at least one project field is required")
			}
			method, path = http.MethodPut, resourcePath("projects", p.pos[0])
		}
		return writeAndRender(ctx, io, rt, c, token, method, path, "project", optFieldsQuery(p.vals["opt-fields"]), data)

	case "tasks":
		p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("project gid is required")
		}
		items, err := c.ListObjects(ctx, token, resourcePath("projects", p.pos[0], "tasks"), optFieldsQuery(p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "tasks", items)

	case "task-counts":
		p, err := parseFlags(args, nil, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("project gid is required")
		}
		q := optFieldsQuery("num_tasks,num_incomplete_tasks,num_completed_tasks,num_milestones,num_incomplete_milestones,num_completed_milestones")
		item, err := c.GetObject(ctx, token, resourcePath("projects", p.pos[0], "task_counts"), q)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "task_counts", item)

	case "add-followers", "remove-followers":
		p, err := parseFlags(args, nil, []string{"follower"}, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("project gid is required")
		}
		if len(p.lists["follower"]) == 0 {
			return fmt.Errorf("at least one --follower is required")
		}
		data := asana.Object{"followers": strings.Join(p.lists["follower"], ",")}
		return post(ctx, io, rt, c, token, resourcePath("projects", p.pos[0], asanaAction(sub)), "project", data)

	case "duplicate":
		p, err := parseFlags(args, []string{"name", "include", "start-on", "due-on", "skip-weekends"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["name"] == "" {
			return fmt.Errorf("project gid and --name are required")
		}
		if err := exclusive(p, "start-on", "due-on"); err != nil {
			return err
		}
		for _, flag := range []string{"start-on", "due-on"} {
			if err := validateDate(p.vals[flag], flag); err != nil {
				return err
			}
		}
		data := asana.Object{"name": p.vals["name"]}
		if p.vals["include"] != "" {
			data["include"] = p.vals["include"]
		}
		if p.vals["start-on"] != "" || p.vals["due-on"] != "" {
			schedule := asana.Object{}
			if p.vals["start-on"] != "" {
				schedule["start_on"] = p.vals["start-on"]
			}
			if p.vals["due-on"] != "" {
				schedule["due_on"] = p.vals["due-on"]
			}
			if p.vals["skip-weekends"] == "" {
				return fmt.Errorf("--skip-weekends is required when shifting dates")
			}
			skip, err := parseBool(p.vals["skip-weekends"], "skip-weekends")
			if err != nil {
				return err
			}
			schedule["should_skip_weekends"] = skip
			data["schedule_dates"] = schedule
		}
		return post(ctx, io, rt, c, token, resourcePath("projects", p.pos[0], "duplicate"), "job", data)

	case "save-as-template":
		p, err := parseFlags(args, []string{"name", "team", "workspace", "public"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["name"] == "" || p.vals["public"] == "" {
			return fmt.Errorf("project gid, --name, and --public are required")
		}
		if err := exclusive(p, "team", "workspace"); err != nil {
			return err
		}
		public, err := parseBool(p.vals["public"], "public")
		if err != nil {
			return err
		}
		data := asana.Object{"name": p.vals["name"], "public": public}
		if p.vals["team"] != "" {
			data["team"] = p.vals["team"]
		}
		if p.vals["workspace"] != "" {
			data["workspace"] = p.vals["workspace"]
		}
		return post(ctx, io, rt, c, token, resourcePath("projects", p.pos[0], "saveAsTemplate"), "job", data)

	case "delete":
		p, err := parseFlags(args, nil, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("project gid is required")
		}
		return deleteAndRender(ctx, io, rt, c, token, resourcePath("projects", p.pos[0]), p.pos[0], "project")
	}
	return fmt.Errorf("unknown command")
}

func sectionsExtendedCmd(sub string, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token string) error {
	ctx := context.Background()
	switch sub {
	case "get":
		p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("section gid is required")
		}
		item, err := c.GetObject(ctx, token, resourcePath("sections", p.pos[0]), optFieldsQuery(p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "section", item)

	case "create":
		p, err := parseFlags(args, []string{"project", "name", "insert-before", "insert-after"}, nil, nil, 0)
		if err != nil {
			return err
		}
		if p.vals["project"] == "" || p.vals["name"] == "" {
			return fmt.Errorf("--project and --name are required")
		}
		if err := exclusive(p, "insert-before", "insert-after"); err != nil {
			return err
		}
		data := asana.Object{"name": p.vals["name"]}
		if p.vals["insert-before"] != "" {
			data["insert_before"] = p.vals["insert-before"]
		}
		if p.vals["insert-after"] != "" {
			data["insert_after"] = p.vals["insert-after"]
		}
		return post(ctx, io, rt, c, token, resourcePath("projects", p.vals["project"], "sections"), "section", data)

	case "update":
		p, err := parseFlags(args, []string{"name"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["name"] == "" {
			return fmt.Errorf("section gid and --name are required")
		}
		return writeAndRender(ctx, io, rt, c, token, http.MethodPut, resourcePath("sections", p.pos[0]), "section", nil, asana.Object{"name": p.vals["name"]})

	case "tasks":
		p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("section gid is required")
		}
		items, err := c.ListObjects(ctx, token, resourcePath("sections", p.pos[0], "tasks"), optFieldsQuery(p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "tasks", items)

	case "add-task":
		p, err := parseFlags(args, []string{"task", "insert-before", "insert-after"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["task"] == "" {
			return fmt.Errorf("section gid and --task are required")
		}
		if err := exclusive(p, "insert-before", "insert-after"); err != nil {
			return err
		}
		data := asana.Object{"task": p.vals["task"]}
		if p.vals["insert-before"] != "" {
			data["insert_before"] = p.vals["insert-before"]
		}
		if p.vals["insert-after"] != "" {
			data["insert_after"] = p.vals["insert-after"]
		}
		return post(ctx, io, rt, c, token, resourcePath("sections", p.pos[0], "addTask"), "task", data)

	case "move":
		p, err := parseFlags(args, []string{"project", "before-section", "after-section"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["project"] == "" {
			return fmt.Errorf("section gid and --project are required")
		}
		if err := exclusive(p, "before-section", "after-section"); err != nil {
			return err
		}
		data := asana.Object{"section": p.pos[0]}
		if p.vals["before-section"] != "" {
			data["before_section"] = p.vals["before-section"]
		}
		if p.vals["after-section"] != "" {
			data["after_section"] = p.vals["after-section"]
		}
		return post(ctx, io, rt, c, token, resourcePath("projects", p.vals["project"], "sections", "insert"), "section", data)

	case "delete":
		p, err := parseFlags(args, nil, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("section gid is required")
		}
		return deleteAndRender(ctx, io, rt, c, token, resourcePath("sections", p.pos[0]), p.pos[0], "section")
	}
	return fmt.Errorf("unknown command")
}
