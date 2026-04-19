package settings

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const SecretName = "vinculum-settings"

// Settings holds all user-configurable runtime settings.
type Settings struct {
	OpenCodeAPIKey string `json:"openCodeApiKey"`
	OpenCodeModel  string `json:"openCodeModel"`
	OpenCodeBaseURL string `json:"openCodeBaseURL"`
}

const defaultModel   = "gpt-4o"
const defaultBaseURL = "https://api.openai.com/v1"

// Store reads and writes settings from a Kubernetes Secret, with an
// in-memory cache so callers don't hit the API on every request.
type Store struct {
	k8s       client.Client
	namespace string

	mu    sync.RWMutex
	cache *Settings
}

func NewStore(k8s client.Client, namespace string) *Store {
	return &Store{k8s: k8s, namespace: namespace}
}

// Get returns the cached settings, loading from K8s if the cache is cold.
func (s *Store) Get(ctx context.Context) Settings {
	s.mu.RLock()
	if s.cache != nil {
		c := *s.cache
		s.mu.RUnlock()
		return c
	}
	s.mu.RUnlock()

	// Cache miss — load from K8s (ignore errors, return empty).
	loaded, _ := s.Load(ctx)
	return loaded
}

// Load fetches settings from the K8s Secret and updates the cache.
func (s *Store) Load(ctx context.Context) (Settings, error) {
	var secret corev1.Secret
	err := s.k8s.Get(ctx, client.ObjectKey{Name: SecretName, Namespace: s.namespace}, &secret)
	if apierrors.IsNotFound(err) {
		cfg := Settings{OpenCodeModel: defaultModel, OpenCodeBaseURL: defaultBaseURL}
		s.setCache(cfg)
		return cfg, nil
	}
	if err != nil {
		return Settings{OpenCodeModel: defaultModel}, err
	}

	cfg := Settings{
		OpenCodeAPIKey:  string(secret.Data["opencode-api-key"]),
		OpenCodeModel:   string(secret.Data["opencode-model"]),
		OpenCodeBaseURL: string(secret.Data["opencode-base-url"]),
	}
	if cfg.OpenCodeModel == "" {
		cfg.OpenCodeModel = defaultModel
	}
	if cfg.OpenCodeBaseURL == "" {
		cfg.OpenCodeBaseURL = defaultBaseURL
	}
	s.setCache(cfg)
	return cfg, nil
}

// Save persists settings to the K8s Secret and updates the cache.
func (s *Store) Save(ctx context.Context, cfg Settings) error {
	if cfg.OpenCodeModel == "" {
		cfg.OpenCodeModel = defaultModel
	}

	ns := s.namespace
	if ns == "" {
		ns = "default"
	}

	data := map[string][]byte{
		"opencode-api-key":  []byte(cfg.OpenCodeAPIKey),
		"opencode-model":    []byte(cfg.OpenCodeModel),
		"opencode-base-url": []byte(cfg.OpenCodeBaseURL),
	}

	var existing corev1.Secret
	err := s.k8s.Get(ctx, client.ObjectKey{Name: SecretName, Namespace: ns}, &existing)
	if apierrors.IsNotFound(err) {
		obj := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: SecretName, Namespace: ns},
			Type:       corev1.SecretTypeOpaque,
			Data:       data,
		}
		if err := s.k8s.Create(ctx, obj); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		existing.Data = data
		if err := s.k8s.Update(ctx, &existing); err != nil {
			return err
		}
	}

	s.setCache(cfg)
	return nil
}

func (s *Store) setCache(cfg Settings) {
	s.mu.Lock()
	s.cache = &cfg
	s.mu.Unlock()
}
