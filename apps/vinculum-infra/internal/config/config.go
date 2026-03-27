package config

import (
	"net/url"
	"os"
	"slices"
	"strings"
)

type Config struct {
	ServerAddr    string
	AutoBootstrap bool
	Keycloak      KeycloakConfig
	Forgejo       ForgejoConfig
}

type KeycloakConfig struct {
	BaseURL         string
	IssuerURL       string
	AdminRealm      string
	AdminUsername   string
	AdminPassword   string
	Realm           string
	AdminGroup      string
	BootstrapUser   string
	BootstrapPass   string
	ForgejoClientID string
	RedirectURIs    []string
	WebOrigins      []string
}

type ForgejoConfig struct {
	BaseURL          string
	PublicURL        string
	AdminUsername    string
	AdminPassword    string
	OrgName          string
	OrgVisibility    string
	AuthSourceName   string
	PodNamespace     string
	PodLabelSelector string
}

func Load() Config {
	return Config{
		ServerAddr:    envOrDefault("SERVER_ADDR", ":8081"),
		AutoBootstrap: envOrDefault("AUTO_BOOTSTRAP", "false") == "true",
		Keycloak: KeycloakConfig{
			BaseURL:         normalizeBaseURL(envOrDefault("KEYCLOAK_BASE_URL", "http://localhost:8080")),
			IssuerURL:       normalizeBaseURL(envOrDefault("KEYCLOAK_ISSUER_URL", "http://localhost:8080/realms/vinculum")),
			AdminRealm:      envOrDefault("KEYCLOAK_ADMIN_REALM", "master"),
			AdminUsername:   envOrDefault("KEYCLOAK_ADMIN_USERNAME", "admin"),
			AdminPassword:   envOrDefault("KEYCLOAK_ADMIN_PASSWORD", "admin"),
			Realm:           envOrDefault("KEYCLOAK_REALM", "vinculum"),
			AdminGroup:      envOrDefault("KEYCLOAK_FORGEJO_ADMIN_GROUP", "forgejo_admins"),
			BootstrapUser:   envOrDefault("KEYCLOAK_BOOTSTRAP_USERNAME", "picard"),
			BootstrapPass:   envOrDefault("KEYCLOAK_BOOTSTRAP_PASSWORD", "picard"),
			ForgejoClientID: envOrDefault("KEYCLOAK_FORGEJO_CLIENT_ID", "forgejo"),
			RedirectURIs:    splitCSV(envOrDefault("KEYCLOAK_FORGEJO_REDIRECT_URIS", "http://localhost:3000/user/oauth2/*")),
			WebOrigins:      splitCSV(envOrDefault("KEYCLOAK_FORGEJO_WEB_ORIGINS", "http://localhost:3000")),
		},
		Forgejo: ForgejoConfig{
			BaseURL:          normalizeBaseURL(envOrDefault("FORGEJO_BASE_URL", "http://localhost:3000")),
			PublicURL:        normalizeBaseURL(envOrDefault("FORGEJO_PUBLIC_URL", "http://localhost:3000")),
			AdminUsername:    envOrDefault("FORGEJO_ADMIN_USERNAME", "vinculum"),
			AdminPassword:    envOrDefault("FORGEJO_ADMIN_PASSWORD", "vinculum"),
			OrgName:          envOrDefault("FORGEJO_ORG_NAME", "vinculum"),
			OrgVisibility:    envOrDefault("FORGEJO_ORG_VISIBILITY", "private"),
			AuthSourceName:   envOrDefault("FORGEJO_AUTH_SOURCE_NAME", "Vinculum"),
			PodNamespace:     envOrDefault("FORGEJO_POD_NAMESPACE", "vinculum-system"),
			PodLabelSelector: envOrDefault("FORGEJO_POD_LABEL_SELECTOR", "app.kubernetes.io/instance=vinculum-infra,app.kubernetes.io/name=forgejo"),
		},
	}
}

func (c KeycloakConfig) RealmIssuerURL() string {
	if c.IssuerURL != "" {
		return c.IssuerURL
	}

	return c.BaseURL + "/realms/" + c.Realm
}

func (c KeycloakConfig) DiscoveryURL() string {
	return c.RealmIssuerURL() + "/.well-known/openid-configuration"
}

func (c KeycloakConfig) EffectiveRedirectURIs() []string {
	return appendDevForgejoURLs(c.RedirectURIs, "/user/oauth2/*")
}

func (c KeycloakConfig) EffectiveWebOrigins() []string {
	return appendDevForgejoURLs(c.WebOrigins, "")
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}

	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}

	return items
}

func normalizeBaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimRight(value, "/")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

func appendDevForgejoURLs(values []string, suffix string) []string {
	merged := append([]string(nil), values...)

	for _, candidate := range []string{
		"http://localhost:3000",
		"https://localhost:3000",
	} {
		full := candidate + suffix
		if !slices.Contains(merged, full) {
			merged = append(merged, full)
		}
	}

	return merged
}
