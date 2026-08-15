package asana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
)

const MaxAttachmentSize int64 = 100 * 1024 * 1024

type AttachmentUpload struct {
	Parent       string
	FileName     string
	File         io.Reader
	Size         int64
	URL          string
	Name         string
	ConnectToApp bool
}

func (c *Client) UploadAttachment(ctx context.Context, token string, input AttachmentUpload) (Object, error) {
	if input.Parent == "" {
		return nil, fmt.Errorf("attachment parent is required")
	}
	if input.File != nil && input.URL != "" {
		return nil, fmt.Errorf("attachment file and URL are mutually exclusive")
	}
	if input.File != nil && input.Size > MaxAttachmentSize {
		return nil, fmt.Errorf("attachment exceeds the 100MB limit")
	}
	if input.File == nil && input.URL == "" {
		return nil, fmt.Errorf("attachment file or URL is required")
	}
	if input.URL != "" && input.Name == "" {
		return nil, fmt.Errorf("external attachment name is required")
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("parent", input.Parent); err != nil {
		return nil, err
	}
	if input.File != nil {
		part, err := w.CreateFormFile("file", filepath.Base(input.FileName))
		if err != nil {
			return nil, err
		}
		written, err := io.Copy(part, io.LimitReader(input.File, MaxAttachmentSize+1))
		if err != nil {
			return nil, err
		}
		if written > MaxAttachmentSize {
			return nil, fmt.Errorf("attachment exceeds the 100MB limit")
		}
	} else {
		if err := w.WriteField("resource_subtype", "external"); err != nil {
			return nil, err
		}
		if err := w.WriteField("url", input.URL); err != nil {
			return nil, err
		}
		if err := w.WriteField("name", input.Name); err != nil {
			return nil, err
		}
		if input.ConnectToApp {
			if err := w.WriteField("connect_to_app", "true"); err != nil {
				return nil, err
			}
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	u, err := url.Parse(c.APIBase + "attachments")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeError(resp)
	}
	var env struct {
		Data Object `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return env.Data, nil
}
