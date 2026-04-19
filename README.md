<h1 align="center">Vinculum</h1>

<p align="center">
  <em>Kubernetes-native runner for AI coding agents.</em>
</p>

<p align="center">
  <a href="https://github.com/FlorianWenzel/vinculum/actions/workflows/ci.yaml"><img alt="CI" src="https://github.com/FlorianWenzel/vinculum/actions/workflows/ci.yaml/badge.svg"></a>
  <a href="https://github.com/FlorianWenzel/vinculum/actions/workflows/publish-images.yaml"><img alt="Images" src="https://github.com/FlorianWenzel/vinculum/actions/workflows/publish-images.yaml/badge.svg"></a>
  <a href="https://github.com/FlorianWenzel/vinculum/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlorianWenzel/vinculum?display_name=tag&sort=semver"></a>
  <a href="https://github.com/FlorianWenzel/homebrew-vinculum"><img alt="Homebrew" src="https://img.shields.io/badge/brew-FlorianWenzel%2Fvinculum%2Fvnclm-f9a03c?logo=homebrew&logoColor=white"></a>
  <a href="LICENSE.md"><img alt="License" src="https://img.shields.io/github/license/FlorianWenzel/vinculum"></a>
</p>

<p align="center">
  <img src="demo/vnclm.gif" alt="vnclm demo" width="820">
</p>

---

> *The Vinculum is the interlink device that binds the Borg collective — a single consciousness stretched across every drone.*

Vinculum runs long-lived [`charmbracelet/crush`](https://github.com/charmbracelet/crush) agents as Kubernetes Deployments. Each **Agent** is a pod that holds an open crush session and serves a small HTTP API. Submit **Tasks** against an Agent; they execute serially in-pod, reuse the same workspace PVC, and preserve conversation history across restarts.

One operator. Many agents. One shared link — the vinculum.

## Why

- **Long-lived sessions.** A pod per Agent — no per-prompt pod cold-start, and the crush session + `/workspace` PVC survive restarts.
- **Kube-native.** Declarative `Agent`, `Task`, `AgentSchedule` CRDs. One operator reconciles them into Deployments, PVCs, RBAC, Services.
- **Multi-provider.** Azure OpenAI, Anthropic, OpenAI, opencode, or bring-your-own — a provider is just a labeled Secret.
- **One binary CLI.** `vnclm` port-forwards through your active kubecontext — no exposed operator endpoint, no long-lived local state.

## Architecture

```mermaid
flowchart LR
    user(["👤 user"])
    cli["vnclm CLI"]
    op["Operator<br/>(vinculum-operator)"]
    agent["Agent Pod<br/>(vinculum-agent + crush)"]
    pvc[("Workspace PVC")]
    sec[("Provider Secret")]

    user -- "vnclm run / list / logs" --> cli
    cli -- "port-forward :8084" --> op
    cli -- "port-forward :8090 (logs)" --> agent
    op -- "reconciles<br/>Agent / Task / Schedule CRDs" --> agent
    op -- "POST /task" --> agent
    agent -- "crush session" --> pvc
    agent -- "envFrom" --> sec
    agent -- "patch Task.status" --> op
```

## Components

| Component | Path | Purpose |
|-----------|------|---------|
| **Operator** | [`apps/operator`](apps/operator) | Reconciles `Agent`, `Task`, `AgentSchedule` CRDs into Deployments/PVCs/Secrets. Internal HTTP API on `:8084` for `vnclm`. |
| **vinculum-agent** | [`apps/vinculum-agent`](apps/vinculum-agent) | Runs inside each Agent pod. Supervises `crush server`, exposes `:8090` for task dispatch + log streaming, patches `Task.status`. |
| **vnclm** | [`apps/vnclm`](apps/vnclm) | CLI with port-forward client, interactive wizards, live log streaming, shell completion. |

## Custom Resources

- **`Agent`** — declares a long-running agent. Fields: model, provider secret ref, instructions, workspace size. Operator creates a Deployment (replicas=1, `Recreate`), Service, PVC, RBAC.
- **`Task`** — unit of work for an Agent. Fields: `prompt`, `fresh`, `workspace.mode` (`shared` | `ephemeral`), `timeoutSeconds`, `artifacts`, `env`. Tasks run serially inside the Agent pod; shared workspace by default so edits accumulate.
- **`AgentSchedule`** — cron trigger that stamps `Task`s from a template. Concurrency: `Allow` | `Forbid` | `Replace`.

## Quick start

### 1. Install the chart

```bash
helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum \
  --version 0.1.0 \
  -n vinculum-system --create-namespace
```

### 2. Install the CLI

**Homebrew** (macOS + Linux):

```bash
brew install FlorianWenzel/vinculum/vnclm
```

<details>
<summary><strong>Other install methods</strong></summary>

**Prebuilt binary** (macOS / Linux / Windows — amd64 / arm64):

```bash
VERSION=v0.1.0
OS=darwin      # linux | darwin | windows
ARCH=arm64     # amd64 | arm64
curl -L -o vnclm \
  "https://github.com/FlorianWenzel/vinculum/releases/download/${VERSION}/vnclm-${OS}-${ARCH}"
chmod +x vnclm && sudo mv vnclm /usr/local/bin/vnclm
vnclm completion zsh > ~/.zsh/completions/_vnclm   # optional
```

Checksums (`vnclm-<os>-<arch>.sha256`) are published with every release.

**From source** (Go ≥ 1.25):

```bash
cd apps/vnclm && GOWORK=off go install ./cmd/vnclm
```

</details>

### 3. Create a provider Secret

```bash
vnclm create provider                     # interactive wizard
# or
vnclm create provider --name anthropic-keys --type anthropic \
  --set ANTHROPIC_API_KEY=sk-ant-...
```

### 4. Create an Agent

```bash
vnclm create agent                        # wizard picks the provider you just created
```

### 5. Run a Task

```bash
vnclm ctx set-agent locutus
vnclm run "Compose a haiku about the Borg collective."
```

Logs stream live from the crush session; the CLI blocks until terminal phase (`Succeeded` | `Failed` | `TimedOut`). Run the same agent again later — crush picks up the session, conversation history intact.

## CLI cheatsheet

```
vnclm ctx show | set-agent <name> | clear-agent
vnclm list agents|tasks|schedules|providers       [-o table|wide|json|yaml]
vnclm get <kind> <name>                           [-o yaml|json]
vnclm delete <kind> <name>                        [--yes]
vnclm create provider|agent|task|schedule         # interactive wizard
vnclm create -f manifest.yaml                     # apply file (multi-doc OK)
vnclm logs <task>                                 [-f]              # stream crush output
vnclm run "<prompt>"  [--agent] [--fresh] [--workspace shared|ephemeral] [--timeout N]
```

`vnclm` port-forwards via your active kubeconfig context per invocation — no exposed operator, no persistent local state. Shell completion (`bash` / `zsh` / `fish`) comes pre-installed with the brew tap or via `vnclm completion <shell>`.

## Shell prompt (starship)

Show the current `vnclm` agent next to your kube context in [starship](https://starship.rs). Add to `~/.config/starship.toml`:

```toml
[custom.vnclm_agent]
command = "vnclm ctx current-agent"
when    = '[ -n "$(vnclm ctx current-agent 2>/dev/null)" ]'
format  = "via [🤖 $output]($style) "
style   = "bold purple"
shell   = ["sh", "--noprofile", "--norc"]
```

`vnclm ctx current-agent` reads only `~/.vnclm/config.yml` (no cluster calls, ~10ms) so it's safe to run per-prompt. Module auto-hides when no agent is set.

## Local development

Prerequisites: any Kubernetes cluster with your current kube-context pointing at it. Tilt deploys **into** the current context — it does not provision a cluster.

1. Drop provider credentials into `.tilt-secrets/` (gitignored) — e.g. `.tilt-secrets/azure-openai-api-key`, `.tilt-secrets/azure-openai-endpoint`.
2. `tilt up`

Tilt builds local images for operator + vinculum-agent, installs the operator chart into `vinculum-system`, port-forwards the operator API to `:8084`, and applies [`.local/e2e.yaml`](.local) (sample Agent + Task) once CRDs are ready. The [`Tiltfile`](Tiltfile) allow-lists which contexts it will deploy to — edit the list if your cluster's context name isn't in it.

## Deployment

See [`Deployment.md`](Deployment.md) for chart/image publishing details.

## Demo

The recording above is reproducible — [`demo/vnclm.tape`](demo/vnclm.tape) rendered with [charmbracelet/vhs](https://github.com/charmbracelet/vhs):

```bash
brew install vhs
vhs demo/vnclm.tape          # → demo/vnclm.gif
```

## License

MIT — see [`LICENSE.md`](LICENSE.md).
