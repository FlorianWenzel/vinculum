package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
)

func isNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "404")
}

func isTerminal(p string) bool {
	switch p {
	case "Succeeded", "Failed", "TimedOut":
		return true
	}
	return false
}

func runCmd() *cobra.Command {
	var (
		agent   string
		name    string
		fresh   bool
		wsMode  = "shared"
		timeout int32 = 300
	)
	c := &cobra.Command{
		Use:   "run <prompt>",
		Short: "One-shot: create a Task on currentAgent, stream logs, wait for terminal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			if agent == "" {
				agent = state.cfg.CurrentAgent
			}
			if agent == "" {
				return errf("no agent — pass --agent or set one via `vnclm ctx set-agent`")
			}
			if name == "" {
				name = fmt.Sprintf("run-%d", time.Now().Unix())
			}
			body := map[string]any{
				"name": name,
				"spec": map[string]any{
					"agentRef":       agent,
					"prompt":         args[0],
					"fresh":          fresh,
					"workspace":      map[string]any{"mode": wsMode},
					"timeoutSeconds": timeout,
				},
			}
			if err := withOperator(ctx, kc, func(h *client.HTTP) error {
				return h.PostJSON(ctx, "/api/tasks", body, nil)
			}); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "» task %s dispatched to agent %s\n", name, agent)

			// Stream logs (follow mode). Retry on 404 while task not yet accepted.
			if err := withAgent(ctx, kc, agent, func(h *client.HTTP) error {
				deadline := time.Now().Add(30 * time.Second)
				for {
					err := h.Stream(ctx, "/tasks/"+name+"/logs?follow=1", os.Stdout)
					if err == nil {
						return nil
					}
					if time.Now().After(deadline) || !isNotFound(err) {
						return err
					}
					time.Sleep(500 * time.Millisecond)
				}
			}); err != nil {
				fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
			}

			// Poll final status until terminal (stream can close before status patched).
			var final client.Task
			deadline := time.Now().Add(30 * time.Second)
			for {
				if err := withOperator(ctx, kc, func(h *client.HTTP) error {
					return h.GetJSON(ctx, "/api/tasks/"+name, &final)
				}); err != nil {
					return err
				}
				if isTerminal(final.Status.Phase) || time.Now().After(deadline) {
					break
				}
				time.Sleep(500 * time.Millisecond)
			}
			fmt.Fprintf(os.Stderr, "» phase=%s exit=%d\n", final.Status.Phase, final.Status.ExitCode)
			if final.Status.Phase != "Succeeded" {
				return errf("task %s ended phase=%s reason=%s", name, final.Status.Phase, final.Status.Reason)
			}
			return nil
		},
	}
	c.Flags().StringVar(&agent, "agent", "", "agent (default: currentAgent)")
	c.Flags().StringVar(&name, "name", "", "task name (default: run-<unix>)")
	c.Flags().BoolVar(&fresh, "fresh", false, "no --continue")
	c.Flags().StringVar(&wsMode, "workspace", "shared", "shared|ephemeral")
	c.Flags().Int32Var(&timeout, "timeout", 300, "task timeout seconds")
	_ = c.RegisterFlagCompletionFunc("agent", completeResource(gvrAgents))
	_ = c.RegisterFlagCompletionFunc("workspace", staticValues("shared", "ephemeral"))
	return c
}
