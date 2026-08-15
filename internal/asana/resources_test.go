package asana_test

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ktutumi/asana-cli-go/internal/asana"
)

func TestWriteObjectUsesMethodEnvelopeAndHeaders(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Fatalf("method=%s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("content-type=%q", r.Header.Get("Content-Type"))
				}
				var body map[string]asana.Object
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["data"]["name"] != "Example" {
					t.Fatalf("body=%#v", body)
				}
				_, _ = w.Write([]byte(`{"data":{"gid":"1","name":"Example"}}`))
			}))
			defer server.Close()

			client := asana.NewClient(server.URL, "")
			got, err := client.WriteObject(context.Background(), "test-token", method, "tasks/1", nil, asana.Object{"name": "Example"})
			if err != nil {
				t.Fatal(err)
			}
			if got["gid"] != "1" {
				t.Fatalf("got=%#v", got)
			}
		})
	}
}

func TestDeleteObjectAcceptsEmptyDataAndNoContent(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Fatalf("method=%s", r.Method)
				}
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(`{"data":{}}`))
				}
			}))
			defer server.Close()
			if err := asana.NewClient(server.URL, "").DeleteObject(context.Background(), "token", "tasks/1"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestListObjectsPaginatesWithoutMutatingQuery(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			_, _ = w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":{"offset":"next"}}`))
			return
		}
		if r.URL.Query().Get("offset") != "next" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[{"gid":"2"}]}`))
	}))
	defer server.Close()
	q := url.Values{"opt_fields": {"gid"}}
	items, err := asana.NewClient(server.URL, "").ListObjects(context.Background(), "token", "tasks", q)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || q.Get("offset") != "" {
		t.Fatalf("items=%#v query=%s", items, q.Encode())
	}
}

func TestSearchObjectsDoesNotFollowOffsetPagination(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"data":[{"gid":"1"}],"next_page":{"offset":"must-not-be-followed"}}`))
	}))
	defer server.Close()
	items, err := asana.NewClient(server.URL, "").SearchObjects(context.Background(), "token", "workspaces/w/tasks/search", url.Values{"limit": {"100"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || requests != 1 {
		t.Fatalf("items=%#v requests=%d", items, requests)
	}
}

func TestUploadAttachmentMultipartPreservesNonASCIIFileName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("parent") != "parent-1" {
			t.Fatalf("parent=%q", r.FormValue("parent"))
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if header.Filename != "資料.txt" {
			t.Fatalf("filename=%q", header.Filename)
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"attachment-1"}}`))
	}))
	defer server.Close()
	client := asana.NewClient(server.URL, "")
	got, err := client.UploadAttachment(context.Background(), "token", asana.AttachmentUpload{Parent: "parent-1", FileName: "資料.txt", File: strings.NewReader("safe test data"), Size: 14})
	if err != nil {
		t.Fatal(err)
	}
	if got["gid"] != "attachment-1" {
		t.Fatalf("got=%#v", got)
	}
}

func TestUploadAttachmentExternalAndSizeValidation(t *testing.T) {
	var values map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		values = readMultipartValues(t, reader)
		_, _ = w.Write([]byte(`{"data":{"gid":"external-1"}}`))
	}))
	defer server.Close()
	client := asana.NewClient(server.URL, "")
	_, err := client.UploadAttachment(context.Background(), "token", asana.AttachmentUpload{Parent: "p", URL: "https://example.com/file", Name: "Link", ConnectToApp: true})
	if err != nil {
		t.Fatal(err)
	}
	if values["resource_subtype"][0] != "external" || values["connect_to_app"][0] != "true" {
		t.Fatalf("values=%#v", values)
	}
	_, err = client.UploadAttachment(context.Background(), "token", asana.AttachmentUpload{Parent: "p", FileName: "big", File: strings.NewReader("x"), Size: asana.MaxAttachmentSize + 1})
	if err == nil || !strings.Contains(err.Error(), "100MB") {
		t.Fatalf("err=%v", err)
	}
}

func readMultipartValues(t *testing.T, reader *multipart.Reader) map[string][]string {
	t.Helper()
	values := map[string][]string{}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		data := make([]byte, 1024)
		n, _ := part.Read(data)
		values[part.FormName()] = append(values[part.FormName()], string(data[:n]))
		_ = part.Close()
	}
	return values
}
