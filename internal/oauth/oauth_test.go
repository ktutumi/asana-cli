package oauth_test

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/ktutumi/asana-cli-go/internal/oauth"
)

func TestAuthorizationURLFullMatch(t *testing.T) {
	got, err := oauth.BuildAuthorizationURL("client-1", "http://localhost/cb", "fixed", []string{"users:read", "tasks:read"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "app.asana.com" || parsed.Path != "/-/oauth_authorize" {
		t.Fatalf("url=%s", got)
	}
	q := parsed.Query()
	if q.Get("client_id") != "client-1" {
		t.Fatalf("%#v", q)
	}
	if q.Get("response_type") != "code" {
		t.Fatalf("%#v", q)
	}
	if q.Get("redirect_uri") != "http://localhost/cb" {
		t.Fatalf("%#v", q)
	}
	if q.Get("scope") != "users:read tasks:read" {
		t.Fatalf("%#v", q.Get("scope"))
	}
	if q.Get("state") != "fixed" {
		t.Fatalf("%#v", q)
	}
}

func TestBuildAuthorizationURLUsesPercent20ForSpace(t *testing.T) {
	got, err := oauth.BuildAuthorizationURL("client-1", "http://localhost/cb", "state", []string{"a b", "c d"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if strings.Contains(got, "+") {
		t.Fatalf("URL should not contain +: %s", got)
	}
	if !strings.Contains(got, "%20") {
		t.Fatalf("expected %%20 in %s", got)
	}
}

func TestGenerateStateLengthAndAlphabet(t *testing.T) {
	got, err := oauth.GenerateState()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(got) != 43 {
		t.Fatalf("len=%d", len(got))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(got) {
		t.Fatalf("invalid state chars: %q", got)
	}
}

func TestDefaultScopes(t *testing.T) {
	want := []string{"default"}
	got := oauth.DefaultOAuthScopes
	if len(got) != len(want) {
		t.Fatalf("len=%d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes mismatch at %d: want %q got %q", i, want[i], got[i])
		}
	}
}

func TestDefaultRedirectURI(t *testing.T) {
	got, err := oauth.BuildAuthorizationURL("client", "", "state", []string{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Query().Get("redirect_uri") != oauth.DefaultRedirectURI {
		t.Fatalf("redirect=%q", parsed.Query().Get("redirect_uri"))
	}
}

func TestRedirectURIValidation(t *testing.T) {
	cases := []struct {
		uri    string
		expect bool
	}{
		{"http://127.0.0.1:18787/callback", true},
		{"http://localhost/callback", true},
		{"https://127.0.0.1/callback", false},
		{"http://example.com/callback", false},
		{"http://localhost/callback?x=1", false},
		{"http://localhost/callback#x", false},
	}

	for _, tc := range cases {
		if err := oauth.ValidateRedirectURI(tc.uri); (err == nil) != tc.expect {
			t.Fatalf("uri=%s err=%v", tc.uri, err)
		}
	}
}
