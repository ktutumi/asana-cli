package asana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ktutumi/asana-cli-go/internal/config"
)

type Client struct {
	APIBase       string
	TokenEndpoint string
	HTTPClient    *http.Client
	Now           func() time.Time
}

type Option func(*Client)

func WithClock(now func() time.Time) Option { return func(c *Client) { c.Now = now } }

func NewClient(apiBase, tokenEndpoint string, opts ...Option) *Client {
	if apiBase == "" {
		apiBase = "https://app.asana.com/api/1.0/"
	}
	if tokenEndpoint == "" {
		tokenEndpoint = "https://app.asana.com/-/oauth_token"
	}
	if !strings.HasSuffix(apiBase, "/") {
		apiBase += "/"
	}
	c := &Client{APIBase: apiBase, TokenEndpoint: tokenEndpoint, HTTPClient: http.DefaultClient, Now: time.Now}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) ExchangeCodeForToken(_ context.Context, clientID, clientSecret, redirectURI, code string) (config.TokenData, error) {
	v := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "client_secret": {clientSecret}, "redirect_uri": {redirectURI}, "code": {code}}
	return c.postToken(v)
}
func (c *Client) RefreshAccessToken(_ context.Context, clientID, clientSecret, refreshToken string) (config.TokenData, error) {
	v := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "client_secret": {clientSecret}, "refresh_token": {refreshToken}}
	return c.postToken(v)
}
func (c *Client) postToken(v url.Values) (config.TokenData, error) {
	req, err := http.NewRequest(http.MethodPost, c.TokenEndpoint, strings.NewReader(v.Encode()))
	if err != nil {
		return config.TokenData{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return config.TokenData{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return config.TokenData{}, decodeError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return config.TokenData{}, err
	}
	var raw struct {
		config.TokenData
		Data *config.TokenData `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return config.TokenData{}, fmt.Errorf("token response decode: %w (body: %s)", err, string(bytes.TrimSpace(body)))
	}
	tok := raw.TokenData
	if raw.Data != nil {
		if tok.AccessToken == "" {
			tok.AccessToken = raw.Data.AccessToken
		}
		if tok.RefreshToken == "" {
			tok.RefreshToken = raw.Data.RefreshToken
		}
		if tok.TokenType == "" {
			tok.TokenType = raw.Data.TokenType
		}
		if tok.ExpiresIn == 0 {
			tok.ExpiresIn = raw.Data.ExpiresIn
		}
		if tok.ExpiresAt == "" {
			tok.ExpiresAt = raw.Data.ExpiresAt
		}
	}
	if tok.AccessToken == "" {
		return config.TokenData{}, fmt.Errorf("token response missing access_token (body: %s)", string(bytes.TrimSpace(body)))
	}
	if tok.ExpiresIn > 0 && tok.ExpiresAt == "" {
		tok.ExpiresAt = c.now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second).Format(time.RFC3339)
	}
	return tok, nil
}

type Object map[string]any

func (c *Client) FetchMe(_ context.Context, token string) (Object, error) {
	var out Object
	return out, c.getOne(token, "users/me", nil, &out)
}
func (c *Client) ListWorkspaces(_ context.Context, token string) ([]Object, error) {
	return c.getList(token, "workspaces", nil)
}
func (c *Client) ListProjects(_ context.Context, token string, q url.Values) ([]Object, error) {
	return c.getList(token, "projects", q)
}
func (c *Client) ListTasks(_ context.Context, token string, q url.Values) ([]Object, error) {
	return c.getList(token, "tasks", q)
}
func (c *Client) GetTask(_ context.Context, token, gid string) (Object, error) {
	var out Object
	return out, c.getOne(token, "tasks/"+url.PathEscape(gid), nil, &out)
}
func (c *Client) ListSubtasks(_ context.Context, token, gid string) ([]Object, error) {
	return c.getList(token, "tasks/"+url.PathEscape(gid)+"/subtasks", nil)
}
func (c *Client) ListStories(_ context.Context, token, gid string) ([]Object, error) {
	return c.getList(token, "tasks/"+url.PathEscape(gid)+"/stories", nil)
}
func (c *Client) ListAttachments(_ context.Context, token, gid string) ([]Object, error) {
	return c.getList(token, "tasks/"+url.PathEscape(gid)+"/attachments", nil)
}
func (c *Client) ListComments(_ context.Context, token, gid string) ([]Object, error) {
	q := url.Values{}
	q.Set("opt_fields", "gid,resource_subtype,resource_type,text,html_text,created_at,created_by.name")
	items, err := c.getList(token, "tasks/"+url.PathEscape(gid)+"/stories", q)
	if err != nil {
		return nil, err
	}
	out := []Object{}
	for _, it := range items {
		if s, _ := it["resource_subtype"].(string); s == "comment_added" {
			out = append(out, it)
		}
	}
	return out, nil
}

func (c *Client) getOne(token, path string, q url.Values, out *Object) error {
	var env struct {
		Data Object `json:"data"`
	}
	if err := c.doJSON(token, path, q, &env); err != nil {
		return err
	}
	*out = env.Data
	return nil
}
func (c *Client) getList(token, path string, q url.Values) ([]Object, error) {
	var all []Object
	qq := url.Values{}
	for k, vs := range q {
		for _, v := range vs {
			qq.Add(k, v)
		}
	}
	for {
		var env struct {
			Data     []Object `json:"data"`
			NextPage *struct {
				Offset string `json:"offset"`
			} `json:"next_page"`
		}
		if err := c.doJSON(token, path, qq, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if env.NextPage == nil || env.NextPage.Offset == "" {
			break
		}
		qq.Set("offset", env.NextPage.Offset)
	}
	return all, nil
}
func (c *Client) doJSON(token, path string, q url.Values, out any) error {
	u, err := url.Parse(c.APIBase + path)
	if err != nil {
		return err
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(b, &env) == nil && len(env.Errors) > 0 {
		parts := []string{}
		for _, e := range env.Errors {
			if e.Message != "" {
				parts = append(parts, e.Message)
			}
		}
		if len(parts) > 0 {
			return fmt.Errorf("asana error: %s", strings.Join(parts, "; "))
		}
	}
	return fmt.Errorf("http %d: %s", resp.StatusCode, string(bytes.TrimSpace(b)))
}
