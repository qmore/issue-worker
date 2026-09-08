package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version      int          `yaml:"version"`
	Worker       Worker       `yaml:"worker"`
	GitHub       GitHub       `yaml:"github"`
	Repositories []string     `yaml:"repositories"`
	Labels       Labels       `yaml:"labels"`
	Workspace    Workspace    `yaml:"workspace"`
	Codex        Codex        `yaml:"codex"`
	PullRequests PullRequests `yaml:"pull_requests"`
}

type Worker struct {
	ID           string        `yaml:"id"`
	PollInterval time.Duration `yaml:"-"`
	PollRaw      string        `yaml:"poll_interval"`
	Concurrency  int           `yaml:"concurrency"`
}

type GitHub struct {
	APIURL string `yaml:"api_url"`
}

type Labels struct {
	Ready    string `yaml:"ready"`
	Running  string `yaml:"running"`
	Review   string `yaml:"review"`
	Failed   string `yaml:"failed"`
	NoChange string `yaml:"no_change"`
}

type Workspace struct {
	Root string `yaml:"root"`
}

type Codex struct {
	Backend         string `yaml:"backend"`
	AppServerSocket string `yaml:"app_server_socket"`
	Timeout         string `yaml:"timeout"`
	Model           string `yaml:"model"`
	Effort          string `yaml:"effort"`
	AllowNetwork    bool   `yaml:"allow_network"`
}

type PullRequests struct {
	Monitor        bool   `yaml:"monitor"`
	Command        string `yaml:"command"`
	MaxFixAttempts int    `yaml:"max_fix_attempts"`
}

type RepoConfig struct {
	BaseBranch string   `yaml:"base_branch"`
	Setup      []string `yaml:"setup"`
	Verify     []string `yaml:"verify"`
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "issue-worker", "config.yml"), nil
}

func DefaultWorkspaceRoot() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "issue-worker", "data"), nil
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func LoadRepo(path string) (RepoConfig, error) {
	var rc RepoConfig
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return rc, nil
	}
	if err != nil {
		return rc, err
	}
	if err := yaml.Unmarshal(b, &rc); err != nil {
		return rc, err
	}
	return rc, nil
}

func (c *Config) applyDefaults() {
	if c.Codex.Backend == "" {
		c.Codex.Backend = "exec"
	}
	if c.Codex.Timeout == "" {
		c.Codex.Timeout = "30m"
	}
	if c.PullRequests.Command == "" {
		c.PullRequests.Command = "/issue-worker"
	}
	if c.PullRequests.MaxFixAttempts == 0 {
		c.PullRequests.MaxFixAttempts = 3
	}
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Worker.ID == "" {
		c.Worker.ID = "local"
	}
	if c.Worker.PollRaw == "" {
		c.Worker.PollRaw = "30s"
	}
	if c.Worker.Concurrency == 0 {
		c.Worker.Concurrency = 1
	}
	if c.GitHub.APIURL == "" {
		c.GitHub.APIURL = "https://api.github.com"
	}
	if c.Labels.Ready == "" {
		c.Labels.Ready = "codex:ready"
	}
	if c.Labels.Running == "" {
		c.Labels.Running = "codex:running"
	}
	if c.Labels.Review == "" {
		c.Labels.Review = "codex:review"
	}
	if c.Labels.Failed == "" {
		c.Labels.Failed = "codex:failed"
	}
	if c.Labels.NoChange == "" {
		c.Labels.NoChange = "codex:no-change"
	}
	if c.Workspace.Root == "" {
		if root, err := DefaultWorkspaceRoot(); err == nil {
			c.Workspace.Root = root
		}
	}
	if d, err := time.ParseDuration(c.Worker.PollRaw); err == nil {
		c.Worker.PollInterval = d
	}
}

func (c *Config) validate() error {
	if c.Codex.Backend != "exec" && c.Codex.Backend != "app-server" {
		return fmt.Errorf("invalid codex.backend %q", c.Codex.Backend)
	}
	if d, err := time.ParseDuration(c.Codex.Timeout); err != nil || d <= 0 {
		return fmt.Errorf("invalid codex.timeout %q", c.Codex.Timeout)
	}
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.Worker.PollInterval <= 0 {
		return fmt.Errorf("invalid poll_interval %q", c.Worker.PollRaw)
	}
	if c.Worker.Concurrency != 1 {
		return fmt.Errorf("daemon MVP supports worker.concurrency=1 only")
	}
	if c.PullRequests.MaxFixAttempts < 1 {
		return fmt.Errorf("pull_requests.max_fix_attempts must be at least 1")
	}
	if len(c.Repositories) == 0 {
		return fmt.Errorf("at least one repository must be configured")
	}
	for _, repo := range c.Repositories {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("repository must be owner/name: %q", repo)
		}
	}
	if c.Workspace.Root == "" {
		return fmt.Errorf("workspace.root is empty")
	}
	return nil
}

func Example() string {
	return `version: 1

worker:
  id: mac-mini
  poll_interval: 30s
  concurrency: 1

github:
  api_url: https://api.github.com

repositories:
  - OWNER/REPOSITORY

labels:
  ready: codex:ready
  running: codex:running
  review: codex:review
  failed: codex:failed
  no_change: codex:no-change

workspace:
  root: ""

codex:
  backend: exec
  app_server_socket: ""
  timeout: 30m
  model: ""
  effort: ""
  allow_network: false

pull_requests:
  monitor: false
  command: /issue-worker
  max_fix_attempts: 3
`
}
