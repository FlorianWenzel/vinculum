package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "github.com/florian/vinculum/apps/operator/api/v1alpha1"
	"github.com/florian/vinculum/apps/operator/controllers"
	appconfig "github.com/florian/vinculum/apps/operator/internal/config"
	vmetrics "github.com/florian/vinculum/apps/operator/internal/metrics"
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
	vmetrics.Register()

	scheme := clientgoscheme.Scheme
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
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

	agentReconciler := &controllers.AgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    controllers.AgentReconcilerConfig{AgentDefaultImage: cfg.AgentDefaultImage, OperatorURL: cfg.OperatorURL},
	}
	taskReconciler := &controllers.TaskReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	scheduleReconciler := &controllers.AgentScheduleReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		os.Exit(1)
	}

	go startAPIServer(mgr.GetClient(), watchNamespace, cfg)
	ctrl.Log.Info("vinculum-operator manager configured", "watchNamespace", watchNamespace)
	go startPoller(mgr.GetClient(), watchNamespace, agentReconciler, taskReconciler, scheduleReconciler)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		os.Exit(1)
	}
}

func startPoller(k8s client.Client, namespace string, agent *controllers.AgentReconciler, task *controllers.TaskReconciler, schedule *controllers.AgentScheduleReconciler) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx := context.Background()
		listOpts := []client.ListOption{}
		if namespace != "" {
			listOpts = append(listOpts, client.InNamespace(namespace))
		}
		var agents v1alpha1.AgentList
		if err := k8s.List(ctx, &agents, listOpts...); err == nil {
			for _, item := range agents.Items {
				_, _ = agent.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var taskList v1alpha1.TaskList
		if err := k8s.List(ctx, &taskList, listOpts...); err == nil {
			for _, item := range taskList.Items {
				_, _ = task.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
		var schedules v1alpha1.AgentScheduleList
		if err := k8s.List(ctx, &schedules, listOpts...); err == nil {
			for _, item := range schedules.Items {
				_, _ = schedule.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
			}
		}
	}
}

func startAPIServer(k8s client.Client, namespace string, cfg appconfig.Config) {
	mux := newAPIMux(k8s, namespace, cfg)
	_ = http.ListenAndServe(":8084", mux)
}
