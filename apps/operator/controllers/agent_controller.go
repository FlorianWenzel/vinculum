package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
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
		return r.writeStatus(ctx, &agent, "Disabled", "", "", false, 0, "", "", "")
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
	reason, message := "", ""
	if ready {
		phase = "Ready"
	} else {
		// Look for an init container that failed and surface it. This makes
		// `kubectl get agent` show "InitContainerFailed: git clone … fatal: Repository not found"
		// instead of an opaque "Pending".
		if r, m, found := r.inspectInitFailure(ctx, &agent); found {
			phase = "Failed"
			reason = r
			message = m
		}
	}
	if res, err := r.writeStatus(ctx, &agent, phase, reason, message, ready, readyReplicas, deployName, svcName, pvcName); err != nil {
		return res, err
	}
	if !ready {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// inspectInitFailure returns (reason, message, found) when any init container
// across the agent's pods is in a non-transient failure state. We look at
// both the current `terminated` state and the prior termination state under
// CrashLoopBackOff — k8s puts the actual exit on lastTerminationState during
// the backoff wait.
func (r *AgentReconciler) inspectInitFailure(ctx context.Context, agent *v1alpha1.Agent) (string, string, bool) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(agent.Namespace), client.MatchingLabels{"vinculum.dev/agent": agent.Name}); err != nil {
		return "", "", false
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		for _, ic := range p.Status.InitContainerStatuses {
			if t := ic.State.Terminated; t != nil && t.ExitCode != 0 {
				return "InitContainerFailed", formatInitFailure(ic.Name, t.ExitCode, t.Reason, t.Message), true
			}
			if w := ic.State.Waiting; w != nil && (w.Reason == "CrashLoopBackOff" || w.Reason == "ImagePullBackOff" || w.Reason == "ErrImagePull") {
				// For CrashLoop, prefer the last actual termination message;
				// for Image*BackOff, the Waiting.Message is the kubelet's
				// "Back-off pulling image…" string.
				if lt := ic.LastTerminationState.Terminated; lt != nil && lt.ExitCode != 0 {
					return "InitContainerFailed", formatInitFailure(ic.Name, lt.ExitCode, lt.Reason, lt.Message), true
				}
				return w.Reason, fmt.Sprintf("init container %q: %s", ic.Name, strings.TrimSpace(w.Message)), true
			}
		}
	}
	return "", "", false
}

func formatInitFailure(name string, exit int32, reason, message string) string {
	tail := strings.TrimSpace(message)
	if tail == "" {
		tail = reason
	}
	// Compress to a single line and cap so it fits in `kubectl get` columns.
	tail = strings.ReplaceAll(tail, "\n", " ")
	if len(tail) > 280 {
		tail = tail[:277] + "..."
	}
	return fmt.Sprintf("init %q exited %d: %s", name, exit, tail)
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
				Resources: []string{"tasks", "messages"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{"vinculum.dev"},
				Resources: []string{"tasks/status", "messages/status"},
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
		providerID, modelID, ok := splitModelSpec(agent.Spec.Model)
		if !ok {
			return "", fmt.Errorf("agent.spec.model %q must be in the form '<provider>/<model-id>' (e.g. 'openrouter/anthropic/claude-haiku-4.5')", agent.Spec.Model)
		}
		providerCfg, err := crushProviderConfig(providerID, modelID)
		if err != nil {
			return "", err
		}
		cfg["providers"] = map[string]any{providerID: providerCfg}
		selected := map[string]any{"model": modelID, "provider": providerID}
		// crush v0.69 expects exactly the slot names "large" and "small".
		// Until vinculum models per-agent sizing, both slots point at the
		// same model — Agent.spec.model is one knob, not two.
		cfg["models"] = map[string]any{
			"large": selected,
			"small": selected,
		}
	}
	// Default: no permission prompts. Non-interactive `crush run` would
	// otherwise hang waiting for human approval of each tool use. Users
	// can lock this down via spec.allowedTools.
	allowed := agent.Spec.AllowedTools
	if len(allowed) == 0 {
		allowed = []string{"*"}
	}
	cfg["permissions"] = map[string]any{"allowed_tools": allowed}
	// Block crush's silent fallback to embedded default providers (Anthropic
	// Sonnet etc.). With this on, crush only uses providers we explicitly
	// configure — and fails loudly if the configured one is unavailable
	// instead of routing to a paid default. See the v0.69 schema:
	// https://charm.land/crush.json → Options.disable_default_providers.
	// Pairs with the providers/models blocks above: together they form the
	// only path crush has to an LLM. No path = no surprise spend.
	cfg["options"] = map[string]any{"disable_default_providers": true}
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

// splitModelSpec parses Agent.spec.model into (providerID, modelID).
// The format is "<provider>/<model-id>", where the model id may itself
// contain slashes (e.g. "openrouter/nvidia/foo:bar"). Returns ok=false if
// the input has no slash or the provider/model component is empty.
func splitModelSpec(s string) (provider, model string, ok bool) {
	i := strings.IndexByte(s, '/')
	if i <= 0 || i >= len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// crushProviderConfig returns the v0.69 ProviderConfig for known provider
// ids. The api_key uses "$VAR" syntax — crush resolves it from the pod env
// at runtime, which envFrom on the providerSecretRef populates.
//
// New providers go here. With disable_default_providers=true crush will
// not look anywhere else, so an unrecognized provider is a hard error
// instead of a silent fallback (which is the whole point).
func crushProviderConfig(providerID, modelID string) (map[string]any, error) {
	base := map[string]any{
		"id": providerID,
		"models": []map[string]any{
			crushModelEntry(modelID),
		},
	}
	switch providerID {
	case "openrouter":
		base["name"] = "OpenRouter"
		base["type"] = "openai-compat"
		base["base_url"] = "https://openrouter.ai/api/v1"
		base["api_key"] = "$OPENROUTER_API_KEY"
	case "anthropic":
		base["name"] = "Anthropic"
		base["type"] = "anthropic"
		base["base_url"] = "https://api.anthropic.com/v1"
		base["api_key"] = "$ANTHROPIC_API_KEY"
	case "openai":
		base["name"] = "OpenAI"
		base["type"] = "openai"
		base["base_url"] = "https://api.openai.com/v1"
		base["api_key"] = "$OPENAI_API_KEY"
	case "azure":
		base["name"] = "Azure OpenAI"
		base["type"] = "azure"
		base["base_url"] = "$AZURE_OPENAI_ENDPOINT"
		base["api_key"] = "$AZURE_OPENAI_API_KEY"
	default:
		return nil, fmt.Errorf("unsupported provider %q in agent.spec.model (supported: openrouter, anthropic, openai, azure)", providerID)
	}
	return base, nil
}

// crushModelEntry returns a catwalk.Model record with permissive defaults.
// We don't track per-token spend here, so cost fields are 0; context and
// max-tokens are generous defaults that won't gate normal use. These values
// describe the model to crush — they don't enforce anything at the API.
func crushModelEntry(id string) map[string]any {
	return map[string]any{
		"id":                     id,
		"name":                   id,
		"cost_per_1m_in":         0,
		"cost_per_1m_out":        0,
		"cost_per_1m_in_cached":  0,
		"cost_per_1m_out_cached": 0,
		"context_window":         200000,
		"default_max_tokens":     8192,
		"can_reason":             false,
		"supports_attachments":   false,
	}
}

// extraMountPieces translates agent.spec.mounts into k8s Volumes +
// VolumeMounts. Returns an error for malformed entries (missing source,
// both sources set, empty name/path). When agent has no mounts, returns
// (nil, nil, nil) and the caller adds nothing.
//
// Single-key mounts use Items + SubPath so other keys in the source
// aren't materialized in the container — the MountPath becomes a single
// file. Multi-key mounts project the whole Secret/ConfigMap into the
// directory at MountPath, one file per key (standard k8s behavior).
func extraMountPieces(agent *v1alpha1.Agent) ([]corev1.Volume, []corev1.VolumeMount, error) {
	if len(agent.Spec.Mounts) == 0 {
		return nil, nil, nil
	}
	var (
		volumes []corev1.Volume
		mounts  []corev1.VolumeMount
		seen    = map[string]bool{}
	)
	for i, m := range agent.Spec.Mounts {
		if strings.TrimSpace(m.Name) == "" {
			return nil, nil, fmt.Errorf("mounts[%d]: name is required", i)
		}
		if strings.TrimSpace(m.MountPath) == "" {
			return nil, nil, fmt.Errorf("mounts[%d] (%s): mountPath is required", i, m.Name)
		}
		if seen[m.Name] {
			return nil, nil, fmt.Errorf("mounts[%d]: duplicate name %q", i, m.Name)
		}
		seen[m.Name] = true
		hasSecret := m.Secret != nil && strings.TrimSpace(m.Secret.Name) != ""
		hasCM := m.ConfigMap != nil && strings.TrimSpace(m.ConfigMap.Name) != ""
		if hasSecret == hasCM {
			return nil, nil, fmt.Errorf("mounts[%d] (%s): exactly one of secret or configMap is required", i, m.Name)
		}
		volumeName := "mount-" + m.Name

		// readOnly defaults to true. Set to false only when explicit.
		readOnly := true
		if m.ReadOnly != nil {
			readOnly = *m.ReadOnly
		}

		vol := corev1.Volume{Name: volumeName}
		vm := corev1.VolumeMount{
			Name:      volumeName,
			MountPath: m.MountPath,
			ReadOnly:  readOnly,
		}

		switch {
		case hasSecret:
			src := &corev1.SecretVolumeSource{
				SecretName: m.Secret.Name,
			}
			if key := strings.TrimSpace(m.Secret.Key); key != "" {
				// project only this key, name it after the mount target's
				// basename so subPath has something stable to reference.
				file := filepath.Base(m.MountPath)
				src.Items = []corev1.KeyToPath{{Key: key, Path: file}}
				vm.SubPath = file
			}
			vol.VolumeSource = corev1.VolumeSource{Secret: src}
		case hasCM:
			src := &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: m.ConfigMap.Name},
			}
			if key := strings.TrimSpace(m.ConfigMap.Key); key != "" {
				file := filepath.Base(m.MountPath)
				src.Items = []corev1.KeyToPath{{Key: key, Path: file}}
				vm.SubPath = file
			}
			vol.VolumeSource = corev1.VolumeSource{ConfigMap: src}
		}

		volumes = append(volumes, vol)
		mounts = append(mounts, vm)
	}
	return volumes, mounts, nil
}

// appendMounts flattens N VolumeMount slices into a single slice while
// preserving order. Tiny helper to keep the deployment-build call sites
// readable when the mount source count grows.
func appendMounts(base []corev1.VolumeMount, extras ...[]corev1.VolumeMount) []corev1.VolumeMount {
	out := base
	for _, e := range extras {
		out = append(out, e...)
	}
	return out
}

// appendVolumes is appendMounts for Volume slices.
func appendVolumes(base []corev1.Volume, extras ...[]corev1.Volume) []corev1.Volume {
	out := base
	for _, e := range extras {
		out = append(out, e...)
	}
	return out
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
		// FallbackToLogsOnError populates state.terminated.message with the
		// container's last log lines on non-zero exit, without us having to
		// scrape pod logs from the operator. Surfaces "could not read
		// Username" / "Repository not found" into Agent.status.message.
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
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
	peerEnabled := agent.PeerEnabled()
	if (peerEnabled || agent.Spec.Orchestrator) && r.Cfg.OperatorURL != "" {
		envVars = append(envVars,
			corev1.EnvVar{Name: "VINCULUM_OPERATOR_URL", Value: r.Cfg.OperatorURL},
			corev1.EnvVar{Name: "VINCULUM_PEER", Value: strconv.FormatBool(peerEnabled)},
			corev1.EnvVar{Name: "VINCULUM_ORCHESTRATOR", Value: strconv.FormatBool(agent.Spec.Orchestrator)},
		)
	}

	gitInitContainers, gitVolumes, gitMainMounts, gitMainEnv, gitMainEnvFrom := gitPodPieces(agent)
	mountVolumes, mountMainMounts, mountErr := extraMountPieces(agent)
	if mountErr != nil {
		return "", 0, mountErr
	}
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
						VolumeMounts: appendMounts(
							[]corev1.VolumeMount{
								{Name: "workspace", MountPath: "/workspace"},
								{Name: "config", MountPath: configMount},
								{Name: "tmp", MountPath: "/tmp"},
							},
							gitMainMounts,
							mountMainMounts,
						),
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
					Volumes: appendVolumes(
						[]corev1.Volume{
							{Name: "workspace", VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
							}},
							{Name: "config", VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: names.ConfigMap},
								},
							}},
							{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
						},
						gitVolumes,
						mountVolumes,
					),
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

func (r *AgentReconciler) writeStatus(ctx context.Context, agent *v1alpha1.Agent, phase, reason, message string, ready bool, readyReplicas int32, deployName, svcName, pvcName string) (ctrl.Result, error) {
	now := metav1.Now()
	changed := false
	if agent.Status.Phase != phase {
		agent.Status.Phase = phase
		changed = true
	}
	if agent.Status.Reason != reason {
		agent.Status.Reason = reason
		changed = true
	}
	if agent.Status.Message != message {
		agent.Status.Message = message
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
