package worker

// The stdio transport creates durable tasks in the user's normal Codex history.
// A configured socket instead attaches to an existing shared app-server daemon.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/qmore/issue-worker/internal/config"
	gh "github.com/qmore/issue-worker/internal/github"
)

type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type rpcRead struct {
	message rpcMessage
	err     error
}

type appSession struct {
	input       io.WriteCloser
	messages    chan rpcRead
	stopped     chan struct{}
	cmd         *exec.Cmd
	nextID      int
	threadID    string
	turnID      string
	title       string
	lastMessage string
	turnStatus  string
	turnError   string
	cfg         config.Codex
	log         *log.Logger
}

func connectApp(cfg config.Codex, logger *log.Logger) (*appSession, error) {
	args := appServerArgs(cfg)
	cmd := exec.Command("codex", args...)
	cmd.Env = codexEnv()
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	s := &appSession{input: input, messages: make(chan rpcRead, 32), stopped: make(chan struct{}), cmd: cmd, cfg: cfg, log: logger}
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 16*1024*1024)
		for scanner.Scan() {
			var m rpcMessage
			err := json.Unmarshal(scanner.Bytes(), &m)
			select {
			case s.messages <- rpcRead{m, err}:
			case <-s.stopped:
				return
			}
			if err != nil {
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case s.messages <- rpcRead{err: err}:
		case <-s.stopped:
		}
	}()
	return s, nil
}

func appServerArgs(cfg config.Codex) []string {
	if cfg.AppServerSocket != "" {
		return []string{"app-server", "proxy", "--sock", expandHome(cfg.AppServerSocket)}
	}
	return []string{"app-server"}
}

func (s *appSession) close() {
	close(s.stopped)
	s.input.Close()
	if s.cmd != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
}

func (s *appSession) send(v any) error { return json.NewEncoder(s.input).Encode(v) }

func (s *appSession) receive(ctx context.Context) (rpcMessage, error) {
	select {
	case <-ctx.Done():
		return rpcMessage{}, ctx.Err()
	case r := <-s.messages:
		return r.message, r.err
	}
}

func (s *appSession) call(ctx context.Context, method string, params any, result any) error {
	s.nextID++
	id := s.nextID
	if err := s.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		m, err := s.receive(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		if string(m.ID) == fmt.Sprint(id) && m.Method == "" {
			if m.Error != nil {
				return fmt.Errorf("%s: %s", method, m.Error.Message)
			}
			if result != nil {
				return json.Unmarshal(m.Result, result)
			}
			return nil
		}
		if err := s.event(m); err != nil {
			return err
		}
	}
}

func (s *appSession) initialize(ctx context.Context) error {
	if err := s.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "issue_worker", "title": "issue-worker", "version": "0.1.0"}}, nil); err != nil {
		return err
	}
	return s.send(map[string]any{"method": "initialized", "params": map[string]any{}})
}

// CheckAppServer probes the shared daemon without creating a thread or a turn.
func CheckAppServer(ctx context.Context, cfg config.Codex) error {
	s, err := connectApp(cfg, log.Default())
	if err != nil {
		return err
	}
	defer s.close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.initialize(ctx)
}

func appThreadParams(cfg config.Codex, dir string) map[string]any {
	env := map[string]string{}
	for _, e := range inheritedSafeEnv() {
		k, v, _ := strings.Cut(e, "=")
		env[k] = v
	}
	params := map[string]any{
		"cwd": dir, "ephemeral": false, "approvalPolicy": "never", "sandbox": "workspace-write",
		"config": map[string]any{
			"sandbox_workspace_write.network_access": cfg.AllowNetwork,
			"shell_environment_policy.inherit":       "none", "shell_environment_policy.set": env,
		},
	}
	if cfg.Model != "" {
		params["model"] = cfg.Model
	}
	return params
}

func (w *Worker) openAppSession(ctx context.Context, dir, repo string, issue gh.Issue) (*appSession, error) {
	s, err := connectApp(w.cfg.Codex, w.log)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			s.close()
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.initialize(ctx); err != nil {
		return nil, fmt.Errorf("connect Codex app server: %w", err)
	}
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := s.call(ctx, "thread/start", appThreadParams(w.cfg.Codex, dir), &response); err != nil {
		return nil, err
	}
	s.threadID = response.Thread.ID
	if s.threadID == "" {
		return nil, errors.New("app server returned no thread id")
	}
	s.title = appTaskTitle(repo, issue)
	w.log.Printf("%s#%d desktop thread=%s worktree=%s", repo, issue.Number, s.threadID, dir)
	// The thread is durable in Codex; no GitHub credential or raw execution output
	// is copied to its title or posted back to GitHub by this integration.
	s.status("setup")
	if err := s.persistIssue(ctx, buildPrompt(repo, issue)); err != nil {
		return nil, fmt.Errorf("persist Issue in Codex task: %w", err)
	}
	ok = true
	return s, nil
}

func appTaskTitle(repo string, issue gh.Issue) string {
	title := fmt.Sprintf("issue-worker %s#%d %s", repo, issue.Number, issue.Title)
	runes := []rune(title)
	if len(runes) > 200 {
		title = string(runes[:199]) + "…"
	}
	return title
}

func (s *appSession) persistIssue(ctx context.Context, prompt string) error {
	item := map[string]any{
		"type":    "message",
		"role":    "user",
		"content": []map[string]string{{"type": "input_text", "text": prompt}},
	}
	return s.call(ctx, "thread/inject_items", map[string]any{"threadId": s.threadID, "items": []any{item}}, nil)
}

func (s *appSession) status(stage string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.call(ctx, "thread/name/set", map[string]string{"threadId": s.threadID, "name": "[" + stage + "] " + s.title}, nil); err != nil {
		s.log.Printf("desktop title update: %v", err)
	}
}

func (s *appSession) event(m rpcMessage) error {
	// No interactive approval or tool-input request may hang unattended execution.
	if len(m.ID) != 0 && m.Method != "" {
		_ = s.send(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "issue-worker does not support interactive requests"}})
		return fmt.Errorf("app server requested interactive method %s", m.Method)
	}
	var p struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Delta    string `json:"delta"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
		Item struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Phase string `json:"phase"`
		} `json:"item"`
	}
	if m.Method == "" {
		return nil
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return err
	}
	if p.ThreadID != s.threadID || s.threadID == "" {
		return nil
	}
	if s.turnID != "" && p.TurnID != "" && p.TurnID != s.turnID {
		return nil
	}
	switch m.Method {
	case "turn/started":
		if s.turnID == "" {
			s.turnID = p.Turn.ID
		}
	case "item/started":
		s.log.Printf("desktop thread=%s item=%s started", s.threadID, p.Item.Type)
	case "item/completed":
		if p.Item.Type == "agentMessage" {
			s.log.Printf("Codex: %s", p.Item.Text)
			if p.Item.Phase == "final_answer" || p.Item.Phase == "" {
				s.lastMessage = p.Item.Text
			}
		}
	case "turn/completed":
		if s.turnID != "" && p.Turn.ID != s.turnID {
			return nil
		}
		s.turnID = p.Turn.ID
		s.turnStatus = p.Turn.Status
		if p.Turn.Error != nil {
			s.turnError = p.Turn.Error.Message
		}
	}
	return nil
}

func (s *appSession) run(ctx context.Context, prompt string) (string, error) {
	timeout, err := time.ParseDuration(s.cfg.Timeout)
	if err != nil || timeout <= 0 {
		return "", errors.New("invalid codex.timeout")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// A proxy disconnect does not terminate the shared daemon's turn. Always
	// explicitly interrupt unfinished work before returning to wrapper Git logic.
	defer func() {
		if s.turnID != "" && s.turnStatus == "" {
			stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			err := s.call(stopCtx, "turn/interrupt", map[string]string{"threadId": s.threadID, "turnId": s.turnID}, nil)
			if err != nil {
				s.log.Printf("INTERRUPT FAILED: inspect desktop thread=%s before retrying: %v", s.threadID, err)
			}
		}
	}()
	params := map[string]any{"threadId": s.threadID, "input": []map[string]any{{"type": "text", "text": prompt}},
		"approvalPolicy": "never", "sandboxPolicy": map[string]any{"type": "workspaceWrite", "networkAccess": s.cfg.AllowNetwork}}
	if s.cfg.Effort != "" {
		params["effort"] = s.cfg.Effort
	}
	var response struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := s.call(ctx, "turn/start", params, &response); err != nil {
		return "", err
	}
	if response.Turn.ID == "" {
		return "", errors.New("app server returned no turn id")
	}
	s.turnID = response.Turn.ID
	for s.turnStatus == "" {
		m, err := s.receive(ctx)
		if err != nil {
			return "", err
		}
		if err := s.event(m); err != nil {
			return "", err
		}
	}
	if s.turnStatus != "completed" {
		return "", fmt.Errorf("turn %s: %s", s.turnStatus, s.turnError)
	}
	return strings.TrimSpace(s.lastMessage), nil
}
