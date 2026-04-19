package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
)

const (
	defaultWorkspaceSize = "10Gi"
	agentPort            = 8090
	configHashAnnotation = "vinculum.dev/config-hash"
)

type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    AgentReconcilerConfig
}

type AgentReconcilerConfig struct {
	AgentDefaultImage string
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
	configHash, err := r.ensureConfigMap(ctx, &agent, names)
	if err != nil {
		return ctrl.Result{}, err
	}
	deployName, readyReplicas, err := r.ensureDeployment(ctx, &agent, names, pvcName, configHash)
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

func (r *AgentReconciler) ensureConfigMap(ctx context.Context, agent *v1alpha1.Agent, names agentNames) (string, error) {
	data := map[string]string{}
	crushConfig, err := renderCrushConfig(agent)
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

func renderCrushConfig(agent *v1alpha1.Agent) (string, error) {
	cfg := map[string]any{
		"$schema": "https://charm.land/crush.json",
	}
	if agent.Spec.Model != "" {
		cfg["model"] = agent.Spec.Model
	}
	if len(agent.Spec.MCPServers) > 0 {
		mcp := map[string]any{}
		for _, server := range agent.Spec.MCPServers {
			entry := map[string]any{
				"disabled": !server.Enabled,
				"timeout":  120,
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

func (r *AgentReconciler) ensureDeployment(ctx context.Context, agent *v1alpha1.Agent, names agentNames, pvcName, configHash string) (string, int32, error) {
	instructionMount := agent.Spec.InstructionMountPath
	if instructionMount == "" {
		instructionMount = "/etc/vinculum"
	}

	envVars := []corev1.EnvVar{
		{Name: "AGENT_NAME", Value: agent.Name},
		{Name: "AGENT_NAMESPACE", Value: agent.Namespace},
		{Name: "WORKSPACE_ROOT", Value: "/workspace"},
		{Name: "XDG_DATA_HOME", Value: "/workspace/.crush-data"},
		{Name: "XDG_CONFIG_HOME", Value: "/etc/vinculum"},
		{Name: "CRUSH_SOCKET", Value: "/tmp/crush.sock"},
		{Name: "SERVER_ADDR", Value: fmt.Sprintf(":%d", agentPort)},
	}
	if agent.Spec.InstructionInline != nil && agent.Spec.InstructionInline.FileName != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "INSTRUCTION_FILE", Value: instructionMount + "/" + agent.Spec.InstructionInline.FileName})
	}
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
					Containers: []corev1.Container{{
						Name:            "agent",
						Image:           chooseAgentImage(agent.Spec.Image, r.Cfg.AgentDefaultImage),
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args:            []string{"serve"},
						Env:             envVars,
						EnvFrom:         envFrom,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: agentPort,
							Protocol:      corev1.ProtocolTCP,
						}},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "workspace", MountPath: "/workspace"},
							{Name: "config", MountPath: instructionMount},
							{Name: "tmp", MountPath: "/tmp"},
						},
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
					Volumes: []corev1.Volume{
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
	return ctrl.Result{}, nil
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
