package commands

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
)

func deleteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm", "del"},
		Short:   "Delete a resource",
	}
	c.AddCommand(
		deleteAgentCmd(),
		deleteTaskCmd(),
		deleteScheduleCmd(),
		deleteProviderCmd(),
	)
	return c
}

func confirm(what, name string) bool {
	if state.yes {
		return true
	}
	fmt.Printf("delete %s %q? [y/N] ", what, name)
	var resp string
	_, _ = fmt.Scanln(&resp)
	return resp == "y" || resp == "Y" || resp == "yes"
}

func deleteAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "agent <name>",
		Short:             "Delete agent (cascades owned resources)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrAgents),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm("agent", args[0]) {
				return errf("aborted")
			}
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.Delete(ctx, "/api/agents/"+args[0]); err != nil {
					return err
				}
				fmt.Printf("agent %q deleted\n", args[0])
				return nil
			})
		},
	}
}

func deleteTaskCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "task <name>",
		Short:             "Delete task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrTasks),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm("task", args[0]) {
				return errf("aborted")
			}
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.Delete(ctx, "/api/tasks/"+args[0]); err != nil {
					return err
				}
				fmt.Printf("task %q deleted\n", args[0])
				return nil
			})
		},
	}
}

func deleteScheduleCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "schedule <name>",
		Short:             "Delete schedule",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrSchedules),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm("schedule", args[0]) {
				return errf("aborted")
			}
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.Delete(ctx, "/api/schedules/"+args[0]); err != nil {
					return err
				}
				fmt.Printf("schedule %q deleted\n", args[0])
				return nil
			})
		},
	}
}

func deleteProviderCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "provider <name>",
		Short:             "Delete provider Secret",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProviders(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirm("provider", args[0]) {
				return errf("aborted")
			}
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
				return errf("secret %s is not a vinculum provider", args[0])
			}
			if err := kc.Core.CoreV1().Secrets(kc.Namespace).Delete(ctx, args[0], metav1.DeleteOptions{}); err != nil {
				return err
			}
			fmt.Printf("provider %q deleted\n", args[0])
			return nil
		},
	}
}
