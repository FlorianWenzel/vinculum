package config

import (
	"os"
	"strings"
)

type Config struct {
	AgentDefaultImage string
}

func Load() Config {
	return Config{
		AgentDefaultImage: envOrDefault("AGENT_DEFAULT_IMAGE", "ghcr.io/florianwenzel/vinculum-agent:latest"),
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
