package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
	"github.com/florian/vinculum/apps/vnclm/internal/config"
	"github.com/florian/vinculum/apps/vnclm/internal/kube"
)

type rootState struct {
	contextOverride   string
	namespaceOverride string
	outputFormat      string
	yes               bool

	cfg     config.Config
	cfgPath string
}

var state = &rootState{}

// Root returns the top-level cobra command.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "vnclm",
		Short:         "vinculum CLI",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			state.cfg = cfg
			state.cfgPath = path
			return nil
		},
	}
	root.PersistentFlags().StringVar(&state.contextOverride, "context", "", "kube context (default: current)")
	root.PersistentFlags().StringVarP(&state.namespaceOverride, "namespace", "n", "", "namespace (default: kube context's)")
	root.PersistentFlags().StringVarP(&state.outputFormat, "output", "o", "table", "output: table|wide|json|yaml")
	root.PersistentFlags().BoolVar(&state.yes, "yes", false, "skip confirmation prompts")
	_ = root.RegisterFlagCompletionFunc("output", staticValues("table", "wide", "json", "yaml"))

	root.AddCommand(
		ctxCmd(),
		listCmd(),
		getCmd(),
		deleteCmd(),
		createCmd(),
		logsCmd(),
		runCmd(),
	)
	return root
}

// loadKube builds a kube client respecting flags.
func loadKube() (*kube.Client, error) {
	return kube.Load(state.contextOverride, state.namespaceOverride)
}

// withOperator starts a port-forward to the operator service and calls fn
// with an HTTP client pointed at the local tunnel. Tears it down on return.
func withOperator(ctx context.Context, kc *kube.Client, fn func(*client.HTTP) error) error {
	pf, err := kc.ForwardService(ctx, kc.Namespace, state.cfg.OperatorService, state.cfg.OperatorPort)
	if err != nil {
		return fmt.Errorf("port-forward operator: %w", err)
	}
	defer pf.Close()
	return fn(client.New(pf.URL()))
}

// withAgent starts a port-forward to the agent service (agent-<name>:8090).
func withAgent(ctx context.Context, kc *kube.Client, agentName string, fn func(*client.HTTP) error) error {
	svc := "agent-" + agentName
	pf, err := kc.ForwardService(ctx, kc.Namespace, svc, 8090)
	if err != nil {
		return fmt.Errorf("port-forward agent %s: %w", agentName, err)
	}
	defer pf.Close()
	return fn(client.New(pf.URL()))
}

func errf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func fprintln(a ...any) {
	fmt.Fprintln(os.Stdout, a...)
}
