package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	OpenBrowser                                         func(string) error
	HTTPClient                                          interface{}
}

func NewRuntimeOptionsFromEnv() RuntimeOptions {
	return RuntimeOptions{APIBase: getenv("ASANA_API_BASE", "https://app.asana.com/api/1.0/"), TokenEndpoint: getenv("ASANA_OAUTH_TOKEN_ENDPOINT", "https://app.asana.com/-/oauth_token"), Browser: os.Getenv("BROWSER"), Output: "table"}
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
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func client(rt RuntimeOptions) *asana.Client { return asana.NewClient(rt.APIBase, rt.TokenEndpoint) }
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
	tok, err := loadToken(rt.ConfigPath)
	if err != nil {
		return err
	}
	c := client(rt)
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
		p, err := parseFlags(args, []string{"workspace"}, nil, nil, 1)
		if err != nil {
			return err
		}
		w, err := target(p, "workspace")
		if err != nil {
			return err
		}
		v, err := c.ListProjects(context.Background(), tok, values("workspace", w))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "projects", v)
	case "tasks:list":
		p, err := parseFlags(args, []string{"project"}, nil, nil, 1)
		if err != nil {
			return err
		}
		pr, err := target(p, "project")
		if err != nil {
			return err
		}
		v, err := c.ListTasks(context.Background(), tok, values("project", pr))
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, "tasks", v)
	case "tasks:get", "tasks:subtasks", "tasks:stories", "tasks:comments", "tasks:attachments":
		p, err := parseFlags(args, []string{"task"}, nil, nil, 1)
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
			v, err = c.ListSubtasks(context.Background(), tok, gid)
		} else if sub == "stories" {
			v, err = c.ListStories(context.Background(), tok, gid)
		} else if sub == "comments" {
			v, err = c.ListComments(context.Background(), tok, gid)
		} else {
			v, err = c.ListAttachments(context.Background(), tok, gid)
		}
		if err != nil {
			return err
		}
		return render(io.out(), rt.Output, sub, v)
	}
	return fmt.Errorf("unknown command")
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

var columns = map[string][]string{"workspaces": {"gid", "name"}, "projects": {"gid", "name", "workspace.name"}, "tasks": {"gid", "name", "completed", "created_at", "modified_at"}, "task": {"gid", "name", "completed", "notes", "created_at", "modified_at"}, "subtasks": {"gid", "name", "completed"}, "stories": {"gid", "resource_subtype", "text", "created_at", "created_by.name"}, "comments": {"gid", "resource_subtype", "text", "html_text", "created_at", "created_by.name"}, "attachments": {"gid", "name", "download_url", "created_at"}, "me": {"gid", "name", "email"}, "token": {"access_token", "refresh_token", "token_type", "expires_in", "expires_at"}, "status": {"status", "authenticated", "clientId", "redirectUri", "token.access_token", "token.refresh_token", "token.expires_at"}}

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
	return `asana-cli - Asana OAuth and read-only API CLI

Primary commands:
  auth, me, workspaces, projects, tasks

Usage:
  asana-cli [--config PATH] [--output json|table|compact] <command>

Commands:
  auth        OAuth authentication commands
  me          Show current user
  workspaces  List workspaces
  projects    List projects
  tasks       List and inspect tasks

Global flags:
  --config PATH      Credentials file path
  --output FORMAT    json, table, or compact
  --help, -h         Show help
  --version, -V      Show version
`
}
func commandHelp(cmd string) string {
	return "Usage: asana-cli " + cmd + " [options]\n\nRun with valid subcommands and flags.\n"
}
