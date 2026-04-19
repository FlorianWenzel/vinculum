package commands

import (
	"context"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
	"github.com/florian/vinculum/apps/vnclm/internal/output"
)

func getCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "get",
		Short: "Get a single resource (agent|task|schedule|provider)",
	}
	c.AddCommand(
		getAgentCmd(),
		getTaskCmd(),
		getScheduleCmd(),
		getProviderCmd(),
	)
	return c
}

func writeObject(v any) error {
	format, err := output.Parse(state.outputFormat)
	if err != nil {
		return err
	}
	switch format {
	case output.JSON:
		return output.WriteJSON(os.Stdout, v)
	case output.YAML, output.Table, output.Wide:
		// For single-object get, default to YAML — it's the most useful form.
		return output.WriteYAML(os.Stdout, v)
	}
	return output.WriteYAML(os.Stdout, v)
}

func getAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "agent <name>",
		Short:             "Get agent by name",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrAgents),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				var a client.Agent
				if err := h.GetJSON(ctx, "/api/agents/"+args[0], &a); err != nil {
					return err
				}
				return writeObject(a)
			})
		},
	}
}

func getTaskCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "task <name>",
		Short:             "Get task by name",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrTasks),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				var t client.Task
				if err := h.GetJSON(ctx, "/api/tasks/"+args[0], &t); err != nil {
					return err
				}
				return writeObject(t)
			})
		},
	}
}

func getScheduleCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "schedule <name>",
		Short:             "Get schedule by name",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrSchedules),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				var s client.Schedule
				if err := h.GetJSON(ctx, "/api/schedules/"+args[0], &s); err != nil {
					return err
				}
				return writeObject(s)
			})
		},
	}
}

func getProviderCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "provider <name>",
		Short:             "Get provider (Secret) by name",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProviders(),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
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
		},
	}
}
