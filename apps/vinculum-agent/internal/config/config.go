package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	AgentName     string
	Namespace     string
	ServerAddr    string
	WorkspaceRoot string
	DataDir       string

	CrushModel string
	CrushQuiet bool
	CrushDebug bool

	InstructionFile string
}

func Load() Config {
	workspace := envOrDefault("WORKSPACE_ROOT", "/workspace")
	dataDir := envOrDefault("XDG_DATA_HOME", filepath.Join(workspace, ".crush-data"))
	return Config{
		AgentName:     strings.TrimSpace(os.Getenv("AGENT_NAME")),
		Namespace:     strings.TrimSpace(os.Getenv("AGENT_NAMESPACE")),
		ServerAddr:    envOrDefault("SERVER_ADDR", ":8090"),
		WorkspaceRoot: workspace,
		DataDir:       dataDir,

		CrushModel: strings.TrimSpace(os.Getenv("CRUSH_MODEL")),
		CrushQuiet: strings.EqualFold(envOrDefault("CRUSH_QUIET", "true"), "true"),
		CrushDebug: strings.EqualFold(envOrDefault("CRUSH_DEBUG", "false"), "true"),

		InstructionFile: strings.TrimSpace(os.Getenv("INSTRUCTION_FILE")),
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
