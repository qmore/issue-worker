package auth

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	keychainService = "issue-worker"
	keychainAccount = "github"
)

func ReadGitHubToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("ISSUE_WORKER_GITHUB_TOKEN")); token != "" {
		return token, nil
	}
	if runtime.GOOS != "darwin" {
		return "", errors.New("ISSUE_WORKER_GITHUB_TOKEN is not set and macOS Keychain is unavailable")
	}
	cmd := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", keychainAccount, "-w")
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", errors.New("empty GitHub credential")
	}
	return token, nil
}

func StoreGitHubToken(token string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("macOS Keychain storage is only available on darwin")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("empty token")
	}
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", keychainService, "-a", keychainAccount, "-w", token)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
