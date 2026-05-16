package controllers

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func findEnv(envs []corev1.EnvVar, name string) (string, bool) {
	for _, e := range envs {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

// TestOrchestratorEnvInjected verifies that flipping spec.orchestrator to true
// causes the agent Deployment to gain VINCULUM_OPERATOR_URL, and that
// non-orchestrator agents do not get the var.
func TestOrchestratorEnvInjected(t *testing.T) {
	scheme := newScheme(t)

	cases := []struct {
		name         string
		orchestrator bool
		operatorURL  string
		wantPresent  bool
		wantValue    string
	}{
		{name: "non-orchestrator-omits-url", orchestrator: false, operatorURL: "http://vinculum-operator:8084", wantPresent: false},
		{name: "orchestrator-injects-url", orchestrator: true, operatorURL: "http://vinculum-operator.vinculum.svc:8084", wantPresent: true, wantValue: "http://vinculum-operator.vinculum.svc:8084"},
		{name: "orchestrator-empty-url-skips", orchestrator: true, operatorURL: "", wantPresent: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &v1alpha1.Agent{}
			agent.Name = "locutus"
			agent.Namespace = "vinculum"
			agent.Spec = v1alpha1.AgentSpec{
				Image:        "ghcr.io/florianwenzel/vinculum-agent:test",
				Enabled:      true,
				Orchestrator: tc.orchestrator,
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(agent).Build()
			r := &AgentReconciler{Client: c, Scheme: scheme, Cfg: AgentReconcilerConfig{
				AgentDefaultImage: "ghcr.io/florianwenzel/vinculum-agent:test",
				OperatorURL:       tc.operatorURL,
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
			containers := dep.Spec.Template.Spec.Containers
			if len(containers) != 1 {
				t.Fatalf("want 1 container, got %d", len(containers))
			}

			// AGENT_NAME and AGENT_NAMESPACE should always be set; the MCP
			// server depends on them at runtime.
			if v, ok := findEnv(containers[0].Env, "AGENT_NAME"); !ok || v != "locutus" {
				t.Errorf("AGENT_NAME wrong: %q present=%v", v, ok)
			}
			if v, ok := findEnv(containers[0].Env, "AGENT_NAMESPACE"); !ok || v != "vinculum" {
				t.Errorf("AGENT_NAMESPACE wrong: %q present=%v", v, ok)
			}

			got, present := findEnv(containers[0].Env, "VINCULUM_OPERATOR_URL")
			if present != tc.wantPresent {
				t.Errorf("VINCULUM_OPERATOR_URL present=%v, want %v", present, tc.wantPresent)
			}
			if present && got != tc.wantValue {
				t.Errorf("VINCULUM_OPERATOR_URL=%q, want %q", got, tc.wantValue)
			}
		})
	}
}
