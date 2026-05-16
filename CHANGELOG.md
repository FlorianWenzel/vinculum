# Changelog

All notable changes to this project are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release artifacts and one-line summaries also live on the
[GitHub Releases](https://github.com/FlorianWenzel/vinculum/releases) page.

## [Unreleased]

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
