package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/config"
)

func ctxCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ctx",
		Short: "Show or change vnclm context (current agent, etc.)",
	}
	c.AddCommand(ctxShowCmd(), ctxSetAgentCmd(), ctxClearAgentCmd(), ctxCurrentAgentCmd())
	return c
}

// ctxCurrentAgentCmd prints just the currentAgent name (or empty) to stdout.
// Intended for shell prompts (starship custom module, tmux status-line, etc.).
// No cluster calls, no errors for unset agent — exits 0 with no output.
func ctxCurrentAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current-agent",
		Short: "Print currentAgent name (empty if unset). Safe for shell prompts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if state.cfg.CurrentAgent != "" {
				fmt.Println(state.cfg.CurrentAgent)
			}
			return nil
		},
	}
}

func ctxShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print current vnclm context",
		RunE: func(cmd *cobra.Command, args []string) error {
			kc, err := loadKube()
			if err != nil {
				return err
			}
			fmt.Printf("kubeContext:   %s\n", kc.Context)
			fmt.Printf("namespace:     %s\n", kc.Namespace)
			fmt.Printf("currentAgent:  %s\n", state.cfg.CurrentAgent)
			fmt.Printf("operatorSvc:   %s:%d\n", state.cfg.OperatorService, state.cfg.OperatorPort)
			fmt.Printf("configPath:    %s\n", state.cfgPath)
			return nil
		},
	}
}

func ctxSetAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "set-agent <name>",
		Short:             "Set currentAgent for run/logs",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeResource(gvrAgents),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := state.cfg
			cfg.CurrentAgent = args[0]
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("currentAgent set to %s\n", args[0])
			return nil
		},
	}
}

func ctxClearAgentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-agent",
		Short: "Clear currentAgent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := state.cfg
			cfg.CurrentAgent = ""
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("currentAgent cleared")
			return nil
		},
	}
}
