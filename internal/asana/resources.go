package asana

import (
	"context"
	"net/http"
	"net/url"
)

// GetObject fetches a single Asana resource wrapped in the standard data envelope.
func (c *Client) GetObject(ctx context.Context, token, path string, q url.Values) (Object, error) {
	var env struct {
		Data Object `json:"data"`
	}
	err := c.doRequestJSON(ctx, token, http.MethodGet, path, q, nil, &env)
	return env.Data, err
}

// ListObjects fetches and paginates an Asana collection.
func (c *Client) ListObjects(ctx context.Context, token, path string, q url.Values) ([]Object, error) {
	var all []Object
	qq := cloneValues(q)
	for {
		var env struct {
			Data     []Object `json:"data"`
			NextPage *struct {
				Offset string `json:"offset"`
			} `json:"next_page"`
		}
		if err := c.doRequestJSON(ctx, token, http.MethodGet, path, qq, nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if env.NextPage == nil || env.NextPage.Offset == "" {
			return all, nil
		}
		qq.Set("offset", env.NextPage.Offset)
	}
}

// SearchObjects fetches one search page. Asana search does not support the
// offset pagination used by ordinary collection endpoints.
func (c *Client) SearchObjects(ctx context.Context, token, path string, q url.Values) ([]Object, error) {
	var env struct {
		Data []Object `json:"data"`
	}
	if err := c.doRequestJSON(ctx, token, http.MethodGet, path, q, nil, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// WriteObject sends a data-enveloped POST or PUT and returns the response object.
func (c *Client) WriteObject(ctx context.Context, token, method, path string, q url.Values, data Object) (Object, error) {
	if method != http.MethodPost && method != http.MethodPut {
		return nil, &MethodError{Method: method}
	}
	var env struct {
		Data Object `json:"data"`
	}
	err := c.doRequestJSON(ctx, token, method, path, q, data, &env)
	return env.Data, err
}

// DeleteObject deletes a resource. Both 200 empty-data and 204 responses are accepted.
func (c *Client) DeleteObject(ctx context.Context, token, path string) error {
	var env struct {
		Data any `json:"data"`
	}
	return c.doRequestJSON(ctx, token, http.MethodDelete, path, nil, nil, &env)
}

type MethodError struct{ Method string }

func (e *MethodError) Error() string { return "unsupported write method: " + e.Method }

func cloneValues(q url.Values) url.Values {
	out := url.Values{}
	for key, values := range q {
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}
