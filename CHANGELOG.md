# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release artifacts and one-line summaries also live on the
[GitHub Releases](https://github.com/FlorianWenzel/vinculum/releases) page.

## [Unreleased]

## [0.6.3] — 2026-05-23

### Added
- **OpenCode Go provider support.** `Agent.spec.model` now accepts
  `opencode-go/<model-id>` to route through opencode.ai's flatrate
  Go plan ($12/5h, $30/week, $60/month), with access to DeepSeek V4
  Pro/Flash, Qwen3.5/3.6 Plus, Kimi K2.5/K2.6, GLM-5/5.1,
  MiMo-V2.5/Pro, and MiniMax M2.5/M2.7. The operator's
  `crushProviderConfig` renders an `openai-compat` provider at
  `https://opencode.ai/zen/go/v1` keyed off `$OPENCODE_API_KEY`.
  Solves the per-token credit-exhaustion failure mode we hit twice
  on per-token providers — flatrate caps requests, never overages.

## [0.6.2] — 2026-05-18

### Added
- **`Agent.spec.mounts` — generic file/dir mounts from arbitrary
  Secrets and ConfigMaps.** Each entry produces a Volume + VolumeMount
  on the main agent container, sourced from a Secret or ConfigMap
  (exactly one). When the source's `key` is set, only that single key
  is projected and `mountPath` is treated as a file (volume uses
  `subPath`); when omitted, every key in the source is materialized
  under `mountPath` as one file per key. Read-only by default. Use
  this for kubeconfig (`~/.kube/config`), extra CA bundles, license
  files, or anything else the agent's shell touches that crush
  itself doesn't read.

  ```yaml
  spec:
    mounts:
      - name: kubeconfig
        mountPath: /home/agent/.kube/config
        secret:
          name: unimatrix-deploy-kubeconfig
          key: config
  ```

- **`kubectl` baked into the vinculum-agent image** (pinned `v1.31.4`,
  both amd64 + arm64). Pairs with `Agent.spec.mounts` for the kubeconfig:
  the devops drone can now run `kubectl set image / rollout status /
  rollout undo` against a target cluster without any extra
  installation step.

## [0.6.1] — 2026-05-18

### Fixed
- **Crush's silent Sonnet-4 fallback.** The agent image was pinned to
  `crush v0.69.1`, but the operator's `renderCrushConfig` still emitted
  the legacy v0.60 schema (top-level `model: "<provider>/<id>"`). v0.69
  ignored that field and silently routed every request to its embedded
  default provider — Anthropic Sonnet 4 — burning openrouter credit on
  a model nobody asked for. The renderer now emits the proper v0.69
  schema: explicit `providers: { <id>: ProviderConfig }`, `models: {
  large, small }` slot config, and `options.disable_default_providers:
  true` so crush refuses to fall back if any future schema drift
  recurs. Together these form the only path crush has to an LLM —
  no path = no surprise spend. Supported provider ids today:
  `openrouter`, `anthropic`, `openai`, `azure` (extensible).

### Added
- **Recurring orchestrators (PM-bot pattern) section in the README.**
  Documents the composition of `AgentSchedule` + an `orchestrator:
  true` Agent that wakes up on a cron and drives the rest of the
  team — checking in on peers, dispatching follow-up Tasks, and
  carrying state across ticks via the persistent crush session.

## [0.6.0] — 2026-05-17

### Added
- **Peer-to-peer messaging.** New `Message` CRD makes drone-to-drone
  chatter a first-class resource. Each Agent now has `spec.peer: true`
  by default, gaining three new MCP tools — `send_message`,
  `list_peers`, `get_message` — alongside the existing orchestrator
  surface. Messages are async: `send_message` returns immediately, and
  replies arrive as fresh inbound Messages with `inReplyTo`, firing a
  new crush turn on the original sender (no `await_reply`). Threads
  are browsable via `kubectl get messages` and `status.replyMessages`.
- **Orchestrator vs. peer split.** `vnclm-mcp` now registers tools by
  capability — `VINCULUM_PEER` (default-on) exposes the chat surface,
  `VINCULUM_ORCHESTRATOR` exposes `dispatch_task` and friends. A drone
  can be neither, peer-only, orchestrator-only, or both. Existing
  orchestrator agents keep working unchanged.
- **`vinculum_messages_total{from,to,phase}` metric** on the operator
  next to the existing task/dispatch counters.

### Changed
- `Agent.spec.peer` defaults to true: every Agent gets the bundled
  vinculum MCP wired into its crush session out of the box. Set
  `peer: false` to opt out.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.6.0)

## [0.5.3] — 2026-05-17

### Fixed
- **Multi-arch image builds.** `publish-images.yaml` was only building
  `linux/amd64`, which crashed under Rosetta/qemu emulation on
  arm64 nodes (M-series Mac via Rancher Desktop, Graviton, …) with a
  Go runtime SIGSEGV in `runtime.netpoll`. Workflow now builds
  `linux/amd64,linux/arm64` so the published `:0.5.3` image runs
  natively on both. Existing arm64 users on `:0.5.1` / `:0.5.2`
  should `helm upgrade --version 0.5.3` and roll their agent pods.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.5.3)

## [0.5.2] — 2026-05-17

### Added
- **`gh` CLI baked into the agent image.** Pinned to `cli/cli v2.92.0`,
  installed for both amd64 and arm64 in
  `apps/vinculum-agent/Dockerfile`. Authentication is automatic when
  the Agent has `gitCredentials.tokenSecretRef` — `gh` reads
  `GITHUB_TOKEN` / `GH_TOKEN` from env with no extra config.
  Prompts should prefer `gh issue create` / `gh pr create` /
  `gh pr review` over hand-rolled `curl`; the live 3-drone e2e
  surfaced that LLMs are more reliable at calling typed CLI flags
  than at constructing signed REST requests by hand.

### Docs
- New README sub-section "LLM-driven GitHub ops via `gh`" under
  Coding agents.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.5.2)

## [0.5.1] — 2026-05-16

### Fixed
- **CI: no more clobbered image tags.** `:<appVersion>` (e.g. `:0.5.0`)
  was previously produced by both main pushes AND `v*` tag pushes,
  yielding two different image bytes at the same tag. With kubelet's
  `IfNotPresent` pull policy, nodes kept the first one cached and a
  fresh `helm upgrade` quietly kept running the older binary. The
  appVersion tag is now produced **only by tag pushes**.
  Main pushes still produce `:latest`, `:sha-<short>`, and `:main`.

### Docs
- New "Upgrading an existing install" section in `Deployment.md`
  covering pinned-image agents (`spec.image: …:tilt-dev` etc.) and the
  digest-pinning escape hatch when you suspect a stale tag.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.5.1)

## [0.5.0] — 2026-05-16

### Added
- **`WebhookTrigger` CRD.** Inbound GitHub webhook deliveries (push,
  pull_request, etc.) can now stamp Tasks declaratively. Per-trigger
  HMAC-SHA256 verification against a referenced Secret; repo/branch
  filtering; `${event.repo}` / `${event.sha}` / `${event.pr.*}`
  template-var substitution in `prompt`, `headBranch`, `prTitle`,
  `prBody`, `commitMessage`.
- **Operator `POST /webhook/github` endpoint** on the existing `:8084`
  service. ClusterIP — wire your own Ingress/LB/tunnel for public
  exposure. The handler is stdlib-only; one delivery can fan out to
  multiple matching triggers.
- **`TaskTemplate` gains `model` + `git`** so AgentSchedule / WebhookTrigger
  templates can drive the same coding workflow Tasks can today.
- **`vnclm create webhook`** CLI subcommand with flags for events,
  filter, secret-ref, and the git workflow fields.
- 9 unit tests for the webhook handler: signature verify, event/filter
  matching, full happy-path with task creation + status update, bad
  signature rejection, no-match no-op, suspended trigger, template
  substitution.

### Changed
- Operator RBAC widened to manage `webhooktriggers` and `webhooktriggers/status`.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.5.0)

## [0.4.2] — 2026-05-16

### Fixed
- **Idempotent re-pushes work end-to-end.** v0.4.1 added the
  PR-already-exists handler but the `git push` ahead of it would fail
  non-fast-forward on the second run with the same head branch (because
  the agent re-checks-out from base each Task). v0.4.2 pushes with
  `--force-with-lease` so the prior commit on the same head branch is
  safely overwritten. New test
  `TestPush_ForceWithLease_OverwritesDivergedRemote` codifies it.

## [0.4.1] — 2026-05-16

### Added
- **Init container error surfacing.** When the `git-clone` init container
  fails (bad URL, auth refused, …), the operator now writes a structured
  `reason` and `message` onto the Agent's status, and propagates them
  onto any Pending Task targeting that Agent. `kubectl get agent` /
  `kubectl get task` now show the real failure instead of an opaque
  "Pending". The init container's `terminationMessagePolicy` is set to
  `FallbackToLogsOnError` so stderr surfaces for free.
- **Idempotent PR creation.** GitHub returning `422 "A pull request
  already exists for ..."` is no longer a hard failure — the agent looks
  up the existing open PR for the same head/base and returns its URL.
  Re-running a Task with the same head branch is now safe.
- **Integration tests for the git workflow.** `gitPreCrush` /
  `gitPostCrush` now have direct unit coverage against a local bare repo:
  branch checkout, default head-branch from task name, no-base-branch
  validation, NoChanges short-circuit, dirty-tree commit + push, and a
  regression test that `AGENTS.md` stays out of commits.

### Changed
- **`--continue` + per-Task model override guard.** When `Task.spec.model`
  is set and `Fresh=false`, the agent now forces a fresh crush run rather
  than feeding the override model the prior model's session state.

### Docs
- New `CHANGELOG.md`.

## [0.4.0] — 2026-05-16

### Added
- **Per-Task model override** via `Task.spec.model` (and `vnclm run --model`).
- **Crush tools / permissions** via `Agent.spec.allowedTools` and
  `Agent.spec.disabledTools`, rendered into `crush.json` under
  `permissions.allowed_tools` and `tools.<name>` blocks. Default
  `allowed_tools: ["*"]` so non-interactive `crush run` doesn't block.
- **Custom Prometheus metrics** on the operator's existing `:8082/metrics`:
  `vinculum_tasks_total{agent,phase}`,
  `vinculum_task_duration_seconds{agent,phase}`,
  `vinculum_agent_ready{agent}`,
  `vinculum_orchestrator_dispatches_total{from,to}`.
- **Opt-in NetworkPolicy** via `helm install --set networkPolicy.enabled=true`.
  Allows DNS + in-namespace + public 443/22; everything else blocked.
  Extra hosts/namespaces via `networkPolicy.extraEgressCIDRs` /
  `extraEgressNamespaces`.
- **PVC artifact sink.** `Task.spec.artifacts.type=pvc` with a `subPath`
  now `cp -a`s the source dir into `<WORKSPACE_ROOT>/<subPath>` and
  surfaces `pvc://<claim>/<path>` in `Task.status.artifactURLs`.

### Changed
- **Hardened pod SecurityContext** on agent + git-clone containers:
  `RunAsNonRoot=true`, `RunAsUser=10001`, `AllowPrivilegeEscalation=false`,
  capabilities `Drop: [ALL]`, `seccompProfile=RuntimeDefault`.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.4.0)

## [0.3.0] — 2026-05-16

### Added
- **Coding agents.** Declarative `Agent.spec.repo` (URL, branch, path);
  operator adds a `git-clone` init container that hydrates
  `/workspace/<path>` on pod start. Subsequent restarts hit the cache and
  just `git fetch --prune`.
- **Git credentials.** `Agent.spec.gitCredentials.sshKeySecretRef` and
  `tokenSecretRef`; mounted into both init and main containers. HTTPS
  auth via a stdlib-only credential helper that reads `GIT_TOKEN` from
  env.
- **Task git workflow.** `Task.spec.git` with `baseBranch`, `headBranch`,
  `commitMessage`, `prTitle`, `prBody`, `skipPR`. The agent wraps each
  crush run: fetch → checkout → crush → status → commit → push → PR via
  the GitHub REST API. `NoChanges` short-circuits cleanly.
- **`vnclm` CLI flags.** `create agent --repo-url --repo-branch
  --repo-path --ssh-key-secret --token-secret --git-user --git-email
  --orchestrator`. `run` / `create task --base-branch --head-branch
  --commit --pr-title --pr-body --skip-pr`.
- New `apps/vinculum-agent/internal/git` package: thin `exec.Cmd`
  wrapper plus a stdlib `net/http` GitHub PR client. 9 unit tests.

### Fixed
- `crush run --yolo` removed — flag was dropped in crush 0.69+;
  non-interactive `run` auto-accepts anyway.
- Crush config ConfigMap is now mounted at `$XDG_CONFIG_HOME/crush/` so
  crush actually loads `crush.json` (and therefore MCP servers).
- Pinned `CRUSH_VERSION` bumped 0.60.0 → 0.69.1.
- `AGENTS.md` (instruction file symlink) is added to `.git/info/exclude`
  in the pre-crush step so it never lands in a commit.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.3.0)

## [0.2.0] — 2026-05-15

### Added
- **Orchestrator agents.** `Agent.spec.orchestrator: true` injects
  `VINCULUM_OPERATOR_URL` into the pod; a bundled `vnclm-mcp` stdio MCP
  server exposes six operator-backed tools to the running LLM:
  `list_agents`, `dispatch_task`, `get_task`, `wait_task`,
  `get_task_logs`, `cancel_task`. Self-dispatch is refused.
- New `apps/vnclm-mcp` Go module: stdlib JSON-RPC 2.0 over stdio,
  plus a stdlib HTTP client for the operator's API.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.2.0)

## [0.1.0] — 2026-04-19

### Added
- Initial public release: operator, agent, CLI, helm chart, and Homebrew
  formula.
- CRDs: `Agent`, `Task`, `AgentSchedule`, `MCPServer`.
- Long-lived crush agents running as Kubernetes Deployments, with
  persistent workspace PVCs and serial in-pod Task execution.
- MCP server support (stdio + http), attachable to any Agent via
  `mcpServerRefs`.

[Release notes](https://github.com/FlorianWenzel/vinculum/releases/tag/v0.1.0)
