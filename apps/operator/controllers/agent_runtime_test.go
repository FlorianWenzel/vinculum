package controllers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
)

// TestRuntimeEnvSetFromSpec verifies the operator injects AGENT_RUNTIME
// (and, for claude-code, CLAUDE_MCP_CONFIG) into the agent pod env from
// Agent.spec.runtime.
func TestRuntimeEnvSetFromSpec(t *testing.T) {
	scheme := newScheme(t)
	cases := []struct {
		name              string
		runtime           v1alpha1.AgentRuntime
		wantRuntime       string
		wantClaudeMCPCfg  bool
		wantClaudeMCPPath string
	}{
		{name: "default-empty-is-crush", runtime: "", wantRuntime: "crush", wantClaudeMCPCfg: false},
		{name: "explicit-crush", runtime: v1alpha1.RuntimeCrush, wantRuntime: "crush", wantClaudeMCPCfg: false},
		{name: "claude-code", runtime: v1alpha1.RuntimeClaudeCode, wantRuntime: "claude-code", wantClaudeMCPCfg: true, wantClaudeMCPPath: "/etc/vinculum/crush/mcp.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &v1alpha1.Agent{}
			agent.Name = "tester"
			agent.Namespace = "vinculum"
			agent.Spec = v1alpha1.AgentSpec{
				Image:   "ghcr.io/florianwenzel/vinculum-agent:test",
				Enabled: true,
				Runtime: tc.runtime,
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(agent).Build()
			r := &AgentReconciler{Client: c, Scheme: scheme, Cfg: AgentReconcilerConfig{
				AgentDefaultImage: "ghcr.io/florianwenzel/vinculum-agent:test",
			}}

			names := resourceNames(agent.Name)
			if _, err := r.ensurePVC(context.Background(), agent, names); err != nil {
				t.Fatal(err)
			}
			if _, _, err := r.ensureDeployment(context.Background(), agent, names, names.PVC, "hash", nil); err != nil {
				t.Fatal(err)
			}

			var dep appsv1.Deployment
			if err := c.Get(context.Background(), types.NamespacedName{Name: names.Deployment, Namespace: agent.Namespace}, &dep); err != nil {
				t.Fatal(err)
			}
			env := dep.Spec.Template.Spec.Containers[0].Env

			got, ok := findEnv(env, "AGENT_RUNTIME")
			if !ok {
				t.Fatalf("AGENT_RUNTIME not set")
			}
			if got != tc.wantRuntime {
				t.Errorf("AGENT_RUNTIME=%q, want %q", got, tc.wantRuntime)
			}

			mcpPath, mcpSet := findEnv(env, "CLAUDE_MCP_CONFIG")
			if mcpSet != tc.wantClaudeMCPCfg {
				t.Errorf("CLAUDE_MCP_CONFIG set=%v, want %v (value=%q)", mcpSet, tc.wantClaudeMCPCfg, mcpPath)
			}
			if tc.wantClaudeMCPCfg && mcpPath != tc.wantClaudeMCPPath {
				t.Errorf("CLAUDE_MCP_CONFIG=%q, want %q", mcpPath, tc.wantClaudeMCPPath)
			}
		})
	}
}

// TestRenderClaudeMCPConfig asserts the mcp.json shape claude-code
// expects (mcpServers map keyed by name, each with command/args/env).
// Disabled or HTTP-typed entries are skipped.
func TestRenderClaudeMCPConfig(t *testing.T) {
	resolved := []resolvedMCP{
		{Name: "vinculum", Command: "/usr/local/bin/vnclm-mcp", Args: []string{"serve"}, Env: map[string]string{"K": "v"}, Enabled: true},
		{Name: "disabled-one", Command: "/usr/local/bin/x", Enabled: false},
		{Name: "http-only", URL: "http://example.test", Enabled: true},
	}
	out, err := renderClaudeMCPConfig(resolved)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	servers, _ := parsed["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatalf("missing mcpServers key: %s", out)
	}
	if _, ok := servers["vinculum"]; !ok {
		t.Errorf("expected 'vinculum' entry, got %v", servers)
	}
	if _, ok := servers["disabled-one"]; ok {
		t.Errorf("disabled entry should be omitted, got %v", servers)
	}
	if _, ok := servers["http-only"]; ok {
		t.Errorf("http-only entry should be omitted (stdio-only schema), got %v", servers)
	}
	vinculum, _ := servers["vinculum"].(map[string]any)
	if vinculum["command"] != "/usr/local/bin/vnclm-mcp" {
		t.Errorf("wrong command: %v", vinculum["command"])
	}
}

// TestConfigMapKeysByRuntime verifies the ConfigMap holds crush.json
// when runtime=crush and mcp.json when runtime=claude-code (never both).
func TestConfigMapKeysByRuntime(t *testing.T) {
	scheme := newScheme(t)
	cases := []struct {
		name        string
		runtime     v1alpha1.AgentRuntime
		wantKey     string
		dontWantKey string
	}{
		{name: "crush-writes-crush-json", runtime: v1alpha1.RuntimeCrush, wantKey: "crush.json", dontWantKey: "mcp.json"},
		{name: "claude-writes-mcp-json", runtime: v1alpha1.RuntimeClaudeCode, wantKey: "mcp.json", dontWantKey: "crush.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &v1alpha1.Agent{}
			agent.Name = "cfg-test"
			agent.Namespace = "vinculum"
			agent.Spec = v1alpha1.AgentSpec{
				Image:   "ghcr.io/florianwenzel/vinculum-agent:test",
				Enabled: true,
				Runtime: tc.runtime,
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(agent).Build()
			r := &AgentReconciler{Client: c, Scheme: scheme}
			names := resourceNames(agent.Name)
			if _, err := r.ensureConfigMap(context.Background(), agent, names, nil); err != nil {
				t.Fatal(err)
			}
			var cm corev1.ConfigMap
			if err := c.Get(context.Background(), types.NamespacedName{Name: names.ConfigMap, Namespace: agent.Namespace}, &cm); err != nil {
				t.Fatal(err)
			}
			if _, ok := cm.Data[tc.wantKey]; !ok {
				t.Errorf("ConfigMap missing %q, have keys %v", tc.wantKey, mapKeys(cm.Data))
			}
			if _, ok := cm.Data[tc.dontWantKey]; ok {
				t.Errorf("ConfigMap should not contain %q for runtime=%q, but it does (data: %v)", tc.dontWantKey, tc.runtime, mapKeys(cm.Data))
			}
		})
	}
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestEffectiveRuntimeDefault covers the helper directly so other tests
// can rely on it without exercising the controller plumbing.
func TestEffectiveRuntimeDefault(t *testing.T) {
	a := &v1alpha1.Agent{}
	if got := a.EffectiveRuntime(); got != v1alpha1.RuntimeCrush {
		t.Errorf("EffectiveRuntime on empty spec = %q, want %q", got, v1alpha1.RuntimeCrush)
	}
	a.Spec.Runtime = v1alpha1.RuntimeClaudeCode
	if got := a.EffectiveRuntime(); got != v1alpha1.RuntimeClaudeCode {
		t.Errorf("EffectiveRuntime with claude-code spec = %q, want %q", got, v1alpha1.RuntimeClaudeCode)
	}
	if !strings.HasPrefix(string(a.EffectiveRuntime()), "claude") {
		t.Errorf("sanity: claude runtime string changed?")
	}
}
