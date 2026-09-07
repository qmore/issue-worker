package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/qmore/issue-worker/internal/auth"
	"github.com/qmore/issue-worker/internal/config"
	gh "github.com/qmore/issue-worker/internal/github"
	"github.com/qmore/issue-worker/internal/worker"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "issue-worker:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "init":
		return cmdInit(args[1:])
	case "login":
		return cmdLogin(args[1:])
	case "doctor":
		return cmdDoctor(args[1:])
	case "run", "start":
		return cmdRun(args[1:], false)
	case "poll":
		return cmdRun(args[1:], true)
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Print(`issue-worker - GitHub Issue driven local Codex worker

Usage:
  issue-worker init [--config PATH]
  issue-worker login
  issue-worker doctor [--config PATH]
  issue-worker run [--config PATH]
  issue-worker poll [--config PATH]
  issue-worker version

MVP flow:
  issue-worker init
  edit config.yml
  issue-worker login
  codex login
  issue-worker doctor
  issue-worker run
`)
}

func configFlag(name string, args []string) (string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	path := fs.String("config", "", "configuration file path")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *path != "" {
		return *path, nil
	}
	return config.DefaultPath()
}

func cmdInit(args []string) error {
	path, err := configFlag("init", args)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(config.Example()), 0600); err != nil {
		return err
	}
	fmt.Println("Created:", path)
	fmt.Println("Edit repositories: before running the worker.")
	return nil
}

func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fromEnv := fs.Bool("from-env", false, "store ISSUE_WORKER_GITHUB_TOKEN in macOS Keychain")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var token string
	if *fromEnv {
		token = strings.TrimSpace(os.Getenv("ISSUE_WORKER_GITHUB_TOKEN"))
		if token == "" {
			return errors.New("ISSUE_WORKER_GITHUB_TOKEN is empty")
		}
	} else {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("stdin is not a terminal; use ISSUE_WORKER_GITHUB_TOKEN=... issue-worker login --from-env")
		}
		fmt.Print("Fine-grained GitHub PAT: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		token = strings.TrimSpace(string(b))
		for i := range b {
			b[i] = 0
		}
		if token == "" {
			return errors.New("empty token")
		}
	}

	if err := auth.StoreGitHubToken(token); err != nil {
		return fmt.Errorf("store token in macOS Keychain: %w", err)
	}
	fmt.Println("Stored GitHub credential in macOS Keychain (service=issue-worker, account=github).")
	return nil
}

func cmdDoctor(args []string) error {
	path, err := configFlag("doctor", args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	failed := false
	for _, name := range []string{"git", "codex"} {
		if err := worker.CheckCommand(name); err != nil {
			fmt.Printf("[FAIL] %s installed: %v\n", name, err)
			failed = true
		} else {
			fmt.Printf("[OK] %s installed\n", name)
		}
	}

	if cfg.Codex.Backend == "app-server" {
		if err := worker.CheckAppServer(context.Background(), cfg.Codex); err != nil {
			fmt.Printf("[FAIL] desktop app server: %v\n", err)
			failed = true
		} else {
			fmt.Println("[OK] desktop app server")
		}
	}

	token, err := auth.ReadGitHubToken()
	if err != nil {
		fmt.Printf("[FAIL] GitHub credential: %v\n", err)
		failed = true
	} else {
		fmt.Println("[OK] GitHub credential available")
		client := gh.New(cfg.GitHub.APIURL, token)
		ctx := context.Background()
		for _, repo := range cfg.Repositories {
			r, err := client.GetRepository(ctx, repo)
			if err != nil {
				fmt.Printf("[FAIL] repo access %s: %v\n", repo, err)
				failed = true
				continue
			}
			if !r.Private {
				fmt.Printf("[FAIL] repo %s is public; daemon MVP accepts private targets only\n", repo)
				failed = true
				continue
			}
			fmt.Printf("[OK] repo access: %s (default=%s)\n", r.FullName, r.DefaultBranch)
		}
	}

	root := cfg.Workspace.Root
	if strings.HasPrefix(root, "~/") {
		if home, e := os.UserHomeDir(); e == nil {
			root = filepath.Join(home, strings.TrimPrefix(root, "~/"))
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		fmt.Printf("[FAIL] workspace writable: %v\n", err)
		failed = true
	} else {
		tmp, err := os.CreateTemp(root, ".doctor-*")
		if err != nil {
			fmt.Printf("[FAIL] workspace writable: %v\n", err)
			failed = true
		} else {
			name := tmp.Name()
			_ = tmp.Close()
			_ = os.Remove(name)
			fmt.Println("[OK] workspace writable")
		}
	}

	if failed {
		return errors.New("doctor found one or more problems")
	}
	fmt.Println("[OK] daemon configuration")
	return nil
}

func cmdRun(args []string, once bool) error {
	path, err := configFlag("run", args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	token, err := auth.ReadGitHubToken()
	if err != nil {
		return err
	}
	client := gh.New(cfg.GitHub.APIURL, token)
	logger := log.New(os.Stdout, "", log.LstdFlags)
	w := worker.New(cfg, client, token, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if once {
		return w.PollOnce(ctx)
	}
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
