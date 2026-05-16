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
	"github.com/florian/vinculum/apps/vnclm/internal/theme"
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
		createMCPCmd(),
		createWebhookCmd(&fromFile),
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
		name          string
		model         string
		provider      string
		instructions  string
		enabled       = true
		orchestrator  bool
		concurrency   int32
		workspaceSize = "10Gi"
		image         string
		mcpRefs       []string
		repoURL       string
		repoBranch    string
		repoPath      string
		sshKeySecret  string
		tokenSecret   string
		gitUserName   string
		gitUserEmail  string
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
				if err := runAgentWizard(ctx, kc, &name, &model, &provider, &instructions, &enabled, &concurrency, &workspaceSize, &mcpRefs); err != nil {
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
				"orchestrator":      orchestrator,
				"workspaceSize":     workspaceSize,
				"mcpServerRefs":     mcpRefs,
			}
			if repoURL != "" {
				repoBlock := map[string]any{"url": repoURL}
				if repoBranch != "" {
					repoBlock["branch"] = repoBranch
				}
				if repoPath != "" {
					repoBlock["path"] = repoPath
				}
				body["repo"] = repoBlock
			}
			if sshKeySecret != "" || tokenSecret != "" || gitUserName != "" || gitUserEmail != "" {
				gc := map[string]any{}
				if sshKeySecret != "" {
					gc["sshKeySecretRef"] = map[string]any{"name": sshKeySecret}
				}
				if tokenSecret != "" {
					gc["tokenSecretRef"] = map[string]any{"name": tokenSecret}
				}
				if gitUserName != "" {
					gc["userName"] = gitUserName
				}
				if gitUserEmail != "" {
					gc["userEmail"] = gitUserEmail
				}
				body["gitCredentials"] = gc
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
	c.Flags().BoolVar(&orchestrator, "orchestrator", false, "expose operator API to this agent (vnclm-mcp tools)")
	c.Flags().Int32Var(&concurrency, "concurrency", 1, "")
	c.Flags().StringVar(&workspaceSize, "workspace-size", "10Gi", "PVC size for /workspace")
	c.Flags().StringVar(&image, "image", "", "agent container image (default: operator's default)")
	c.Flags().StringSliceVar(&mcpRefs, "mcp", nil, "MCPServer name to attach (repeatable)")
	c.Flags().StringVar(&repoURL, "repo-url", "", "git repo to clone into the workspace on pod start")
	c.Flags().StringVar(&repoBranch, "repo-branch", "", "default branch to check out after clone")
	c.Flags().StringVar(&repoPath, "repo-path", "", `subdir under /workspace for the clone (default "repo")`)
	c.Flags().StringVar(&sshKeySecret, "ssh-key-secret", "", "Secret with key id_ed25519 (+optional known_hosts) for git over SSH")
	c.Flags().StringVar(&tokenSecret, "token-secret", "", "Secret with key 'token' for git over HTTPS / GitHub PR creation")
	c.Flags().StringVar(&gitUserName, "git-user", "", `commit author/committer name (default "vinculum-agent")`)
	c.Flags().StringVar(&gitUserEmail, "git-email", "", `commit author/committer email (default "agent@vinculum.local")`)
	_ = c.RegisterFlagCompletionFunc("provider", completeProviders())
	_ = c.RegisterFlagCompletionFunc("mcp", completeResource(gvrMCPs))
	return c
}

func runAgentWizard(ctx context.Context, kc *kube.Client, name, model, provider, instructions *string, enabled *bool, concurrency *int32, workspaceSize *string, mcpRefs *[]string) error {
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

	mcpNames, _ := listMCPNames(ctx, kc)
	mcpOpts := make([]huh.Option[string], 0, len(mcpNames))
	for _, n := range mcpNames {
		mcpOpts = append(mcpOpts, huh.NewOption(n, n))
	}

	fields := []huh.Field{
		huh.NewInput().Title("Name").Value(name).Validate(nonEmpty),
		huh.NewInput().Title("Model (e.g. azure/gpt-4o)").Value(model).Validate(nonEmpty),
		huh.NewSelect[string]().Title("Provider").Options(providerOpts...).Value(provider),
		huh.NewInput().Title("Instructions (optional, inline AGENTS.md)").Value(instructions),
		huh.NewInput().Title("Concurrency").Value(&concStr),
		huh.NewInput().Title("Workspace size").Value(&wsStr),
		huh.NewConfirm().Title("Enabled?").Value(enabled),
	}
	if len(mcpOpts) > 0 {
		fields = append(fields, huh.NewMultiSelect[string]().Title("Attach MCP servers").Options(mcpOpts...).Value(mcpRefs))
	}
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme.Huh())
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
		name       string
		agent      string
		prompt     string
		fresh      bool
		wsMode     = "shared"
		timeout    int32 = 300
		baseBranch string
		headBranch string
		commitMsg  string
		prTitle    string
		prBody     string
		skipPR     bool
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
			spec := map[string]any{
				"agentRef":       agent,
				"prompt":         prompt,
				"fresh":          fresh,
				"workspace":      map[string]any{"mode": wsMode},
				"timeoutSeconds": timeout,
			}
			if hasGitFlags := baseBranch != "" || headBranch != "" || commitMsg != "" || prTitle != "" || prBody != "" || skipPR; hasGitFlags {
				g := map[string]any{}
				if baseBranch != "" {
					g["baseBranch"] = baseBranch
				}
				if headBranch != "" {
					g["headBranch"] = headBranch
				}
				if commitMsg != "" {
					g["commitMessage"] = commitMsg
				}
				if prTitle != "" {
					g["prTitle"] = prTitle
				}
				if prBody != "" {
					g["prBody"] = prBody
				}
				if skipPR {
					g["skipPR"] = true
				}
				spec["git"] = g
			}
			body := map[string]any{"name": name, "spec": spec}
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
	c.Flags().StringVar(&baseBranch, "base-branch", "", "git base branch (requires Agent.spec.repo)")
	c.Flags().StringVar(&headBranch, "head-branch", "", `git head branch (default "vinculum/task-<name>")`)
	c.Flags().StringVar(&commitMsg, "commit", "", `commit message (default "vinculum: <task>")`)
	c.Flags().StringVar(&prTitle, "pr-title", "", "open a GitHub PR with this title after push")
	c.Flags().StringVar(&prBody, "pr-body", "", "PR body (default: crush stdoutTail)")
	c.Flags().BoolVar(&skipPR, "skip-pr", false, "commit + push but do not open a PR")
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
	).WithTheme(theme.Huh())
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
	).WithTheme(theme.Huh())
	return form.Run()
}

func createWebhookCmd(fromFile *string) *cobra.Command {
	var (
		name       string
		agent      string
		secretRef  string
		events     []string
		repoFilter string
		branchFilt string
		prompt     string
		baseBranch string
		headBranch string
		commitMsg  string
		prTitle    string
		prBody     string
		skipPR     bool
		suspend    bool
	)
	c := &cobra.Command{
		Use:   "webhook",
		Short: "Create a WebhookTrigger (GitHub-only in v1)",
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
			if name == "" || agent == "" || secretRef == "" || len(events) == 0 || prompt == "" {
				return errf("name, agent, secret-ref, events, and prompt are required (see --help)")
			}
			tpl := map[string]any{"prompt": prompt}
			if hasGit := baseBranch != "" || headBranch != "" || commitMsg != "" || prTitle != "" || prBody != "" || skipPR; hasGit {
				git := map[string]any{}
				if baseBranch != "" {
					git["baseBranch"] = baseBranch
				}
				if headBranch != "" {
					git["headBranch"] = headBranch
				}
				if commitMsg != "" {
					git["commitMessage"] = commitMsg
				}
				if prTitle != "" {
					git["prTitle"] = prTitle
				}
				if prBody != "" {
					git["prBody"] = prBody
				}
				if skipPR {
					git["skipPR"] = true
				}
				tpl["git"] = git
			}
			body := map[string]any{
				"name": name,
				"spec": map[string]any{
					"source":       "github",
					"events":       events,
					"agentRef":     agent,
					"secretRef":    map[string]any{"name": secretRef},
					"filter":       map[string]any{"repo": repoFilter, "branch": branchFilt},
					"suspend":      suspend,
					"taskTemplate": tpl,
				},
			}
			return withOperator(ctx, kc, func(h *client.HTTP) error {
				if err := h.PostJSON(ctx, "/api/webhooktriggers", body, nil); err != nil {
					return err
				}
				fmt.Printf("webhooktrigger %q created (agent=%s)\n", name, agent)
				return nil
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "trigger name")
	c.Flags().StringVar(&agent, "agent", "", "Agent that stamped Tasks target")
	c.Flags().StringVar(&secretRef, "secret-ref", "", "Secret with key 'secret' (HMAC shared secret)")
	c.Flags().StringSliceVar(&events, "events", nil, "github event types: push,pull_request,pull_request.opened,…")
	c.Flags().StringVar(&repoFilter, "repo", "", "filter to owner/repo (optional)")
	c.Flags().StringVar(&branchFilt, "branch", "", "filter to branch (optional)")
	c.Flags().StringVar(&prompt, "prompt", "", "Task prompt template; ${event.repo} ${event.sha} ${event.ref} ${event.pr.*} substituted")
	c.Flags().StringVar(&baseBranch, "base-branch", "", "git baseBranch template")
	c.Flags().StringVar(&headBranch, "head-branch", "", "git headBranch template")
	c.Flags().StringVar(&commitMsg, "commit", "", "commit message template")
	c.Flags().StringVar(&prTitle, "pr-title", "", "PR title template")
	c.Flags().StringVar(&prBody, "pr-body", "", "PR body template")
	c.Flags().BoolVar(&skipPR, "skip-pr", false, "commit + push only, no PR")
	c.Flags().BoolVar(&suspend, "suspend", false, "start trigger suspended")
	_ = c.RegisterFlagCompletionFunc("agent", completeResource(gvrAgents))
	return c
}
