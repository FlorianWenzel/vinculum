package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/florian/vinculum/apps/vinculum-infra/internal/config"
)

type Client struct {
	cfg        config.ForgejoConfig
	httpClient *http.Client
	kube       kubernetes.Interface
	restConfig *rest.Config
}

type organization struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

type createOrganizationRequest struct {
	Username    string `json:"username"`
	FullName    string `json:"full_name,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Description string `json:"description,omitempty"`
}

type AuthSourceResult struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
	Updated bool   `json:"updated"`
}

type user struct {
	ID       int64  `json:"id"`
	UserName string `json:"login"`
	Email    string `json:"email"`
}

type execResult struct {
	stdout string
	stderr string
}

func NewClient(cfg config.ForgejoConfig) *Client {
	restConfig, kube, err := newKubeClient()
	if err != nil {
		return &Client{
			cfg:        cfg,
			httpClient: &http.Client{},
		}
	}

	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{},
		kube:       kube,
		restConfig: restConfig,
	}
}

func (c *Client) EnsureAdminUser(ctx context.Context) (bool, error) {
	var existing user
	status, _, err := c.doJSON(ctx, http.MethodGet, c.cfg.BaseURL+"/api/v1/users/"+c.cfg.AdminUsername, nil, &existing)
	if err == nil && status == http.StatusOK {
		return false, nil
	}
	if err == nil && status != http.StatusNotFound {
		return false, fmt.Errorf("unexpected Forgejo admin lookup status %d", status)
	}
	body := map[string]any{
		"email":                fmt.Sprintf("%s@vinculum.local", c.cfg.AdminUsername),
		"full_name":            "Vinculum",
		"must_change_password": false,
		"password":             c.cfg.AdminPassword,
		"send_notify":          false,
		"username":             c.cfg.AdminUsername,
	}
	status, _, err = c.doJSON(ctx, http.MethodPost, c.cfg.BaseURL+"/api/v1/admin/users", body, nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusCreated && status != http.StatusConflict && status != http.StatusUnprocessableEntity {
		return false, fmt.Errorf("unexpected Forgejo admin user create status %d", status)
	}
	patchBody := map[string]any{"password": c.cfg.AdminPassword, "admin": true}
	status, _, err = c.doJSON(ctx, http.MethodPatch, c.cfg.BaseURL+"/api/v1/admin/users/"+c.cfg.AdminUsername, patchBody, nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("unexpected Forgejo admin user patch status %d", status)
	}
	return true, nil
}

func (c *Client) EnsureOrganization(ctx context.Context) (bool, error) {
	var existing organization
	status, _, err := c.doJSON(ctx, http.MethodGet, c.cfg.BaseURL+"/api/v1/orgs/"+c.cfg.OrgName, nil, &existing)
	if err == nil && status == http.StatusOK {
		return false, nil
	}
	if err == nil && status != http.StatusNotFound {
		return false, fmt.Errorf("unexpected Forgejo organization lookup status %d", status)
	}

	body := createOrganizationRequest{
		Username:    c.cfg.OrgName,
		FullName:    "Vinculum",
		Visibility:  c.cfg.OrgVisibility,
		Description: "Platform-owned organization managed by Vinculum.",
	}

	status, _, err = c.doJSON(ctx, http.MethodPost, c.cfg.BaseURL+"/api/v1/admin/users/"+c.cfg.AdminUsername+"/orgs", body, nil)
	if err != nil {
		return false, err
	}
	if status == http.StatusUnprocessableEntity && c.cfg.AdminUsername != c.cfg.OrgName {
		migrated, err := c.migrateLegacyUserCollision(ctx)
		if err != nil {
			return false, err
		}
		if migrated {
			status, _, err = c.doJSON(ctx, http.MethodPost, c.cfg.BaseURL+"/api/v1/admin/users/"+c.cfg.AdminUsername+"/orgs", body, nil)
			if err != nil {
				return false, err
			}
		}
	}
	if status != http.StatusCreated {
		return false, fmt.Errorf("unexpected Forgejo organization create status %d", status)
	}

	return true, nil
}

func (c *Client) migrateLegacyUserCollision(ctx context.Context) (bool, error) {
	var existing user
	status, _, err := c.doJSON(ctx, http.MethodGet, c.cfg.BaseURL+"/api/v1/users/"+c.cfg.OrgName, nil, &existing)
	if err == nil && status == http.StatusNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("unexpected Forgejo legacy user lookup status %d", status)
	}
	if existing.UserName != c.cfg.OrgName || existing.UserName == c.cfg.AdminUsername {
		return false, nil
	}

	legacyEmails := map[string]struct{}{
		"admin@vinculum.local":                          {},
		fmt.Sprintf("%s@vinculum.local", c.cfg.OrgName): {},
	}
	if _, ok := legacyEmails[strings.ToLower(existing.Email)]; !ok {
		return false, fmt.Errorf("forgejo organization name %q collides with existing user %q", c.cfg.OrgName, existing.UserName)
	}

	status, _, err = c.doJSON(ctx, http.MethodDelete, c.cfg.BaseURL+"/api/v1/admin/users/"+c.cfg.OrgName, nil, nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return false, fmt.Errorf("unexpected Forgejo legacy user delete status %d", status)
	}

	return true, nil
}

func (c *Client) EnsureOIDCAuthSource(ctx context.Context, issuerURL, clientID, clientSecret, adminGroup string) (AuthSourceResult, error) {
	if c.kube == nil || c.restConfig == nil {
		return AuthSourceResult{}, fmt.Errorf("kubernetes client is not configured for Forgejo auth source reconciliation")
	}

	sourceID, exists, err := c.lookupAuthSource(ctx)
	if err != nil {
		return AuthSourceResult{}, err
	}

	command := []string{
		"forgejo", "admin", "auth",
	}
	if exists {
		command = append(command, "update-oauth", "--id", strconv.Itoa(sourceID))
	} else {
		command = append(command, "add-oauth")
	}

	command = append(command,
		"--name", c.cfg.AuthSourceName,
		"--provider", "openidConnect",
		"--key", clientID,
		"--secret", clientSecret,
		"--auto-discover-url", issuerURL,
		"--group-claim-name", "groups",
		"--admin-group", adminGroup,
		"--skip-local-2fa",
		"--scopes", "openid",
		"--scopes", "profile",
		"--scopes", "email",
	)

	result, err := c.execInForgejoPod(ctx, command)
	if err != nil {
		return AuthSourceResult{}, err
	}
	if strings.TrimSpace(result.stderr) != "" {
		return AuthSourceResult{}, fmt.Errorf("forgejo auth command reported stderr: %s", strings.TrimSpace(result.stderr))
	}

	return AuthSourceResult{
		Name:    c.cfg.AuthSourceName,
		Created: !exists,
		Updated: exists,
	}, nil
}

func (c *Client) lookupAuthSource(ctx context.Context) (int, bool, error) {
	result, err := c.execInForgejoPod(ctx, []string{"forgejo", "admin", "auth", "list"})
	if err != nil {
		return 0, false, err
	}

	for _, line := range strings.Split(result.stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] == "ID" {
			continue
		}
		if fields[1] != c.cfg.AuthSourceName {
			continue
		}

		id, err := strconv.Atoi(fields[0])
		if err != nil {
			return 0, false, fmt.Errorf("invalid Forgejo auth source id %q", fields[0])
		}

		return id, true, nil
	}

	return 0, false, nil
}

func (c *Client) execInForgejoPod(ctx context.Context, command []string) (execResult, error) {
	podName, err := c.findForgejoPod(ctx)
	if err != nil {
		return execResult{}, err
	}

	req := c.kube.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(c.cfg.PodNamespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "forgejo",
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, http.MethodPost, req.URL())
	if err != nil {
		return execResult{}, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return execResult{}, fmt.Errorf("forgejo exec failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return execResult{stdout: stdout.String(), stderr: stderr.String()}, nil
}

func (c *Client) findForgejoPod(ctx context.Context) (string, error) {
	pods, err := c.kube.CoreV1().Pods(c.cfg.PodNamespace).List(ctx, metav1.ListOptions{LabelSelector: c.cfg.PodLabelSelector})
	if err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no running Forgejo pod found with selector %q in namespace %q", c.cfg.PodLabelSelector, c.cfg.PodNamespace)
}

func newKubeClient() (*rest.Config, kubernetes.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := clientConfig.ClientConfig()
	if err != nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, nil, err
		}
	}

	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	return config, kube, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, in any, out any) (int, []byte, error) {
	status, body, err := c.doJSONWithBasicAuth(ctx, method, endpoint, c.cfg.AdminUsername, c.cfg.AdminPassword, in, out)
	if (status == http.StatusUnauthorized || status == http.StatusForbidden) && (c.cfg.AdminUsername != "forgejo_admin" || c.cfg.AdminPassword != "forgejo_admin") {
		return c.doJSONWithBasicAuth(ctx, method, endpoint, "forgejo_admin", "forgejo_admin", in, out)
	}
	return status, body, err
}

func (c *Client) doJSONWithBasicAuth(ctx context.Context, method, endpoint, username, password string, in any, out any) (int, []byte, error) {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
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

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, responseBody, nil
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, responseBody, fmt.Errorf("forgejo request %s %s failed with status %d: %s", method, endpoint, resp.StatusCode, string(responseBody))
	}

	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			return resp.StatusCode, responseBody, err
		}
	}

	return resp.StatusCode, responseBody, nil
}
