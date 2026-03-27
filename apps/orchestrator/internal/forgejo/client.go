package forgejo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/florian/vinculum/apps/orchestrator/internal/config"
)

type Client struct {
	cfg        config.Config
	httpClient *http.Client
}

type User struct {
	ID       int64  `json:"id"`
	UserName string `json:"login"`
	Email    string `json:"email"`
}

type Repo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	SSHURL   string `json:"ssh_url"`
}

type PullRequest struct {
	Number  int64  `json:"number"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Merged  bool   `json:"merged"`
	Head    struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type PublicKey struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

type Webhook struct {
	ID     int64             `json:"id"`
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
	Active bool              `json:"active"`
}

type ContentEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	SHA         string `json:"sha"`
	Content     string `json:"content,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

type tokenResponse struct {
	SHA1 string `json:"sha1"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{cfg: cfg, httpClient: &http.Client{}}
}

func (c *Client) EnsureUser(ctx context.Context, username, email, displayName string, admin bool) (User, error) {
	user, status, err := c.getUser(ctx, username)
	if err != nil && status != http.StatusNotFound {
		return User{}, err
	}
	if status == http.StatusOK {
		return user, nil
	}
	body := map[string]any{
		"email":                email,
		"full_name":            displayName,
		"must_change_password": false,
		"password":             username + "-vinculum!",
		"send_notify":          false,
		"username":             username,
	}
	status, _, err = c.doJSON(ctx, http.MethodPost, "/api/v1/admin/users", body, nil)
	if err != nil {
		return User{}, err
	}
	if status != http.StatusCreated && status != http.StatusConflict && status != http.StatusUnprocessableEntity {
		return User{}, fmt.Errorf("unexpected Forgejo user create status %d", status)
	}
	user, _, err = c.getUser(ctx, username)
	if err != nil {
		return User{}, err
	}
	patchBody := map[string]any{"password": username + "-vinculum!"}
	if admin {
		patchBody["admin"] = true
	}
	status, _, err = c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/admin/users/%s", username), patchBody, nil)
	if err != nil {
		return User{}, err
	}
	if status != http.StatusOK {
		return User{}, fmt.Errorf("unexpected Forgejo user patch status %d", status)
	}
	if admin {
		user, _, err = c.getUser(ctx, username)
		if err != nil {
			return User{}, err
		}
	}
	return user, err
}

func (c *Client) EnsurePublicKey(ctx context.Context, username, title, key string) error {
	keys, err := c.listUserKeys(ctx, username)
	if err == nil {
		for _, existing := range keys {
			if existing.Title == title && strings.TrimSpace(existing.Key) == strings.TrimSpace(key) {
				return nil
			}
			if existing.Title == title {
				if _, _, err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/admin/users/%s/keys/%d", username, existing.ID), nil, nil); err != nil {
					return err
				}
			}
		}
	}
	body := map[string]any{"title": title, "key": key, "read_only": false}
	status, _, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%s/keys", username), body, nil)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") || strings.Contains(err.Error(), "already been used") {
			return nil
		}
		return err
	}
	if status != http.StatusCreated && status != http.StatusConflict && status != http.StatusUnprocessableEntity {
		return fmt.Errorf("unexpected Forgejo key create status %d", status)
	}
	return nil
}

func (c *Client) listUserKeys(ctx context.Context, username string) ([]PublicKey, error) {
	var keys []PublicKey
	_, _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/users/%s/keys", username), nil, &keys)
	return keys, err
}

func (c *Client) EnsureRepository(ctx context.Context, owner, name, description, defaultBranch string, private, autoInit bool) (Repo, error) {
	repo, status, err := c.getRepo(ctx, owner, name)
	if err != nil && status != http.StatusNotFound {
		return Repo{}, err
	}
	if status == http.StatusOK {
		return repo, nil
	}
	body := map[string]any{
		"name":           name,
		"description":    description,
		"private":        private,
		"auto_init":      autoInit,
		"default_branch": defaultBranch,
	}
	status, _, err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/orgs/%s/repos", owner), body, nil)
	if err != nil && (status == http.StatusNotFound || strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(err.Error(), "GetOrgByName")) {
		status, _, err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%s/repos", owner), body, nil)
	}
	if err != nil {
		return Repo{}, err
	}
	if status != http.StatusCreated && status != http.StatusConflict && status != http.StatusUnprocessableEntity {
		return Repo{}, fmt.Errorf("unexpected Forgejo repo create status %d", status)
	}
	repo, _, err = c.getRepo(ctx, owner, name)
	if err != nil {
		return Repo{
			Name:     name,
			FullName: owner + "/" + name,
			CloneURL: fmt.Sprintf("%s/%s/%s.git", c.cfg.ForgejoBaseURL, owner, name),
			SSHURL:   c.SSHCloneURL(owner, name),
		}, nil
	}
	return repo, err
}

func (c *Client) EnsureCollaborator(ctx context.Context, owner, repo, username, permission string) error {
	body := map[string]any{"permission": permission}
	status, _, err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/api/v1/repos/%s/%s/collaborators/%s", owner, repo, username), body, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusCreated {
		return fmt.Errorf("unexpected Forgejo collaborator status %d", status)
	}
	return nil
}

func (c *Client) CreateTokenAsUser(ctx context.Context, username, password, tokenName string) (string, error) {
	body := map[string]any{"name": tokenName, "scopes": []string{"write:repository", "read:repository"}}
	var resp tokenResponse
	status, _, err := c.doJSONWithBasicAuth(ctx, http.MethodPost, fmt.Sprintf("/api/v1/users/%s/tokens", username), username, password, body, &resp)
	if err != nil {
		return "", err
	}
	if status != http.StatusCreated {
		return "", fmt.Errorf("unexpected Forgejo token create status %d", status)
	}
	return resp.SHA1, nil
}

func (c *Client) EnsureRepositoryWebhook(ctx context.Context, owner, repo, callbackURL string) (Webhook, error) {
	hooks, err := c.ListRepositoryWebhooks(ctx, owner, repo)
	if err != nil {
		return Webhook{}, err
	}
	for _, hook := range hooks {
		if hook.Config["url"] == callbackURL {
			return hook, nil
		}
	}
	body := map[string]any{
		"type":   "gitea",
		"active": true,
		"events": []string{"pull_request"},
		"config": map[string]any{
			"url":          callbackURL,
			"content_type": "json",
		},
	}
	var hook Webhook
	status, _, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/hooks", owner, repo), body, &hook)
	if err != nil {
		return Webhook{}, err
	}
	if status != http.StatusCreated {
		return Webhook{}, fmt.Errorf("unexpected Forgejo webhook status %d", status)
	}
	return hook, nil
}

func (c *Client) ListRepositoryWebhooks(ctx context.Context, owner, repo string) ([]Webhook, error) {
	var hooks []Webhook
	_, _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/repos/%s/%s/hooks", owner, repo), nil, &hooks)
	return hooks, err
}

func (c *Client) EnsurePullRequest(ctx context.Context, owner, repo, head, base, title, body string) (PullRequest, error) {
	return c.ensurePullRequest(ctx, owner, repo, head, base, title, body, func(method, path string, in any, out any) (int, []byte, error) {
		return c.doJSON(ctx, method, path, in, out)
	})
}

func (c *Client) EnsurePullRequestWithToken(ctx context.Context, token, owner, repo, head, base, title, body string) (PullRequest, error) {
	return c.ensurePullRequest(ctx, owner, repo, head, base, title, body, func(method, path string, in any, out any) (int, []byte, error) {
		return c.doJSONWithToken(ctx, method, path, token, in, out)
	})
}

func (c *Client) ensurePullRequest(ctx context.Context, owner, repo, head, base, title, body string, do func(method, path string, in any, out any) (int, []byte, error)) (PullRequest, error) {
	pulls, err := c.listPullRequests(ctx, owner, repo, do)
	if err != nil {
		return PullRequest{}, err
	}
	for _, pr := range pulls {
		if pr.State == "open" && pr.Head.Ref == head && pr.Base.Ref == base {
			return pr, nil
		}
	}
	requestBody := map[string]any{
		"head":  head,
		"base":  base,
		"title": title,
		"body":  body,
	}
	var pr PullRequest
	status, _, err := do(http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner, repo), requestBody, &pr)
	if err != nil {
		if status == http.StatusUnprocessableEntity {
			pulls, listErr := c.listPullRequests(ctx, owner, repo, do)
			if listErr == nil {
				for _, existing := range pulls {
					if existing.State == "open" && existing.Head.Ref == head && existing.Base.Ref == base {
						return existing, nil
					}
				}
			}
		}
		return PullRequest{}, err
	}
	if status != http.StatusCreated {
		return PullRequest{}, fmt.Errorf("unexpected Forgejo pull request status %d", status)
	}
	return pr, nil
}

func (c *Client) ListPullRequests(ctx context.Context, owner, repo string) ([]PullRequest, error) {
	return c.listPullRequests(ctx, owner, repo, func(method, path string, in any, out any) (int, []byte, error) {
		return c.doJSON(ctx, method, path, in, out)
	})
}

func (c *Client) listPullRequests(ctx context.Context, owner, repo string, do func(method, path string, in any, out any) (int, []byte, error)) ([]PullRequest, error) {
	var pulls []PullRequest
	_, _, err := do(http.MethodGet, fmt.Sprintf("/api/v1/repos/%s/%s/pulls?state=all", owner, repo), nil, &pulls)
	return pulls, err
}

func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int64) (PullRequest, error) {
	var pr PullRequest
	_, _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", owner, repo, number), nil, &pr)
	return pr, err
}

func (c *Client) MergePullRequest(ctx context.Context, owner, repo string, number int64) (PullRequest, error) {
	body := map[string]any{"Do": "merge", "merge_when_checks_succeed": false}
	var pr PullRequest
	status, _, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge", owner, repo, number), body, &pr)
	if err != nil {
		return PullRequest{}, err
	}
	if status != http.StatusOK {
		return PullRequest{}, fmt.Errorf("unexpected Forgejo merge status %d", status)
	}
	pr, err = c.GetPullRequest(ctx, owner, repo, number)
	if err != nil {
		return PullRequest{}, err
	}
	return pr, nil
}

func (c *Client) GetFile(ctx context.Context, owner, repo, path, ref string) ([]byte, string, error) {
	endpoint := fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", owner, repo, strings.TrimPrefix(path, "/"))
	if strings.TrimSpace(ref) != "" {
		endpoint += "?ref=" + ref
	}
	var entry ContentEntry
	if _, _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &entry); err != nil {
		return nil, "", err
	}
	if entry.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(entry.Content, "\n", ""))
		if err != nil {
			return nil, "", err
		}
		return decoded, entry.SHA, nil
	}
	return []byte(entry.Content), entry.SHA, nil
}

func (c *Client) ListDirectory(ctx context.Context, owner, repo, path, ref string) ([]ContentEntry, error) {
	endpoint := fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", owner, repo, strings.TrimPrefix(path, "/"))
	if strings.TrimSpace(ref) != "" {
		endpoint += "?ref=" + ref
	}
	var entries []ContentEntry
	_, _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &entries)
	return entries, err
}

func (c *Client) UpsertFile(ctx context.Context, owner, repo, path, branch, message string, content []byte) error {
	trimmedPath := strings.TrimPrefix(path, "/")
	body := map[string]any{
		"branch":  branch,
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
	}
	_, sha, err := c.GetFile(ctx, owner, repo, trimmedPath, branch)
	if err == nil && sha != "" {
		body["sha"] = sha
		status, _, updateErr := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", owner, repo, trimmedPath), body, nil)
		if updateErr != nil {
			return updateErr
		}
		if status != http.StatusCreated && status != http.StatusOK {
			return fmt.Errorf("unexpected Forgejo file update status %d", status)
		}
		return nil
	}
	status, _, createErr := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", owner, repo, trimmedPath), body, nil)
	if createErr != nil {
		return createErr
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("unexpected Forgejo file create status %d", status)
	}
	return nil
}

func (c *Client) SSHCloneURL(owner, repo string) string {
	return fmt.Sprintf("ssh://git@%s:%s/%s/%s.git", c.cfg.ForgejoSSHHost, c.cfg.ForgejoSSHPort, owner, repo)
}

func (c *Client) getUser(ctx context.Context, username string) (User, int, error) {
	var user User
	status, _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/users/%s", username), nil, &user)
	return user, status, err
}

func (c *Client) getRepo(ctx context.Context, owner, name string) (Repo, int, error) {
	var repo Repo
	status, _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/repos/%s/%s", owner, name), nil, &repo)
	return repo, status, err
}

func (c *Client) doJSON(ctx context.Context, method, path string, in any, out any) (int, []byte, error) {
	return c.doJSONWithBasicAuth(ctx, method, path, c.cfg.ForgejoAdminUser, c.cfg.ForgejoAdminPass, in, out)
}

func (c *Client) doJSONWithToken(ctx context.Context, method, path, token string, in any, out any) (int, []byte, error) {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.ForgejoBaseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "token "+strings.TrimSpace(token))
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, data, fmt.Errorf("not found")
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, data, fmt.Errorf("forgejo request %s %s failed: %s", method, path, string(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, data, err
		}
	}
	return resp.StatusCode, data, nil
}

func (c *Client) doJSONWithBasicAuth(ctx context.Context, method, path, username, password string, in any, out any) (int, []byte, error) {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.ForgejoBaseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, data, fmt.Errorf("not found")
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, data, fmt.Errorf("forgejo request %s %s failed: %s", method, path, string(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, data, err
		}
	}
	return resp.StatusCode, data, nil
}
