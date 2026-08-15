package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/asana"
	"github.com/ktutumi/asana-cli-go/internal/config"
	"github.com/ktutumi/asana-cli-go/internal/oauth"
)

const Version = "0.1.0"

type CliIO struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Out, ErrOut    io.Writer
}

func NewStdIO() *CliIO {
	return &CliIO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Out: os.Stdout, ErrOut: os.Stderr}
}
func (c *CliIO) out() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	if c.Stdout != nil {
		return c.Stdout
	}
	return io.Discard
}
func (c *CliIO) err() io.Writer {
	if c.ErrOut != nil {
		return c.ErrOut
	}
	if c.Stderr != nil {
		return c.Stderr
	}
	return io.Discard
}

type RuntimeOptions struct {
	ConfigPath, Output, APIBase, TokenEndpoint, Browser string
	ClientSecret                                        string
	OpenBrowser                                         func(string) error
	HTTPClient                                          interface{}
}

func NewRuntimeOptionsFromEnv() RuntimeOptions {
	return RuntimeOptions{APIBase: getenv("ASANA_API_BASE", "https://app.asana.com/api/1.0/"), TokenEndpoint: getenv("ASANA_OAUTH_TOKEN_ENDPOINT", "https://app.asana.com/-/oauth_token"), Browser: os.Getenv("BROWSER"), ClientSecret: os.Getenv("ASANA_CLIENT_SECRET"), Output: "table"}
}
func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func RunCLI(args []string, io *CliIO, rt RuntimeOptions) (code int) {
	if rt.Output == "" {
		rt.Output = "table"
	}
	if rt.APIBase == "" {
		rt.APIBase = "https://app.asana.com/api/1.0/"
	}
	if rt.TokenEndpoint == "" {
		rt.TokenEndpoint = "https://app.asana.com/-/oauth_token"
	}
	g, rest, err := parseGlobal(args, rt)
	if err != nil {
		fmt.Fprintln(io.err(), err)
		return 1
	}
	rt = g
	if len(rest) == 0 || rest[0] == "--help" || rest[0] == "-h" {
		fmt.Fprint(io.out(), rootHelp())
		return 0
	}
	if rest[0] == "--version" || rest[0] == "-V" {
		fmt.Fprintln(io.out(), Version)
		return 0
	}
	if len(rest) > 1 && (rest[1] == "--help" || rest[1] == "-h") {
		fmt.Fprint(io.out(), commandHelp(rest[0]))
		return 0
	}
	if len(rest) > 2 && (rest[2] == "--help" || rest[2] == "-h") {
		fmt.Fprint(io.out(), commandHelp(rest[0]+" "+rest[1]))
		return 0
	}
	if err := dispatch(rest, io, rt); err != nil {
		fmt.Fprintln(io.err(), err)
		return 1
	}
	return 0
}

func parseGlobal(args []string, rt RuntimeOptions) (RuntimeOptions, []string, error) {
	out := rt
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = append(rest, args[i:]...)
			break
		}
		switch {
		case a == "--help" || a == "-h" || a == "--version" || a == "-V":
			rest = append(rest, args[i:]...)
			return out, rest, nil
		case a == "--config":
			i++
			if i >= len(args) {
				return out, nil, fmt.Errorf("--config requires a value")
			}
			out.ConfigPath = args[i]
		case strings.HasPrefix(a, "--config="):
			out.ConfigPath = strings.TrimPrefix(a, "--config=")
		case a == "--output":
			i++
			if i >= len(args) {
				return out, nil, fmt.Errorf("--output requires a value")
			}
			out.Output = args[i]
		case strings.HasPrefix(a, "--output="):
			out.Output = strings.TrimPrefix(a, "--output=")
		default:
			return out, nil, fmt.Errorf("unknown global flag: %s", a)
		}
		if out.Output != "json" && out.Output != "table" && out.Output != "compact" {
			return out, nil, fmt.Errorf("invalid output: %s", out.Output)
		}
	}
	return out, rest, nil
}

type parsed struct {
	vals  map[string]string
	lists map[string][]string
	bools map[string]bool
	pos   []string
}

func parseFlags(args []string, value, repeat, bools []string, max int) (parsed, error) {
	p := parsed{map[string]string{}, map[string][]string{}, map[string]bool{}, nil}
	vset := set(value)
	rset := set(repeat)
	bset := set(bools)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			name := strings.TrimPrefix(a, "--")
			val := ""
			has := false
			if j := strings.Index(name, "="); j >= 0 {
				val = name[j+1:]
				name = name[:j]
				has = true
			}
			if bset[name] {
				if has {
					return p, fmt.Errorf("--%s does not take a value", name)
				}
				p.bools[name] = true
				continue
			}
			if !vset[name] && !rset[name] {
				return p, fmt.Errorf("flag is not applicable: --%s", name)
			}
			if !has {
				i++
				if i >= len(args) || strings.HasPrefix(args[i], "--") {
					return p, fmt.Errorf("--%s requires a value", name)
				}
				val = args[i]
			}
			if rset[name] {
				p.lists[name] = append(p.lists[name], val)
			} else {
				p.vals[name] = val
			}
		} else {
			p.pos = append(p.pos, a)
			if len(p.pos) > max {
				return p, fmt.Errorf("extra positional argument: %s", a)
			}
		}
	}
	return p, nil
}
func set(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}
func target(p parsed, flag string) (string, error) {
	if len(p.pos) > 0 && p.vals[flag] != "" {
		return "", fmt.Errorf("duplicate target: positional and --%s", flag)
	}
	if p.vals[flag] != "" {
		return p.vals[flag], nil
	}
	if len(p.pos) > 0 {
		return p.pos[0], nil
	}
	return "", nil
}

func dispatch(args []string, io *CliIO, rt RuntimeOptions) error {
	switch args[0] {
	case "auth":
		return authCmd(args[1:], io, rt)
	case "me":
		return apiCmd("me", args[1:], io, rt)
	case "workspaces":
		return apiCmd("workspaces", args[1:], io, rt)
	case "projects", "project":
		return apiCmd("projects", args[1:], io, rt)
	case "tasks":
		return apiCmd("tasks", args[1:], io, rt)
	case "sections":
		return apiCmd("sections", args[1:], io, rt)
	case "stories":
		return apiCmd("stories", args[1:], io, rt)
	case "attachments":
		return apiCmd("attachments", args[1:], io, rt)
	case "memberships":
		return apiCmd("memberships", args[1:], io, rt)
	case "jobs":
		return apiCmd("jobs", args[1:], io, rt)
	case "teams":
		return apiCmd("teams", args[1:], io, rt)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func client(rt RuntimeOptions) *asana.Client {
	c := asana.NewClient(rt.APIBase, rt.TokenEndpoint)
	if hc, ok := rt.HTTPClient.(*http.Client); ok && hc != nil {
		c.HTTPClient = hc
	}
	return c
}

const tokenRefreshBuffer = 5 * time.Minute

func tokenNeedsRefresh(tok config.TokenData, now time.Time, buffer time.Duration) bool {
	if tok.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, tok.ExpiresAt)
	if err != nil {
		return false
	}
	return !expiresAt.After(now.Add(buffer))
}

func loadAPIToken(ctx context.Context, path string, rt RuntimeOptions, c *asana.Client, now func() time.Time) (string, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return "", err
	}
	if cfg.Token == nil || cfg.Token.AccessToken == "" {
		return "", errors.New("access token is not configured; run `asana-cli auth login` or use `auth url` and `auth exchange` manual flow")
	}

	if rt.ClientSecret == "" {
		return cfg.Token.AccessToken, nil
	}

	if !tokenNeedsRefresh(*cfg.Token, now().UTC(), tokenRefreshBuffer) {
		return cfg.Token.AccessToken, nil
	}

	if cfg.ClientID == "" {
		return "", fmt.Errorf("access token is expired or near expiration, but client id is not configured; run `asana-cli auth login` or `asana-cli auth exchange`")
	}
	if cfg.Token.RefreshToken == "" {
		return "", fmt.Errorf("access token is expired or near expiration, but refresh token is not configured; run `asana-cli auth login` or `asana-cli auth exchange`")
	}

	refreshed, err := c.RefreshAccessToken(ctx, cfg.ClientID, rt.ClientSecret, cfg.Token.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh access token: %w", err)
	}

	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = cfg.Token.RefreshToken
	}

	if err := config.SaveConfig(path, config.StoredConfig{
		ClientID:    cfg.ClientID,
		RedirectURI: cfg.RedirectURI,
		Token:       &refreshed,
	}); err != nil {
		return "", err
	}

	return refreshed.AccessToken, nil
}

func loadToken(path string) (string, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return "", err
	}
	if cfg.Token == nil || cfg.Token.AccessToken == "" {
		return "", errors.New("access token is not configured; run `asana-cli auth login` or use `auth url` and `auth exchange` manual flow")
	}
	return cfg.Token.AccessToken, nil
}

func authCmd(args []string, io *CliIO, rt RuntimeOptions) error {
	if len(args) == 0 {
		return fmt.Errorf("auth subcommand required")
	}
	sub := args[0]
	switch sub {
	case "url":
		p, err := parseFlags(args[1:], []string{"client-id", "redirect-uri", "state"}, []string{"scope"}, nil, 0)
		if err != nil {
			return err
		}
		cid := p.vals["client-id"]
		if cid == "" {
			cfg, _ := config.LoadConfig(rt.ConfigPath)
			cid = cfg.ClientID
		}
		if cid == "" {
			return fmt.Errorf("--client-id is required")
		}
		st := p.vals["state"]
		if st == "" {
			st, err = oauth.GenerateState()
			if err != nil {
				return err
			}
		}
		fmt.Fprintln(io.out(), oauth.BuildAuthURL(oauth.AuthURLParams{ClientID: cid, RedirectURI: p.vals["redirect-uri"], Scopes: p.lists["scope"], State: st}))
		return nil
	case "exchange":
		p, err := parseFlags(args[1:], []string{"code", "client-id", "client-secret", "redirect-uri"}, nil, nil, 0)
		if err != nil {
			return err
		}
		if p.vals["code"] == "" {
			return fmt.Errorf("--code is required")
		}
		if p.vals["client-secret"] == "" {
			return fmt.Errorf("--client-secret is required")
		}
		cfg, _ := config.LoadConfig(rt.ConfigPath)
		cid := first(p.vals["client-id"], cfg.ClientID)
		red := first(p.vals["redirect-uri"], cfg.RedirectURI, oauth.DefaultRedirectURI)
		if cid == "" {
			return fmt.Errorf("--client-id is required")
		}
		tok, err := client(rt).ExchangeCodeForToken(context.Background(), cid, p.vals["client-secret"], red, p.vals["code"])
		if err != nil {
			return err
		}
		if err := config.SaveConfig(rt.ConfigPath, config.StoredConfig{ClientID: cid, RedirectURI: red, Token: &tok}); err != nil {
			return err
		}
		return render(io.out(), tokenOutput(rt.Output), "token", redactToken(tok))
	case "refresh":
		p, err := parseFlags(args[1:], []string{"refresh-token", "client-id", "client-secret"}, nil, nil, 0)
		if err != nil {
			return err
		}
		if p.vals["client-secret"] == "" {
			return fmt.Errorf("--client-secret is required")
		}
		cfg, _ := config.LoadConfig(rt.ConfigPath)
		cid := first(p.vals["client-id"], cfg.ClientID)
		ref := p.vals["refresh-token"]
		if ref == "" && cfg.Token != nil {
			ref = cfg.Token.RefreshToken
		}
		if cid == "" {
			return fmt.Errorf("--client-id is required")
		}
		if ref == "" {
			return fmt.Errorf("--refresh-token is required")
		}
		tok, err := client(rt).RefreshAccessToken(context.Background(), cid, p.vals["client-secret"], ref)
		if err != nil {
			return err
		}
		if err := config.SaveConfig(rt.ConfigPath, config.StoredConfig{ClientID: cid, Token: &tok}); err != nil {
			return err
		}
		return render(io.out(), tokenOutput(rt.Output), "token", redactToken(tok))
	case "status":
		p, err := parseFlags(args[1:], []string{"config"}, nil, nil, 0)
		if err != nil {
			return err
		}
		if p.vals["config"] != "" {
			rt.ConfigPath = p.vals["config"]
		}
		cfg, err := config.LoadConfig(rt.ConfigPath)
		if err != nil {
			return err
		}
		m := map[string]any{"status": "status", "clientId": cfg.ClientID, "redirectUri": cfg.RedirectURI, "authenticated": cfg.Token != nil && cfg.Token.AccessToken != ""}
		if cfg.Token != nil {
			m["token"] = redactToken(*cfg.Token)
		}
		return render(io.out(), rt.Output, "status", m)
	case "login":
		p, err := parseFlags(args[1:], []string{"client-id", "client-secret", "redirect-uri", "listen-timeout-ms", "state"}, []string{"scope"}, []string{"no-open"}, 0)
		if err != nil {
			return err
		}
		if p.vals["client-secret"] == "" {
			return fmt.Errorf("--client-secret is required")
		}
		cfg, _ := config.LoadConfig(rt.ConfigPath)
		cid := first(p.vals["client-id"], cfg.ClientID)
		if cid == "" {
			return fmt.Errorf("--client-id is required")
		}
		red := first(p.vals["redirect-uri"], cfg.RedirectURI, oauth.DefaultRedirectURI)
		if err := oauth.ValidateLocalRedirect(red); err != nil {
			return err
		}
		st := p.vals["state"]
		if st == "" {
			st, err = oauth.GenerateState()
			if err != nil {
				return err
			}
		}
		ms := 120000
		if p.vals["listen-timeout-ms"] != "" {
			ms, _ = strconv.Atoi(p.vals["listen-timeout-ms"])
		}
		loginCtx, loginCancel := context.WithCancel(context.Background())
		defer loginCancel()
		srv, err := startCallbackFromURI(loginCtx, red, time.Duration(ms)*time.Millisecond)
		if err != nil {
			return err
		}
		defer srv.Shutdown()
		au := oauth.BuildAuthURL(oauth.AuthURLParams{ClientID: cid, RedirectURI: srv.URL, Scopes: p.lists["scope"], State: st})
		if !p.bools["no-open"] {
			if err := openURL(rt, au); err != nil {
				fmt.Fprintf(io.err(), "Open this URL manually: %s\n", au)
			}
		} else {
			fmt.Fprintf(io.err(), "Open this URL: %s\n", au)
		}
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
		defer cancel()
		r, err := srv.WaitForCode(waitCtx)
		if err != nil {
			return err
		}
		if r.State != st {
			return fmt.Errorf("state mismatch")
		}
		tok, err := client(rt).ExchangeCodeForToken(context.Background(), cid, p.vals["client-secret"], srv.URL, r.Code)
		if err != nil {
			return err
		}
		if err := config.SaveConfig(rt.ConfigPath, config.StoredConfig{ClientID: cid, RedirectURI: red, Token: &tok}); err != nil {
			return err
		}
		return render(io.out(), tokenOutput(rt.Output), "token", redactToken(tok))
	default:
		return fmt.Errorf("unknown auth subcommand: %s", sub)
	}
}

func apiCmd(kind string, args []string, io *CliIO, rt RuntimeOptions) error {
	sub := ""
	if kind == "me" {
		sub = "get"
	} else {
		if len(args) == 0 {
			return fmt.Errorf("%s subcommand required", kind)
		}
		sub = args[0]
		args = args[1:]
		if sub == "ls" {
			sub = "list"
		}
	}
	c := client(rt)
	tok, err := loadAPIToken(context.Background(), rt.ConfigPath, rt, c, time.Now)
	if err != nil {
		return err
	}
	switch kind + ":" + sub {
	case "me:get":
		v, err := c.FetchMe(context.Background(), tok)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "me", v)
	case "workspaces:list":
		p, err := parseFlags(args, nil, nil, nil, 0)
		if err != nil {
			return err
		}
		_ = p
		v, err := c.ListWorkspaces(context.Background(), tok)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "workspaces", v)
	case "projects:list":
		p, err := parseFlags(args, []string{"workspace", "opt-fields"}, nil, nil, 1)
		if err != nil {
			return err
		}
		w, err := target(p, "workspace")
		if err != nil {
			return err
		}
		q := listOptFieldsQuery(rt.Output, "projects", p.vals["opt-fields"])
		if w != "" {
			q.Set("workspace", w)
		}
		v, err := c.ListProjects(context.Background(), tok, q)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "projects", v)
	case "tasks:list":
		return tasksListCmd(args, io, rt, c, tok)
	case "sections:list":
		p, err := parseFlags(args, []string{"project"}, nil, nil, 1)
		if err != nil {
			return err
		}
		pr, err := target(p, "project")
		if err != nil {
			return err
		}
		if pr == "" {
			return fmt.Errorf("project gid is required")
		}
		v, err := c.ListSections(context.Background(), tok, pr)
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "sections", v)
	case "tasks:get", "tasks:subtasks", "tasks:stories", "tasks:comments", "tasks:attachments":
		valueFlags := []string{"task"}
		if sub == "subtasks" || sub == "attachments" {
			valueFlags = append(valueFlags, "opt-fields")
		}
		p, err := parseFlags(args, valueFlags, nil, nil, 1)
		if err != nil {
			return err
		}
		gid, err := target(p, "task")
		if err != nil {
			return err
		}
		if gid == "" {
			return fmt.Errorf("task gid is required")
		}
		if sub == "get" {
			v, err := c.GetTask(context.Background(), tok, gid)
			if err != nil {
				return err
			}
			return render(io.out(), rt.Output, "task", v)
		}
		var v []asana.Object
		if sub == "subtasks" {
			v, err = c.ListObjects(context.Background(), tok, resourcePath("tasks", gid, "subtasks"), listOptFieldsQuery(rt.Output, "subtasks", p.vals["opt-fields"]))
		} else if sub == "stories" {
			v, err = c.ListStories(context.Background(), tok, gid)
		} else if sub == "comments" {
			v, err = c.ListComments(context.Background(), tok, gid)
		} else {
			q := listOptFieldsQuery(rt.Output, "attachments", p.vals["opt-fields"])
			q.Set("parent", gid)
			v, err = c.ListObjects(context.Background(), tok, "attachments", q)
		}
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, sub, v)
	}
	return extendedAPICmd(kind, sub, args, io, rt, c, tok)
}

func tokenOutput(format string) string {
	if format == "json" {
		return "json"
	}
	return "compact"
}

func first(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}
func redactToken(t config.TokenData) map[string]any {
	return map[string]any{"access_token": red(t.AccessToken), "refresh_token": red(t.RefreshToken), "token_type": t.TokenType, "expires_in": t.ExpiresIn, "expires_at": t.ExpiresAt}
}
func red(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}
func values(k, v string) url.Values {
	q := url.Values{}
	if v != "" {
		q.Set(k, v)
	}
	return q
}

func startCallbackFromURI(ctx context.Context, raw string, _ time.Duration) (*oauth.CallbackServer, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	port := 80
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil {
			return nil, err
		}
	}
	return oauth.StartCallbackServer(ctx, u.Hostname(), port, u.Path)
}

func openURL(rt RuntimeOptions, u string) error {
	if rt.OpenBrowser != nil {
		return rt.OpenBrowser(u)
	}
	if rt.Browser != "" {
		return exec.Command(rt.Browser, u).Start()
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

var columns = map[string][]string{"workspaces": {"gid", "name"}, "projects": {"gid", "name", "workspace.name"}, "project": {"gid", "name", "archived", "privacy_setting", "workspace.name"}, "sections": {"gid", "name"}, "section": {"gid", "name", "project.gid"}, "tasks": {"gid", "name", "completed", "created_at", "modified_at"}, "task": {"gid", "name", "completed", "notes", "created_at", "modified_at"}, "subtasks": {"gid", "name", "completed"}, "stories": {"gid", "resource_subtype", "text", "created_at", "created_by.name"}, "story": {"gid", "resource_subtype", "text", "html_text", "created_at", "created_by.name"}, "comments": {"gid", "resource_subtype", "text", "html_text", "created_at", "created_by.name"}, "attachments": {"gid", "name", "download_url", "created_at"}, "attachment": {"gid", "name", "resource_subtype", "download_url", "created_at"}, "memberships": {"gid", "access_level", "member.gid", "member.name", "parent.gid", "parent.name"}, "membership": {"gid", "access_level", "member.gid", "member.name", "parent.gid", "parent.name"}, "job": {"gid", "status", "new_project.gid", "new_task.gid", "new_project_template.gid"}, "result": {"deleted", "gid", "resource_type"}, "task_counts": {"num_tasks", "num_incomplete_tasks", "num_completed_tasks", "num_milestones", "num_incomplete_milestones", "num_completed_milestones"}, "me": {"gid", "name", "email"}, "token": {"access_token", "refresh_token", "token_type", "expires_in", "expires_at"}, "status": {"status", "authenticated", "clientId", "redirectUri", "token.access_token", "token.refresh_token", "token.expires_at"}}

func render(w io.Writer, format, typ string, data any) error {
	if format == "json" {
		b, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(append(b, '\n'))
		return err
	}
	cols := columns[typ]
	if len(cols) == 0 {
		cols = guessCols(data)
	}
	arr, isArr := asSlice(data)
	if format == "table" {
		if isArr {
			fmt.Fprintln(w, strings.Join(cols, "\t"))
			for _, it := range arr {
				fmt.Fprintln(w, joinVals(it, cols, "\t", false))
			}
		} else {
			for _, c := range cols {
				fmt.Fprintf(w, "%s\t%s\n", c, escape(fmt.Sprint(value(data, c))))
			}
		}
		return nil
	}
	if isArr {
		for _, it := range arr {
			fmt.Fprintln(w, joinVals(it, cols, " ", true))
		}
	} else {
		for _, c := range cols {
			fmt.Fprintf(w, "%s=%s\n", c, escape(fmt.Sprint(value(data, c))))
		}
	}
	return nil
}
func asSlice(d any) ([]any, bool) {
	switch v := d.(type) {
	case []asana.Object:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	case []map[string]any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	case []any:
		return v, true
	}
	return nil, false
}
func joinVals(d any, cols []string, sep string, kv bool) string {
	parts := []string{}
	for _, c := range cols {
		v := escape(fmt.Sprint(value(d, c)))
		if kv {
			parts = append(parts, c+"="+v)
		} else {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, sep)
}
func value(d any, path string) any {
	cur := d
	for _, p := range strings.Split(path, ".") {
		switch m := cur.(type) {
		case map[string]any:
			cur = m[p]
		case asana.Object:
			cur = m[p]
		case map[string]string:
			cur = m[p]
		default:
			return ""
		}
	}
	if cur == nil {
		return ""
	}
	return cur
}
func escape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\r", "\\r", "\n", "\\n")
	return r.Replace(s)
}
func guessCols(d any) []string {
	m := map[string]bool{}
	arr, is := asSlice(d)
	if is && len(arr) > 0 {
		for k := range arr[0].(map[string]any) {
			m[k] = true
		}
	} else if mm, ok := d.(map[string]any); ok {
		for k := range mm {
			m[k] = true
		}
	}
	xs := []string{}
	for k := range m {
		xs = append(xs, k)
	}
	sort.Strings(xs)
	return xs
}

func rootHelp() string {
	return `asana-cli - Asana OAuth and API CLI

  Primary commands:
  auth, me, workspaces, teams, projects, sections, tasks, stories,
  attachments, memberships, jobs

Usage:
  asana-cli [--config PATH] [--output json|table|compact] <command>

Commands:
  auth        OAuth authentication commands
  me          Show current user
  workspaces  List workspaces and manage workspace projects
  teams       Manage team projects
  projects    Manage projects
  sections    Manage sections and task placement
  tasks       Manage tasks, relations, and comments
  stories     Get, update, or delete stories
  attachments List, upload, inspect, or delete attachments
  memberships Manage project memberships
  jobs        Inspect asynchronous jobs

Global flags:
  --config PATH      Credentials file path
  --output FORMAT    json, table, or compact
  --help, -h         Show help
  --version, -V      Show version
`
}
func commandHelp(cmd string) string {
	help := map[string]string{
		"tasks": `Usage: asana-cli tasks <subcommand> [options]

Read:
  list --project|--section|--tag|--user-task-list GID [--opt-fields FIELDS]
  get TASK_GID | subtasks TASK_GID [--opt-fields FIELDS] | stories TASK_GID | comments TASK_GID
  attachments TASK_GID [--opt-fields FIELDS] | projects TASK_GID [--opt-fields FIELDS]
  dependencies TASK_GID | dependents TASK_GID
  search --workspace GID [filters] [--limit N] [--opt-fields FIELDS]
  get-custom-id CUSTOM_ID --workspace GID

Write:
  create --name NAME (--workspace GID|--parent GID|--project GID|--membership PROJECT=SECTION)
  create-subtask PARENT_GID --name NAME [task fields]
  update TASK_GID [task fields]
  set-parent TASK_GID --parent GID [--insert-before GID|--insert-after GID]
  unset-parent TASK_GID
  comment TASK_GID (--text TEXT|--html-text HTML)
  add-project|remove-project TASK_GID --project GID
  add-dependencies|remove-dependencies TASK_GID --dependency GID...
  add-dependents|remove-dependents TASK_GID --dependent GID...
  add-tag|remove-tag TASK_GID --tag GID...
  add-followers|remove-followers TASK_GID --follower GID...
  duplicate TASK_GID [--name NAME] [--include FIELDS]
  delete TASK_GID

Task fields include --notes/--html-notes, --assignee, --completed true|false,
--approval-status, --resource-subtype, --start-on/--start-at, --due-on/--due-at,
repeatable --follower, --project, --tag, --custom-field GID=STRING,
--custom-field-json GID=JSON, and --membership PROJECT_GID=SECTION_GID.

Search requires Asana Premium. Results are eventually consistent and do not
support normal offset pagination; --limit accepts at most 100 items. Filters
include --assignee, --projects-any, --sections-any, --tags-any, --text,
--completed, --is-subtask, --modified-at-after, --due-on-before/--due-on-after,
--start-on-before/--start-on-after, --sort-by, and --sort-ascending.
Deleted tasks remain in the deleting user's Asana trash and can be recovered
for 30 days; afterward Asana removes them permanently.
`,
		"projects": `Usage: asana-cli projects <subcommand> [options]

  list [WORKSPACE_GID] [--opt-fields FIELDS]
  get PROJECT_GID
  create --workspace GID --name NAME [project fields]
  update PROJECT_GID [project fields]
  tasks PROJECT_GID [--opt-fields FIELDS]
  task-counts PROJECT_GID
  add-followers|remove-followers PROJECT_GID --follower GID...
  duplicate PROJECT_GID --name NAME [--include FIELDS]
    [(--start-on DATE|--due-on DATE) --skip-weekends true|false]
  save-as-template PROJECT_GID --name NAME --public true|false [--team GID|--workspace GID]
  delete PROJECT_GID

Project fields include --notes/--html-notes, --color, --icon, --default-view,
--privacy-setting, --archived true|false, --owner, --start-on, --due-on,
--default-access-level, repeatable --follower, --custom-field GID=STRING, and
--custom-field-json GID=JSON.
Task-count requests have an additional Asana rate/cost limit.
`,
		"sections": `Usage: asana-cli sections <subcommand> [options]

  list PROJECT_GID | get SECTION_GID
  create --project GID --name NAME [--insert-before GID|--insert-after GID]
  update SECTION_GID --name NAME
  tasks SECTION_GID [--opt-fields FIELDS]
  add-task SECTION_GID --task GID [--insert-before GID|--insert-after GID]
  move SECTION_GID --project GID [--before-section GID|--after-section GID]
  delete SECTION_GID

Asana only permits deleting an empty section and does not permit deleting the
last section in a project.
`,
		"stories": `Usage: asana-cli stories get|update|delete STORY_GID [options]

Only comment stories can be updated. Use exactly one of --text or --html-text.
`,
		"attachments": `Usage: asana-cli attachments <subcommand> [options]

  list PARENT_GID [--opt-fields FIELDS]
  get ATTACHMENT_GID
  upload --parent GID --file PATH
  upload --parent GID --url URL --name NAME [--connect-to-app]
  delete ATTACHMENT_GID

Parents may be tasks, projects, or project briefs. File uploads are limited to
100MB. Non-ASCII filenames are sent as UTF-8 multipart filenames.
`,
		"memberships": `Usage: asana-cli memberships <subcommand> [options]

  list (--parent GID|--member GID) [--opt-fields FIELDS]
  get MEMBERSHIP_GID
  create --parent PROJECT_GID --member USER_OR_TEAM_GID [--access-level LEVEL]
  update MEMBERSHIP_GID --access-level LEVEL
  delete MEMBERSHIP_GID
`,
		"jobs": `Usage: asana-cli jobs get JOB_GID

Use this command to inspect the job GID returned by duplicate and
save-as-template operations.
`,
		"workspaces": `Usage: asana-cli workspaces list
       asana-cli workspaces projects WORKSPACE_GID [--opt-fields FIELDS]
       asana-cli workspaces create-project WORKSPACE_GID --name NAME
`,
		"teams": `Usage: asana-cli teams projects TEAM_GID [--opt-fields FIELDS]
       asana-cli teams create-project TEAM_GID --name NAME
`,
	}
	if text, ok := help[cmd]; ok {
		return text
	}
	if parent, _, ok := strings.Cut(cmd, " "); ok {
		if text, exists := help[parent]; exists {
			return text
		}
	}
	return "Usage: asana-cli " + cmd + " [options]\n"
}
