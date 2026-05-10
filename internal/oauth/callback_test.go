package oauth_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/oauth"
)

func TestCallbackServerSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := oauth.StartCallbackServer(ctx, "127.0.0.1", 0, "/callback")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Shutdown()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host == "127.0.0.1:0" {
		t.Fatalf("expected actual port when using 0")
	}

	res, err := http.Get(srv.URL + "?code=abc&state=xyz")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", res.StatusCode)
	}

	got, err := srv.WaitForCode(context.Background())
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got.Code != "abc" || got.State != "xyz" {
		t.Fatalf("unexpected code/state %#v", got)
	}
}

func TestCallbackServerMissingCodeReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := oauth.StartCallbackServer(ctx, "127.0.0.1", 0, "/callback")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Shutdown()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	res.Body.Close()

	_, err = srv.WaitForCode(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCallbackServerPathMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := oauth.StartCallbackServer(ctx, "127.0.0.1", 0, "/callback")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Shutdown()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u.Path = "/favicon.ico"
	res, err := http.Get(u.String())
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.StatusCode)
	}
	res.Body.Close()

	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal("context canceled")
	}
}

func TestCallbackServerRepeatedStartsKeepCallbackPath(t *testing.T) {
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		srv, err := oauth.StartCallbackServer(ctx, "127.0.0.1", 0, "/callback")
		if err != nil {
			cancel()
			t.Fatalf("start %d: %v", i, err)
		}
		if got, want := srv.URL[len(srv.URL)-len("/callback"):], "/callback"; got != want {
			t.Fatalf("start %d URL=%s", i, srv.URL)
		}
		srv.Shutdown()
		cancel()
	}
}

func TestCallbackServerCancelCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	srv, err := oauth.StartCallbackServer(ctx, "127.0.0.1", 0, "/callback")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()
	if _, err := srv.WaitForCode(ctx); err == nil {
		t.Fatal("expected context canceled")
	}
}
