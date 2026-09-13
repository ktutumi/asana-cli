package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/cli"
	"github.com/ktutumi/asana-cli-go/internal/config"
)

const testPAT = "pat-test-opaque-123"

func TestPATHelpGuidance(t *testing.T) {
	out := &bytes.Buffer{}
	code := cli.RunCLI([]string{"--help"}, &cli.CliIO{Out: out, ErrOut: &bytes.Buffer{}}, cli.RuntimeOptions{})
	if code != 0 {
		t.Fatalf("help exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "ASANA_PAT") {
		t.Fatal("root help does not document ASANA_PAT authentication")
	}
}

func TestPATRuntimeOptionsEnvironmentAndExplicitIsolation(t *testing.T) {
	t.Setenv("ASANA_PAT", testPAT)
	if got := cli.NewRuntimeOptionsFromEnv().PAT; got != testPAT {
		t.Fatal("ASANA_PAT was not copied exactly into RuntimeOptions")
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-access" {
			t.Fatal("an explicit RuntimeOptions unexpectedly used ambient ASANA_PAT")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1","name":"Saved"}}`))
	}))
	t.Cleanup(api.Close)

	cfgPath := filepath.Join(t.TempDir(), "credentials.json")
	writeConfig(t, cfgPath, config.StoredConfig{Token: &config.TokenData{AccessToken: "saved-access"}})
	code, _, _ := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL, PAT: ""})
	if code != 0 {
		t.Fatal("explicit RuntimeOptions with an empty PAT did not use the saved token")
	}
}

func TestPATUsesExactBearerWithoutCreatingConfig(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testPAT {
			t.Fatal("API request did not use the exact PAT as its Bearer token")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1","name":"PAT user"}}`))
	}))
	t.Cleanup(api.Close)

	missingParent := filepath.Join(t.TempDir(), "not-created")
	cfgPath := filepath.Join(missingParent, "credentials.json")
	code, _, _ := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL, PAT: testPAT})
	if code != 0 {
		t.Fatal("PAT-only API command failed")
	}
	if _, err := os.Stat(missingParent); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("PAT-only API command created a config directory")
	}
}

func TestPATPrecedesExpiredOAuthWithoutChangingConfig(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testPAT {
			t.Fatal("PAT did not take precedence over the stored OAuth token")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1"}}`))
	}))
	t.Cleanup(api.Close)

	tokenCalls := 0
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(tokenEndpoint.Close)

	cfgPath := filepath.Join(t.TempDir(), "credentials.json")
	original := []byte("{\n  \"clientId\": \"saved-client\",\n  \"token\": {\n    \"access_token\": \"saved-access\",\n    \"refresh_token\": \"saved-refresh\",\n    \"expires_at\": \"2000-01-01T00:00:00Z\"\n  }\n}\n")
	if err := os.WriteFile(cfgPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgPath, 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(cfgPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	before := snapshotConfig(t, cfgPath)

	code, _, _ := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{
		ConfigPath:    cfgPath,
		APIBase:       api.URL,
		TokenEndpoint: tokenEndpoint.URL,
		ClientSecret:  "test-client-secret",
		PAT:           testPAT,
	})
	if code != 0 {
		t.Fatal("PAT API command failed with an expired stored OAuth token")
	}
	if tokenCalls != 0 {
		t.Fatal("PAT API command attempted OAuth refresh")
	}
	assertConfigUnchanged(t, cfgPath, before)
}

func TestPATBypassesInvalidAndExplicitConfigPaths(t *testing.T) {
	apiCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		if r.Header.Get("Authorization") != "Bearer "+testPAT {
			t.Fatal("PAT API request used an unexpected Authorization header")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1"}}`))
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	invalidJSON := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidJSON, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, _ := runPATCLI(t, []string{"--config", invalidJSON, "me"}, cli.RuntimeOptions{APIBase: api.URL, PAT: testPAT})
	if code != 0 {
		t.Fatal("PAT API command depended on an explicitly selected invalid config")
	}

	directoryConfig := filepath.Join(dir, "config-directory")
	if err := os.Mkdir(directoryConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runPATCLI(t, []string{"auth", "status", "--config", directoryConfig}, cli.RuntimeOptions{APIBase: api.URL, PAT: testPAT, Output: "compact"})
	if code != 0 || !strings.Contains(out, "authSource=env:ASANA_PAT") {
		t.Fatal("PAT status depended on an unreadable explicit config")
	}
	if apiCalls != 1 {
		t.Fatal("auth status made an HTTP request")
	}
}

func TestPATUnsetOrEmptyUsesOAuthAndMissingTokenGuidance(t *testing.T) {
	t.Setenv("ASANA_PAT", "")
	if got := cli.NewRuntimeOptionsFromEnv().PAT; got != "" {
		t.Fatal("empty ASANA_PAT was not treated as unset")
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-access" {
			t.Fatal("empty PAT did not fall back to the saved access token")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1"}}`))
	}))
	t.Cleanup(api.Close)

	cfgPath := filepath.Join(t.TempDir(), "credentials.json")
	writeConfig(t, cfgPath, config.StoredConfig{Token: &config.TokenData{AccessToken: "saved-access"}})
	code, _, _ := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL, PAT: ""})
	if code != 0 {
		t.Fatal("empty PAT did not preserve the saved-token path")
	}

	missing := filepath.Join(t.TempDir(), "missing.json")
	code, _, errOut := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: missing, APIBase: api.URL, PAT: ""})
	if code != 1 || !strings.Contains(errOut, "ASANA_PAT") {
		t.Fatal("missing-token guidance does not mention ASANA_PAT")
	}
}

func TestPATRejectsInvalidHeaderValuesWithoutFallback(t *testing.T) {
	apiCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		_, _ = w.Write([]byte(`{"data":{"gid":"unexpected"}}`))
	}))
	t.Cleanup(api.Close)

	tokenCalls := 0
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		_, _ = w.Write([]byte(`{"access_token":"unexpected"}`))
	}))
	t.Cleanup(tokenEndpoint.Close)

	cfgPath := filepath.Join(t.TempDir(), "credentials.json")
	writeConfig(t, cfgPath, config.StoredConfig{ClientID: "cid", Token: &config.TokenData{AccessToken: "saved-access", RefreshToken: "saved-refresh", ExpiresAt: "2000-01-01T00:00:00Z"}})
	invalid := []struct {
		name string
		pat  string
	}{
		{name: "whitespace only", pat: " 	"},
		{name: "leading whitespace", pat: " " + testPAT},
		{name: "trailing whitespace", pat: testPAT + " "},
		{name: "newline", pat: testPAT + "\nnext"},
		{name: "carriage return", pat: testPAT + "\rnext"},
		{name: "control character", pat: testPAT + "\x01next"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			beforeAPI, beforeToken := apiCalls, tokenCalls
			code, _, errOut := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{
				ConfigPath:    cfgPath,
				APIBase:       api.URL,
				TokenEndpoint: tokenEndpoint.URL,
				ClientSecret:  "test-client-secret",
				PAT:           tt.pat,
			})
			if code != 1 {
				t.Fatal("invalid PAT did not fail")
			}
			if apiCalls != beforeAPI || tokenCalls != beforeToken {
				t.Fatal("invalid PAT made an API request or attempted OAuth fallback")
			}
			if strings.Contains(errOut, tt.pat) {
				t.Fatal("invalid PAT value was exposed in CLI error output")
			}
		})
	}
}

func TestPATPreservesOpaqueBearerValue(t *testing.T) {
	opaque := "opaque.PAT_123-~"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+opaque {
			t.Fatal("opaque PAT was changed before it reached the API")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1"}}`))
	}))
	t.Cleanup(api.Close)

	code, _, _ := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: filepath.Join(t.TempDir(), "missing.json"), APIBase: api.URL, PAT: opaque})
	if code != 0 {
		t.Fatal("valid opaque PAT failed")
	}
}

func TestPATStatusReportsSourcesInAllFormats(t *testing.T) {
	apiCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(api.Close)

	dir := t.TempDir()
	invalidConfig := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidConfig, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.json")
	writeConfig(t, configPath, config.StoredConfig{
		ClientID:    "saved	client",
		RedirectURI: "saved\nredirect",
		Token:       &config.TokenData{AccessToken: "saved-access", RefreshToken: "saved-refresh", TokenType: "Bearer", ExpiresAt: "2030-01-01T00:00:00Z"},
	})
	nonePath := filepath.Join(dir, "none.json")

	cases := []struct {
		name              string
		pat               string
		configPath        string
		wantSource        string
		wantAuthenticated bool
	}{
		{name: "PAT", pat: testPAT, configPath: invalidConfig, wantSource: "env:ASANA_PAT", wantAuthenticated: true},
		{name: "config", configPath: configPath, wantSource: "config", wantAuthenticated: true},
		{name: "none", configPath: nonePath, wantSource: "none", wantAuthenticated: false},
	}
	for _, tc := range cases {
		for _, format := range []string{"json", "table", "compact"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				code, out, _ := runPATCLI(t, []string{"--output", format, "auth", "status", "--config", tc.configPath}, cli.RuntimeOptions{APIBase: api.URL, PAT: tc.pat})
				if code != 0 {
					t.Fatal("auth status failed")
				}
				status := assertStatusSource(t, format, out, tc.wantSource, tc.wantAuthenticated)
				if tc.name == "PAT" {
					if strings.Contains(out, testPAT) || strings.Contains(out, "saved-access") {
						t.Fatal("PAT status exposed a token or mixed stored OAuth data")
					}
					if format == "json" {
						token, ok := status["token"].(map[string]any)
						if !ok || token["access_token"] != "***" || token["token_type"] != "Bearer" || token["expires_in"] != float64(0) || token["refresh_token"] != "" || token["expires_at"] != "" {
							t.Fatal("PAT JSON status did not contain the specified redacted local state")
						}
						if status["clientId"] != "" || status["redirectUri"] != "" {
							t.Fatal("PAT JSON status mixed saved configuration metadata")
						}
					}
				}
				if tc.name == "config" {
					if strings.Contains(out, "saved-access") || strings.Contains(out, "saved-refresh") {
						t.Fatal("config status exposed a stored token")
					}
					expectedClient := "saved" + string([]byte{92}) + "tclient"
					expectedRedirect := "saved" + string([]byte{92}) + "nredirect"
					if !strings.Contains(out, expectedClient) || !strings.Contains(out, expectedRedirect) {
						t.Fatal("status no longer escapes table or compact scalar output")
					}
				}
			})
		}
	}
	if apiCalls != 0 {
		t.Fatal("auth status made an HTTP request")
	}
}

func TestPATRejectionDoesNotFallbackOrChangeConfig(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			apiCalls := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiCalls++
				if r.Header.Get("Authorization") != "Bearer "+testPAT {
					t.Fatal("rejected API request did not use the PAT")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"errors":[{"message":"server reflected ` + testPAT + `"}]}`))
			}))
			t.Cleanup(api.Close)

			tokenCalls := 0
			tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenCalls++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(tokenEndpoint.Close)

			cfgPath := filepath.Join(t.TempDir(), "credentials.json")
			original := []byte(`{"clientId":"saved-client","token":{"access_token":"saved-access","refresh_token":"saved-refresh","expires_at":"2000-01-01T00:00:00Z"}}`)
			if err := os.WriteFile(cfgPath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(cfgPath, 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshotConfig(t, cfgPath)

			code, _, errOut := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL, TokenEndpoint: tokenEndpoint.URL, ClientSecret: "test-client-secret", PAT: testPAT})
			if code != 1 || apiCalls != 1 || tokenCalls != 0 {
				t.Fatal("rejected PAT request retried or attempted OAuth fallback")
			}
			if strings.Contains(errOut, testPAT) || !strings.Contains(errOut, "***") {
				t.Fatal("reflected API error leaked the PAT instead of masking it")
			}
			assertConfigUnchanged(t, cfgPath, before)
		})
	}
}

func TestPATMasksAllCLIErrorBoundaries(t *testing.T) {
	pat := "pat-boundary-value"
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "global parsing", args: []string{"--output", pat}},
		{name: "command dispatch", args: []string{"unknown-" + pat}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runPATCLI(t, tc.args, cli.RuntimeOptions{PAT: pat})
			if code != 1 || strings.Contains(errOut, pat) || !strings.Contains(errOut, "***") {
				t.Fatal("CLI error boundary exposed the PAT")
			}
		})
	}
}

func TestPATPropagatesToJSONWritesPaginationAndMultipart(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(filePath, []byte("attachment body"), 0o600); err != nil {
		t.Fatal(err)
	}

	pageCalls := 0
	writeCalls := 0
	uploadCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testPAT {
			t.Fatal("PAT was not propagated to an API transport")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/tasks":
			writeCalls++
			var envelope map[string]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope["data"]["workspace"] != "workspace-1" || envelope["data"]["name"] != "Created" {
				t.Fatal("JSON write did not preserve the expected request data")
			}
			_, _ = w.Write([]byte(`{"data":{"gid":"task-1","name":"Created"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/workspaces":
			pageCalls++
			if r.URL.Query().Get("offset") == "" {
				_, _ = w.Write([]byte(`{"data":[{"gid":"workspace-1"}],"next_page":{"offset":"page-2"}}`))
				return
			}
			if r.URL.Query().Get("offset") != "page-2" {
				t.Fatal("pagination did not use the returned offset")
			}
			_, _ = w.Write([]byte(`{"data":[{"gid":"workspace-2"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/attachments":
			uploadCalls++
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("parent") != "parent-1" {
				t.Fatal("multipart upload did not preserve the parent")
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil || string(body) != "attachment body" {
				t.Fatal("multipart upload did not preserve the temporary file")
			}
			_, _ = w.Write([]byte(`{"data":{"gid":"attachment-1","name":"attachment.txt"}}`))
		default:
			t.Fatal("unexpected PAT API route")
		}
	}))
	t.Cleanup(api.Close)

	missingParent := filepath.Join(t.TempDir(), "not-created")
	opts := cli.RuntimeOptions{ConfigPath: filepath.Join(missingParent, "credentials.json"), APIBase: api.URL, Output: "json", PAT: testPAT}
	for _, args := range [][]string{
		{"tasks", "create", "--workspace", "workspace-1", "--name", "Created"},
		{"workspaces", "list"},
		{"attachments", "upload", "--parent", "parent-1", "--file", filePath},
	} {
		code, _, _ := runPATCLI(t, args, opts)
		if code != 0 {
			t.Fatal("PAT command for a write, paginated list, or multipart upload failed")
		}
	}
	if writeCalls != 1 || pageCalls != 2 || uploadCalls != 1 {
		t.Fatal("not every JSON write, pagination request, and multipart upload completed")
	}
	if _, err := os.Stat(missingParent); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("PAT transport created a config directory")
	}
}

func TestPATDoesNotBlockExplicitOAuthOperationsOrPersist(t *testing.T) {
	invalidPAT := "invalid-pat\nvalue"
	tokenCalls := 0
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), testPAT) || strings.Contains(string(body), invalidPAT) {
			t.Fatal("an explicit OAuth operation sent the PAT to the token endpoint")
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		switch form.Get("grant_type") {
		case "authorization_code":
			if form.Get("client_id") != "oauth-client" || form.Get("client_secret") != "oauth-secret" || form.Get("code") != "oauth-code" {
				t.Fatal("auth exchange did not use its explicit OAuth values")
			}
			_, _ = w.Write([]byte(`{"access_token":"oauth-access","refresh_token":"oauth-refresh","token_type":"Bearer","expires_in":3600}`))
		case "refresh_token":
			if form.Get("client_id") != "oauth-client" || form.Get("client_secret") != "oauth-secret" || form.Get("refresh_token") != "oauth-refresh" {
				t.Fatal("auth refresh did not use its explicit OAuth values")
			}
			_, _ = w.Write([]byte(`{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","token_type":"Bearer","expires_in":3600}`))
		default:
			t.Fatal("unexpected OAuth grant type")
		}
	}))
	t.Cleanup(tokenEndpoint.Close)

	for _, args := range [][]string{
		{"auth", "url", "--client-id", "oauth-client", "--state", "fixed"},
		{"--help"},
		{"--version"},
		{"--skill"},
	} {
		code, _, _ := runPATCLI(t, args, cli.RuntimeOptions{PAT: invalidPAT})
		if code != 0 {
			t.Fatal("invalid PAT blocked an explicit OAuth utility or informational command")
		}
	}

	loginCode, _, loginErr := runPATCLI(t, []string{"auth", "login", "--client-id", "oauth-client", "--client-secret", "oauth-secret", "--redirect-uri", "http://example.com/callback"}, cli.RuntimeOptions{ConfigPath: filepath.Join(t.TempDir(), "missing.json"), PAT: invalidPAT})
	if loginCode != 1 || !strings.Contains(loginErr, "localhost") || strings.Contains(loginErr, invalidPAT) {
		t.Fatal("invalid PAT changed explicit auth login validation")
	}

	for _, pat := range []string{testPAT, invalidPAT} {
		exchangePath := filepath.Join(t.TempDir(), "exchange.json")
		code, out, errOut := runPATCLI(t, []string{"auth", "exchange", "--client-id", "oauth-client", "--client-secret", "oauth-secret", "--redirect-uri", "http://127.0.0.1/callback", "--code", "oauth-code"}, cli.RuntimeOptions{ConfigPath: exchangePath, TokenEndpoint: tokenEndpoint.URL, Output: "json", PAT: pat})
		if code != 0 || strings.Contains(out, pat) || strings.Contains(errOut, pat) {
			t.Fatal("PAT blocked or leaked through auth exchange")
		}
		assertFileDoesNotContain(t, exchangePath, pat)

		refreshPath := filepath.Join(t.TempDir(), "refresh.json")
		writeConfig(t, refreshPath, config.StoredConfig{ClientID: "oauth-client", Token: &config.TokenData{AccessToken: "oauth-access", RefreshToken: "oauth-refresh"}})
		code, out, errOut = runPATCLI(t, []string{"auth", "refresh", "--client-secret", "oauth-secret"}, cli.RuntimeOptions{ConfigPath: refreshPath, TokenEndpoint: tokenEndpoint.URL, Output: "json", PAT: pat})
		if code != 0 || strings.Contains(out, pat) || strings.Contains(errOut, pat) {
			t.Fatal("PAT blocked or leaked through auth refresh")
		}
		assertFileDoesNotContain(t, refreshPath, pat)
	}
	if tokenCalls != 4 {
		t.Fatal("explicit OAuth exchange and refresh did not each use the OAuth endpoint for valid and invalid PAT values")
	}
}

func TestPATDocumentationAndEmbeddedSkill(t *testing.T) {
	invalidPAT := "invalid-pat\nvalue"
	code, help, _ := runPATCLI(t, []string{"--help"}, cli.RuntimeOptions{PAT: invalidPAT})
	if code != 0 || !strings.Contains(help, "ASANA_PAT") || !strings.Contains(help, "takes precedence") || !strings.Contains(help, "not persisted") {
		t.Fatal("root help does not describe PAT precedence and non-persistence")
	}
	code, skill, _ := runPATCLI(t, []string{"--skill"}, cli.RuntimeOptions{PAT: invalidPAT})
	if code != 0 || !strings.Contains(skill, "ASANA_PAT") || !strings.Contains(skill, "unset ASANA_PAT") || !strings.Contains(skill, "auth status") {
		t.Fatal("embedded operator skill does not give PAT guidance")
	}

	root := repositoryRoot(t)
	for _, tc := range []struct {
		path    string
		phrases []string
	}{
		{path: "README.md", phrases: []string{"ASANA_PAT", "takes precedence", "not persisted", "unset ASANA_PAT", "does not verify"}},
		{path: "README.ja.md", phrases: []string{"ASANA_PAT", "優先", "永続保存", "unset ASANA_PAT", "検証しません"}},
	} {
		body, err := os.ReadFile(filepath.Join(root, tc.path))
		if err != nil {
			t.Fatal(err)
		}
		for _, phrase := range tc.phrases {
			if !strings.Contains(string(body), phrase) {
				t.Fatal("PAT guidance is missing from a README")
			}
		}
	}
}

func TestPATUnsetReturnsToSavedOAuthAndPreservesModes(t *testing.T) {
	apiCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		want := "Bearer " + testPAT
		if apiCalls == 2 {
			want = "Bearer saved-access"
		}
		if r.Header.Get("Authorization") != want {
			t.Fatal("unsetting PAT did not return to the saved OAuth token")
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"user-1"}}`))
	}))
	t.Cleanup(api.Close)

	configDir := filepath.Join(t.TempDir(), "config")
	cfgPath := filepath.Join(configDir, "credentials.json")
	if err := config.SaveConfig(cfgPath, config.StoredConfig{ClientID: "saved-client", Token: &config.TokenData{AccessToken: "saved-access", RefreshToken: "saved-refresh"}}); err != nil {
		t.Fatal(err)
	}
	code, _, _ := runPATCLI(t, []string{"me"}, cli.RuntimeOptions{ConfigPath: cfgPath, APIBase: api.URL, PAT: testPAT})
	if code != 0 {
		t.Fatal("PAT path failed before returning to OAuth")
	}
	t.Setenv("ASANA_PAT", "")
	options := cli.NewRuntimeOptionsFromEnv()
	options.ConfigPath = cfgPath
	options.APIBase = api.URL
	code, _, _ = runPATCLI(t, []string{"me"}, options)
	if code != 0 || apiCalls != 2 {
		t.Fatal("unsetting ASANA_PAT did not restore the saved OAuth path")
	}

	configInfo, err := os.Stat(configDir)
	if err != nil || configInfo.Mode().Perm() != 0o700 {
		t.Fatal("config directory mode changed from 0700")
	}
	fileInfo, err := os.Stat(cfgPath)
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatal("credentials file mode changed from 0600")
	}
}

func runPATCLI(t *testing.T, args []string, options cli.RuntimeOptions) (int, string, string) {
	t.Helper()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.RunCLI(args, &cli.CliIO{Out: out, ErrOut: errOut}, options)
	return code, out.String(), errOut.String()
}

type configSnapshot struct {
	bytes   []byte
	modTime time.Time
	mode    os.FileMode
}

func snapshotConfig(t *testing.T, path string) configSnapshot {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return configSnapshot{bytes: b, modTime: info.ModTime(), mode: info.Mode().Perm()}
}

func assertConfigUnchanged(t *testing.T, path string, before configSnapshot) {
	t.Helper()
	after := snapshotConfig(t, path)
	if !bytes.Equal(after.bytes, before.bytes) || !after.modTime.Equal(before.modTime) || after.mode != before.mode {
		t.Fatal("PAT path changed saved OAuth config bytes, mtime, or mode")
	}
}

func assertStatusSource(t *testing.T, format, output, wantSource string, wantAuthenticated bool) map[string]any {
	t.Helper()
	if format == "json" {
		var status map[string]any
		if err := json.Unmarshal([]byte(output), &status); err != nil {
			t.Fatal(err)
		}
		if status["authSource"] != wantSource || status["authenticated"] != wantAuthenticated {
			t.Fatal("JSON status source or authenticated state was incorrect")
		}
		return status
	}
	separator := "	"
	prefix := "authSource	"
	if format == "compact" {
		separator = "="
		prefix = "authSource="
	}
	if !strings.Contains(output, prefix+wantSource) {
		t.Fatal("table or compact status source was incorrect")
	}
	if !strings.Contains(output, "authenticated"+separator+strconv.FormatBool(wantAuthenticated)) {
		t.Fatal("table or compact authenticated state was incorrect")
	}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[len(lines)-1], prefix) {
		t.Fatal("authSource was not appended as the final status field")
	}
	return nil
}

func assertFileDoesNotContain(t *testing.T, path, secret string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), secret) {
		t.Fatal("PAT was persisted to OAuth config")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate the repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
