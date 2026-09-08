package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListReadyIssuesUsesETagAndFiltersPRs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing auth header")
		}
		if calls == 2 {
			if r.Header.Get("If-None-Match") != `"etag-1"` {
				t.Fatalf("If-None-Match = %q", r.Header.Get("If-None-Match"))
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if !strings.Contains(r.URL.RawQuery, "labels=codex%3Aready") {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		w.Header().Set("ETag", `"etag-1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"number":1,"title":"job","body":"do it","labels":[{"name":"codex:ready"}]},
			{"number":2,"title":"pr","pull_request":{},"labels":[{"name":"codex:ready"}]}
		]`))
	}))
	defer server.Close()

	client := NewWithHTTP(server.URL, "token", server.Client())
	issues, etag, notModified, err := client.ListReadyIssues(context.Background(), "owner/repo", "codex:ready", "")
	if err != nil {
		t.Fatal(err)
	}
	if notModified || etag != `"etag-1"` || len(issues) != 1 || issues[0].Number != 1 {
		t.Fatalf("issues=%v etag=%q notModified=%v", issues, etag, notModified)
	}

	issues, _, notModified, err = client.ListReadyIssues(context.Background(), "owner/repo", "codex:ready", etag)
	if err != nil {
		t.Fatal(err)
	}
	if !notModified || len(issues) != 0 {
		t.Fatalf("expected 304, issues=%v", issues)
	}
}

func TestPullRequestPollingUsesLatestCommentsAndHeadSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/owner/repo/issues/comments":
			if r.URL.Query().Get("direction") != "desc" || r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("comment query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[]`))
		case "/repos/owner/repo/actions/runs":
			if r.URL.Query().Get("head_sha") != "abc123" || r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("workflow query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"workflow_runs":[{"id":1,"workflow_id":2,"run_attempt":3,"name":"CI"}]}`))
		default:
			http.Error(w, r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewWithHTTP(server.URL, "token", server.Client())
	if _, err := client.ListRepositoryIssueComments(context.Background(), "owner/repo"); err != nil {
		t.Fatal(err)
	}
	runs, err := client.ListWorkflowRuns(context.Background(), "owner/repo", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].WorkflowID != 2 || runs[0].RunAttempt != 3 {
		t.Fatalf("runs = %#v", runs)
	}
}
