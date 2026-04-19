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
	"github.com/florian/vinculum/apps/vnclm/internal/theme"
)

func getCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "get",
		Aliases: []string{"ls", "list", "l"},
		Short:   "Get resources (list when no name, single when name given)",
	}
	c.AddCommand(
		getAgentsCmd(),
		getTasksCmd(),
		getSchedulesCmd(),
		getProvidersCmd(),
		getMCPsCmd(),
	)
	return c
}

func getAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "agents [name]",
		Aliases:           []string{"agent", "a"},
		Short:             "List agents or get one by name",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeResource(gvrAgents),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if len(args) == 1 {
					var a client.Agent
					if err := h.GetJSON(ctx, "/api/agents/"+args[0], &a); err != nil {
						return err
					}
					return writeObject(a)
				}
				var list []client.Agent
				if err := h.GetJSON(ctx, "/api/agents", &list); err != nil {
					return err
				}
				return renderAgents(list)
			})
		},
	}
}

func getTasksCmd() *cobra.Command {
	var agentFilter string
	c := &cobra.Command{
		Use:               "tasks [name]",
		Aliases:           []string{"task", "t"},
		Short:             "List tasks or get one by name",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeResource(gvrTasks),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if len(args) == 1 {
					var t client.Task
					if err := h.GetJSON(ctx, "/api/tasks/"+args[0], &t); err != nil {
						return err
					}
					return writeObject(t)
				}
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
	c.Flags().StringVar(&agentFilter, "agent", "", "filter by agent name (list mode)")
	return c
}

func getSchedulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "schedules [name]",
		Aliases:           []string{"schedule", "s"},
		Short:             "List schedules or get one by name",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeResource(gvrSchedules),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if len(args) == 1 {
					var s client.Schedule
					if err := h.GetJSON(ctx, "/api/schedules/"+args[0], &s); err != nil {
						return err
					}
					return writeObject(s)
				}
				var list []client.Schedule
				if err := h.GetJSON(ctx, "/api/schedules", &list); err != nil {
					return err
				}
				return renderSchedules(list)
			})
		},
	}
}

func getProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "providers [name]",
		Aliases:           []string{"provider", "p"},
		Short:             "List provider Secrets or get one by name",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProviders(),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				sec, err := kc.Core.CoreV1().Secrets(kc.Namespace).Get(ctx, args[0], metav1.GetOptions{})
				if err != nil {
					return err
				}
				if sec.Labels["vinculum.dev/provider"] != "true" {
					return errf("secret %s is not a vinculum provider (missing label)", args[0])
				}
				p := client.Provider{
					Name:      sec.Name,
					Namespace: sec.Namespace,
					Type:      sec.Annotations["vinculum.dev/provider-type"],
				}
				for k := range sec.Data {
					p.Keys = append(p.Keys, k)
				}
				return writeObject(p)
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

func getMCPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "mcps [name]",
		Aliases:           []string{"mcp", "mcpserver", "mcpservers", "m"},
		Short:             "List MCPServers or get one by name",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeResource(gvrMCPs),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if len(args) == 1 {
					var m client.MCPServer
					if err := h.GetJSON(ctx, "/api/mcps/"+args[0], &m); err != nil {
						return err
					}
					return writeObject(m)
				}
				var list []client.MCPServer
				if err := h.GetJSON(ctx, "/api/mcps", &list); err != nil {
					return err
				}
				return renderMCPs(list)
			})
		},
	}
}

func writeObject(v any) error {
	format, err := output.Parse(state.outputFormat)
	if err != nil {
		return err
	}
	switch format {
	case output.JSON:
		return output.WriteJSON(os.Stdout, v)
	}
	return output.WriteYAML(os.Stdout, v)
}

// combineStylers applies Borg header styling then falls through to per-cell.
func combineStylers(cell func(row, col int, s string) string) output.Styler {
	return func(row, col int, s string) string {
		if row == -1 {
			return theme.Style(theme.Header, s)
		}
		return cell(row, col, s)
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
	styler := combineStylers(func(_, col int, s string) string {
		switch col {
		case 0:
			return theme.Style(theme.Name, s)
		case 2, 3:
			return theme.Bool(s)
		}
		return ""
	})
	return output.WriteTable(os.Stdout, headers, rows, styler)
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
	styler := combineStylers(func(_, col int, s string) string {
		switch col {
		case 0:
			return theme.Style(theme.Name, s)
		case 1:
			return theme.Style(theme.Accent, s)
		case 2:
			return theme.Phase(s)
		}
		return ""
	})
	return output.WriteTable(os.Stdout, headers, rows, styler)
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
	styler := combineStylers(func(_, col int, s string) string {
		switch col {
		case 0:
			return theme.Style(theme.Name, s)
		case 1:
			return theme.Style(theme.Accent, s)
		case 2:
			return theme.Style(theme.Body, s)
		case 3:
			return theme.Bool(s)
		}
		return ""
	})
	return output.WriteTable(os.Stdout, headers, rows, styler)
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
	styler := combineStylers(func(_, col int, s string) string {
		switch col {
		case 0:
			return theme.Style(theme.Name, s)
		case 1:
			return theme.Style(theme.Accent, s)
		case 2:
			return theme.Style(theme.Dim, s)
		}
		return ""
	})
	return output.WriteTable(os.Stdout, headers, rows, styler)
}

func renderMCPs(list []client.MCPServer) error {
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
	headers := []string{"NAME", "TYPE", "TARGET", "SECRET", "ENABLED", "AGE"}
	var rows []output.Row
	for _, m := range list {
		transport := "stdio"
		target := m.Spec.Command
		if m.Spec.URL != "" {
			transport = "http"
			target = m.Spec.URL
		}
		if m.Spec.Command != "" && len(m.Spec.Args) > 0 {
			target = m.Spec.Command + " " + strings.Join(m.Spec.Args, " ")
		}
		secret := ""
		if m.Spec.SecretRef != nil {
			secret = m.Spec.SecretRef.Name
		}
		enabled := "false"
		if m.Spec.Enabled {
			enabled = "true"
		}
		rows = append(rows, output.Row{m.Metadata.Name, transport, target, secret, enabled, ageSince(m.Metadata.CreationTimestamp)})
	}
	styler := combineStylers(func(_, col int, s string) string {
		switch col {
		case 0:
			return theme.Style(theme.Name, s)
		case 1:
			return theme.Style(theme.Accent, s)
		case 2:
			return theme.Style(theme.Dim, s)
		case 4:
			return theme.Bool(s)
		}
		return ""
	})
	return output.WriteTable(os.Stdout, headers, rows, styler)
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
