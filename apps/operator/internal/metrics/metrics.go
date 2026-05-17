// Package metrics defines the vinculum-operator's custom Prometheus
// metrics. They are registered on controller-runtime's metrics registry so
// they ship alongside the standard reconcile-loop metrics on :8082/metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// TasksTotal counts Task lifecycle events as they enter a new phase.
	// Incremented once per phase transition; not gauged, since a Task's
	// time-in-phase is captured by TaskDurationSeconds instead.
	TasksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vinculum_tasks_total",
			Help: "Total Task phase transitions observed by the operator, labeled by agent and terminal phase.",
		},
		[]string{"agent", "phase"},
	)

	// TaskDurationSeconds is observed once per Task when it reaches a
	// terminal phase. Buckets cover a few seconds (LLM no-op) through ~30
	// minutes (heavy multi-tool runs).
	TaskDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vinculum_task_duration_seconds",
			Help:    "Wall-clock duration of a Task from StartTime to CompletionTime, by agent and terminal phase.",
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200, 1800, 3600},
		},
		[]string{"agent", "phase"},
	)

	// AgentReady is a gauge per Agent: 1 when the operator considers the
	// Agent ready, 0 otherwise.
	AgentReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vinculum_agent_ready",
			Help: "1 when the named Agent's pod is Ready, 0 otherwise.",
		},
		[]string{"agent"},
	)

	// OrchestratorDispatchesTotal counts Tasks created via the operator
	// HTTP API by an in-cluster orchestrator agent (the vnclm-mcp adds an
	// X-Vinculum-From-Agent header on its POST /api/tasks calls).
	OrchestratorDispatchesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vinculum_orchestrator_dispatches_total",
			Help: "Tasks created by an orchestrator Agent via the operator API, labeled by source (from) and target (to) agent names.",
		},
		[]string{"from", "to"},
	)

	// MessagesTotal counts Message lifecycle events (creation and terminal
	// transitions) so peer chatter is observable in Prometheus alongside
	// Task metrics.
	MessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vinculum_messages_total",
			Help: "Peer Message phase transitions, labeled by sender (from), receiver (to), and phase.",
		},
		[]string{"from", "to", "phase"},
	)
)

// Register registers all vinculum metrics on the controller-runtime
// registry. Idempotent: subsequent calls are no-ops.
func Register() {
	for _, c := range []prometheus.Collector{
		TasksTotal,
		TaskDurationSeconds,
		AgentReady,
		OrchestratorDispatchesTotal,
		MessagesTotal,
	} {
		// MustRegister panics on AlreadyRegisteredError; use Register so a
		// second call (e.g. controller restart in tests) is a no-op.
		if err := ctrlmetrics.Registry.Register(c); err != nil {
			if _, dup := err.(prometheus.AlreadyRegisteredError); !dup {
				panic(err)
			}
		}
	}
}
