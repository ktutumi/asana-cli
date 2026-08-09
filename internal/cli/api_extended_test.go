package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			args:       []string{"tasks", "create", "--workspace", "w1", "--name", "New task", "--completed", "false", "--custom-field", "cf1=3"},
			method:     http.MethodPost,
			escaped:    "/tasks",
			wantFields: map[string]any{"workspace": "w1", "name": "New task", "completed": false, "custom_fields": map[string]any{"cf1": float64(3)}},
		},
		{
			name:       "update only specified fields",
			args:       []string{"tasks", "update", "task/1", "--due-on", "2026-08-10"},
			method:     http.MethodPut,
			escaped:    "/tasks/task%2F1",
			wantFields: map[string]any{"due_on": "2026-08-10"},
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
