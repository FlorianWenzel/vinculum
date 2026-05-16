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

func findContainer(containers []corev1.Container, name string) (*corev1.Container, bool) {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i], true
		}
	}
	return nil, false
}

func findVolume(volumes []corev1.Volume, name string) (*corev1.Volume, bool) {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i], true
		}
	}
	return nil, false
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

// TestRepoInitContainerInjected covers the matrix of "should a git-clone init
// container be added, with the right env vars and mounts" for every
// permutation of Repo / GitCredentials we accept on AgentSpec.
func TestRepoInitContainerInjected(t *testing.T) {
	scheme := newScheme(t)

	build := func(t *testing.T, agentSpec v1alpha1.AgentSpec) *appsv1.Deployment {
		t.Helper()
		agentSpec.Image = "ghcr.io/florianwenzel/vinculum-agent:test"
		agentSpec.Enabled = true
		ag := &v1alpha1.Agent{}
		ag.Name = "locutus"
		ag.Namespace = "vinculum"
		ag.Spec = agentSpec

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ag).Build()
		r := &AgentReconciler{Client: c, Scheme: scheme, Cfg: AgentReconcilerConfig{
			AgentDefaultImage: "ghcr.io/florianwenzel/vinculum-agent:test",
		}}
		names := resourceNames(ag.Name)
		if _, err := r.ensurePVC(context.Background(), ag, names); err != nil {
			t.Fatal(err)
		}
		if _, _, err := r.ensureDeployment(context.Background(), ag, names, names.PVC, "hash", nil); err != nil {
			t.Fatal(err)
		}
		var dep appsv1.Deployment
		if err := c.Get(context.Background(), types.NamespacedName{Name: names.Deployment, Namespace: ag.Namespace}, &dep); err != nil {
			t.Fatal(err)
		}
		return &dep
	}

	t.Run("no-repo-no-init-container", func(t *testing.T) {
		dep := build(t, v1alpha1.AgentSpec{})
		if len(dep.Spec.Template.Spec.InitContainers) != 0 {
			t.Fatalf("want 0 init containers, got %d", len(dep.Spec.Template.Spec.InitContainers))
		}
		if _, ok := findVolume(dep.Spec.Template.Spec.Volumes, "git-ssh"); ok {
			t.Error("git-ssh volume should not be present without ssh credentials")
		}
		main := dep.Spec.Template.Spec.Containers[0]
		if _, ok := findEnv(main.Env, "REPO_PATH"); ok {
			t.Error("REPO_PATH should not be injected without spec.repo")
		}
	})

	t.Run("repo-only-public-clone", func(t *testing.T) {
		dep := build(t, v1alpha1.AgentSpec{
			Repo: &v1alpha1.AgentRepo{URL: "https://github.com/octocat/Hello-World.git", Branch: "master"},
		})
		init, ok := findContainer(dep.Spec.Template.Spec.InitContainers, "git-clone")
		if !ok {
			t.Fatal("git-clone init container missing")
		}
		if init.Image != gitImage {
			t.Errorf("init image = %q, want %q", init.Image, gitImage)
		}
		if v, _ := findEnv(init.Env, "REPO_URL"); v != "https://github.com/octocat/Hello-World.git" {
			t.Errorf("REPO_URL=%q", v)
		}
		if v, _ := findEnv(init.Env, "REPO_PATH"); v != "/workspace/repo" {
			t.Errorf("REPO_PATH=%q", v)
		}
		if v, _ := findEnv(init.Env, "REPO_BRANCH"); v != "master" {
			t.Errorf("REPO_BRANCH=%q", v)
		}
		// No credentials: no GIT_SSH_COMMAND, no GIT_TOKEN.
		if _, ok := findEnv(init.Env, "GIT_SSH_COMMAND"); ok {
			t.Error("GIT_SSH_COMMAND should be absent without ssh cred")
		}
		if _, ok := findEnv(init.Env, "GIT_TOKEN"); ok {
			t.Error("GIT_TOKEN should be absent without token cred")
		}
		// Main container should see REPO_PATH but no credential env.
		main := dep.Spec.Template.Spec.Containers[0]
		if v, _ := findEnv(main.Env, "REPO_PATH"); v != "/workspace/repo" {
			t.Errorf("main REPO_PATH=%q", v)
		}
	})

	t.Run("repo-with-ssh-key", func(t *testing.T) {
		dep := build(t, v1alpha1.AgentSpec{
			Repo: &v1alpha1.AgentRepo{URL: "git@github.com:acme/api.git", Branch: "main", Path: "src"},
			GitCredentials: &v1alpha1.GitCredentials{
				SSHKeySecretRef: &v1alpha1.SecretRef{Name: "acme-deploy-key"},
				UserName:        "Vinculum Bot",
				UserEmail:       "bot@acme.test",
			},
		})
		init, _ := findContainer(dep.Spec.Template.Spec.InitContainers, "git-clone")
		if init == nil {
			t.Fatal("git-clone init container missing")
		}
		if v, _ := findEnv(init.Env, "REPO_PATH"); v != "/workspace/src" {
			t.Errorf("init REPO_PATH=%q (custom path lost)", v)
		}
		ssh, _ := findEnv(init.Env, "GIT_SSH_COMMAND")
		if ssh == "" || !contains(ssh, gitSSHMountPath+"/id_ed25519") {
			t.Errorf("init GIT_SSH_COMMAND=%q does not reference key path", ssh)
		}

		vol, ok := findVolume(dep.Spec.Template.Spec.Volumes, "git-ssh")
		if !ok {
			t.Fatal("git-ssh volume missing")
		}
		if vol.Secret == nil || vol.Secret.SecretName != "acme-deploy-key" {
			t.Errorf("git-ssh volume secret=%+v", vol.Secret)
		}

		main := dep.Spec.Template.Spec.Containers[0]
		mountFound := false
		for _, m := range main.VolumeMounts {
			if m.Name == "git-ssh" && m.MountPath == gitSSHMountPath {
				mountFound = true
			}
		}
		if !mountFound {
			t.Error("main container missing git-ssh volume mount")
		}
		if v, _ := findEnv(main.Env, "GIT_SSH_COMMAND"); v == "" {
			t.Error("main container missing GIT_SSH_COMMAND")
		}
		if v, _ := findEnv(main.Env, "GIT_AUTHOR_NAME"); v != "Vinculum Bot" {
			t.Errorf("GIT_AUTHOR_NAME=%q", v)
		}
		if v, _ := findEnv(main.Env, "GIT_AUTHOR_EMAIL"); v != "bot@acme.test" {
			t.Errorf("GIT_AUTHOR_EMAIL=%q", v)
		}
		if _, ok := findEnv(main.Env, "GIT_TOKEN"); ok {
			t.Error("GIT_TOKEN should not be set without tokenSecretRef")
		}
	})

	t.Run("repo-with-token-only", func(t *testing.T) {
		dep := build(t, v1alpha1.AgentSpec{
			Repo: &v1alpha1.AgentRepo{URL: "https://github.com/acme/api.git"},
			GitCredentials: &v1alpha1.GitCredentials{
				TokenSecretRef: &v1alpha1.SecretRef{Name: "acme-pat"},
			},
		})
		init, _ := findContainer(dep.Spec.Template.Spec.InitContainers, "git-clone")
		if init == nil {
			t.Fatal("git-clone init container missing")
		}
		// GIT_TOKEN must come from a secretKeyRef pointing at "token" in the
		// referenced Secret.
		var gitTokenEnv *corev1.EnvVar
		for i := range init.Env {
			if init.Env[i].Name == "GIT_TOKEN" {
				gitTokenEnv = &init.Env[i]
				break
			}
		}
		if gitTokenEnv == nil {
			t.Fatal("init GIT_TOKEN missing")
		}
		if gitTokenEnv.ValueFrom == nil || gitTokenEnv.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("GIT_TOKEN ValueFrom=%+v", gitTokenEnv.ValueFrom)
		}
		ref := gitTokenEnv.ValueFrom.SecretKeyRef
		if ref.Name != "acme-pat" || ref.Key != "token" {
			t.Errorf("GIT_TOKEN secretKeyRef={name=%q,key=%q}", ref.Name, ref.Key)
		}
		if v, _ := findEnv(init.Env, "GIT_CONFIG_KEY_0"); v != "credential.helper" {
			t.Errorf("GIT_CONFIG_KEY_0=%q", v)
		}

		// Token Secret should be envFrom on init container so additional keys
		// (e.g. GITHUB_TOKEN) flow through, and on main container too.
		mainEnvFromHasToken := false
		for _, ef := range dep.Spec.Template.Spec.Containers[0].EnvFrom {
			if ef.SecretRef != nil && ef.SecretRef.Name == "acme-pat" {
				mainEnvFromHasToken = true
			}
		}
		if !mainEnvFromHasToken {
			t.Error("main container envFrom missing acme-pat secret")
		}

		// SSH-only env should be absent.
		if _, ok := findEnv(init.Env, "GIT_SSH_COMMAND"); ok {
			t.Error("GIT_SSH_COMMAND should be absent without ssh cred")
		}
		if _, ok := findVolume(dep.Spec.Template.Spec.Volumes, "git-ssh"); ok {
			t.Error("git-ssh volume should be absent without ssh cred")
		}
	})

	t.Run("repo-with-both-creds", func(t *testing.T) {
		dep := build(t, v1alpha1.AgentSpec{
			Repo: &v1alpha1.AgentRepo{URL: "git@gitlab.com:acme/api.git"},
			GitCredentials: &v1alpha1.GitCredentials{
				SSHKeySecretRef: &v1alpha1.SecretRef{Name: "deploy-key"},
				TokenSecretRef:  &v1alpha1.SecretRef{Name: "pat"},
			},
		})
		init, _ := findContainer(dep.Spec.Template.Spec.InitContainers, "git-clone")
		if init == nil {
			t.Fatal("git-clone init container missing")
		}
		if _, ok := findEnv(init.Env, "GIT_SSH_COMMAND"); !ok {
			t.Error("expected GIT_SSH_COMMAND when ssh cred set")
		}
		if _, ok := findEnv(init.Env, "GIT_TOKEN"); !ok {
			t.Error("expected GIT_TOKEN when token cred set")
		}
		if _, ok := findVolume(dep.Spec.Template.Spec.Volumes, "git-ssh"); !ok {
			t.Error("expected git-ssh volume when ssh cred set")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && (indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
