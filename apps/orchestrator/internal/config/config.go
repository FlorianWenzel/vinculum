package config

import (
	"os"
	"strings"
)

type Config struct {
	ForgejoBaseURL    string
	ForgejoAdminUser  string
	ForgejoAdminPass  string
	ForgejoSSHHost    string
	ForgejoSSHPort    string
	DroneDefaultImage string
	WebhookURL        string
}

func Load() Config {
	return Config{
		ForgejoBaseURL:    envOrDefault("FORGEJO_BASE_URL", "http://vinculum-infra-forgejo-http.vinculum-system.svc.cluster.local:3000"),
		ForgejoAdminUser:  envOrDefault("FORGEJO_ADMIN_USERNAME", "hive_queen"),
		ForgejoAdminPass:  envOrDefault("FORGEJO_ADMIN_PASSWORD", "hive_queen"),
		ForgejoSSHHost:    envOrDefault("FORGEJO_SSH_HOST", "vinculum-infra-forgejo-ssh.vinculum-system.svc.cluster.local"),
		ForgejoSSHPort:    envOrDefault("FORGEJO_SSH_PORT", "22"),
		DroneDefaultImage: envOrDefault("DRONE_DEFAULT_IMAGE", "ttl.sh/vinculum-agent:12h"),
		WebhookURL:        envOrDefault("ORCHESTRATOR_WEBHOOK_URL", "http://orchestrator-orchestrator.vinculum-system.svc.cluster.local:8084/api/webhooks/forgejo"),
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
