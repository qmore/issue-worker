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
