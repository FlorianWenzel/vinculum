package git

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

// ParseGitHubRepo extracts "owner/repo" from a GitHub remote URL. Supports
// https://github.com/owner/repo(.git), git@github.com:owner/repo(.git),
// and ssh://git@github.com/owner/repo(.git). Returns an error if the URL
// is not a recognizable GitHub URL.
func ParseGitHubRepo(remoteURL string) (owner, repo string, err error) {
	s := strings.TrimSpace(remoteURL)
	if s == "" {
		return "", "", errors.New("empty remote URL")
	}
	// scp-like: git@github.com:owner/repo(.git)
	if strings.HasPrefix(s, "git@") {
		if i := strings.Index(s, ":"); i > 0 {
			host := strings.TrimPrefix(s[:i], "git@")
			if !isGitHubHost(host) {
				return "", "", fmt.Errorf("not a github.com URL: %q", remoteURL)
			}
			path := strings.TrimSuffix(s[i+1:], ".git")
			return splitOwnerRepo(path)
		}
		return "", "", fmt.Errorf("unrecognized scp-like url: %q", remoteURL)
	}
	// ssh:// and https://
	u, e := url.Parse(s)
	if e != nil {
		return "", "", fmt.Errorf("parse url: %w", e)
	}
	if !isGitHubHost(u.Host) {
		return "", "", fmt.Errorf("not a github.com URL: %q", remoteURL)
	}
	return splitOwnerRepo(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git"))
}

func isGitHubHost(h string) bool {
	h = strings.ToLower(h)
	return h == "github.com" || h == "www.github.com" || strings.HasSuffix(h, ".github.com")
}

func splitOwnerRepo(path string) (string, string, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected owner/repo, got %q", path)
	}
	return parts[0], parts[1], nil
}

// GitHubPRRequest is the body sent to POST /repos/{owner}/{repo}/pulls.
type GitHubPRRequest struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body,omitempty"`
}

// GitHubPRResponse is the slice of the create-PR response we surface.
type GitHubPRResponse struct {
	HTMLURL string `json:"html_url"`
	Number  int    `json:"number"`
}

// GitHubClient wraps GitHub's REST PR endpoint. The baseURL field defaults
// to https://api.github.com and is overridable for tests + Enterprise.
type GitHubClient struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewGitHubClient returns a client that authenticates with the given token
// (typically the GITHUB_TOKEN env injected via tokenSecretRef).
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		baseURL: "https://api.github.com",
		token:   token,
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

// WithBaseURL returns a copy with a different API base URL. Used by tests
// to point at an httptest stub.
func (c *GitHubClient) WithBaseURL(u string) *GitHubClient {
	cp := *c
	cp.baseURL = strings.TrimRight(u, "/")
	return &cp
}

// CreatePR opens a pull request. Returns the parsed response (HTMLURL +
// Number). Non-2xx responses surface the GitHub error message in the
// returned error so users see exactly what GitHub refused.
func (c *GitHubClient) CreatePR(ctx context.Context, owner, repo string, req GitHubPRRequest) (*GitHubPRResponse, error) {
	if c.token == "" {
		return nil, errors.New("github token is required (set gitCredentials.tokenSecretRef on the Agent)")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("PR title is required")
	}
	if strings.TrimSpace(req.Head) == "" || strings.TrimSpace(req.Base) == "" {
		return nil, errors.New("PR head and base are required")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls", c.baseURL, owner, repo)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setAuthHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		// GitHub returns {"message":"...","errors":[{...}]}; if we hit
		// "A pull request already exists for ..." (422), find the existing
		// PR and return it — re-running the same Task should be idempotent.
		if resp.StatusCode == http.StatusUnprocessableEntity && bytesContainsAlreadyExists(respBody) {
			if existing, lookupErr := c.findOpenPR(ctx, owner, repo, req.Head, req.Base); lookupErr == nil && existing != nil {
				return existing, nil
			}
		}
		var errPayload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &errPayload)
		msg := errPayload.Message
		if msg == "" {
			msg = strings.TrimSpace(string(respBody))
		}
		return nil, fmt.Errorf("github %d: %s", resp.StatusCode, msg)
	}
	var out GitHubPRResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

// findOpenPR returns the open PR matching the given head/base if one
// exists. The GitHub API accepts head as "<owner>:<branch>" when
// listing PRs across forks; for same-repo PRs the owner-less form works
// too. We try the qualified form for robustness.
func (c *GitHubClient) findOpenPR(ctx context.Context, owner, repo, head, base string) (*GitHubPRResponse, error) {
	q := url.Values{}
	q.Set("state", "open")
	q.Set("head", owner+":"+head)
	q.Set("base", base)
	q.Set("per_page", "1")
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls?%s", c.baseURL, owner, repo, q.Encode())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeaders(httpReq)
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github %d listing PRs", resp.StatusCode)
	}
	var list []GitHubPRResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode pr list: %w", err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

func (c *GitHubClient) setAuthHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// bytesContainsAlreadyExists checks the GitHub error response body for the
// canonical "A pull request already exists" marker. We match on the bytes
// directly rather than parsing the nested `errors[]` array because the
// exact field name varies (`message` on the top-level, `message` on each
// errors[] entry, sometimes only the top-level message).
func bytesContainsAlreadyExists(body []byte) bool {
	return bytesContains(body, "pull request already exists")
}

func bytesContains(haystack []byte, needle string) bool {
	return strings.Contains(strings.ToLower(string(haystack)), strings.ToLower(needle))
}
