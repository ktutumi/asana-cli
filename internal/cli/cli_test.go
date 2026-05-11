package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/cli"
	"github.com/ktutumi/asana-cli-go/internal/config"
)

func TestRootHelp(t *testing.T) {
	ioOut := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	code := cli.RunCLI([]string{"--help"}, ioOut, cli.RuntimeOptions{})
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if got := ioOut.Out.(*bytes.Buffer).String(); !strings.Contains(got, "Primary commands") {
		t.Fatalf("help output: %s", got)
	}
}

func TestVersion(t *testing.T) {
	ioOut := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	code := cli.RunCLI([]string{"--version"}, ioOut, cli.RuntimeOptions{})
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if got := strings.TrimSpace(ioOut.Out.(*bytes.Buffer).String()); got == "" {
		t.Fatalf("empty version")
	}
}

func TestUnknownGlobalFlag(t *testing.T) {
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"--mystery"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, cli.RuntimeOptions{})
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
	if !strings.Contains(errBuf.String(), "unknown global flag") {
		t.Fatalf("err=%s", errBuf.String())
	}
}

func TestInvalidOutput(t *testing.T) {
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"--output", "invalid", "me"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, cli.RuntimeOptions{})
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
	if !strings.Contains(errBuf.String(), "invalid output") {
		t.Fatalf("err=%s", errBuf.String())
	}
}

func TestConfigFlagVariants(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	writeConfig(t, path, config.StoredConfig{ClientID: "id"})

	ioOut := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	code := cli.RunCLI([]string{"--config", path, "auth", "status"}, ioOut, cli.RuntimeOptions{})
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}

	ioOut2 := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	code = cli.RunCLI([]string{"--config=" + path, "auth", "status"}, ioOut2, cli.RuntimeOptions{})
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}

	if got := ioOut.Out.(*bytes.Buffer).String(); !strings.Contains(got, "status") {
		t.Fatalf("got=%s", got)
	}
}

func TestEnvOverrideAffectsClient(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer atok" {
			t.Fatalf("authorization=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"u1","name":"user"}}`))
	}))
	t.Cleanup(apiServer.Close)

	t.Setenv("ASANA_API_BASE", apiServer.URL+"/")
	opts := cli.NewRuntimeOptionsFromEnv()

	cfg := config.StoredConfig{ClientID: "id", Token: &config.TokenData{AccessToken: "atok"}}
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	writeConfig(t, path, cfg)
	opts.ConfigPath = path

	ioOut := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	code := cli.RunCLI([]string{"me"}, ioOut, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if got := ioOut.Out.(*bytes.Buffer).String(); !strings.Contains(got, "u1") {
		t.Fatalf("got=%s", got)
	}
}

func TestParseAndRejectInapplicableFlag(t *testing.T) {
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"auth", "refresh", "--code", "abc"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t)})
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
	if !strings.Contains(errBuf.String(), "not applicable") {
		t.Fatalf("err=%s", errBuf.String())
	}
}

func TestTasksDuplicateTarget(t *testing.T) {
	ioOut := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t)}

	code := cli.RunCLI([]string{"tasks", "get", "task-1", "--task", "task-2"}, ioOut, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
}

func TestAuthExchangeRequiresFlagsAndSavesConfig(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "grant_type=authorization_code") {
			t.Fatalf("body=%s", string(raw))
		}
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":1,"expires_at":"now"}`))
	}))
	t.Cleanup(tokenSrv.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid"})

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	opts := cli.RuntimeOptions{ConfigPath: cfgPath, TokenEndpoint: tokenSrv.URL + "/oauth", Output: "compact"}
	code := cli.RunCLI([]string{"auth", "exchange", "--code", "code", "--client-secret", "secret"}, &cli.CliIO{Out: outBuf, ErrOut: errBuf}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d err=%s", code, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "access_token=***") {
		t.Fatalf("output=%s", outBuf.String())
	}
}

func TestAuthRefreshUsesSavedTokenWhenFlagMissing(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=refresh_token") {
			t.Fatalf("body=%s", string(body))
		}
		if !strings.Contains(string(body), "refresh_token=rt1") {
			t.Fatalf("body=%s", string(body))
		}
		_, _ = w.Write([]byte(`{"access_token":"at2","refresh_token":"rt2","token_type":"Bearer","expires_in":5}`))
	}))
	t.Cleanup(tokenSrv.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{RefreshToken: "rt1", AccessToken: "old"}})

	opts := cli.RuntimeOptions{ConfigPath: cfgPath, TokenEndpoint: tokenSrv.URL}
	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"auth", "refresh", "--client-secret", "secret"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if !strings.Contains(outBuf.String(), "refresh_token=***") {
		t.Fatalf("output=%s", outBuf.String())
	}
}

func TestAuthLoginRejectsBadRedirect(t *testing.T) {
	errBuf := &bytes.Buffer{}
	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t)}
	code := cli.RunCLI([]string{"auth", "login", "--client-id", "cid", "--client-secret", "secret", "--redirect-uri", "https://example.com/cb"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
}

func TestTasksGetMakesRequestAndRenders(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/tasks/task%2F1" {
			t.Fatalf("path=%s", r.URL.EscapedPath())
		}
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"task/1","name":"Task Name"}}`))
	}))
	t.Cleanup(api.Close)

	cfgPath := mustWriteConfigWithToken(t)
	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/"}

	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"tasks", "get", "task/1"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d output=%s", code, outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "gid") {
		t.Fatalf("unexpected output %s", outBuf.String())
	}
}

func TestTasksListQueryAndDuplicateRules(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("project") != "pid" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"gid":"t1","name":"A"}]}`))
	}))
	t.Cleanup(api.Close)

	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: api.URL + "/", Output: "json"}
	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"tasks", "list", "pid", "--project", "pid"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}

	code = cli.RunCLI([]string{"tasks", "list", "--project", "pid"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if !strings.Contains(outBuf.String(), "t1") {
		t.Fatalf("output=%s", outBuf.String())
	}
}

func TestAuthURLHasDefaultScopesAndState(t *testing.T) {
	cfgPath := mustWriteConfigWithToken(t)
	buf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"auth", "url", "--client-id", "cid"}, &cli.CliIO{Out: buf, ErrOut: &bytes.Buffer{}}, cli.RuntimeOptions{ConfigPath: cfgPath})
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}

	got := buf.String()
	parsed, err := url.Parse(strings.TrimSpace(got))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("scope") == "" {
		t.Fatalf("scope missing in %s", got)
	}
	if q.Get("state") == "" {
		t.Fatalf("state missing in %s", got)
	}
}

func TestRenderEscapingInCompact(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"g\tid","name":"line1\nline2","email":"x"}}`))
	}))
	t.Cleanup(api.Close)

	cfgPath := mustWriteConfigWithToken(t)
	outBuf := &bytes.Buffer{}
	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/", Output: "compact"}
	code := cli.RunCLI([]string{"me"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if !strings.Contains(outBuf.String(), "gid=g\\tid") {
		t.Fatalf("out=%s", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "name=line1\\nline2") {
		t.Fatalf("out=%s", outBuf.String())
	}
}

func TestHelpForAuthSubcommandDoesNotRequireSecret(t *testing.T) {
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"auth", "exchange", "--help"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t)})
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
}

func TestSectionsListMakesRequestAndRenders(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/proj-1/sections" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"gid":"s1","name":"To Do"},{"gid":"s2","name":"Done"}]}`))
	}))
	t.Cleanup(api.Close)

	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: api.URL + "/", Output: "json"}
	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"sections", "list", "proj-1"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d", code)
	}
	if !strings.Contains(outBuf.String(), "s1") || !strings.Contains(outBuf.String(), "To Do") {
		t.Fatalf("output=%s", outBuf.String())
	}
}

func TestSectionsListRequiresProject(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(api.Close)

	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: api.URL + "/"}
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"sections", "list"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
	if !strings.Contains(errBuf.String(), "project gid is required") {
		t.Fatalf("err=%s", errBuf.String())
	}
}

func TestSectionsListDuplicateTarget(t *testing.T) {
	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t)}
	ioOut := &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	code := cli.RunCLI([]string{"sections", "list", "proj-1", "--project", "proj-2"}, ioOut, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
}

func TestEnvReadsClientSecret(t *testing.T) {
	t.Setenv("ASANA_CLIENT_SECRET", "secret")
	opts := cli.NewRuntimeOptionsFromEnv()
	if opts.ClientSecret != "secret" {
		t.Fatalf("ClientSecret=%q", opts.ClientSecret)
	}
}

func TestAPICmdRefreshesExpiredTokenWhenClientSecretSet(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=refresh_token") {
			t.Fatalf("body=%s", string(body))
		}
		if !strings.Contains(string(body), "client_id=cid") {
			t.Fatalf("body=%s", string(body))
		}
		if !strings.Contains(string(body), "client_secret=secret") {
			t.Fatalf("body=%s", string(body))
		}
		if !strings.Contains(string(body), "refresh_token=rt1") {
			t.Fatalf("body=%s", string(body))
		}
		_, _ = w.Write([]byte(`{"access_token":"at2","refresh_token":"rt2","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at2" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"u1","name":"user"}}`))
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	expiresAt := time.Now().UTC().Add(1 * time.Minute).Format(time.RFC3339)
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "at1", RefreshToken: "rt1", TokenType: "Bearer", ExpiresAt: expiresAt}})

	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/", TokenEndpoint: tokenSrv.URL, ClientSecret: "secret"}
	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"me"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d output=%s", code, outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "u1") {
		t.Fatalf("output=%s", outBuf.String())
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Token.AccessToken != "at2" {
		t.Fatalf("access token not updated: %s", cfg.Token.AccessToken)
	}
	if cfg.Token.RefreshToken != "rt2" {
		t.Fatalf("refresh token not updated: %s", cfg.Token.RefreshToken)
	}
}

func TestAPICmdDoesNotRefreshWithoutClientSecret(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("token server should not be called")
	}))
	t.Cleanup(tokenSrv.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at1" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"u1","name":"user"}}`))
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	expiresAt := time.Now().UTC().Add(1 * time.Minute).Format(time.RFC3339)
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "at1", RefreshToken: "rt1", TokenType: "Bearer", ExpiresAt: expiresAt}})

	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/", TokenEndpoint: tokenSrv.URL, ClientSecret: ""}
	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"me"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d output=%s", code, outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "u1") {
		t.Fatalf("output=%s", outBuf.String())
	}
}

func TestAPICmdDoesNotRefreshWhenTokenStillValid(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("token server should not be called")
	}))
	t.Cleanup(tokenSrv.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at1" {
			t.Fatalf("auth=%s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"u1","name":"user"}}`))
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	expiresAt := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "at1", RefreshToken: "rt1", TokenType: "Bearer", ExpiresAt: expiresAt}})

	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/", TokenEndpoint: tokenSrv.URL, ClientSecret: "secret"}
	outBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"me"}, &cli.CliIO{Out: outBuf, ErrOut: &bytes.Buffer{}}, opts)
	if code != 0 {
		t.Fatalf("want 0 got %d output=%s", code, outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "u1") {
		t.Fatalf("output=%s", outBuf.String())
	}
}

func TestAPICmdRefreshFailureDoesNotCallAPI(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(tokenSrv.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("API server should not be called")
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	expiresAt := time.Now().UTC().Add(1 * time.Minute).Format(time.RFC3339)
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "at1", RefreshToken: "rt1", TokenType: "Bearer", ExpiresAt: expiresAt}})

	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/", TokenEndpoint: tokenSrv.URL, ClientSecret: "secret"}
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"me"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
	if !strings.Contains(errBuf.String(), "refresh access token") {
		t.Fatalf("err=%s", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "secret") || strings.Contains(errBuf.String(), "rt1") {
		t.Fatalf("secret or token leaked in err=%s", errBuf.String())
	}
}

func TestAPICmdExpiredTokenWithoutRefreshTokenErrors(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("API server should not be called")
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "credentials.json")
	expiresAt := time.Now().UTC().Add(1 * time.Minute).Format(time.RFC3339)
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "at1", RefreshToken: "", TokenType: "Bearer", ExpiresAt: expiresAt}})

	opts := cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL + "/", ClientSecret: "secret"}
	errBuf := &bytes.Buffer{}
	code := cli.RunCLI([]string{"me"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errBuf}, opts)
	if code != 1 {
		t.Fatalf("want 1 got %d", code)
	}
	if !strings.Contains(errBuf.String(), "refresh token is not configured") {
		t.Fatalf("err=%s", errBuf.String())
	}
}

func writeConfig(t *testing.T, path string, cfg config.StoredConfig) {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mustWriteConfigWithToken(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	writeConfig(t, path, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "at", RefreshToken: "rt1", TokenType: "Bearer", ExpiresIn: 1, ExpiresAt: "now"}})
	return path
}
