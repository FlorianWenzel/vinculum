package commands

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
)

func logsCmd() *cobra.Command {
	var follow bool
	c := &cobra.Command{
		Use:               "logs <task>",
		Short:             "Stream crush logs for a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrTasks),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			kc, err := loadKube()
			if err != nil {
				return err
			}

			// Look up the task via the operator API to resolve its agent.
			var task client.Task
			if err := withOperator(ctx, kc, func(h *client.HTTP) error {
				return h.GetJSON(ctx, "/api/tasks/"+args[0], &task)
			}); err != nil {
				return err
			}
			if task.Spec.AgentRef == "" {
				return errf("task %s has no agentRef", args[0])
			}
			return withAgent(ctx, kc, task.Spec.AgentRef, func(h *client.HTTP) error {
				path := "/tasks/" + args[0] + "/logs"
				if follow {
					path += "?follow=1"
				}
				return h.Stream(ctx, path, os.Stdout)
			})
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "follow live output (blocks until task terminal)")
	return c
}

