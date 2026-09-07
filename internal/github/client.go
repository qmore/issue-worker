package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiVersion = "2022-11-28"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Issue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PullRequest *struct{} `json:"pull_request,omitempty"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type Repository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	Private       bool   `json:"private"`
}

type PullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func NewWithHTTP(baseURL, token string, hc *http.Client) *Client {
	c := New(baseURL, token)
	c.http = hc
	return c
}

func (c *Client) do(ctx context.Context, method, path, etag string, body any, out any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return resp, fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return resp, err
		}
	}
	return resp, nil
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository %q", repo)
	}
	return parts[0], parts[1], nil
}

func (c *Client) ListReadyIssues(ctx context.Context, repo, label, etag string) ([]Issue, string, bool, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, "", false, err
	}
	q := url.Values{}
	q.Set("state", "open")
	q.Set("labels", label)
	q.Set("per_page", "50")
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	path := fmt.Sprintf("/repos/%s/%s/issues?%s", url.PathEscape(owner), url.PathEscape(name), q.Encode())
	var issues []Issue
	resp, err := c.do(ctx, http.MethodGet, path, etag, nil, &issues)
	if err != nil {
		return nil, "", false, err
	}
	newETag := resp.Header.Get("ETag")
	if resp.StatusCode == http.StatusNotModified {
		return nil, newETag, true, nil
	}
	filtered := issues[:0]
	for _, issue := range issues {
		if issue.PullRequest == nil {
			filtered = append(filtered, issue)
		}
	}
	return filtered, newETag, false, nil
}

func (c *Client) GetIssue(ctx context.Context, repo string, number int) (Issue, error) {
	var issue Issue
	_, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/issues/%d", repo, number), "", nil, &issue)
	return issue, err
}

func (c *Client) GetRepository(ctx context.Context, repo string) (Repository, error) {
	var r Repository
	_, err := c.do(ctx, http.MethodGet, "/repos/"+repo, "", nil, &r)
	return r, err
}

func (c *Client) EnsureLabel(ctx context.Context, repo, name, color, description string) error {
	body := map[string]string{"name": name, "color": color, "description": description}
	_, err := c.do(ctx, http.MethodPost, "/repos/"+repo+"/labels", "", body, nil)
	if err == nil {
		return nil
	}
	// Existing labels return 422. PATCH makes the operation idempotent while still
	// failing closed for insufficient permissions or invalid repositories.
	_, patchErr := c.do(ctx, http.MethodPatch, "/repos/"+repo+"/labels/"+url.PathEscape(name), "", map[string]string{
		"new_name":    name,
		"color":       color,
		"description": description,
	}, nil)
	return patchErr
}

func (c *Client) AddLabel(ctx context.Context, repo string, number int, label string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/labels", repo, number), "", map[string][]string{"labels": []string{label}}, nil)
	return err
}

func (c *Client) RemoveLabel(ctx context.Context, repo string, number int, label string) error {
	_, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/repos/%s/issues/%d/labels/%s", repo, number, url.PathEscape(label)), "", nil, nil)
	return err
}

func (c *Client) Comment(ctx context.Context, repo string, number int, body string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number), "", map[string]string{"body": body}, nil)
	return err
}

func (c *Client) CreatePR(ctx context.Context, repo, title, head, base, body string) (PullRequest, error) {
	var pr PullRequest
	_, err := c.do(ctx, http.MethodPost, "/repos/"+repo+"/pulls", "", map[string]any{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
	}, &pr)
	return pr, err
}

func HasLabel(issue Issue, name string) bool {
	for _, l := range issue.Labels {
		if l.Name == name {
			return true
		}
	}
	return false
}
