package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/qmore/issue-worker/internal/config"
	gh "github.com/qmore/issue-worker/internal/github"
)

func fakeApp(t *testing.T, respond func(map[string]json.RawMessage) []string) *appSession {
	t.Helper()
	reader, writer := io.Pipe()
	s := &appSession{input: writer, messages: make(chan rpcRead, 32), stopped: make(chan struct{}), threadID: "thread-1", cfg: config.Codex{Timeout: "1s"}, log: log.New(io.Discard, "", 0)}
	go func() {
		defer reader.Close()
		decoder := json.NewDecoder(reader)
		for {
			var request map[string]json.RawMessage
			if decoder.Decode(&request) != nil {
				return
			}
			for _, raw := range respond(request) {
				var m rpcMessage
				err := json.Unmarshal([]byte(raw), &m)
				select {
				case s.messages <- rpcRead{m, err}:
				case <-s.stopped:
					return
				}
			}
		}
	}()
	t.Cleanup(s.close)
	return s
}

func TestAppRunHandlesInterleavedEvents(t *testing.T) {
	s := fakeApp(t, func(r map[string]json.RawMessage) []string {
		return []string{
			`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1"}}}`,
			`{"method":"item/completed","params":{"threadId":"other","item":{"type":"agentMessage","text":"wrong"}}}`,
			`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","text":"implemented","phase":"final_answer"}}}`,
			`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed"}}}`,
			`{"id":` + string(r["id"]) + `,"result":{"turn":{"id":"turn-1"}}}`,
		}
	})
	got, err := s.run(context.Background(), "test")
	if err != nil || got != "implemented" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestAppTimeoutInterruptsSharedTurn(t *testing.T) {
	interrupted := make(chan bool, 1)
	s := fakeApp(t, func(r map[string]json.RawMessage) []string {
		var method string
		_ = json.Unmarshal(r["method"], &method)
		if method == "turn/interrupt" {
			interrupted <- true
			return []string{`{"id":` + string(r["id"]) + `,"result":{}}`}
		}
		return []string{`{"id":` + string(r["id"]) + `,"result":{"turn":{"id":"turn-1"}}}`}
	})
	s.cfg.Timeout = "20ms"
	_, err := s.run(context.Background(), "test")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	select {
	case <-interrupted:
	default:
		t.Fatal("shared turn was not interrupted")
	}
}

func TestAppRunRejectsFailedAndInterruptedTurns(t *testing.T) {
	for _, status := range []string{"failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			s := fakeApp(t, func(r map[string]json.RawMessage) []string {
				return []string{
					`{"id":` + string(r["id"]) + `,"result":{"turn":{"id":"turn-1"}}}`,
					`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"` + status + `","error":{"message":"test failure"}}}}`,
				}
			})
			_, err := s.run(context.Background(), "test")
			if err == nil || !strings.Contains(err.Error(), status) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestAppParamsDoNotInheritSecrets(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-sentinel")
	t.Setenv("ISSUE_WORKER_GITHUB_TOKEN", "secret-sentinel")
	raw, _ := json.Marshal(appThreadParams(config.Codex{}, "/tmp/worktree"))
	for _, want := range []string{`"approvalPolicy":"never"`, `"sandbox":"workspace-write"`, `"ephemeral":false`, `"shell_environment_policy.inherit":"none"`, `"sandbox_workspace_write.network_access":false`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(string(raw), "secret-sentinel") {
		t.Fatal("token sent to shared server")
	}
}

func TestAppServerCommand(t *testing.T) {
	tests := []struct {
		cfg  config.Codex
		want []string
	}{
		{config.Codex{}, []string{"app-server"}},
		{config.Codex{AppServerSocket: "/tmp/codex.sock"}, []string{"app-server", "proxy", "--sock", "/tmp/codex.sock"}},
	}
	for _, tt := range tests {
		if got := appServerArgs(tt.cfg); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("appServerArgs(%+v) = %v, want %v", tt.cfg, got, tt.want)
		}
	}
}

func TestAppTaskTitleIsBounded(t *testing.T) {
	got := appTaskTitle("owner/repo", gh.Issue{Number: 12, Title: strings.Repeat("長", 300)})
	if len([]rune(got)) != 200 || !strings.HasSuffix(got, "…") {
		t.Fatalf("unexpected title: %q", got)
	}
}

// Opt-in integration probe runs a harmless model turn, then verifies that a
// second app-server process can read and archive the durable task.
func TestLiveAppServer(t *testing.T) {
	if os.Getenv("ISSUE_WORKER_TEST_APP_SERVER") != "1" {
		t.Skip("set ISSUE_WORKER_TEST_APP_SERVER=1 for local desktop probe")
	}
	s, err := connectApp(config.Codex{}, log.Default())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.initialize(ctx); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := s.call(ctx, "thread/start", appThreadParams(config.Codex{}, t.TempDir()), &response); err != nil {
		t.Fatal(err)
	}
	s.threadID = response.Thread.ID
	if err := s.call(ctx, "thread/name/set", map[string]string{"threadId": s.threadID, "name": "issue-worker integration check"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.persistIssue(ctx, "Reply with exactly: issue-worker integration check passed. Do not call tools."); err != nil {
		t.Fatal(err)
	}
	s.close()

	reader, err := connectApp(config.Codex{}, log.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.close()
	if err := reader.initialize(ctx); err != nil {
		t.Fatal(err)
	}
	var read struct {
		Thread struct {
			Name      string `json:"name"`
			Ephemeral bool   `json:"ephemeral"`
		} `json:"thread"`
	}
	if err := reader.call(ctx, "thread/read", map[string]any{"threadId": s.threadID, "includeTurns": false}, &read); err != nil {
		t.Fatal(err)
	}
	if read.Thread.Name != "issue-worker integration check" || read.Thread.Ephemeral {
		t.Fatalf("thread not persisted/named: %+v", read)
	}
	var resumed struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := reader.call(ctx, "thread/resume", map[string]string{"threadId": s.threadID}, &resumed); err != nil {
		t.Fatal(err)
	}
	reader.threadID = resumed.Thread.ID
	reader.cfg.Timeout = "1m"
	if _, err := reader.run(ctx, "Follow the request above."); err != nil {
		t.Fatal(err)
	}
	if err := reader.call(ctx, "thread/archive", map[string]string{"threadId": s.threadID}, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("shared desktop thread created, read, and archived: %s", s.threadID)
}
