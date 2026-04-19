package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
	"github.com/florian/vinculum/apps/vnclm/internal/output"
)

func listCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "l"},
		Short:   "List resources",
	}
	c.AddCommand(listAgentsCmd(), listTasksCmd(), listSchedulesCmd(), listProvidersCmd())
	return c
}

func listAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "agents",
		Aliases: []string{"agent", "a"},
		Short:   "List agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				var list []client.Agent
				if err := h.GetJSON(ctx, "/api/agents", &list); err != nil {
					return err
				}
				return renderAgents(list)
			})
		},
	}
}

func renderAgents(list []client.Agent) error {
	format, err := output.Parse(state.outputFormat)
	if err != nil {
		return err
	}
	switch format {
	case output.JSON:
		return output.WriteJSON(os.Stdout, list)
	case output.YAML:
		return output.WriteYAML(os.Stdout, list)
	}
	headers := []string{"NAME", "MODEL", "READY", "ENABLED", "AGE"}
	if format == output.Wide {
		headers = []string{"NAME", "MODEL", "READY", "ENABLED", "PROVIDER", "PVC", "AGE"}
	}
	var rows []output.Row
	for _, a := range list {
		age := ageSince(a.Metadata.CreationTimestamp)
		ready := "false"
		if a.Status.Ready {
			ready = "true"
		}
		enabled := "false"
		if a.Spec.Enabled {
			enabled = "true"
		}
		row := output.Row{a.Metadata.Name, a.Spec.Model, ready, enabled, age}
		if format == output.Wide {
			provider := ""
			if a.Spec.ProviderSecretRef != nil {
				provider = a.Spec.ProviderSecretRef.Name
			}
			row = output.Row{a.Metadata.Name, a.Spec.Model, ready, enabled, provider, a.Status.PVCName, age}
		}
		rows = append(rows, row)
	}
	return output.WriteTable(os.Stdout, headers, rows)
}

func listTasksCmd() *cobra.Command {
	var agentFilter string
	c := &cobra.Command{
		Use:     "tasks",
		Aliases: []string{"task", "t"},
		Short:   "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				var list []client.Task
				if err := h.GetJSON(ctx, "/api/tasks", &list); err != nil {
					return err
				}
				if agentFilter != "" {
					filtered := list[:0]
					for _, t := range list {
						if t.Spec.AgentRef == agentFilter {
							filtered = append(filtered, t)
						}
					}
					list = filtered
				}
				return renderTasks(list)
			})
		},
	}
	c.Flags().StringVar(&agentFilter, "agent", "", "filter by agent name")
	return c
}

func renderTasks(list []client.Task) error {
	format, err := output.Parse(state.outputFormat)
	if err != nil {
		return err
	}
	switch format {
	case output.JSON:
		return output.WriteJSON(os.Stdout, list)
	case output.YAML:
		return output.WriteYAML(os.Stdout, list)
	}
	headers := []string{"NAME", "AGENT", "PHASE", "EXIT", "AGE"}
	if format == output.Wide {
		headers = []string{"NAME", "AGENT", "PHASE", "EXIT", "REASON", "POD", "DURATION", "AGE"}
	}
	var rows []output.Row
	for _, t := range list {
		age := ageSince(t.Metadata.CreationTimestamp)
		exit := "-"
		if !t.Status.CompletionTime.IsZero() {
			exit = itoa(t.Status.ExitCode)
		}
		row := output.Row{t.Metadata.Name, t.Spec.AgentRef, t.Status.Phase, exit, age}
		if format == output.Wide {
			dur := "-"
			if !t.Status.StartTime.IsZero() {
				end := t.Status.CompletionTime
				if end.IsZero() {
					end = time.Now()
				}
				dur = end.Sub(t.Status.StartTime).Round(time.Second).String()
			}
			row = output.Row{t.Metadata.Name, t.Spec.AgentRef, t.Status.Phase, exit, t.Status.Reason, t.Status.PodName, dur, age}
		}
		rows = append(rows, row)
	}
	return output.WriteTable(os.Stdout, headers, rows)
}

func listSchedulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "schedules",
		Aliases: []string{"schedule", "s"},
		Short:   "List schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				var list []client.Schedule
				if err := h.GetJSON(ctx, "/api/schedules", &list); err != nil {
					return err
				}
				return renderSchedules(list)
			})
		},
	}
}

func renderSchedules(list []client.Schedule) error {
	format, err := output.Parse(state.outputFormat)
	if err != nil {
		return err
	}
	switch format {
	case output.JSON:
		return output.WriteJSON(os.Stdout, list)
	case output.YAML:
		return output.WriteYAML(os.Stdout, list)
	}
	headers := []string{"NAME", "AGENT", "SCHEDULE", "SUSPEND", "AGE"}
	var rows []output.Row
	for _, s := range list {
		suspend := "false"
		if s.Spec.Suspend {
			suspend = "true"
		}
		rows = append(rows, output.Row{s.Metadata.Name, s.Spec.AgentRef, s.Spec.Schedule, suspend, ageSince(s.Metadata.CreationTimestamp)})
	}
	return output.WriteTable(os.Stdout, headers, rows)
}

func listProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "providers",
		Aliases: []string{"provider", "p"},
		Short:   "List provider Secrets (labeled vinculum.dev/provider=true)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			secrets, err := kc.Core.CoreV1().Secrets(kc.Namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "vinculum.dev/provider=true",
			})
			if err != nil {
				return err
			}
			providers := make([]client.Provider, 0, len(secrets.Items))
			for _, s := range secrets.Items {
				p := client.Provider{
					Name:      s.Name,
					Namespace: s.Namespace,
					Type:      s.Annotations["vinculum.dev/provider-type"],
				}
				for k := range s.Data {
					p.Keys = append(p.Keys, k)
				}
				providers = append(providers, p)
			}
			return renderProviders(providers)
		},
	}
}

func renderProviders(list []client.Provider) error {
	format, err := output.Parse(state.outputFormat)
	if err != nil {
		return err
	}
	switch format {
	case output.JSON:
		return output.WriteJSON(os.Stdout, list)
	case output.YAML:
		return output.WriteYAML(os.Stdout, list)
	}
	headers := []string{"NAME", "TYPE", "KEYS"}
	var rows []output.Row
	for _, p := range list {
		rows = append(rows, output.Row{p.Name, p.Type, strings.Join(p.Keys, ",")})
	}
	return output.WriteTable(os.Stdout, headers, rows)
}

func ageSince(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }
