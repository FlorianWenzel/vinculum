package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ServerAddr        string
	WorkDir           string
	InstructionsDir   string
	OpenCodeHost      string
	OpenCodePort      string
	OpenCodeModel     string
	OpenCodeAgent     string
	OpenCodeLogLevel  string
	AutoRunPrompt     string
	AutoRunDir        string
	AutoRunWithDocs   bool
	TaskRepoURL       string
	TaskBaseBranch    string
	TaskWorkingBranch string
	GitAuthorName     string
	GitAuthorEmail    string
	AutoCommitMessage string
}

func Load() Config {
	workDir := envOrDefault("WORK_DIR", "/workspace")
	return Config{
		ServerAddr:        envOrDefault("SERVER_ADDR", ":8090"),
		WorkDir:           workDir,
		InstructionsDir:   envOrDefault("INSTRUCTIONS_DIR", filepath.Join(workDir, "instructions")),
		OpenCodeHost:      envOrDefault("OPENCODE_HOST", "127.0.0.1"),
		OpenCodePort:      envOrDefault("OPENCODE_PORT", "4096"),
		OpenCodeModel:     strings.TrimSpace(os.Getenv("OPENCODE_MODEL")),
		OpenCodeAgent:     strings.TrimSpace(os.Getenv("OPENCODE_AGENT")),
		OpenCodeLogLevel:  envOrDefault("OPENCODE_LOG_LEVEL", "INFO"),
		AutoRunPrompt:     strings.TrimSpace(os.Getenv("AUTO_RUN_PROMPT")),
		AutoRunDir:        strings.TrimSpace(os.Getenv("AUTO_RUN_DIR")),
		AutoRunWithDocs:   strings.EqualFold(envOrDefault("AUTO_RUN_WITH_INSTRUCTIONS", "false"), "true"),
		TaskRepoURL:       strings.TrimSpace(os.Getenv("TASKRUN_REPO_URL")),
		TaskBaseBranch:    envOrDefault("TASKRUN_BASE_BRANCH", "main"),
		TaskWorkingBranch: strings.TrimSpace(os.Getenv("TASKRUN_WORKING_BRANCH")),
		GitAuthorName:     envOrDefault("GIT_AUTHOR_NAME", "Vinculum Drone"),
		GitAuthorEmail:    envOrDefault("GIT_AUTHOR_EMAIL", "drone@vinculum.local"),
		AutoCommitMessage: envOrDefault("AUTO_COMMIT_MESSAGE", "Implement requested changes"),
	}
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
