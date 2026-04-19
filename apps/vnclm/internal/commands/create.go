package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/client"
	"github.com/florian/vinculum/apps/vnclm/internal/kube"
)

func createCmd() *cobra.Command {
	var fromFile string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a resource (wizard, flags, or -f file.yaml)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromFile == "" {
				return cmd.Help()
			}
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			_, err = applyYAMLFile(ctx, kc, fromFile)
			return err
		},
	}
	c.PersistentFlags().StringVarP(&fromFile, "file", "f", "", "YAML file to apply")

	c.AddCommand(
		createAgentCmd(&fromFile),
		createTaskCmd(&fromFile),
		createScheduleCmd(&fromFile),
		createProviderCmd(),
	)
	return c
}

// applyYAMLFile parses a multi-doc YAML file and applies each doc via the
// dynamic client. Returns the number of objects created.
func applyYAMLFile(ctx context.Context, kc *kube.Client, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	decoder := yaml.NewYAMLOrJSONDecoder(f, 4096)
	count := 0
	for {
		raw := map[string]any{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return count, err
		}
		if len(raw) == 0 {
			continue
		}
		u := &unstructured.Unstructured{Object: raw}
		gvk := u.GroupVersionKind()
		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: strings.ToLower(gvk.Kind) + "s",
		}
		ns := u.GetNamespace()
		if ns == "" {
			ns = kc.Namespace
			u.SetNamespace(ns)
		}
		_, err := kc.Dynamic.Resource(gvr).Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
		if err != nil {
			return count, fmt.Errorf("create %s %q: %w", gvk.Kind, u.GetName(), err)
		}
		fmt.Printf("%s %q created\n", gvk.Kind, u.GetName())
		count++
	}
	return count, nil
}

func createAgentCmd(fromFile *string) *cobra.Command {
	var (
		name         string
		model        string
		provider     string
		instructions string
		enabled      = true
		concurrency  int32
		workspaceSize = "10Gi"
		image        string
	)
	c := &cobra.Command{
		Use:   "agent",
		Short: "Create an Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			if *fromFile != "" {
				_, err := applyYAMLFile(ctx, kc, *fromFile)
				return err
			}

			if name == "" {
				if err := runAgentWizard(ctx, kc, &name, &model, &provider, &instructions, &enabled, &concurrency, &workspaceSize); err != nil {
					return err
				}
			}
			if name == "" {
				return errf("name required")
			}

			body := map[string]any{
				"name":              name,
				"model":             model,
				"image":             image,
				"concurrency":       concurrency,
				"instructions":      instructions,
				"providerSecretRef": provider,
				"enabled":           enabled,
				"workspaceSize":     workspaceSize,
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.PostJSON(ctx, "/api/agents", body, nil); err != nil {
					return err
				}
				fmt.Printf("agent %q created\n", name)
				return nil
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "agent name")
	c.Flags().StringVar(&model, "model", "", "model (e.g. azure/gpt-4o)")
	c.Flags().StringVar(&provider, "provider", "", "provider Secret name")
	c.Flags().StringVar(&instructions, "instructions", "", "inline AGENTS.md content")
	c.Flags().BoolVar(&enabled, "enabled", true, "enable agent")
	c.Flags().Int32Var(&concurrency, "concurrency", 1, "")
	c.Flags().StringVar(&workspaceSize, "workspace-size", "10Gi", "PVC size for /workspace")
	c.Flags().StringVar(&image, "image", "", "agent container image (default: operator's default)")
	_ = c.RegisterFlagCompletionFunc("provider", completeProviders())
	return c
}

func runAgentWizard(ctx context.Context, kc *kube.Client, name, model, provider, instructions *string, enabled *bool, concurrency *int32, workspaceSize *string) error {
	providers, err := listProviderNames(ctx, kc)
	if err != nil {
		return err
	}
	if len(providers) == 0 {
		return errf("no providers found — run `vnclm create provider` first")
	}
	providerOpts := make([]huh.Option[string], 0, len(providers))
	for _, p := range providers {
		providerOpts = append(providerOpts, huh.NewOption(p, p))
	}
	concStr := strconv.Itoa(int(*concurrency))
	if concStr == "0" {
		concStr = "1"
	}
	wsStr := *workspaceSize
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(name).Validate(nonEmpty),
			huh.NewInput().Title("Model (e.g. azure/gpt-4o)").Value(model).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Provider").Options(providerOpts...).Value(provider),
			huh.NewInput().Title("Instructions (optional, inline AGENTS.md)").Value(instructions),
			huh.NewInput().Title("Concurrency").Value(&concStr),
			huh.NewInput().Title("Workspace size").Value(&wsStr),
			huh.NewConfirm().Title("Enabled?").Value(enabled),
		),
	)
	if err := form.Run(); err != nil {
		return err
	}
	if n, err := strconv.Atoi(concStr); err == nil {
		*concurrency = int32(n)
	}
	*workspaceSize = wsStr
	return nil
}

func listProviderNames(ctx context.Context, kc *kube.Client) ([]string, error) {
	secrets, err := kc.Core.CoreV1().Secrets(kc.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "vinculum.dev/provider=true",
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(secrets.Items))
	for _, s := range secrets.Items {
		out = append(out, s.Name)
	}
	return out, nil
}

func createTaskCmd(fromFile *string) *cobra.Command {
	var (
		name    string
		agent   string
		prompt  string
		fresh   bool
		wsMode  = "shared"
		timeout int32 = 300
	)
	c := &cobra.Command{
		Use:   "task",
		Short: "Create a Task",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			if *fromFile != "" {
				_, err := applyYAMLFile(ctx, kc, *fromFile)
				return err
			}
			if agent == "" {
				agent = state.cfg.CurrentAgent
			}
			if name == "" || prompt == "" || agent == "" {
				if err := runTaskWizard(ctx, kc, &name, &agent, &prompt, &fresh, &wsMode, &timeout); err != nil {
					return err
				}
			}
			body := map[string]any{
				"name": name,
				"spec": map[string]any{
					"agentRef":       agent,
					"prompt":         prompt,
					"fresh":          fresh,
					"workspace":      map[string]any{"mode": wsMode},
					"timeoutSeconds": timeout,
				},
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.PostJSON(ctx, "/api/tasks", body, nil); err != nil {
					return err
				}
				fmt.Printf("task %q created (agent=%s)\n", name, agent)
				return nil
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "task name")
	c.Flags().StringVar(&agent, "agent", "", "agent name (default: currentAgent)")
	c.Flags().StringVar(&prompt, "prompt", "", "task prompt")
	c.Flags().BoolVar(&fresh, "fresh", false, "ignore prior session (no --continue)")
	c.Flags().StringVar(&wsMode, "workspace", "shared", "workspace mode: shared|ephemeral")
	c.Flags().Int32Var(&timeout, "timeout", 300, "task timeout seconds")
	_ = c.RegisterFlagCompletionFunc("agent", completeResource(gvrAgents))
	_ = c.RegisterFlagCompletionFunc("workspace", staticValues("shared", "ephemeral"))
	return c
}

func runTaskWizard(ctx context.Context, kc *kube.Client, name, agent, prompt *string, fresh *bool, wsMode *string, timeout *int32) error {
	agents, err := listAgentNames(ctx, kc)
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		return errf("no agents found — create one first")
	}
	agentOpts := make([]huh.Option[string], 0, len(agents))
	for _, a := range agents {
		agentOpts = append(agentOpts, huh.NewOption(a, a))
	}
	timeoutStr := strconv.Itoa(int(*timeout))
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(name).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Agent").Options(agentOpts...).Value(agent),
			huh.NewText().Title("Prompt").Value(prompt).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Workspace").Options(
				huh.NewOption("shared (reuse /workspace)", "shared"),
				huh.NewOption("ephemeral (/workspace/task-<id>, cleaned up)", "ephemeral"),
			).Value(wsMode),
			huh.NewConfirm().Title("Fresh? (no --continue)").Value(fresh),
			huh.NewInput().Title("Timeout (seconds)").Value(&timeoutStr),
		),
	)
	if err := form.Run(); err != nil {
		return err
	}
	if n, err := strconv.Atoi(timeoutStr); err == nil {
		*timeout = int32(n)
	}
	return nil
}

func listAgentNames(ctx context.Context, kc *kube.Client) ([]string, error) {
	var out []string
	err := withOperator(ctx, kc, func(h *client.HTTP) error {
		var list []client.Agent
		if err := h.GetJSON(ctx, "/api/agents", &list); err != nil {
			return err
		}
		for _, a := range list {
			out = append(out, a.Metadata.Name)
		}
		return nil
	})
	return out, err
}

func createScheduleCmd(fromFile *string) *cobra.Command {
	var (
		name, agent, cron, tz, prompt string
		suspend                       bool
		concurrency                   = "Allow"
	)
	c := &cobra.Command{
		Use:   "schedule",
		Short: "Create an AgentSchedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}
			if *fromFile != "" {
				_, err := applyYAMLFile(ctx, kc, *fromFile)
				return err
			}
			if name == "" || agent == "" || cron == "" || prompt == "" {
				if err := runScheduleWizard(ctx, kc, &name, &agent, &cron, &tz, &prompt, &suspend, &concurrency); err != nil {
					return err
				}
			}
			body := map[string]any{
				"name": name,
				"spec": map[string]any{
					"agentRef":          agent,
					"schedule":          cron,
					"timezone":          tz,
					"suspend":           suspend,
					"concurrencyPolicy": concurrency,
					"taskTemplate":      map[string]any{"prompt": prompt},
				},
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.PostJSON(ctx, "/api/schedules", body, nil); err != nil {
					return err
				}
				fmt.Printf("schedule %q created\n", name)
				return nil
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "schedule name")
	c.Flags().StringVar(&agent, "agent", "", "agent ref")
	c.Flags().StringVar(&cron, "cron", "", "cron expression")
	c.Flags().StringVar(&tz, "timezone", "", "timezone (e.g. Europe/Berlin)")
	c.Flags().StringVar(&prompt, "prompt", "", "task template prompt")
	c.Flags().BoolVar(&suspend, "suspend", false, "start suspended")
	c.Flags().StringVar(&concurrency, "concurrency-policy", "Allow", "Allow|Forbid|Replace")
	_ = c.RegisterFlagCompletionFunc("agent", completeResource(gvrAgents))
	_ = c.RegisterFlagCompletionFunc("concurrency-policy", staticValues("Allow", "Forbid", "Replace"))
	return c
}

func runScheduleWizard(ctx context.Context, kc *kube.Client, name, agent, cron, tz, prompt *string, suspend *bool, concurrency *string) error {
	agents, err := listAgentNames(ctx, kc)
	if err != nil {
		return err
	}
	agentOpts := make([]huh.Option[string], 0, len(agents))
	for _, a := range agents {
		agentOpts = append(agentOpts, huh.NewOption(a, a))
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(name).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Agent").Options(agentOpts...).Value(agent),
			huh.NewInput().Title("Cron (e.g. '*/5 * * * *')").Value(cron).Validate(nonEmpty),
			huh.NewInput().Title("Timezone (optional)").Value(tz),
			huh.NewText().Title("Prompt").Value(prompt).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Concurrency policy").Options(
				huh.NewOption("Allow — parallel runs allowed", "Allow"),
				huh.NewOption("Forbid — skip if prior still running", "Forbid"),
				huh.NewOption("Replace — cancel prior and start new", "Replace"),
			).Value(concurrency),
			huh.NewConfirm().Title("Start suspended?").Value(suspend),
		),
	)
	return form.Run()
}
