package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/ktutumi/asana-cli-go/internal/asana"
)

func miscExtendedCmd(kind, sub string, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token string) error {
	ctx := context.Background()
	switch kind + ":" + sub {
	case "stories:get":
		return getNamedObject(ctx, args, io, rt, c, token, "stories", "story")
	case "stories:update":
		p, err := parseFlags(args, []string{"text", "html-text"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("story gid is required")
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
		return writeAndRender(ctx, io, rt, c, token, http.MethodPut, resourcePath("stories", p.pos[0]), "story", nil, data)
	case "stories:delete":
		return deleteNamedObject(ctx, args, io, rt, c, token, "stories", "story")

	case "attachments:list":
		p, err := parseFlags(args, []string{"parent", "opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		parent, err := target(p, "parent")
		if err != nil {
			return err
		}
		if parent == "" {
			return fmt.Errorf("parent gid is required")
		}
		q := listOptFieldsQuery(rt.Output, "attachments", p.vals["opt-fields"])
		q.Set("parent", parent)
		items, err := c.ListObjects(ctx, token, "attachments", q)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "attachments", items)
	case "attachments:get":
		return getNamedObject(ctx, args, io, rt, c, token, "attachments", "attachment")
	case "attachments:delete":
		return deleteNamedObject(ctx, args, io, rt, c, token, "attachments", "attachment")
	case "attachments:upload":
		p, err := parseFlags(args, []string{"parent", "file", "url", "name"}, nil, []string{"connect-to-app"}, 0)
		if err != nil {
			return err
		}
		if p.vals["parent"] == "" {
			return fmt.Errorf("--parent is required")
		}
		if err := exclusive(p, "file", "url"); err != nil {
			return err
		}
		input := asana.AttachmentUpload{Parent: p.vals["parent"], URL: p.vals["url"], Name: p.vals["name"], ConnectToApp: p.bools["connect-to-app"]}
		if p.vals["file"] != "" {
			file, err := os.Open(p.vals["file"])
			if err != nil {
				return fmt.Errorf("open attachment: %w", err)
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				return fmt.Errorf("inspect attachment: %w", err)
			}
			input.File, input.FileName, input.Size = file, info.Name(), info.Size()
		} else {
			if p.vals["url"] == "" || p.vals["name"] == "" {
				return fmt.Errorf("either --file, or both --url and --name, are required")
			}
			parsedURL, err := url.ParseRequestURI(p.vals["url"])
			if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
				return fmt.Errorf("--url must be an absolute URL")
			}
		}
		item, err := c.UploadAttachment(ctx, token, input)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "attachment", item)

	case "memberships:list":
		p, err := parseFlags(args, []string{"parent", "member", "opt-fields"}, nil, nil, 0)
		if err != nil {
			return err
		}
		q := listOptFieldsQuery(rt.Output, "memberships", p.vals["opt-fields"])
		if p.vals["parent"] != "" {
			q.Set("parent", p.vals["parent"])
		}
		if p.vals["member"] != "" {
			q.Set("member", p.vals["member"])
		}
		if q.Get("parent") == "" && q.Get("member") == "" {
			return fmt.Errorf("one of --parent or --member is required")
		}
		items, err := c.ListObjects(ctx, token, "memberships", q)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "memberships", items)
	case "memberships:get":
		return getNamedObject(ctx, args, io, rt, c, token, "memberships", "membership")
	case "memberships:create":
		p, err := parseFlags(args, []string{"parent", "member", "access-level"}, nil, nil, 0)
		if err != nil {
			return err
		}
		if p.vals["parent"] == "" || p.vals["member"] == "" {
			return fmt.Errorf("--parent and --member are required")
		}
		data := asana.Object{"parent": p.vals["parent"], "member": p.vals["member"]}
		if p.vals["access-level"] != "" {
			data["access_level"] = p.vals["access-level"]
		}
		return post(ctx, io, rt, c, token, "memberships", "membership", data)
	case "memberships:update":
		p, err := parseFlags(args, []string{"access-level"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["access-level"] == "" {
			return fmt.Errorf("membership gid and --access-level are required")
		}
		return writeAndRender(ctx, io, rt, c, token, http.MethodPut, resourcePath("memberships", p.pos[0]), "membership", nil, asana.Object{"access_level": p.vals["access-level"]})
	case "memberships:delete":
		return deleteNamedObject(ctx, args, io, rt, c, token, "memberships", "membership")

	case "jobs:get":
		return getNamedObject(ctx, args, io, rt, c, token, "jobs", "job")

	case "workspaces:projects", "teams:projects":
		p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 {
			return fmt.Errorf("%s gid is required", kind[:len(kind)-1])
		}
		items, err := c.ListObjects(ctx, token, resourcePath(kind, p.pos[0], "projects"), listOptFieldsQuery(rt.Output, "projects", p.vals["opt-fields"]))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "projects", items)
	case "workspaces:create-project", "teams:create-project":
		p, err := parseFlags(args, projectValueFlags, []string{"follower", "custom-field"}, nil, 1)
		if err != nil {
			return err
		}
		if len(p.pos) == 0 || p.vals["name"] == "" {
			return fmt.Errorf("%s gid and --name are required", kind[:len(kind)-1])
		}
		data, err := projectRequestData(p)
		if err != nil {
			return err
		}
		delete(data, "workspace")
		return post(ctx, io, rt, c, token, resourcePath(kind, p.pos[0], "projects"), "project", data)
	}
	return fmt.Errorf("unknown command")
}

func getNamedObject(ctx context.Context, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token, collection, typ string) error {
	p, err := parseFlags(args, []string{"opt-fields"}, nil, nil, 1)
	if err != nil {
		return err
	}
	if len(p.pos) == 0 {
		return fmt.Errorf("%s gid is required", typ)
	}
	item, err := c.GetObject(ctx, token, resourcePath(collection, p.pos[0]), optFieldsQuery(p.vals["opt-fields"]))
	if err != nil {
		return err
	}
	return render(io.out(), rt.Output, typ, item)
}

func deleteNamedObject(ctx context.Context, args []string, io *CliIO, rt RuntimeOptions, c *asana.Client, token, collection, typ string) error {
	p, err := parseFlags(args, nil, nil, nil, 1)
	if err != nil {
		return err
	}
	if len(p.pos) == 0 {
		return fmt.Errorf("%s gid is required", typ)
	}
	return deleteAndRender(ctx, io, rt, c, token, resourcePath(collection, p.pos[0]), p.pos[0], typ)
}
