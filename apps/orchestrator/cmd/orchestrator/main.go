package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "github.com/florian/vinculum/apps/orchestrator/api/v1alpha1"
	"github.com/florian/vinculum/apps/orchestrator/controllers"
	appconfig "github.com/florian/vinculum/apps/orchestrator/internal/config"
	forgejoclient "github.com/florian/vinculum/apps/orchestrator/internal/forgejo"
	requirementdoc "github.com/florian/vinculum/apps/orchestrator/internal/requirements"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	ctrl.SetLogger(zap.New())

	var metricsAddr string
	var probeAddr string
	watchNamespace := os.Getenv("WATCH_NAMESPACE")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8082", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8083", "The address the probe endpoint binds to.")
	flag.Parse()
	cfg := appconfig.Load()

	scheme := clientgoscheme.Scheme
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cacheOptions := cache.Options{}
	if watchNamespace != "" {
		cacheOptions.DefaultNamespaces = map[string]cache.Config{watchNamespace: {}}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		Cache:                  cacheOptions,
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		os.Exit(1)
	}

	forgejo := forgejoclient.NewClient(cfg)

	droneReconciler := &controllers.DroneReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Forgejo: forgejo,
	}
	repoReconciler := &controllers.ForgejoRepositoryReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Forgejo:    forgejo,
		WebhookURL: cfg.WebhookURL,
	}
	accessReconciler := &controllers.DroneRepositoryAccessReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Forgejo: forgejo,
	}

	taskRunReconciler := &controllers.TaskRunReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Forgejo: forgejo,
	}
	requirementReconciler := &controllers.RequirementReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Forgejo: forgejo}
	reviewReconciler := &controllers.ReviewReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Forgejo: forgejo}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		os.Exit(1)
	}

	go startAPIServer(mgr.GetClient(), watchNamespace, cfg, forgejo)
	ctrl.Log.Info("vinculum-orchestrator manager configured", "watchNamespace", watchNamespace, "mode", "poller")
	go startPoller(mgr.GetClient(), watchNamespace, droneReconciler, repoReconciler, accessReconciler, taskRunReconciler, requirementReconciler, reviewReconciler)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		os.Exit(1)
	}
}

func startPoller(k8s client.Client, namespace string, drone *controllers.DroneReconciler, repo *controllers.ForgejoRepositoryReconciler, access *controllers.DroneRepositoryAccessReconciler, task *controllers.TaskRunReconciler, requirement *controllers.RequirementReconciler, review *controllers.ReviewReconciler) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx := context.Background()
		listOpts := []client.ListOption{}
		if namespace != "" {
			listOpts = append(listOpts, client.InNamespace(namespace))
		}
		var drones v1alpha1.DroneList
		if err := k8s.List(ctx, &drones, listOpts...); err == nil {
			for _, item := range drones.Items {
				_, _ = drone.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var repos v1alpha1.ForgejoRepositoryList
		if err := k8s.List(ctx, &repos, listOpts...); err == nil {
			for _, item := range repos.Items {
				_, _ = repo.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var accesses v1alpha1.DroneRepositoryAccessList
		if err := k8s.List(ctx, &accesses, listOpts...); err == nil {
			for _, item := range accesses.Items {
				_, _ = access.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var taskRuns v1alpha1.TaskRunList
		if err := k8s.List(ctx, &taskRuns, listOpts...); err == nil {
			for _, item := range taskRuns.Items {
				_, _ = task.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var requirements v1alpha1.RequirementList
		if err := k8s.List(ctx, &requirements, listOpts...); err == nil {
			for _, item := range requirements.Items {
				_, _ = requirement.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var reviews v1alpha1.ReviewList
		if err := k8s.List(ctx, &reviews, listOpts...); err == nil {
			for _, item := range reviews.Items {
				_, _ = review.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
	}
}

func startAPIServer(k8s client.Client, namespace string, cfg appconfig.Config, forgejo *forgejoclient.Client) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/webhooks/forgejo", forgejoWebhookHandler(k8s, namespace))
	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		listOpts := []client.ListOption{}
		if namespace != "" {
			listOpts = append(listOpts, client.InNamespace(namespace))
		}
		var drones v1alpha1.DroneList
		var tasks v1alpha1.TaskList
		var repos v1alpha1.RepositoryList
		var accesses v1alpha1.DroneRepositoryAccessList
		var requirements v1alpha1.RequirementList
		var reviews v1alpha1.ReviewList
		var jobs batchv1.JobList
		_ = k8s.List(ctx, &drones, listOpts...)
		_ = k8s.List(ctx, &tasks, listOpts...)
		_ = k8s.List(ctx, &repos, listOpts...)
		_ = k8s.List(ctx, &accesses, listOpts...)
		_ = k8s.List(ctx, &requirements, listOpts...)
		_ = k8s.List(ctx, &reviews, listOpts...)
		_ = k8s.List(ctx, &jobs, listOpts...)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"drones":       drones.Items,
			"tasks":        tasks.Items,
			"repositories": repos.Items,
			"accesses":     accesses.Items,
			"requirements": requirements.Items,
			"reviews":      reviews.Items,
			"jobs":         jobs.Items,
		})
	})
	mux.HandleFunc("/api/requirement-drafts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			RepositoryRef string   `json:"repositoryRef"`
			Title         string   `json:"title"`
			Body          string   `json:"body"`
			Status        string   `json:"status"`
			DependsOn     []string `json:"dependsOn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		slug := slugify(spec.Title)
		requirementPath := filepath.ToSlash(filepath.Join("requirements", slug+".md"))
		doc, err := requirementdoc.RenderDocument(requirementdoc.Document{Title: spec.Title, Slug: slug, Status: valueOr(spec.Status, "TODO"), DependsOn: spec.DependsOn, Branch: "req/" + slug, Body: spec.Body})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": slug, "filePath": requirementPath, "markdown": doc, "branch": "req/" + slug})
	})
	mux.HandleFunc("/api/requirements", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			Name          string   `json:"name"`
			Title         string   `json:"title"`
			RepositoryRef string   `json:"repositoryRef"`
			FilePath      string   `json:"filePath"`
			Status        string   `json:"status"`
			DependsOn     []string `json:"dependsOn"`
			Body          string   `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		var repo v1alpha1.Repository
		if err := k8s.Get(r.Context(), client.ObjectKey{Name: spec.RepositoryRef, Namespace: ns}, &repo); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		slug := slugify(firstNonEmpty(spec.Name, spec.Title, filepath.Base(spec.FilePath)))
		filePath := strings.TrimSpace(spec.FilePath)
		if filePath == "" {
			filePath = filepath.ToSlash(filepath.Join(repositoryRequirementsPath(&repo), slug+".md"))
		}
		markdown, err := requirementdoc.RenderDocument(requirementdoc.Document{
			Title:     spec.Title,
			Slug:      slug,
			Status:    valueOr(spec.Status, "TODO"),
			DependsOn: spec.DependsOn,
			Branch:    repositoryBranchPrefix(&repo) + slug,
			Body:      spec.Body,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if forgejo == nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "forgejo client unavailable"})
			return
		}
		if _, err := forgejo.EnsureRepository(r.Context(), repo.Spec.Owner, repo.Spec.Name, repo.Spec.Description, repositoryBaseBranch(&repo), repo.Spec.Private, repo.Spec.AutoInit); err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if err := forgejo.UpsertFile(r.Context(), repo.Spec.Owner, repo.Spec.Name, filePath, repositoryBaseBranch(&repo), "Add requirement "+slug, []byte(markdown)); err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		obj := &v1alpha1.Requirement{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Requirement"}, ObjectMeta: metav1.ObjectMeta{Name: slug, Namespace: ns}, Spec: v1alpha1.RequirementSpec{RepositoryRef: spec.RepositoryRef, FilePath: filePath}}
		if err := k8s.Create(r.Context(), obj); err != nil {
			if apierrors.IsAlreadyExists(err) {
				if getErr := k8s.Get(r.Context(), client.ObjectKey{Name: slug, Namespace: ns}, obj); getErr == nil {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(obj)
					return
				}
			}
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	})
	mux.HandleFunc("/api/task-drafts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			RepositoryRef string `json:"repositoryRef"`
			Brainstorm    string `json:"brainstorm"`
			TaskName      string `json:"taskName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		title := spec.TaskName
		if title == "" {
			title = spec.RepositoryRef + "-task"
		}
		prompt := "Work in repository " + spec.RepositoryRef + ".\n\nUser brainstorming:\n" + spec.Brainstorm + "\n\nTurn this into an actionable implementation task. The repository will already be checked out on a task branch. Make the smallest correct change, run relevant verification, and leave the repository ready to commit and push."
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": title, "prompt": prompt})
	})
	mux.HandleFunc("/api/drones", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			Name            string `json:"name"`
			Role            string `json:"role"`
			ForgejoUsername string `json:"forgejoUsername"`
			Model           string `json:"model"`
			Instructions    string `json:"instructions"`
			ProviderAuth    string `json:"providerAuth"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		obj := &v1alpha1.Drone{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Drone"}, ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns}, Spec: v1alpha1.DroneSpec{Role: spec.Role, ForgejoUsername: spec.ForgejoUsername, Forgejo: v1alpha1.ForgejoUserSpec{AutoProvision: true, Email: spec.ForgejoUsername + "@vinculum.local", DisplayName: spec.Name}, Image: cfg.DroneDefaultImage, Concurrency: 1, Model: spec.Model, InstructionInline: &v1alpha1.InlineFile{FileName: "AGENT.md", Content: spec.Instructions}, InstructionMountPath: "/instructions", ProviderAuthInline: &v1alpha1.InlineProviderAuth{FileKey: "auth.json", Content: spec.ProviderAuth}, Enabled: true}}
		if err := k8s.Create(r.Context(), obj); err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	})
	mux.HandleFunc("/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			Name        string `json:"name"`
			Owner       string `json:"owner"`
			Description string `json:"description"`
			Private     bool   `json:"private"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		obj := &v1alpha1.Repository{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Repository"}, ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns}, Spec: v1alpha1.RepositorySpec{Owner: spec.Owner, Name: spec.Name, Description: spec.Description, Private: spec.Private, AutoInit: true, DefaultBranch: "main"}}
		if err := k8s.Create(r.Context(), obj); err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	})
	mux.HandleFunc("/api/accesses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			Name          string `json:"name"`
			DroneRef      string `json:"droneRef"`
			RepositoryRef string `json:"repositoryRef"`
			Permission    string `json:"permission"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		obj := &v1alpha1.DroneRepositoryAccess{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "DroneRepositoryAccess"}, ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns}, Spec: v1alpha1.DroneRepositoryAccessSpec{DroneRef: spec.DroneRef, RepositoryRef: spec.RepositoryRef, Permission: spec.Permission}}
		if err := k8s.Create(r.Context(), obj); err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	})
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			Name             string `json:"name"`
			RequirementRef   string `json:"requirementRef"`
			TaskRef          string `json:"taskRef"`
			RepositoryRef    string `json:"repositoryRef"`
			ReviewerDroneRef string `json:"reviewerDroneRef"`
			Verdict          string `json:"verdict"`
			Summary          string `json:"summary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		obj := &v1alpha1.Review{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Review"}, ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns}, Spec: v1alpha1.ReviewSpec{RequirementRef: spec.RequirementRef, TaskRef: spec.TaskRef, RepositoryRef: spec.RepositoryRef, ReviewerDroneRef: spec.ReviewerDroneRef, Verdict: spec.Verdict, Summary: spec.Summary}}
		if err := k8s.Create(r.Context(), obj); err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	})
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var spec struct {
				Name     string `json:"name"`
				DroneRef string `json:"droneRef"`
			}
			if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			ns := namespace
			if ns == "" {
				ns = "default"
			}
			var obj v1alpha1.Task
			if err := k8s.Get(r.Context(), client.ObjectKey{Name: spec.Name, Namespace: ns}, &obj); err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			obj.Spec.DroneRef = spec.DroneRef
			if err := k8s.Update(r.Context(), &obj); err != nil {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(obj)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var spec struct {
			Name           string   `json:"name"`
			RequirementRef string   `json:"requirementRef,omitempty"`
			DroneRef       string   `json:"droneRef,omitempty"`
			RepositoryRef  string   `json:"repositoryRef"`
			DependsOn      []string `json:"dependsOn,omitempty"`
			Prompt         string   `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		obj := &v1alpha1.Task{TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "Task"}, ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: ns}, Spec: v1alpha1.TaskSpec{RequirementRef: spec.RequirementRef, DroneRef: spec.DroneRef, RepositoryRef: spec.RepositoryRef, BaseBranch: "main", WorkingBranch: spec.Name, DependsOn: spec.DependsOn, Role: "coder", Prompt: spec.Prompt, Instructions: true}}
		if err := k8s.Create(r.Context(), obj); err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(obj)
	})
	mux.HandleFunc("/api/taskruns", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/tasks"
		mux.ServeHTTP(w, r)
	})
	_ = http.ListenAndServe(":8084", mux)
}

func slugify(value string) string {
	trimmed := strings.TrimSpace(strings.TrimSuffix(value, filepath.Ext(value)))
	if trimmed == "" {
		return "requirement"
	}
	return strings.Trim(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, trimmed), "-")
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func repositoryRequirementsPath(repo *v1alpha1.Repository) string {
	if repo == nil || strings.TrimSpace(repo.Spec.RequirementsPath) == "" {
		return "requirements"
	}
	return filepath.ToSlash(strings.Trim(strings.TrimSpace(repo.Spec.RequirementsPath), "/"))
}

func repositoryBranchPrefix(repo *v1alpha1.Repository) string {
	if repo != nil && strings.TrimSpace(repo.Spec.RequirementBranchPrefix) != "" {
		return strings.TrimSpace(repo.Spec.RequirementBranchPrefix)
	}
	return "req/"
}

func repositoryBaseBranch(repo *v1alpha1.Repository) string {
	if repo != nil && strings.TrimSpace(repo.Spec.DefaultBaseBranch) != "" {
		return strings.TrimSpace(repo.Spec.DefaultBaseBranch)
	}
	if repo != nil && strings.TrimSpace(repo.Spec.DefaultBranch) != "" {
		return strings.TrimSpace(repo.Spec.DefaultBranch)
	}
	return "main"
}
