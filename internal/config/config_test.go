package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("version: 1\nrepositories:\n  - owner/repo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worker.PollInterval != 30*time.Second {
		t.Fatalf("poll interval = %s", cfg.Worker.PollInterval)
	}
	if cfg.Worker.Concurrency != 1 || cfg.Labels.Ready != "codex:ready" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.Codex.Backend != "exec" || cfg.Codex.Timeout != "30m" {
		t.Fatalf("unexpected codex defaults: %#v", cfg.Codex)
	}
	if cfg.PullRequests.Monitor || cfg.PullRequests.Command != "/issue-worker" || cfg.PullRequests.MaxFixAttempts != 3 {
		t.Fatalf("unexpected pull request defaults: %#v", cfg.PullRequests)
	}
}

func TestLoadRejectsInvalidCodexSettings(t *testing.T) {
	for _, content := range []string{
		"version: 1\nrepositories: [owner/repo]\ncodex:\n  backend: other\n",
		"version: 1\nrepositories: [owner/repo]\ncodex:\n  timeout: never\n",
		"version: 1\nrepositories: [owner/repo]\npull_requests:\n  max_fix_attempts: -1\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yml")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("Load accepted invalid config: %s", content)
		}
	}
}

func TestLoadRejectsConcurrencyAboveOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "version: 1\nworker:\n  concurrency: 2\nrepositories:\n  - owner/repo\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected concurrency validation error")
	}
}
