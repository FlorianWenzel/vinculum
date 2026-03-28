package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/florian/vinculum/apps/vinculum-infra/internal/config"
)

type Client struct {
	cfg        config.KeycloakConfig
	httpClient *http.Client
}

type ForgejoClientInfo struct {
	ClientID string
	Secret   string
	Created  bool
}

type HiveUIClientInfo struct {
	ClientID string
	Created  bool
}

type BootstrapUserInfo struct {
	Username string `json:"username"`
	Group    string `json:"group"`
	Created  bool   `json:"created"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type realmRepresentation struct {
	Realm                 string `json:"realm"`
	Enabled               bool   `json:"enabled"`
	DisplayName           string `json:"displayName,omitempty"`
	LoginWithEmailAllowed bool   `json:"loginWithEmailAllowed,omitempty"`
	ResetPasswordAllowed  bool   `json:"resetPasswordAllowed,omitempty"`
	RegistrationAllowed   bool   `json:"registrationAllowed,omitempty"`
	LoginTheme            string `json:"loginTheme,omitempty"`
}

type clientRepresentation struct {
	ID                        string            `json:"id,omitempty"`
	ClientID                  string            `json:"clientId"`
	Secret                    string            `json:"secret,omitempty"`
	Name                      string            `json:"name,omitempty"`
	Description               string            `json:"description,omitempty"`
	Enabled                   bool              `json:"enabled"`
	Protocol                  string            `json:"protocol"`
	PublicClient              bool              `json:"publicClient"`
	StandardFlowEnabled       bool              `json:"standardFlowEnabled"`
	DirectAccessGrantsEnabled bool              `json:"directAccessGrantsEnabled"`
	RedirectURIs              []string          `json:"redirectUris,omitempty"`
	WebOrigins                []string          `json:"webOrigins,omitempty"`
	BaseURL                   string            `json:"baseUrl,omitempty"`
	RootURL                   string            `json:"rootUrl,omitempty"`
	AdminURL                  string            `json:"adminUrl,omitempty"`
	FrontchannelLogout        bool              `json:"frontchannelLogout"`
	Attributes                map[string]string `json:"attributes,omitempty"`
}

type groupRepresentation struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type userRepresentation struct {
	ID            string   `json:"id,omitempty"`
	Username      string   `json:"username"`
	Enabled       bool     `json:"enabled"`
	Email         string   `json:"email,omitempty"`
	EmailVerified bool     `json:"emailVerified,omitempty"`
	FirstName     string   `json:"firstName,omitempty"`
	LastName      string   `json:"lastName,omitempty"`
	Groups        []string `json:"groups,omitempty"`
}

type credentialRepresentation struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type protocolMapperRepresentation struct {
	ID             string            `json:"id,omitempty"`
	Name           string            `json:"name"`
	Protocol       string            `json:"protocol"`
	ProtocolMapper string            `json:"protocolMapper"`
	Config         map[string]string `json:"config,omitempty"`
}

type secretRepresentation struct {
	Value string `json:"value"`
}

func NewClient(cfg config.KeycloakConfig) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

func (c *Client) EnsureRealm(ctx context.Context) (bool, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return false, err
	}

	realmURL := c.cfg.BaseURL + "/admin/realms/" + c.cfg.Realm
	status, _, err := c.doJSON(ctx, http.MethodGet, realmURL, token, nil, nil)
	if err == nil && status != http.StatusOK && status != http.StatusNotFound {
		return false, fmt.Errorf("unexpected Keycloak realm lookup status %d", status)
	}

	body := c.desiredRealm()
	if status == http.StatusNotFound {
		status, _, err = c.doJSON(ctx, http.MethodPost, c.cfg.BaseURL+"/admin/realms", token, body, nil)
		if err != nil {
			return false, err
		}
		if status != http.StatusCreated {
			return false, fmt.Errorf("unexpected Keycloak realm create status %d", status)
		}

		return true, nil
	}

	status, _, err = c.doJSON(ctx, http.MethodPut, realmURL, token, body, nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusNoContent {
		return false, fmt.Errorf("unexpected Keycloak realm update status %d", status)
	}

	return false, nil
}

func (c *Client) desiredRealm() realmRepresentation {
	return realmRepresentation{
		Realm:                 c.cfg.Realm,
		Enabled:               true,
		DisplayName:           "Vinculum",
		LoginWithEmailAllowed: true,
		ResetPasswordAllowed:  true,
		RegistrationAllowed:   false,
		LoginTheme:            "vinculum",
	}
}

func (c *Client) EnsureForgejoClient(ctx context.Context) (ForgejoClientInfo, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return ForgejoClientInfo{}, err
	}

	lookupURL := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", c.cfg.BaseURL, c.cfg.Realm, url.QueryEscape(c.cfg.ForgejoClientID))
	var existing []clientRepresentation
	status, _, err := c.doJSON(ctx, http.MethodGet, lookupURL, token, nil, &existing)
	if err != nil {
		return ForgejoClientInfo{}, err
	}
	if status != http.StatusOK {
		return ForgejoClientInfo{}, fmt.Errorf("unexpected Keycloak client lookup status %d", status)
	}

	representation := c.desiredForgejoClient()
	created := false
	var clientID string

	if len(existing) == 0 {
		status, _, err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/admin/realms/%s/clients", c.cfg.BaseURL, c.cfg.Realm), token, representation, nil)
		if err != nil {
			return ForgejoClientInfo{}, err
		}
		if status != http.StatusCreated {
			return ForgejoClientInfo{}, fmt.Errorf("unexpected Keycloak client create status %d", status)
		}
		created = true

		status, _, err = c.doJSON(ctx, http.MethodGet, lookupURL, token, nil, &existing)
		if err != nil {
			return ForgejoClientInfo{}, err
		}
		if status != http.StatusOK || len(existing) == 0 {
			return ForgejoClientInfo{}, fmt.Errorf("unable to re-read created Keycloak client")
		}
	}

	clientID = existing[0].ID
	representation.ID = clientID
	status, _, err = c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/admin/realms/%s/clients/%s", c.cfg.BaseURL, c.cfg.Realm, clientID), token, representation, nil)
	if err != nil {
		return ForgejoClientInfo{}, err
	}
	if status != http.StatusNoContent {
		return ForgejoClientInfo{}, fmt.Errorf("unexpected Keycloak client update status %d", status)
	}

	if err := c.ensureGroupsMapper(ctx, token, clientID); err != nil {
		return ForgejoClientInfo{}, err
	}

	var secret secretRepresentation
	status, _, err = c.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/admin/realms/%s/clients/%s/client-secret", c.cfg.BaseURL, c.cfg.Realm, clientID), token, nil, &secret)
	if err != nil {
		return ForgejoClientInfo{}, err
	}
	if status != http.StatusOK {
		return ForgejoClientInfo{}, fmt.Errorf("unexpected Keycloak client secret status %d", status)
	}

	return ForgejoClientInfo{
		ClientID: c.cfg.ForgejoClientID,
		Secret:   secret.Value,
		Created:  created,
	}, nil
}

func (c *Client) EnsureHiveUIClient(ctx context.Context) (HiveUIClientInfo, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return HiveUIClientInfo{}, err
	}

	lookupURL := fmt.Sprintf("%s/admin/realms/%s/clients?clientId=%s", c.cfg.BaseURL, c.cfg.Realm, url.QueryEscape(c.cfg.HiveUIClientID))
	var existing []clientRepresentation
	status, _, err := c.doJSON(ctx, http.MethodGet, lookupURL, token, nil, &existing)
	if err != nil {
		return HiveUIClientInfo{}, err
	}
	if status != http.StatusOK {
		return HiveUIClientInfo{}, fmt.Errorf("unexpected Keycloak hive UI client lookup status %d", status)
	}

	representation := c.desiredHiveUIClient()
	created := false
	var clientID string

	if len(existing) == 0 {
		status, _, err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/admin/realms/%s/clients", c.cfg.BaseURL, c.cfg.Realm), token, representation, nil)
		if err != nil {
			return HiveUIClientInfo{}, err
		}
		if status != http.StatusCreated {
			return HiveUIClientInfo{}, fmt.Errorf("unexpected Keycloak hive UI client create status %d", status)
		}
		created = true

		status, _, err = c.doJSON(ctx, http.MethodGet, lookupURL, token, nil, &existing)
		if err != nil {
			return HiveUIClientInfo{}, err
		}
		if status != http.StatusOK || len(existing) == 0 {
			return HiveUIClientInfo{}, fmt.Errorf("unable to re-read created Keycloak hive UI client")
		}
	}

	clientID = existing[0].ID
	representation.ID = clientID
	status, _, err = c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/admin/realms/%s/clients/%s", c.cfg.BaseURL, c.cfg.Realm, clientID), token, representation, nil)
	if err != nil {
		return HiveUIClientInfo{}, err
	}
	if status != http.StatusNoContent {
		return HiveUIClientInfo{}, fmt.Errorf("unexpected Keycloak hive UI client update status %d", status)
	}

	return HiveUIClientInfo{ClientID: c.cfg.HiveUIClientID, Created: created}, nil
}

func (c *Client) EnsureBootstrapUser(ctx context.Context) (BootstrapUserInfo, error) {
	token, err := c.adminToken(ctx)
	if err != nil {
		return BootstrapUserInfo{}, err
	}

	groupID, createdGroup, err := c.ensureGroup(ctx, token, c.cfg.AdminGroup)
	if err != nil {
		return BootstrapUserInfo{}, err
	}

	userID, createdUser, err := c.ensureUser(ctx, token)
	if err != nil {
		return BootstrapUserInfo{}, err
	}

	if err := c.ensureUserPassword(ctx, token, userID); err != nil {
		return BootstrapUserInfo{}, err
	}
	if err := c.ensureUserInGroup(ctx, token, userID, groupID); err != nil {
		return BootstrapUserInfo{}, err
	}

	return BootstrapUserInfo{
		Username: c.cfg.BootstrapUser,
		Group:    c.cfg.AdminGroup,
		Created:  createdGroup || createdUser,
	}, nil
}

func (c *Client) desiredForgejoClient() clientRepresentation {
	baseURL := ""
	webOrigins := c.cfg.EffectiveWebOrigins()
	redirectURIs := c.cfg.EffectiveRedirectURIs()
	if len(webOrigins) > 0 {
		baseURL = webOrigins[0]
	}

	return clientRepresentation{
		ClientID:                  c.cfg.ForgejoClientID,
		Name:                      "Forgejo",
		Description:               "OIDC client for Forgejo managed by Vinculum.",
		Enabled:                   true,
		Protocol:                  "openid-connect",
		PublicClient:              false,
		StandardFlowEnabled:       true,
		DirectAccessGrantsEnabled: false,
		RedirectURIs:              redirectURIs,
		WebOrigins:                webOrigins,
		BaseURL:                   baseURL,
		RootURL:                   baseURL,
		AdminURL:                  baseURL,
		FrontchannelLogout:        true,
		Attributes: map[string]string{
			"post.logout.redirect.uris": strings.Join(redirectURIs, "##"),
		},
	}
}

func (c *Client) desiredHiveUIClient() clientRepresentation {
	baseURL := ""
	webOrigins := c.cfg.EffectiveHiveUIWebOrigins()
	redirectURIs := c.cfg.EffectiveHiveUIRedirectURIs()
	if len(webOrigins) > 0 {
		baseURL = webOrigins[0]
	}

	return clientRepresentation{
		ClientID:                  c.cfg.HiveUIClientID,
		Name:                      "Hive UI",
		Description:               "OIDC client for the Hive UI managed by Vinculum.",
		Enabled:                   true,
		Protocol:                  "openid-connect",
		Secret:                    c.cfg.HiveUIClientSecret,
		PublicClient:              false,
		StandardFlowEnabled:       true,
		DirectAccessGrantsEnabled: false,
		RedirectURIs:              redirectURIs,
		WebOrigins:                webOrigins,
		BaseURL:                   baseURL,
		RootURL:                   baseURL,
		AdminURL:                  baseURL,
		FrontchannelLogout:        true,
		Attributes: map[string]string{
			"pkce.code.challenge.method": "S256",
			"post.logout.redirect.uris":  strings.Join(redirectURIs, "##"),
		},
	}
}

func (c *Client) ensureGroupsMapper(ctx context.Context, token, clientID string) error {
	endpoint := fmt.Sprintf("%s/admin/realms/%s/clients/%s/protocol-mappers/models", c.cfg.BaseURL, c.cfg.Realm, clientID)
	var existing []protocolMapperRepresentation
	status, _, err := c.doJSON(ctx, http.MethodGet, endpoint, token, nil, &existing)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected Keycloak mapper lookup status %d", status)
	}

	body := protocolMapperRepresentation{
		Name:           "groups",
		Protocol:       "openid-connect",
		ProtocolMapper: "oidc-group-membership-mapper",
		Config: map[string]string{
			"full.path":            "false",
			"id.token.claim":       "true",
			"access.token.claim":   "true",
			"userinfo.token.claim": "true",
			"claim.name":           "groups",
			"jsonType.label":       "String",
		},
	}

	for _, mapper := range existing {
		if mapper.Name != body.Name {
			continue
		}
		body.ID = mapper.ID
		status, _, err = c.doJSON(ctx, http.MethodPut, endpoint+"/"+mapper.ID, token, body, nil)
		if err != nil {
			return err
		}
		if status != http.StatusNoContent {
			return fmt.Errorf("unexpected Keycloak mapper update status %d", status)
		}
		return nil
	}

	status, _, err = c.doJSON(ctx, http.MethodPost, endpoint, token, body, nil)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("unexpected Keycloak mapper create status %d", status)
	}

	return nil
}

func (c *Client) ensureGroup(ctx context.Context, token, groupName string) (string, bool, error) {
	endpoint := fmt.Sprintf("%s/admin/realms/%s/groups?search=%s", c.cfg.BaseURL, c.cfg.Realm, url.QueryEscape(groupName))
	var groups []groupRepresentation
	status, _, err := c.doJSON(ctx, http.MethodGet, endpoint, token, nil, &groups)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusOK {
		return "", false, fmt.Errorf("unexpected Keycloak group lookup status %d", status)
	}
	for _, group := range groups {
		if group.Name == groupName {
			return group.ID, false, nil
		}
	}

	body := groupRepresentation{Name: groupName}
	status, _, err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/admin/realms/%s/groups", c.cfg.BaseURL, c.cfg.Realm), token, body, nil)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusCreated {
		return "", false, fmt.Errorf("unexpected Keycloak group create status %d", status)
	}

	status, _, err = c.doJSON(ctx, http.MethodGet, endpoint, token, nil, &groups)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusOK {
		return "", false, fmt.Errorf("unexpected Keycloak group re-read status %d", status)
	}
	for _, group := range groups {
		if group.Name == groupName {
			return group.ID, true, nil
		}
	}

	return "", false, fmt.Errorf("unable to re-read created Keycloak group %q", groupName)
}

func (c *Client) ensureUser(ctx context.Context, token string) (string, bool, error) {
	endpoint := fmt.Sprintf("%s/admin/realms/%s/users?username=%s&exact=true", c.cfg.BaseURL, c.cfg.Realm, url.QueryEscape(c.cfg.BootstrapUser))
	var users []userRepresentation
	status, _, err := c.doJSON(ctx, http.MethodGet, endpoint, token, nil, &users)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusOK {
		return "", false, fmt.Errorf("unexpected Keycloak user lookup status %d", status)
	}

	body := userRepresentation{
		Username:      c.cfg.BootstrapUser,
		Enabled:       true,
		Email:         c.cfg.BootstrapUser + "@vinculum.local",
		EmailVerified: true,
		FirstName:     "Jean-Luc",
		LastName:      "Picard",
	}

	created := false
	if len(users) == 0 {
		status, _, err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("%s/admin/realms/%s/users", c.cfg.BaseURL, c.cfg.Realm), token, body, nil)
		if err != nil {
			return "", false, err
		}
		if status != http.StatusCreated {
			return "", false, fmt.Errorf("unexpected Keycloak user create status %d", status)
		}
		created = true
		status, _, err = c.doJSON(ctx, http.MethodGet, endpoint, token, nil, &users)
		if err != nil {
			return "", false, err
		}
		if status != http.StatusOK || len(users) == 0 {
			return "", false, fmt.Errorf("unable to re-read created Keycloak user")
		}
	}

	body.ID = users[0].ID
	status, _, err = c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/admin/realms/%s/users/%s", c.cfg.BaseURL, c.cfg.Realm, users[0].ID), token, body, nil)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusNoContent {
		return "", false, fmt.Errorf("unexpected Keycloak user update status %d", status)
	}

	return users[0].ID, created, nil
}

func (c *Client) ensureUserPassword(ctx context.Context, token, userID string) error {
	body := credentialRepresentation{Type: "password", Value: c.cfg.BootstrapPass, Temporary: false}
	status, _, err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/admin/realms/%s/users/%s/reset-password", c.cfg.BaseURL, c.cfg.Realm, userID), token, body, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("unexpected Keycloak password reset status %d", status)
	}
	return nil
}

func (c *Client) ensureUserInGroup(ctx context.Context, token, userID, groupID string) error {
	status, _, err := c.doJSON(ctx, http.MethodPut, fmt.Sprintf("%s/admin/realms/%s/users/%s/groups/%s", c.cfg.BaseURL, c.cfg.Realm, userID, groupID), token, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("unexpected Keycloak group membership status %d", status)
	}
	return nil
}

func (c *Client) adminToken(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("client_id", "admin-cli")
	form.Set("grant_type", "password")
	form.Set("username", c.cfg.AdminUsername)
	form.Set("password", c.cfg.AdminPassword)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/realms/"+c.cfg.AdminRealm+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var token tokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return "", err
	}

	if token.AccessToken == "" {
		return "", fmt.Errorf("keycloak returned an empty access token")
	}

	return token.AccessToken, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint, token string, in any, out any) (int, []byte, error) {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	if resp.StatusCode >= 400 {
		return resp.StatusCode, responseBody, fmt.Errorf("keycloak request %s %s failed with status %d: %s", method, endpoint, resp.StatusCode, string(responseBody))
	}

	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			return resp.StatusCode, responseBody, err
		}
	}

	return resp.StatusCode, responseBody, nil
}
