package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
	"github.com/florian/vinculum/apps/vnclm/internal/kube"
	"github.com/florian/vinculum/apps/vnclm/internal/theme"
)

func createMCPCmd() *cobra.Command {
	var (
		name      string
		url       string
		command   string
		args      []string
		envPairs  map[string]string
		secretRef string
		enabled   = true
		timeout   int32 = 120
	)
	c := &cobra.Command{
		Use:     "mcp",
		Aliases: []string{"mcpserver"},
		Short:   "Create an MCPServer (wizard by default)",
		RunE: func(cmd *cobra.Command, args2 []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}

			if name == "" || (url == "" && command == "") {
				if err := runMCPWizard(ctx, kc, &name, &url, &command, &args, envPairs, &secretRef, &enabled, &timeout); err != nil {
					return err
				}
			}
			if name == "" {
				return errf("name required")
			}
			if (url == "") == (command == "") {
				return errf("exactly one of --url or --command must be set")
			}

			spec := map[string]any{
				"enabled": enabled,
				"timeout": timeout,
			}
			if url != "" {
				spec["url"] = url
			}
			if command != "" {
				spec["command"] = command
				if len(args) > 0 {
					spec["args"] = args
				}
			}
			if len(envPairs) > 0 {
				spec["env"] = envPairs
			}
			if secretRef != "" {
				spec["secretRef"] = map[string]string{"name": secretRef}
			}

			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.PostJSON(ctx, "/api/mcps", map[string]any{"name": name, "spec": spec}, nil); err != nil {
					return err
				}
				fmt.Printf("mcp %q created\n", name)
				return nil
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "mcp server name")
	c.Flags().StringVar(&url, "url", "", "http transport URL")
	c.Flags().StringVar(&command, "command", "", "stdio command")
	c.Flags().StringArrayVar(&args, "arg", nil, "stdio arg (repeatable)")
	c.Flags().StringToStringVar(&envPairs, "env", nil, "env k=v (repeatable)")
	c.Flags().StringVar(&secretRef, "secret-ref", "", "Secret name whose keys are injected into the agent pod")
	c.Flags().BoolVar(&enabled, "enabled", true, "enable this server")
	c.Flags().Int32Var(&timeout, "timeout", 120, "crush mcp timeout seconds")
	return c
}

func runMCPWizard(ctx context.Context, kc *kube.Client, name, url, command *string, args *[]string, envPairs map[string]string, secretRef *string, enabled *bool, timeout *int32) error {
	transport := "stdio"
	if *url != "" {
		transport = "http"
	}
	argsStr := strings.Join(*args, " ")
	envStr := flattenEnv(envPairs)
	timeoutStr := strconv.Itoa(int(*timeout))

	secretNames, _ := listSecretNames(ctx, kc)
	secretOpts := []huh.Option[string]{huh.NewOption("(none)", "")}
	for _, n := range secretNames {
		secretOpts = append(secretOpts, huh.NewOption(n, n))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(name).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Transport").Options(
				huh.NewOption("stdio (spawn local process)", "stdio"),
				huh.NewOption("http (connect to URL)", "http"),
			).Value(&transport),
		),
		huh.NewGroup(
			huh.NewInput().Title("URL").Value(url).Validate(nonEmpty),
		).WithHideFunc(func() bool { return transport != "http" }),
		huh.NewGroup(
			huh.NewInput().Title("Command (e.g. npx)").Value(command).Validate(nonEmpty),
			huh.NewInput().Title("Args (space-separated)").Value(&argsStr),
		).WithHideFunc(func() bool { return transport != "stdio" }),
		huh.NewGroup(
			huh.NewInput().Title("Env (k=v,k2=v2)").Value(&envStr),
			huh.NewSelect[string]().Title("Secret ref (envFrom)").Options(secretOpts...).Value(secretRef),
			huh.NewInput().Title("Timeout (seconds)").Value(&timeoutStr),
			huh.NewConfirm().Title("Enabled?").Value(enabled),
		),
	).WithTheme(theme.Huh())
	if err := form.Run(); err != nil {
		return err
	}
	if argsStr != "" {
		*args = splitSpace(argsStr)
	}
	if envStr != "" {
		for k, v := range parseEnv(envStr) {
			envPairs[k] = v
		}
	}
	if n, err := strconv.Atoi(timeoutStr); err == nil {
		*timeout = int32(n)
	}
	return nil
}

func listSecretNames(ctx context.Context, kc *kube.Client) ([]string, error) {
	secrets, err := kc.Core.CoreV1().Secrets(kc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(secrets.Items))
	for _, s := range secrets.Items {
		out = append(out, s.Name)
	}
	return out, nil
}

func listMCPNames(ctx context.Context, kc *kube.Client) ([]string, error) {
	var out []string
	err := withOperator(ctx, kc, func(h *client.HTTP) error {
		var list []client.MCPServer
		if err := h.GetJSON(ctx, "/api/mcps", &list); err != nil {
			return err
		}
		for _, m := range list {
			out = append(out, m.Metadata.Name)
		}
		return nil
	})
	return out, err
}

func flattenEnv(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func parseEnv(s string) map[string]string {
	out := map[string]string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

func splitSpace(s string) []string {
	fields := strings.Fields(s)
	return fields
}
