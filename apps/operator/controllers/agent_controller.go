package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
	vmetrics "github.com/florian/vinculum/apps/operator/internal/metrics"
)

const (
	defaultWorkspaceSize = "10Gi"
	agentPort            = 8090
	configHashAnnotation = "vinculum.dev/config-hash"
	defaultRepoPath      = "repo"
	gitImage             = "alpine/git:v2.47.2"
	gitSSHMountPath      = "/etc/vinculum/git-ssh"
	// agentUID matches the `agent` user baked into the vinculum-agent image
	// (apps/vinculum-agent/Dockerfile). The git-clone init container runs
	// under the same UID so cloned files are readable/writable by the main
	// container without invoking `safe.directory` workarounds.
	agentUID = int64(10001)
	// gitCredentialHelper drives `git` over HTTPS using the GIT_TOKEN env var.
	// Username "x-access-token" is the canonical GitHub form and is also
	// accepted by GitLab when paired with a personal access token.
	gitCredentialHelper = `!f() { test "$1" = get && echo username=x-access-token && echo password=$GIT_TOKEN; }; f`
)

type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    AgentReconcilerConfig
}

type AgentReconcilerConfig struct {
	AgentDefaultImage string
	OperatorURL       string
}

func chooseAgentImage(specImage, defaultImage string) string {
	if specImage != "" {
		return specImage
	}
	return defaultImage
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("agent", req.NamespacedName.String())
	var agent v1alpha1.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !agent.Spec.Enabled {
		if err := r.teardown(ctx, &agent); err != nil {
			return ctrl.Result{}, err
		}
		return r.writeStatus(ctx, &agent, "Disabled", false, 0, "", "", "")
	}

	names := resourceNames(agent.Name)

	pvcName, err := r.ensurePVC(ctx, &agent, names)
	if err != nil {
		logger.Error(err, "ensure pvc")
		return ctrl.Result{}, err
	}
	if err := r.ensureServiceAccount(ctx, &agent, names); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureRole(ctx, &agent, names); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureRoleBinding(ctx, &agent, names); err != nil {
		return ctrl.Result{}, err
	}
	resolved, resolveErr := r.resolveMCPServers(ctx, &agent)
	if resolveErr != nil {
		logger.Error(resolveErr, "resolve mcp refs")
	}
	configHash, err := r.ensureConfigMap(ctx, &agent, names, resolved)
	if err != nil {
		return ctrl.Result{}, err
	}
	deployName, readyReplicas, err := r.ensureDeployment(ctx, &agent, names, pvcName, configHash, resolved)
	if err != nil {
		logger.Error(err, "ensure deployment")
		return ctrl.Result{}, err
	}
	svcName, err := r.ensureService(ctx, &agent, names)
	if err != nil {
		return ctrl.Result{}, err
	}

	ready := readyReplicas > 0
	phase := "Pending"
	if ready {
		phase = "Ready"
	}
	if res, err := r.writeStatus(ctx, &agent, phase, ready, readyReplicas, deployName, svcName, pvcName); err != nil {
		return res, err
	}
	if !ready {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

type agentNames struct {
	Deployment     string
	Service        string
	PVC            string
	ConfigMap      string
	ServiceAccount string
	Role           string
	RoleBinding    string
	Labels         map[string]string
}

func resourceNames(agent string) agentNames {
	base := "agent-" + agent
	return agentNames{
		Deployment:     base,
		Service:        base,
		PVC:            base + "-workspace",
		ConfigMap:      base + "-config",
		ServiceAccount: base,
		Role:           base + "-task",
		RoleBinding:    base + "-task",
		Labels: map[string]string{
			"app.kubernetes.io/name":       "vinculum-agent",
			"app.kubernetes.io/instance":   base,
			"app.kubernetes.io/managed-by": "vinculum-operator",
			"vinculum.dev/agent":           agent,
		},
	}
}

func (r *AgentReconciler) ensurePVC(ctx context.Context, agent *v1alpha1.Agent, names agentNames) (string, error) {
	size := agent.Spec.WorkspaceSize
	if size == "" {
		size = defaultWorkspaceSize
	}
	q, err := resource.ParseQuantity(size)
	if err != nil {
		return "", fmt.Errorf("invalid workspaceSize %q: %w", size, err)
	}
	desired := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.PVC,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: agent.Spec.WorkspaceStorageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: q},
			},
		},
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", err
	}
	var existing corev1.PersistentVolumeClaim
	err = r.Get(ctx, types.NamespacedName{Name: names.PVC, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return names.PVC, r.Create(ctx, desired)
	}
	if err != nil {
		return "", err
	}
	return names.PVC, nil
}

func (r *AgentReconciler) ensureServiceAccount(ctx context.Context, agent *v1alpha1.Agent, names agentNames) error {
	desired := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ServiceAccount,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return err
	}
	var existing corev1.ServiceAccount
	err := r.Get(ctx, types.NamespacedName{Name: names.ServiceAccount, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	return err
}

func (r *AgentReconciler) ensureRole(ctx context.Context, agent *v1alpha1.Agent, names agentNames) error {
	desired := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Role,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"vinculum.dev"},
				Resources: []string{"tasks"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{"vinculum.dev"},
				Resources: []string{"tasks/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
		},
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return err
	}
	var existing rbacv1.Role
	err := r.Get(ctx, types.NamespacedName{Name: names.Role, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Rules = desired.Rules
	return r.Update(ctx, &existing)
}

func (r *AgentReconciler) ensureRoleBinding(ctx context.Context, agent *v1alpha1.Agent, names agentNames) error {
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.RoleBinding,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      names.ServiceAccount,
			Namespace: agent.Namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     names.Role,
		},
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return err
	}
	var existing rbacv1.RoleBinding
	err := r.Get(ctx, types.NamespacedName{Name: names.RoleBinding, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	return err
}

func (r *AgentReconciler) ensureConfigMap(ctx context.Context, agent *v1alpha1.Agent, names agentNames, resolved []resolvedMCP) (string, error) {
	data := map[string]string{}
	crushConfig, err := renderCrushConfig(agent, resolved)
	if err != nil {
		return "", err
	}
	data["crush.json"] = crushConfig
	if agent.Spec.InstructionInline != nil && agent.Spec.InstructionInline.FileName != "" {
		data[agent.Spec.InstructionInline.FileName] = agent.Spec.InstructionInline.Content
	}

	hash := hashMap(data)
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ConfigMap,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
		Data: data,
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", err
	}

	var existing corev1.ConfigMap
	err = r.Get(ctx, types.NamespacedName{Name: names.ConfigMap, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return "", err
		}
		return hash, nil
	}
	if err != nil {
		return "", err
	}
	if !mapsEqual(existing.Data, data) {
		existing.Data = data
		if err := r.Update(ctx, &existing); err != nil {
			return "", err
		}
	}
	return hash, nil
}

// resolvedMCP is a normalized view of either an inline MCP entry or a
// dereferenced MCPServer CR, ready to be rendered into crush.json and wired
// into the pod envFrom list.
type resolvedMCP struct {
	Name      string
	URL       string
	Command   string
	Args      []string
	Env       map[string]string
	SecretRef string
	Enabled   bool
	Timeout   int32
}

func (r *AgentReconciler) resolveMCPServers(ctx context.Context, agent *v1alpha1.Agent) ([]resolvedMCP, error) {
	out := make([]resolvedMCP, 0, len(agent.Spec.MCPServers)+len(agent.Spec.MCPServerRefs))
	for _, s := range agent.Spec.MCPServers {
		rm := resolvedMCP{
			Name:    s.Name,
			URL:     s.URL,
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
			Enabled: s.Enabled,
			Timeout: 120,
		}
		if s.SecretRef != nil {
			rm.SecretRef = s.SecretRef.Name
		}
		out = append(out, rm)
	}
	var firstErr error
	for _, name := range agent.Spec.MCPServerRefs {
		var cr v1alpha1.MCPServer
		if err := r.Get(ctx, types.NamespacedName{Namespace: agent.Namespace, Name: name}, &cr); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("mcp ref %q: %w", name, err)
			}
			continue
		}
		rm := resolvedMCP{
			Name:    cr.Name,
			URL:     cr.Spec.URL,
			Command: cr.Spec.Command,
			Args:    cr.Spec.Args,
			Env:     cr.Spec.Env,
			Enabled: cr.Spec.Enabled,
			Timeout: cr.Spec.Timeout,
		}
		if rm.Timeout == 0 {
			rm.Timeout = 120
		}
		if cr.Spec.SecretRef != nil {
			rm.SecretRef = cr.Spec.SecretRef.Name
		}
		out = append(out, rm)
	}
	return out, firstErr
}

func renderCrushConfig(agent *v1alpha1.Agent, resolved []resolvedMCP) (string, error) {
	cfg := map[string]any{
		"$schema": "https://charm.land/crush.json",
	}
	if agent.Spec.Model != "" {
		cfg["model"] = agent.Spec.Model
	}
	// Default: no permission prompts. Non-interactive `crush run` would
	// otherwise hang waiting for human approval of each tool use. Users
	// can lock this down via spec.allowedTools.
	allowed := agent.Spec.AllowedTools
	if len(allowed) == 0 {
		allowed = []string{"*"}
	}
	cfg["permissions"] = map[string]any{"allowed_tools": allowed}
	if len(agent.Spec.DisabledTools) > 0 {
		tools := map[string]any{}
		for _, name := range agent.Spec.DisabledTools {
			tools[name] = map[string]any{"disabled": true}
		}
		cfg["tools"] = tools
	}
	if len(resolved) > 0 {
		mcp := map[string]any{}
		for _, server := range resolved {
			entry := map[string]any{
				"disabled": !server.Enabled,
				"timeout":  server.Timeout,
			}
			if server.URL != "" {
				entry["type"] = "http"
				entry["url"] = server.URL
			} else if server.Command != "" {
				entry["type"] = "stdio"
				entry["command"] = server.Command
				if len(server.Args) > 0 {
					entry["args"] = server.Args
				}
			} else {
				continue
			}
			if len(server.Env) > 0 {
				entry["env"] = server.Env
			}
			mcp[server.Name] = entry
		}
		cfg["mcp"] = mcp
	}
	blob, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

// gitPodPieces returns the Deployment additions needed to clone a repo and
// authenticate to it. When agent.Spec.Repo is nil, every returned slice is
// empty and the caller leaves the Deployment unchanged.
//
// The init container clones (or fetches) into /workspace/<path>. Credential
// material is mounted into both the init container and the main container so
// the agent can `git push` after Tasks complete.
func gitPodPieces(agent *v1alpha1.Agent) (initContainers []corev1.Container, extraVolumes []corev1.Volume, extraMainMounts []corev1.VolumeMount, extraMainEnv []corev1.EnvVar, extraMainEnvFrom []corev1.EnvFromSource) {
	if agent.Spec.Repo == nil || strings.TrimSpace(agent.Spec.Repo.URL) == "" {
		return
	}
	repoPath := strings.TrimSpace(agent.Spec.Repo.Path)
	if repoPath == "" {
		repoPath = defaultRepoPath
	}
	absRepoPath := "/workspace/" + repoPath
	branch := strings.TrimSpace(agent.Spec.Repo.Branch)

	creds := agent.Spec.GitCredentials

	// SSH-key volume mounted into both containers when set.
	var sshMount *corev1.VolumeMount
	if creds != nil && creds.SSHKeySecretRef != nil && creds.SSHKeySecretRef.Name != "" {
		mode := int32(0o400)
		extraVolumes = append(extraVolumes, corev1.Volume{
			Name: "git-ssh",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  creds.SSHKeySecretRef.Name,
					DefaultMode: &mode,
				},
			},
		})
		m := corev1.VolumeMount{Name: "git-ssh", MountPath: gitSSHMountPath, ReadOnly: true}
		sshMount = &m
		extraMainMounts = append(extraMainMounts, m)
	}

	// Env shared by init + main containers.
	sshCommand := ""
	if sshMount != nil {
		// id_ed25519 is the canonical private-key filename we expect inside
		// the Secret. accept-new lets first-contact connections proceed but
		// pins the host key for future connections.
		sshCommand = "ssh -i " + gitSSHMountPath + "/id_ed25519 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=" + gitSSHMountPath + "/known_hosts"
	}

	tokenEnv := []corev1.EnvVar{}
	tokenEnvFrom := []corev1.EnvFromSource{}
	if creds != nil && creds.TokenSecretRef != nil && creds.TokenSecretRef.Name != "" {
		optional := true
		tokenEnv = append(tokenEnv,
			corev1.EnvVar{Name: "GIT_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: creds.TokenSecretRef.Name},
				Key:                  "token", Optional: &optional,
			}}},
			corev1.EnvVar{Name: "GIT_CONFIG_COUNT", Value: "1"},
			corev1.EnvVar{Name: "GIT_CONFIG_KEY_0", Value: "credential.helper"},
			corev1.EnvVar{Name: "GIT_CONFIG_VALUE_0", Value: gitCredentialHelper},
		)
		// Also envFrom so additional keys (e.g. GITHUB_TOKEN for PR API
		// creation in chunk B) flow through as env vars.
		tokenEnvFrom = append(tokenEnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: creds.TokenSecretRef.Name},
			},
		})
	}

	initEnv := []corev1.EnvVar{
		{Name: "REPO_URL", Value: agent.Spec.Repo.URL},
		{Name: "REPO_PATH", Value: absRepoPath},
		{Name: "REPO_BRANCH", Value: branch},
		// HOME must be writable so `git config --global` can persist
		// safe.directory entries. /tmp is /tmp on the workspace pod (the
		// init container shares /tmp by virtue of nothing else mounting
		// over it — alpine has /tmp by default).
		{Name: "HOME", Value: "/tmp"},
	}
	if sshCommand != "" {
		initEnv = append(initEnv, corev1.EnvVar{Name: "GIT_SSH_COMMAND", Value: sshCommand})
	}
	initEnv = append(initEnv, tokenEnv...)

	initMounts := []corev1.VolumeMount{{Name: "workspace", MountPath: "/workspace"}}
	if sshMount != nil {
		initMounts = append(initMounts, *sshMount)
	}

	// safe.directory wildcard avoids "dubious ownership" failures when the
	// PVC's repo dir was created by an older container that ran as a
	// different UID (e.g. a v0.2.0 clone left over after upgrade).
	cloneScript := `set -eu
git config --global --add safe.directory '*'
mkdir -p "$(dirname "$REPO_PATH")"
if [ -d "$REPO_PATH/.git" ]; then
  echo "vinculum: repo cache present at $REPO_PATH, fetching"
  cd "$REPO_PATH"
  git remote set-url origin "$REPO_URL"
  git fetch --all --prune
  if [ -n "$REPO_BRANCH" ]; then
    git checkout -B "$REPO_BRANCH" "origin/$REPO_BRANCH"
  fi
else
  echo "vinculum: cloning $REPO_URL into $REPO_PATH"
  if [ -n "$REPO_BRANCH" ]; then
    git clone --branch "$REPO_BRANCH" "$REPO_URL" "$REPO_PATH"
  else
    git clone "$REPO_URL" "$REPO_PATH"
  fi
fi
`

	initContainers = append(initContainers, corev1.Container{
		Name:            "git-clone",
		Image:           gitImage,
		Command:         []string{"sh", "-c", cloneScript},
		Env:             initEnv,
		EnvFrom:         tokenEnvFrom,
		VolumeMounts:    initMounts,
		SecurityContext: agentSecurityContext(),
	})

	// Main container additions.
	extraMainEnv = append(extraMainEnv, corev1.EnvVar{Name: "REPO_PATH", Value: absRepoPath})
	if branch != "" {
		extraMainEnv = append(extraMainEnv, corev1.EnvVar{Name: "REPO_BRANCH", Value: branch})
	}
	if creds != nil {
		if creds.UserName != "" {
			extraMainEnv = append(extraMainEnv,
				corev1.EnvVar{Name: "GIT_AUTHOR_NAME", Value: creds.UserName},
				corev1.EnvVar{Name: "GIT_COMMITTER_NAME", Value: creds.UserName},
			)
		}
		if creds.UserEmail != "" {
			extraMainEnv = append(extraMainEnv,
				corev1.EnvVar{Name: "GIT_AUTHOR_EMAIL", Value: creds.UserEmail},
				corev1.EnvVar{Name: "GIT_COMMITTER_EMAIL", Value: creds.UserEmail},
			)
		}
	}
	if sshCommand != "" {
		extraMainEnv = append(extraMainEnv, corev1.EnvVar{Name: "GIT_SSH_COMMAND", Value: sshCommand})
	}
	extraMainEnv = append(extraMainEnv, tokenEnv...)
	extraMainEnvFrom = append(extraMainEnvFrom, tokenEnvFrom...)
	return
}

func (r *AgentReconciler) ensureDeployment(ctx context.Context, agent *v1alpha1.Agent, names agentNames, pvcName, configHash string, resolved []resolvedMCP) (string, int32, error) {
	instructionMount := agent.Spec.InstructionMountPath
	if instructionMount == "" {
		instructionMount = "/etc/vinculum"
	}
	// crush follows XDG: it reads its config from $XDG_CONFIG_HOME/crush/crush.json.
	// Mount the rendered ConfigMap at that subdir so crush actually finds crush.json
	// (and MCP servers like the vinculum stdio bridge get loaded).
	configMount := instructionMount + "/crush"

	envVars := []corev1.EnvVar{
		{Name: "AGENT_NAME", Value: agent.Name},
		{Name: "AGENT_NAMESPACE", Value: agent.Namespace},
		{Name: "WORKSPACE_ROOT", Value: "/workspace"},
		{Name: "XDG_DATA_HOME", Value: "/workspace/.crush-data"},
		{Name: "XDG_CONFIG_HOME", Value: instructionMount},
		{Name: "CRUSH_SOCKET", Value: "/tmp/crush.sock"},
		{Name: "SERVER_ADDR", Value: fmt.Sprintf(":%d", agentPort)},
	}
	if agent.Spec.InstructionInline != nil && agent.Spec.InstructionInline.FileName != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "INSTRUCTION_FILE", Value: configMount + "/" + agent.Spec.InstructionInline.FileName})
	}
	if agent.Spec.Orchestrator && r.Cfg.OperatorURL != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "VINCULUM_OPERATOR_URL", Value: r.Cfg.OperatorURL})
	}

	gitInitContainers, gitVolumes, gitMainMounts, gitMainEnv, gitMainEnvFrom := gitPodPieces(agent)
	envVars = append(envVars, gitMainEnv...)

	for k, v := range agent.Spec.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	var envFrom []corev1.EnvFromSource
	if agent.Spec.ProviderSecretRef != nil && agent.Spec.ProviderSecretRef.Name != "" {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: agent.Spec.ProviderSecretRef.Name},
			},
		})
	}
	seenSecrets := map[string]bool{}
	if agent.Spec.ProviderSecretRef != nil {
		seenSecrets[agent.Spec.ProviderSecretRef.Name] = true
	}
	for _, s := range resolved {
		if s.SecretRef == "" || seenSecrets[s.SecretRef] {
			continue
		}
		seenSecrets[s.SecretRef] = true
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: s.SecretRef},
			},
		})
	}
	for _, ef := range gitMainEnvFrom {
		name := ef.SecretRef.LocalObjectReference.Name
		if seenSecrets[name] {
			continue
		}
		seenSecrets[name] = true
		envFrom = append(envFrom, ef)
	}

	replicas := int32(1)
	podLabels := map[string]string{}
	for k, v := range names.Labels {
		podLabels[k] = v
	}
	selector := map[string]string{"vinculum.dev/agent": agent.Name}

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Deployment,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: map[string]string{configHashAnnotation: configHash},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: names.ServiceAccount,
					InitContainers:     gitInitContainers,
					Containers: []corev1.Container{{
						Name:            "agent",
						Image:           chooseAgentImage(agent.Spec.Image, r.Cfg.AgentDefaultImage),
						ImagePullPolicy: corev1.PullIfNotPresent,
						SecurityContext: agentSecurityContext(),
						Args:            []string{"serve"},
						Env:             envVars,
						EnvFrom:         envFrom,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: agentPort,
							Protocol:      corev1.ProtocolTCP,
						}},
						VolumeMounts: append([]corev1.VolumeMount{
							{Name: "workspace", MountPath: "/workspace"},
							{Name: "config", MountPath: configMount},
							{Name: "tmp", MountPath: "/tmp"},
						}, gitMainMounts...),
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(agentPort),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(agentPort),
								},
							},
							InitialDelaySeconds: 30,
							PeriodSeconds:       30,
						},
					}},
					Volumes: append([]corev1.Volume{
						{Name: "workspace", VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
						}},
						{Name: "config", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: names.ConfigMap},
							},
						}},
						{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					}, gitVolumes...),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", 0, err
	}

	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: names.Deployment, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return "", 0, err
		}
		return names.Deployment, 0, nil
	}
	if err != nil {
		return "", 0, err
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if err := r.Update(ctx, &existing); err != nil {
		return "", 0, err
	}
	return names.Deployment, existing.Status.ReadyReplicas, nil
}

func (r *AgentReconciler) ensureService(ctx context.Context, agent *v1alpha1.Agent, names agentNames) (string, error) {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.Service,
			Namespace: agent.Namespace,
			Labels:    names.Labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"vinculum.dev/agent": agent.Name},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       agentPort,
				TargetPort: intstr.FromInt(agentPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", err
	}
	var existing corev1.Service
	err := r.Get(ctx, types.NamespacedName{Name: names.Service, Namespace: agent.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return names.Service, r.Create(ctx, desired)
	}
	if err != nil {
		return "", err
	}
	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	return names.Service, r.Update(ctx, &existing)
}

func (r *AgentReconciler) teardown(_ context.Context, _ *v1alpha1.Agent) error {
	// Owner references drive cascade delete when enabled=false triggers no
	// spec-level change; operator leaves the existing workload in place.
	// A future enhancement could scale the Deployment to 0 here.
	return nil
}

func (r *AgentReconciler) writeStatus(ctx context.Context, agent *v1alpha1.Agent, phase string, ready bool, readyReplicas int32, deployName, svcName, pvcName string) (ctrl.Result, error) {
	now := metav1.Now()
	changed := false
	if agent.Status.Phase != phase {
		agent.Status.Phase = phase
		changed = true
	}
	if agent.Status.Ready != ready {
		agent.Status.Ready = ready
		changed = true
	}
	if agent.Status.ReadyReplicas != readyReplicas {
		agent.Status.ReadyReplicas = readyReplicas
		changed = true
	}
	if agent.Status.DeploymentName != deployName {
		agent.Status.DeploymentName = deployName
		changed = true
	}
	if agent.Status.ServiceName != svcName {
		agent.Status.ServiceName = svcName
		changed = true
	}
	if agent.Status.PVCName != pvcName {
		agent.Status.PVCName = pvcName
		changed = true
	}
	if changed {
		agent.Status.LastSeen = &now
		if err := r.Status().Update(ctx, agent); err != nil && !apierrors.IsConflict(err) {
			return ctrl.Result{}, err
		}
	}
	readyVal := 0.0
	if ready {
		readyVal = 1.0
	}
	vmetrics.AgentReady.WithLabelValues(agent.Name).Set(readyVal)
	return ctrl.Result{}, nil
}

// agentSecurityContext is the locked-down SecurityContext applied to both
// the agent container and the git-clone init container. RunAsUser/Group
// pin to the `agent` user baked into the agent image so cloned files end
// up owned by 10001:10001 and aren't readable only by root.
//
// AllowPrivilegeEscalation=false + drop ALL caps + RuntimeDefault seccomp
// are the standard hardening trio recommended by the Pod Security
// Standards "restricted" profile. Root filesystem is left writable
// (crush + apt-installed tools assume that); a future revision can flip
// readOnlyRootFilesystem once the image is reworked for read-only roots.
func agentSecurityContext() *corev1.SecurityContext {
	nonRoot := true
	noEscalate := false
	uid := agentUID
	return &corev1.SecurityContext{
		RunAsNonRoot:             &nonRoot,
		RunAsUser:                &uid,
		RunAsGroup:               &uid,
		AllowPrivilegeEscalation: &noEscalate,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

func hashMap(m map[string]string) string {
	blob, _ := json.Marshal(m)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])[:16]
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
