package config

import (
	"os"
	"strings"
)

type Config struct {
	AgentDefaultImage string
	// OperatorURL is the in-cluster URL of the operator's HTTP API. Injected
	// into orchestrator-mode agent pods as VINCULUM_OPERATOR_URL so the
	// vinculum MCP server can dispatch Tasks to peer Agents.
	OperatorURL string
}

func Load() Config {
	return Config{
		AgentDefaultImage: envOrDefault("AGENT_DEFAULT_IMAGE", "ghcr.io/florianwenzel/vinculum-agent:latest"),
		OperatorURL:       envOrDefault("OPERATOR_URL", "http://vinculum-operator:8084"),
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
