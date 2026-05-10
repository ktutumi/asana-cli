package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ktutumi/asana-cli-go/internal/config"
)

func writeFile(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadConfigMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	got, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.ClientID != "" || got.RedirectURI != "" || got.Token != nil {
		t.Fatalf("expected empty config, got %#v", got)
	}
}

func TestLoadSaveConfigAndMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	existing := config.StoredConfig{
		ClientID:    "existing-client",
		RedirectURI: "https://existing.local/callback",
		Token: &config.TokenData{
			AccessToken:  "old-access",
			RefreshToken: "old-refresh",
			TokenType:    "Bearer",
			ExpiresIn:    10,
			ExpiresAt:    "old-expiry",
		},
	}
	writeFile(t, path, existing)

	next := config.StoredConfig{
		ClientID:    "",
		RedirectURI: "https://new.local/callback",
		Token: &config.TokenData{
			AccessToken:  "",
			RefreshToken: "new-refresh",
			ExpiresIn:    0,
			TokenType:    "",
		},
	}
	if err := config.SaveConfig(path, next); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.ClientID != existing.ClientID {
		t.Fatalf("merge should preserve existing client_id, want %q got %q", existing.ClientID, loaded.ClientID)
	}
	if loaded.RedirectURI != next.RedirectURI {
		t.Fatalf("redirect uri should be updated: want %q got %q", next.RedirectURI, loaded.RedirectURI)
	}
	if loaded.Token == nil {
		t.Fatalf("token should exist")
	}
	if loaded.Token.AccessToken != existing.Token.AccessToken {
		t.Fatalf("access token should be preserved, got %q", loaded.Token.AccessToken)
	}
	if loaded.Token.RefreshToken != next.Token.RefreshToken {
		t.Fatalf("refresh token should be updated, got %q", loaded.Token.RefreshToken)
	}
	if loaded.Token.ExpiresAt != existing.Token.ExpiresAt {
		t.Fatalf("expires_at should be preserved")
	}
}

func TestSaveConfigDoesNotPersistClientSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	incoming := config.StoredConfig{
		ClientID:    "client-id",
		RedirectURI: "http://127.0.0.1:18787/callback",
		Token: &config.TokenData{
			AccessToken: "abc",
		},
	}
	if err := config.SaveConfig(path, incoming); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if strings.Contains(string(raw), "client_secret") {
		t.Fatalf("client_secret leaked to config")
	}
}

func TestLoadConfigSupportsLegacyJSONKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	raw := `{"clientId":"cid","redirectUri":"http://localhost/cb","token":{"access_token":"tok","refresh_token":"rt","token_type":"Bearer","expires_in":100,"expires_at":"2024-01-01T00:00:00Z"}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.ClientID != "cid" || got.RedirectURI != "http://localhost/cb" || got.Token == nil || got.Token.RefreshToken != "rt" {
		t.Fatalf("unexpected parsed config: %#v", got)
	}
}

func TestSaveConfigPermissionsUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission checks skipped on windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "credentials.json")

	err := config.SaveConfig(path, config.StoredConfig{ClientID: "x"})
	if err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	st, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if st.Mode().Perm()&0o777 != 0o700 {
		t.Fatalf("expected dir perm 0700 got %o", st.Mode().Perm()&0o777)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if fi.Mode().Perm()&0o777 != 0o600 {
		t.Fatalf("expected file perm 0600 got %o", fi.Mode().Perm()&0o777)
	}
}
