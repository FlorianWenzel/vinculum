package bootstrap

import (
	"context"
	"log"
	"time"

	"github.com/florian/vinculum/apps/vinculum-infra/internal/config"
	"github.com/florian/vinculum/apps/vinculum-infra/internal/forgejo"
	"github.com/florian/vinculum/apps/vinculum-infra/internal/keycloak"
)

type Service struct {
	logger   *log.Logger
	keycloak *keycloak.Client
	forgejo  *forgejo.Client
	cfg      config.Config
}

type Result struct {
	Timestamp time.Time      `json:"timestamp"`
	Keycloak  KeycloakResult `json:"keycloak"`
	Forgejo   ForgejoResult  `json:"forgejo"`
	Notes     []string       `json:"notes"`
}

type KeycloakResult struct {
	Realm         string   `json:"realm"`
	RealmCreated  bool     `json:"realmCreated"`
	ClientID      string   `json:"clientId"`
	ClientCreated bool     `json:"clientCreated"`
	ClientSecret  string   `json:"clientSecret"`
	IssuerURL     string   `json:"issuerUrl"`
	RedirectURIs  []string `json:"redirectUris"`
	AdminGroup    string   `json:"adminGroup"`
	BootstrapUser string   `json:"bootstrapUser"`
	UserCreated   bool     `json:"userCreated"`
}

type ForgejoResult struct {
	BaseURL             string `json:"baseUrl"`
	PublicURL           string `json:"publicUrl"`
	Organization        string `json:"organization"`
	OrganizationCreated bool   `json:"organizationCreated"`
	AuthSourceName      string `json:"authSourceName"`
	AuthSourceCreated   bool   `json:"authSourceCreated"`
	AuthSourceUpdated   bool   `json:"authSourceUpdated"`
	AdminUsername       string `json:"adminUsername"`
}

func NewService(logger *log.Logger, keycloakClient *keycloak.Client, forgejoClient *forgejo.Client, cfg config.Config) *Service {
	return &Service{
		logger:   logger,
		keycloak: keycloakClient,
		forgejo:  forgejoClient,
		cfg:      cfg,
	}
}

func (s *Service) Bootstrap(ctx context.Context) (Result, error) {
	realmCreated, err := s.keycloak.EnsureRealm(ctx)
	if err != nil {
		return Result{}, err
	}

	clientInfo, err := s.keycloak.EnsureForgejoClient(ctx)
	if err != nil {
		return Result{}, err
	}

	bootstrapUser, err := s.keycloak.EnsureBootstrapUser(ctx)
	if err != nil {
		return Result{}, err
	}

	_, err = s.forgejo.EnsureAdminUser(ctx)
	if err != nil {
		return Result{}, err
	}

	orgCreated, err := s.forgejo.EnsureOrganization(ctx)
	if err != nil {
		return Result{}, err
	}

	authSource, err := s.forgejo.EnsureOIDCAuthSource(ctx, s.cfg.Keycloak.DiscoveryURL(), clientInfo.ClientID, clientInfo.Secret, s.cfg.Keycloak.AdminGroup)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Timestamp: time.Now().UTC(),
		Keycloak: KeycloakResult{
			Realm:         s.cfg.Keycloak.Realm,
			RealmCreated:  realmCreated,
			ClientID:      clientInfo.ClientID,
			ClientCreated: clientInfo.Created,
			ClientSecret:  clientInfo.Secret,
			IssuerURL:     s.cfg.Keycloak.RealmIssuerURL(),
			RedirectURIs:  s.cfg.Keycloak.EffectiveRedirectURIs(),
			AdminGroup:    bootstrapUser.Group,
			BootstrapUser: bootstrapUser.Username,
			UserCreated:   bootstrapUser.Created,
		},
		Forgejo: ForgejoResult{
			BaseURL:             s.cfg.Forgejo.BaseURL,
			PublicURL:           s.cfg.Forgejo.PublicURL,
			Organization:        s.cfg.Forgejo.OrgName,
			OrganizationCreated: orgCreated,
			AuthSourceName:      authSource.Name,
			AuthSourceCreated:   authSource.Created,
			AuthSourceUpdated:   authSource.Updated,
			AdminUsername:       s.cfg.Forgejo.AdminUsername,
		},
		Notes: []string{
			"The service reconciles the Keycloak realm, themed login, OIDC client, bootstrap admin user, Forgejo organization, and Forgejo OIDC login source.",
			"Configured OIDC URLs stay primary, and localhost dev URLs are added automatically for local browser flows.",
		},
	}

	s.logger.Printf("bootstrap finished: realm=%s client=%s org=%s auth_source=%s", result.Keycloak.Realm, result.Keycloak.ClientID, result.Forgejo.Organization, result.Forgejo.AuthSourceName)

	return result, nil
}
