package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/florian/vinculum/apps/vinculum-agent/internal/config"
)

func TestBuildClaudeArgs(t *testing.T) {
	cases := []struct {
		name      string
		model     string
		mcpConfig string
		want      []string
	}{
		{
			name: "defaults-no-model-no-mcp",
			want: []string{"--print", "--dangerously-skip-permissions"},
		},
		{
			name:  "model-only",
			model: "claude-opus-4-7",
			want:  []string{"--print", "--dangerously-skip-permissions", "--model", "claude-opus-4-7"},
		},
		{
			name:      "model-and-mcp",
			model:     "claude-sonnet-4-6",
			mcpConfig: "/etc/vinculum/crush/mcp.json",
			want: []string{
				"--print", "--dangerously-skip-permissions",
				"--model", "claude-sonnet-4-6",
				"--mcp-config", "/etc/vinculum/crush/mcp.json",
			},
		},
		{
			name:      "mcp-only-no-model",
			mcpConfig: "/some/path.json",
			want:      []string{"--print", "--dangerously-skip-permissions", "--mcp-config", "/some/path.json"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildClaudeArgs(tc.model, tc.mcpConfig)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildClaudeArgs(%q, %q)\n  got:  %v\n  want: %v", tc.model, tc.mcpConfig, got, tc.want)
			}
		})
	}
}

func TestClaudeDriverName(t *testing.T) {
	d := NewClaudeDriver(config.Config{}, nil)
	if d.Name() != "claude-code" {
		t.Errorf("Name() = %q, want claude-code", d.Name())
	}
}

func TestClaudeDriverContinueAlwaysFalse(t *testing.T) {
	d := NewClaudeDriver(config.Config{}, nil)
	// claude --print has no session-resume concept, so
	// ContinueRecoverable should always return false regardless of
	// what stderr we feed it. The Executor's first-run fallback path
	// is then a no-op for claude-code.
	probes := []string{
		"",
		"no sessions found to continue",
		"some other error",
		"401 unauthorized",
	}
	for _, p := range probes {
		if d.ContinueRecoverable(p) {
			t.Errorf("ContinueRecoverable(%q) = true, want false", p)
		}
	}
}

// TestCrushDriverContinueRecognition is the symmetric check for crush:
// the "no sessions found to continue" stderr is the one and only signal
// we recover from. If crush ever changes that wording, this test catches
// it before agents start failing in prod.
func TestCrushDriverContinueRecognition(t *testing.T) {
	d := NewCrushDriver(config.Config{}, nil)
	if !d.ContinueRecoverable("crush: no sessions found to continue") {
		t.Errorf("crush should recover from the canonical stderr")
	}
	if d.ContinueRecoverable("some unrelated error") {
		t.Errorf("crush should NOT recover from unrelated stderr")
	}
	if !strings.Contains(d.Name(), "crush") {
		t.Errorf("crush driver Name() should contain 'crush', got %q", d.Name())
	}
}
