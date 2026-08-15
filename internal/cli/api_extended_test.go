package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ktutumi/asana-cli-go/internal/cli"
)

func TestTaskCreateAndUpdateRequests(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		method     string
		escaped    string
		wantFields map[string]any
	}{
		{
			name:       "create",
			args:       []string{"tasks", "create", "--workspace", "w1", "--name", "New task", "--completed", "false", "--custom-field", "cf1=3", "--custom-field-json", `cf2=["option-1","option-2"]`},
			method:     http.MethodPost,
			escaped:    "/tasks",
			wantFields: map[string]any{"workspace": "w1", "name": "New task", "completed": false, "custom_fields": map[string]any{"cf1": "3", "cf2": []any{"option-1", "option-2"}}},
		},
		{
			name:       "update only specified fields",
			args:       []string{"tasks", "update", "task/1", "--due-on", "2026-08-10"},
			method:     http.MethodPut,
			escaped:    "/tasks/task%2F1",
			wantFields: map[string]any{"due_on": "2026-08-10"},
		},
		{
			name:    "create with membership only",
			args:    []string{"tasks", "create", "--name", "Section task", "--membership", "p1=s1"},
			method:  http.MethodPost,
			escaped: "/tasks",
			wantFields: map[string]any{
				"name":        "Section task",
				"projects":    []any{"p1"},
				"memberships": []any{map[string]any{"project": "p1", "section": "s1"}},
			},
		},
		{
			name:    "create with workspace and membership",
			args:    []string{"tasks", "create", "--workspace", "w1", "--name", "Workspace task", "--membership", "p1=s1"},
			method:  http.MethodPost,
			escaped: "/tasks",
			wantFields: map[string]any{
				"workspace":   "w1",
				"name":        "Workspace task",
				"memberships": []any{map[string]any{"project": "p1", "section": "s1"}},
			},
		},
		{
			name:    "create with parent and membership",
			args:    []string{"tasks", "create", "--parent", "t0", "--name", "Child task", "--membership", "p1=s1"},
			method:  http.MethodPost,
			escaped: "/tasks",
			wantFields: map[string]any{
				"parent":      "t0",
				"name":        "Child task",
				"memberships": []any{map[string]any{"project": "p1", "section": "s1"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.EscapedPath() != tt.escaped {
					t.Fatalf("request=%s %s", r.Method, r.URL.EscapedPath())
				}
				var envelope map[string]map[string]any
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
					t.Fatal(err)
				}
				if got, _ := json.Marshal(envelope["data"]); string(got) != mustJSON(t, tt.wantFields) {
					t.Fatalf("data=%s want=%s", got, mustJSON(t, tt.wantFields))
				}
				_, _ = w.Write([]byte(`{"data":{"gid":"task-1","name":"New task"}}`))
			}))
			defer server.Close()
			out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
			code := cli.RunCLI(tt.args, &cli.CliIO{Out: out, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
		})
	}
}

func TestTaskAndSectionValidation(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--notes", "a", "--html-notes", "b"}, "mutually exclusive"},
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--due-on", "tomorrow"}, "YYYY-MM-DD"},
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--start-on", "2026-08-10"}, "requires --due-on"},
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--custom-field", "invalid"}, "GID=VALUE"},
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--custom-field-json", "cf=invalid"}, "valid JSON"},
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--custom-field", "cf=1", "--custom-field-json", "cf=1"}, "specified by both"},
		{[]string{"tasks", "create", "--workspace", "w", "--name", "n", "--insert-before", "t"}, "not applicable"},
		{[]string{"tasks", "create-subtask", "t", "--name", "n", "--workspace", "w"}, "not supported"},
		{[]string{"tasks", "create-subtask", "t", "--name", "n", "--parent", "p"}, "not supported"},
		{[]string{"tasks", "unset-parent", "t", "--insert-before", "a"}, "not applicable"},
		{[]string{"tasks", "remove-project", "t", "--project", "p", "--section", "s"}, "not applicable"},
		{[]string{"tasks", "set-parent", "t", "--parent", "p", "--insert-before", "a", "--insert-after", "b"}, "mutually exclusive"},
		{[]string{"tasks", "list", "--project", "p", "--section", "s"}, "exactly one"},
		{[]string{"tasks", "search", "--workspace", "w", "--limit", "101"}, "1 to 100"},
		{[]string{"sections", "move", "s", "--project", "p", "--before-section", "a", "--after-section", "b"}, "mutually exclusive"},
	}
	for _, tt := range tests {
		errOut := &bytes.Buffer{}
		code := cli.RunCLI(tt.args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: "http://127.0.0.1:1/"})
		if code != 1 || !strings.Contains(errOut.String(), tt.want) {
			t.Errorf("args=%v code=%d err=%q", tt.args, code, errOut.String())
		}
	}
}

func TestRelationshipCommentMembershipAndDeleteRoutes(t *testing.T) {
	tests := []struct {
		args   []string
		method string
		path   string
		status int
	}{
		{[]string{"tasks", "add-dependencies", "t1", "--dependency", "d1", "--dependency", "d2"}, http.MethodPost, "/tasks/t1/addDependencies", 200},
		{[]string{"tasks", "add-project", "t1", "--project", "p1", "--section", "s1"}, http.MethodPost, "/tasks/t1/addProject", 200},
		{[]string{"tasks", "comment", "t1", "--text", "hello"}, http.MethodPost, "/tasks/t1/stories", 200},
		{[]string{"stories", "update", "story1", "--text", "edited"}, http.MethodPut, "/stories/story1", 200},
		{[]string{"memberships", "create", "--parent", "p1", "--member", "u1", "--access-level", "editor"}, http.MethodPost, "/memberships", 200},
		{[]string{"sections", "delete", "s1"}, http.MethodDelete, "/sections/s1", 204},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args[:2], " "), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path {
					t.Fatalf("request=%s %s", r.Method, r.URL.Path)
				}
				if tt.status == http.StatusNoContent {
					w.WriteHeader(tt.status)
					return
				}
				_, _ = w.Write([]byte(`{"data":{"gid":"result-1"}}`))
			}))
			defer server.Close()
			errOut := &bytes.Buffer{}
			code := cli.RunCLI(tt.args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
		})
	}
}

func TestTaskSearchAndCustomIDEscape(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			if r.URL.Path != "/workspaces/w1/tasks/search" || r.URL.Query().Get("projects.any") != "p1,p2" || r.URL.Query().Get("sort_by") != "due_date" {
				t.Fatalf("path=%s query=%s", r.URL.Path, r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		} else if r.URL.EscapedPath() != "/workspaces/w1/tasks/custom_id/ABC%2F42" {
			t.Fatalf("escaped path=%s", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"task-1"}}`))
	}))
	defer server.Close()
	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"}
	for _, args := range [][]string{
		{"tasks", "search", "--workspace", "w1", "--projects-any", "p1,p2", "--sort-by", "due_date"},
		{"tasks", "get-custom-id", "ABC/42", "--workspace", "w1"},
	} {
		errOut := &bytes.Buffer{}
		if code := cli.RunCLI(args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, opts); code != 0 {
			t.Fatalf("args=%v err=%s", args, errOut.String())
		}
	}
}

func TestProjectTaskCountsRequestsExplicitFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/p1/task_counts" || !strings.Contains(r.URL.Query().Get("opt_fields"), "num_incomplete_tasks") {
			t.Fatalf("path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":{"num_tasks":4,"num_incomplete_tasks":2}}`))
	}))
	defer server.Close()
	errOut := &bytes.Buffer{}
	code := cli.RunCLI([]string{"projects", "task-counts", "p1"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
}

func TestProjectFollowersUseCommaSeparatedRequestField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/p1/addFollowers" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var envelope map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if envelope["data"]["followers"] != "u1,u2" {
			t.Fatalf("body=%#v", envelope)
		}
		_, _ = w.Write([]byte(`{"data":{"gid":"p1"}}`))
	}))
	defer server.Close()
	errOut := &bytes.Buffer{}
	code := cli.RunCLI([]string{"projects", "add-followers", "p1", "--follower", "u1", "--follower", "u2"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
}

func TestJobsGetDisplaysSuccessAndFailure(t *testing.T) {
	for _, status := range []string{"succeeded", "failed"} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/jobs/job-1" {
					t.Fatalf("path=%s", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"data":{"gid":"job-1","status":"` + status + `"}}`))
			}))
			defer server.Close()
			out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
			code := cli.RunCLI([]string{"jobs", "get", "job-1"}, &cli.CliIO{Out: out, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
			if code != 0 || !strings.Contains(out.String(), status) {
				t.Fatalf("code=%d out=%s err=%s", code, out.String(), errOut.String())
			}
		})
	}
}

func TestAttachmentGetDeleteAndAsanaError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/attachments/attachment%2F1" && r.URL.EscapedPath() != "/attachments/attachment%2F1" {
			t.Fatalf("path=%s escaped=%s", r.URL.Path, r.URL.EscapedPath())
		}
		if requests == 1 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"message":"attachment not found"}]}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	opts := cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL}
	errOut := &bytes.Buffer{}
	if code := cli.RunCLI([]string{"attachments", "get", "attachment/1"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, opts); code != 1 || !strings.Contains(errOut.String(), "attachment not found") {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	errOut.Reset()
	if code := cli.RunCLI([]string{"attachments", "delete", "attachment/1"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, opts); code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
}

func TestAttachmentUploadCLI(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(filePath, []byte("report body"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want map[string]string
	}{
		{"file", []string{"attachments", "upload", "--parent", "task-1", "--file", filePath}, map[string]string{"parent": "task-1", "file": "report body"}},
		{"external URL", []string{"attachments", "upload", "--parent", "task-1", "--url", "https://example.com/report", "--name", "Report", "--connect-to-app"}, map[string]string{"parent": "task-1", "url": "https://example.com/report", "name": "Report", "resource_subtype": "external", "connect_to_app": "true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/attachments" {
					t.Fatalf("request=%s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				for key, want := range tt.want {
					if key == "file" {
						file, _, err := r.FormFile("file")
						if err != nil {
							t.Fatal(err)
						}
						defer file.Close()
						got, err := io.ReadAll(file)
						if err != nil {
							t.Fatal(err)
						}
						if string(got) != want {
							t.Fatalf("file=%q want=%q", got, want)
						}
						continue
					}
					if got := r.FormValue(key); got != want {
						t.Fatalf("%s=%q want=%q", key, got, want)
					}
				}
				_, _ = w.Write([]byte(`{"data":{"gid":"attachment-1","name":"Report"}}`))
			}))
			defer server.Close()

			errOut := &bytes.Buffer{}
			code := cli.RunCLI(tt.args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
		})
	}
}

func TestAttachmentListPreservesLegacyTableColumns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"gid":"a1","name":"Report","resource_subtype":"asana","download_url":"https://example.com/a1","created_at":"2026-08-10T00:00:00Z"}]}`))
	}))
	defer server.Close()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.RunCLI([]string{"attachments", "list", "task-1"}, &cli.CliIO{Out: out, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "table"})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	if firstLine, _, _ := strings.Cut(out.String(), "\n"); firstLine != "gid\tname\tdownload_url\tcreated_at" {
		t.Fatalf("header=%q", firstLine)
	}
}

func TestListCommandsRequestDefaultOptFieldsForTable(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		path    string
		want    string
		jsonOut string
	}{
		{
			name:    "projects list",
			args:    []string{"projects", "list", "w1"},
			path:    "/projects",
			want:    "gid,name,workspace.name",
			jsonOut: `{"data":[{"gid":"p1","name":"Launch"}]}`,
		},
		{
			name:    "workspaces projects",
			args:    []string{"workspaces", "projects", "w1"},
			path:    "/workspaces/w1/projects",
			want:    "gid,name,workspace.name",
			jsonOut: `{"data":[{"gid":"p1","name":"Launch"}]}`,
		},
		{
			name:    "teams projects",
			args:    []string{"teams", "projects", "team1"},
			path:    "/teams/team1/projects",
			want:    "gid,name,workspace.name",
			jsonOut: `{"data":[{"gid":"p1","name":"Launch"}]}`,
		},
		{
			name:    "tasks list",
			args:    []string{"tasks", "list", "--project", "p1"},
			path:    "/projects/p1/tasks",
			want:    "gid,name,completed,created_at,modified_at",
			jsonOut: `{"data":[{"gid":"t1","name":"Task"}]}`,
		},
		{
			name:    "sections tasks",
			args:    []string{"sections", "tasks", "s1"},
			path:    "/sections/s1/tasks",
			want:    "gid,name,completed,created_at,modified_at",
			jsonOut: `{"data":[{"gid":"t1","name":"Task"}]}`,
		},
		{
			name:    "projects tasks",
			args:    []string{"projects", "tasks", "p1"},
			path:    "/projects/p1/tasks",
			want:    "gid,name,completed,created_at,modified_at",
			jsonOut: `{"data":[{"gid":"t1","name":"Task"}]}`,
		},
		{
			name:    "tasks projects",
			args:    []string{"tasks", "projects", "t1"},
			path:    "/tasks/t1/projects",
			want:    "gid,name,workspace.name",
			jsonOut: `{"data":[{"gid":"p1","name":"Launch"}]}`,
		},
		{
			name:    "tasks subtasks",
			args:    []string{"tasks", "subtasks", "t1"},
			path:    "/tasks/t1/subtasks",
			want:    "gid,name,completed",
			jsonOut: `{"data":[{"gid":"st1","name":"Sub"}]}`,
		},
		{
			name:    "attachments list",
			args:    []string{"attachments", "list", "t1"},
			path:    "/attachments",
			want:    "gid,name,download_url,created_at",
			jsonOut: `{"data":[{"gid":"a1","name":"Report"}]}`,
		},
		{
			name:    "tasks attachments",
			args:    []string{"tasks", "attachments", "t1"},
			path:    "/attachments",
			want:    "gid,name,download_url,created_at",
			jsonOut: `{"data":[{"gid":"a1","name":"Report"}]}`,
		},
		{
			name:    "tasks search",
			args:    []string{"tasks", "search", "--workspace", "w1", "--text", "Launch"},
			path:    "/workspaces/w1/tasks/search",
			want:    "gid,name,completed,created_at,modified_at",
			jsonOut: `{"data":[{"gid":"t1","name":"Task"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotFields string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != tt.path {
					t.Fatalf("path=%s", r.URL.EscapedPath())
				}
				gotFields = r.URL.Query().Get("opt_fields")
				_, _ = w.Write([]byte(tt.jsonOut))
			}))
			defer server.Close()
			errOut := &bytes.Buffer{}
			code := cli.RunCLI(tt.args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "table"})
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
			if gotFields != tt.want {
				t.Fatalf("opt_fields=%q want=%q", gotFields, tt.want)
			}
		})
	}
}

func TestListCommandsOmitDefaultOptFieldsForJSON(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{name: "projects list", args: []string{"projects", "list", "w1"}, path: "/projects"},
		{name: "tasks list", args: []string{"tasks", "list", "--project", "p1"}, path: "/projects/p1/tasks"},
		{name: "tasks subtasks", args: []string{"tasks", "subtasks", "t1"}, path: "/tasks/t1/subtasks"},
		{name: "attachments list", args: []string{"attachments", "list", "t1"}, path: "/attachments"},
		{name: "tasks attachments", args: []string{"tasks", "attachments", "t1"}, path: "/attachments"},
		{name: "workspaces projects", args: []string{"workspaces", "projects", "w1"}, path: "/workspaces/w1/projects"},
		{name: "tasks projects", args: []string{"tasks", "projects", "t1"}, path: "/tasks/t1/projects"},
		{name: "tasks search", args: []string{"tasks", "search", "--workspace", "w1"}, path: "/workspaces/w1/tasks/search"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotFields string
			var saw bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != tt.path {
					t.Fatalf("path=%s", r.URL.EscapedPath())
				}
				saw = true
				gotFields = r.URL.Query().Get("opt_fields")
				_, _ = w.Write([]byte(`{"data":[]}`))
			}))
			defer server.Close()
			errOut := &bytes.Buffer{}
			code := cli.RunCLI(tt.args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
			if !saw {
				t.Fatal("request was not made")
			}
			if gotFields != "" {
				t.Fatalf("opt_fields=%q want empty", gotFields)
			}
		})
	}
}

func TestListCommandsMergeOptFieldsForTable(t *testing.T) {
	var gotFields string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/projects/p1/tasks" {
			t.Fatalf("path=%s", r.URL.EscapedPath())
		}
		gotFields = r.URL.Query().Get("opt_fields")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	errOut := &bytes.Buffer{}
	code := cli.RunCLI([]string{"tasks", "list", "--project", "p1", "--opt-fields", "assignee.name"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "table"})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	want := "gid,name,completed,created_at,modified_at,assignee.name"
	if gotFields != want {
		t.Fatalf("opt_fields=%q want=%q", gotFields, want)
	}
}

func TestListCommandsKeepExplicitOptFieldsForJSON(t *testing.T) {
	var gotFields string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/projects/p1/tasks" {
			t.Fatalf("path=%s", r.URL.EscapedPath())
		}
		gotFields = r.URL.Query().Get("opt_fields")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	errOut := &bytes.Buffer{}
	code := cli.RunCLI([]string{"tasks", "list", "--project", "p1", "--opt-fields", "assignee.name"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errOut.String())
	}
	if gotFields != "assignee.name" {
		t.Fatalf("opt_fields=%q want=%q", gotFields, "assignee.name")
	}
}

func TestStoryErrorsAreReported(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"errors":[{"message":"story unavailable"}]}`))
			}))
			defer server.Close()
			errOut := &bytes.Buffer{}
			code := cli.RunCLI([]string{"stories", "get", "story-1"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL})
			if code != 1 || !strings.Contains(errOut.String(), "story unavailable") {
				t.Fatalf("code=%d err=%q", code, errOut.String())
			}
		})
	}
}

func TestMultiTagFailureReportsPartialApplication(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			_, _ = w.Write([]byte(`{"data":{"gid":"task-1"}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"message":"tag rejected"}]}`))
	}))
	defer server.Close()
	errOut := &bytes.Buffer{}
	code := cli.RunCLI([]string{"tasks", "add-tag", "task-1", "--tag", "tag-1", "--tag", "tag-2"}, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL})
	if code != 1 || !strings.Contains(errOut.String(), "tag-2 after applying 1 of 2 tags") {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
}

func TestExtendedCommandEndpointMatrix(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		method     string
		path       string
		collection bool
		noContent  bool
	}{
		{"create subtask", []string{"tasks", "create-subtask", "parent/1", "--name", "Subtask"}, http.MethodPost, "/tasks/parent%2F1/subtasks", false, false},
		{"set parent", []string{"tasks", "set-parent", "task1", "--parent", "parent1"}, http.MethodPost, "/tasks/task1/setParent", false, false},
		{"unset parent", []string{"tasks", "unset-parent", "task1"}, http.MethodPost, "/tasks/task1/setParent", false, false},
		{"section create", []string{"sections", "create", "--project", "p1", "--name", "Doing"}, http.MethodPost, "/projects/p1/sections", false, false},
		{"section update", []string{"sections", "update", "s1", "--name", "Done"}, http.MethodPut, "/sections/s1", false, false},
		{"section tasks", []string{"sections", "tasks", "s1"}, http.MethodGet, "/sections/s1/tasks", true, false},
		{"section move", []string{"sections", "move", "s1", "--project", "p1", "--after-section", "s0"}, http.MethodPost, "/projects/p1/sections/insert", false, false},
		{"story get", []string{"stories", "get", "story1"}, http.MethodGet, "/stories/story1", false, false},
		{"story delete", []string{"stories", "delete", "story1"}, http.MethodDelete, "/stories/story1", false, true},
		{"project get", []string{"projects", "get", "p1"}, http.MethodGet, "/projects/p1", false, false},
		{"project create", []string{"projects", "create", "--workspace", "w1", "--name", "Project"}, http.MethodPost, "/projects", false, false},
		{"project update", []string{"projects", "update", "p1", "--name", "Renamed"}, http.MethodPut, "/projects/p1", false, false},
		{"project tasks", []string{"projects", "tasks", "p1"}, http.MethodGet, "/projects/p1/tasks", true, false},
		{"task projects", []string{"tasks", "projects", "t1"}, http.MethodGet, "/tasks/t1/projects", true, false},
		{"workspace projects", []string{"workspaces", "projects", "w1"}, http.MethodGet, "/workspaces/w1/projects", true, false},
		{"workspace project create", []string{"workspaces", "create-project", "w1", "--name", "Project"}, http.MethodPost, "/workspaces/w1/projects", false, false},
		{"team projects", []string{"teams", "projects", "team1"}, http.MethodGet, "/teams/team1/projects", true, false},
		{"team project create", []string{"teams", "create-project", "team1", "--name", "Project"}, http.MethodPost, "/teams/team1/projects", false, false},
		{"dependencies", []string{"tasks", "dependencies", "t1"}, http.MethodGet, "/tasks/t1/dependencies", true, false},
		{"remove dependent", []string{"tasks", "remove-dependents", "t1", "--dependent", "d1"}, http.MethodPost, "/tasks/t1/removeDependents", false, false},
		{"remove tag", []string{"tasks", "remove-tag", "t1", "--tag", "tag1"}, http.MethodPost, "/tasks/t1/removeTag", false, false},
		{"remove followers", []string{"tasks", "remove-followers", "t1", "--follower", "u1"}, http.MethodPost, "/tasks/t1/removeFollowers", false, false},
		{"membership list", []string{"memberships", "list", "--parent", "p1"}, http.MethodGet, "/memberships", true, false},
		{"membership update", []string{"memberships", "update", "m1", "--access-level", "commenter"}, http.MethodPut, "/memberships/m1", false, false},
		{"membership delete", []string{"memberships", "delete", "m1"}, http.MethodDelete, "/memberships/m1", false, true},
		{"task duplicate", []string{"tasks", "duplicate", "t1"}, http.MethodPost, "/tasks/t1/duplicate", false, false},
		{"project duplicate", []string{"projects", "duplicate", "p1", "--name", "Copy"}, http.MethodPost, "/projects/p1/duplicate", false, false},
		{"project template", []string{"projects", "save-as-template", "p1", "--name", "Template", "--public", "false", "--workspace", "w1"}, http.MethodPost, "/projects/p1/saveAsTemplate", false, false},
		{"task delete", []string{"tasks", "delete", "t1"}, http.MethodDelete, "/tasks/t1", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.EscapedPath() != tt.path {
					t.Fatalf("request=%s %s", r.Method, r.URL.EscapedPath())
				}
				if tt.noContent {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if tt.collection {
					_, _ = w.Write([]byte(`{"data":[]}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":{"gid":"result-1"}}`))
			}))
			defer server.Close()
			errOut := &bytes.Buffer{}
			code := cli.RunCLI(tt.args, &cli.CliIO{Out: &bytes.Buffer{}, ErrOut: errOut}, cli.RuntimeOptions{ConfigPath: mustWriteConfigWithToken(t), APIBase: server.URL, Output: "json"})
			if code != 0 {
				t.Fatalf("code=%d err=%s", code, errOut.String())
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
