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
