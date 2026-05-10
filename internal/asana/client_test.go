package asana_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/asana"
)

func TestExchangeCodeForTokenRequestBodyAndMethod(t *testing.T) {
	var capturedMethod, capturedPath, capturedCT, capturedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedCT = r.Header.Get("Content-Type")
		capturedBody = readBody(t, r)
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer ts.Close()

	c := asana.NewClient("", ts.URL)
	if _, err := c.ExchangeCodeForToken(context.Background(), "cid", "secret", "http://localhost/cb", "code123"); err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if capturedMethod != http.MethodPost {
		t.Fatalf("expected POST got %s", capturedMethod)
	}
	if capturedPath != "/" {
		t.Fatalf("path=%s", capturedPath)
	}
	if !strings.Contains(capturedCT, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type=%q", capturedCT)
	}
	if !strings.Contains(capturedBody, "grant_type=authorization_code") {
		t.Fatalf("body=%q", capturedBody)
	}
	if !strings.Contains(capturedBody, "client_secret=secret") {
		t.Fatalf("body=%q", capturedBody)
	}
}

func TestExchangeCodeForTokenDerivesExpiresAt(t *testing.T) {
	baseTime := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"access","token_type":"Bearer","expires_in":10}`))
	}))
	defer ts.Close()

	c := asana.NewClient("", ts.URL, asana.WithClock(func() time.Time { return baseTime }))
	token, err := c.ExchangeCodeForToken(context.Background(), "cid", "secret", "http://localhost/cb", "code123")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	want := baseTime.Add(10 * time.Second).Format(time.RFC3339)
	if token.ExpiresAt != want {
		t.Fatalf("want %s got %s", want, token.ExpiresAt)
	}
}

func TestRefreshAccessTokenRequestBody(t *testing.T) {
	var capturedBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody = readBody(t, r)
		_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"new-refresh","token_type":"Bearer","expires_in":5}`))
	}))
	defer ts.Close()

	c := asana.NewClient("", ts.URL)
	if _, err := c.RefreshAccessToken(context.Background(), "cid", "secret", "rtoken"); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if !strings.Contains(capturedBody, "grant_type=refresh_token") {
		t.Fatalf("body=%q", capturedBody)
	}
	if !strings.Contains(capturedBody, "refresh_token=rtoken") {
		t.Fatalf("body=%q", capturedBody)
	}
}

func TestTokenResponseMissingAccessTokenFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token_type":"Bearer"}`))
	}))
	defer ts.Close()

	c := asana.NewClient("", ts.URL)
	_, err := c.RefreshAccessToken(context.Background(), "cid", "secret", "rtoken")
	if err == nil {
		t.Fatal("expected missing access_token error")
	}
	if !strings.Contains(err.Error(), "token response missing access_token") {
		t.Fatalf("expected error to contain 'token response missing access_token', got: %v", err)
	}
	if !strings.Contains(err.Error(), "{\"token_type\":\"Bearer\"}") {
		t.Fatalf("expected error to contain raw response body, got: %v", err)
	}
}

func TestTokenResponseWithDataWrapperPreservesTopLevelTokens(t *testing.T) {
	baseTime := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"top-level-token","refresh_token":"top-level-refresh","token_type":"Bearer","expires_in":3600,"data":{"id":319233028137921,"gid":"319233028137921","name":"Koichi Tsutsumi","email":"koichi.tsutsumi@gmail.com"}}`))
	}))
	defer ts.Close()

	c := asana.NewClient("", ts.URL, asana.WithClock(func() time.Time { return baseTime }))
	tok, err := c.ExchangeCodeForToken(context.Background(), "cid", "secret", "http://localhost/cb", "code123")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if tok.AccessToken != "top-level-token" {
		t.Fatalf("expected access_token 'top-level-token', got %q", tok.AccessToken)
	}
	if tok.RefreshToken != "top-level-refresh" {
		t.Fatalf("expected refresh_token 'top-level-refresh', got %q", tok.RefreshToken)
	}
	if tok.TokenType != "Bearer" {
		t.Fatalf("expected token_type 'Bearer', got %q", tok.TokenType)
	}
	want := baseTime.Add(3600 * time.Second).Format(time.RFC3339)
	if tok.ExpiresAt != want {
		t.Fatalf("want expires_at %s got %s", want, tok.ExpiresAt)
	}
}

func TestTokenResponseDecodeErrorIncludesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer ts.Close()

	c := asana.NewClient("", ts.URL)
	_, err := c.RefreshAccessToken(context.Background(), "cid", "secret", "rtoken")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "token response decode:") {
		t.Fatalf("expected error to contain 'token response decode:', got: %v", err)
	}
	if !strings.Contains(err.Error(), "not json") {
		t.Fatalf("expected error to contain raw response body 'not json', got: %v", err)
	}
}

func TestAPIAuthorizationHeaderAndPathEscape(t *testing.T) {
	var capturedPath string
	var capturedAuth string
	var capturedQuery string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.EscapedPath()
		capturedAuth = r.Header.Get("Authorization")
		capturedQuery = r.URL.RawQuery

		if strings.HasSuffix(r.URL.EscapedPath(), "/tasks/task%2Fone") {
			_, _ = w.Write([]byte(`{"data":{"gid":"task/one","name":"one"}}`))
			return
		}

		if r.URL.Path == "/projects" {
			payload := map[string]interface{}{"data": []map[string]interface{}{{"gid": "1"}}, "next_page": nil}
			_ = json.NewEncoder(w).Encode(payload)
			return
		}

		t.Fatalf("unexpected path %s", r.URL.Path)
	}))
	defer ts.Close()

	c := asana.NewClient(ts.URL+"/", "")
	if _, err := c.ListProjects(context.Background(), "token", url.Values{"workspace": []string{"ws-1"}}); err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if capturedAuth != "Bearer token" {
		t.Fatalf("authorization=%s", capturedAuth)
	}
	if capturedPath != "/projects" {
		t.Fatalf("path=%s", capturedPath)
	}
	if !strings.Contains(capturedQuery, "workspace=ws-1") {
		t.Fatalf("query=%s", capturedQuery)
	}

	if _, err := c.GetTask(context.Background(), "token", "task/one"); err != nil {
		t.Fatalf("get task: %v", err)
	}
	if capturedPath != "/tasks/task%2Fone" {
		t.Fatalf("escaped path=%s", capturedPath)
	}
}

func TestListTasksPagination(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		switch offset {
		case "":
			_, _ = w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":{"offset":"10"}}`))
		case "10":
			_, _ = w.Write([]byte(`{"data":[{"gid":"2"}]}`))
		default:
			t.Fatalf("unexpected offset %s", offset)
		}
	}))
	defer ts.Close()

	c := asana.NewClient(ts.URL+"/", "")
	items, err := c.ListTasks(context.Background(), "token", nil)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 2 || items[0]["gid"] != "1" || items[1]["gid"] != "2" {
		t.Fatalf("unexpected items %#v", items)
	}
}

func TestListCommentsFiltersAndOptFields(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("opt_fields"); got == "" {
			t.Fatal("opt_fields missing")
		}
		payload := map[string]interface{}{
			"data": []map[string]interface{}{
				{"gid": "1", "resource_subtype": "comment_added", "text": "c1"},
				{"gid": "2", "resource_subtype": "not_comment", "text": "x"},
				{"gid": "3", "resource_subtype": "comment_added", "text": "c2"},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer ts.Close()

	c := asana.NewClient(ts.URL+"/", "")
	comments, err := c.ListComments(context.Background(), "token", "task-1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("len=%d", len(comments))
	}
	if comments[0]["gid"] != "1" || comments[1]["gid"] != "3" {
		t.Fatalf("comments=%#v", comments)
	}
}

func TestAsanaErrorEnvelopeJoinsMessages(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"message":"first"},{"message":"second"}]}`))
	}))
	defer ts.Close()

	c := asana.NewClient(ts.URL+"/", "")
	if _, err := c.FetchMe(context.Background(), "token"); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "first; second") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestListAttachments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/task-1/attachments" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"gid":"a1"}]}`))
	}))
	defer ts.Close()

	c := asana.NewClient(ts.URL+"/", "")
	got, err := c.ListAttachments(context.Background(), "token", "task-1")
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestFetchMe(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"me-1","name":"Me"}}`))
	}))
	defer ts.Close()

	c := asana.NewClient(ts.URL+"/", "")
	got, err := c.FetchMe(context.Background(), "token")
	if err != nil {
		t.Fatalf("fetch me: %v", err)
	}
	if got["gid"] != "me-1" {
		t.Fatalf("got=%#v", got)
	}
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}
