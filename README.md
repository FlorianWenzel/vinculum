# Vinculum

> *The central processor that links all drones in the Collective.*

An autonomous, AI-driven software development platform. Coordinated agent swarms handle coding, review, testing, and deployment in iterative loops — from requirements to production. Built on Kubernetes, powered by [OpenCode](https://opencode.ai), inspired by the Collective.

---

## How It Works

Vinculum turns a set of requirements into working, tested, deployed software — with minimal human intervention.

### Phase 1: Requirements Gathering

A human user creates a new project through the Vinculum web interface. An AI-assisted requirements phase helps iron out functional specs, acceptance criteria, and technical constraints (target runtime, framework, deployment model). The goal is to front-load decisions so the agents rarely need to ask questions later.

### Phase 2: Task Decomposition

Once requirements are locked, Vinculum's orchestrator breaks them into a directed acyclic graph of technical tasks — starting with project scaffolding, testing infrastructure, and CI setup before moving into feature work.

### Phase 3: The Swarm

The orchestrator dispatches tasks to specialized agent **drones**, each running in its own container:

| Drone | Role |
|---|---|
| **Coder** | Implements features, writes tests, creates pull requests |
| **Reviewer** | Reviews PRs for correctness, style, coverage, and spec compliance |
| **Tester** | Deploys PR branches to ephemeral environments and runs test suites |
| **Deployer** | Manages ephemeral and production environments |

Drones work in iterative loops. A coder opens a PR. A reviewer critiques it. The coder addresses feedback. A tester deploys and validates. This cycle repeats until all agents are satisfied and quality gates pass. Then the orchestrator **assimilates** the feature into the main branch.

### Phase 4: Human-in-the-Loop (When Needed)

The orchestrator can escalate to the human user when ambiguity arises — unclear requirements, conflicting constraints, or design decisions that need human judgment. But the architecture is designed to minimize this through thorough upfront requirements.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Vinculum Cluster                   │
│                                                      │
│  ┌───────────┐   ┌──────────────┐   ┌────────────┐  │
│  │    Web     │──▶│ Orchestrator │◀──│  Webhook   │  │
│  │    App     │   │ (Vinculum)   │   │   Relay    │  │
│  └───────────┘   └──────┬───────┘   └─────▲──────┘  │
│                         │                  │         │
│            ┌────────────┼──────────┐       │         │
│            ▼            ▼          ▼       │         │
│     ┌──────────┐ ┌──────────┐ ┌────────┐  │         │
│     │  Coder   │ │ Reviewer │ │ Tester │  │         │
│     │  Drone   │ │  Drone   │ │ Drone  │  │         │
│     └────┬─────┘ └────┬─────┘ └───┬────┘  │         │
│          │             │           │       │         │
│          ▼             ▼           ▼       │         │
│     ┌──────────────────────────────────────┘         │
│     │              Forgejo                           │
│     │        (Git, PRs, Issues)                      │
│     └────────────────────────────────────────────┐   │
│                                                  │   │
│  ┌────────────┐  ┌───────────┐  ┌─────────────┐ │   │
│  │  Keycloak  │  │ SonarQube │  │    Argo     │ │   │
│  │   (Auth)   │  │   (QA)    │  │ (Workflows) │ │   │
│  └────────────┘  └───────────┘  └─────────────┘ │   │
│                                                  │   │
└──────────────────────────────────────────────────────┘
```

---

## Tech Stack

| Component | Technology           | Purpose |
|---|----------------------|---|
| Orchestrator | Go | Task graph management, drone coordination |
| Infra Bootstrap | Go | Provisions and reconciles platform dependencies such as Keycloak and Forgejo |
| Web App | Vue | User interface, project management, requirements gathering |
| Drones | OpenCode (in Docker) | AI-powered coding, reviewing, testing |
| Git Platform | Forgejo              | Repositories, pull requests, issue tracking |
| Auth | Keycloak             | SSO, user management, API auth |
| Code Quality | SonarQube            | Static analysis, coverage gates |
| Workflow Engine | Argo Workflows       | Drone lifecycle, pipeline orchestration |
| Infrastructure | Kubernetes           | Container orchestration |

---

## Repository Structure

Current repo contents:

```
vinculum/
├── apps/
│   ├── hive-ui/             # Vue admin dashboard for the hive
│   ├── vinculum-agent/      # Go control API that wraps OpenCode inside a worker container
│   ├── vinculum-infra/      # Go bootstrap/reconciliation service for Keycloak + Vinculum Code
│   └── orchestrator/        # Go Kubernetes operator for Drone and TaskRun CRDs
├── helm/
│   ├── drone/               # Parameterized chart for spawning OpenCode worker pods
│   ├── infrastructure/      # Shared PostgreSQL, Keycloak, and Forgejo/Vinculum Code
│   ├── orchestrator/        # Operator chart plus Drone/TaskRun CRDs
│   └── vinculum/            # Umbrella chart for the full deployable stack
├── Tiltfile                 # Local inner-loop dev workflow
├── go.work                  # Go workspace
└── README.md
```

Planned repo expansion still includes orchestrator, web UI, drones, shared packages, docs, and Argo templates, but those are not scaffolded yet.

---

## Terminology

Staying true to our roots.

| Term | Meaning |
|---|---|
| **Vinculum** | The orchestrator — central processor linking all drones |
| **Drone** | A containerized agent with a specific role |
| **Alcove** | A drone's idle state, waiting for a designation |
| **Designation** | A task assignment given to a drone |
| **Assimilate** | Merging a completed feature into the main branch |
| **Collective** | The full running Vinculum cluster |
| **Hive Mind** | The shared task queue and state |

---

## Getting Started

> 🚧 **Under active development.** This project is in early stages.

### Prerequisites

- A Kubernetes cluster (local Docker Desktop works well)
- Helm 3
- Tilt (recommended for local development)

### Quick Start

```bash
# Start the local stack
tilt up
```

This brings up:

- shared PostgreSQL with separate `keycloak` and `forgejo` schemas in one `vinculum` database
- Keycloak with a custom Borg-inspired `vinculum` realm theme
- `Vinculum Code` (Forgejo) with custom branding and OIDC login via Keycloak
- the `vinculum-infra` Go service that reconciles realms, clients, users, groups, orgs, and Forgejo auth sources

Useful local endpoints after `tilt up`:

- `http://localhost:8080` - Keycloak
- `http://localhost:3000` - Vinculum Code
- `http://localhost:4173` - Hive admin dashboard
- `http://localhost:8084/api/overview` - Orchestrator overview API
- `http://localhost:10350` - Tilt UI

The current sample end-to-end task uses the free Zen model `opencode/minimax-m2.5-free`.

### Helm Install

Once GitHub Actions has published the charts and images, you can install the full stack directly from the remote chart on GitHub Container Registry:

```bash
helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum \
  --version 0.2.0 \
  -n vinculum-system \
  --create-namespace
```

If you prefer a classic Helm repository, the same packaged charts are also published to GitHub Pages:

```bash
helm repo add vinculum https://florianwenzel.github.io/vinculum
helm repo update
helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace
```

That single chart installs:

- PostgreSQL, Keycloak, and Forgejo
- the `vinculum-infra` bootstrap service
- the orchestrator operator and CRDs
- the Hive UI

The default chart values are cluster-internal and work well with `kubectl port-forward`. For browser-facing ingress or custom domains, override the Forgejo/Keycloak public URLs and enable `hiveUI.ingress`.

Local development now targets the `zora` cluster through `Tilt`, with the full stack consolidated into the `vinculum-system` namespace: infrastructure, `vinculum-infra`, the operator, and the Hive UI. Demo resources are created step-by-step through the API/UI after the platform is healthy.

The Hive admin dashboard lives in `apps/hive-ui` and reads the operator's `/api/overview` endpoint to show drones, repositories, requirements, tasks, reviews, access grants, and jobs from the cluster.

---

## Development

```bash
# Start infra + app with live rebuilds
tilt up

# Run the infra bootstrap service
go run ./apps/vinculum-infra/cmd/vinculum-infra

# Build the infra bootstrap service
go build ./apps/vinculum-infra/cmd/vinculum-infra

# Build the drone runtime service
go build ./apps/vinculum-agent/cmd/vinculum-agent

# Build the orchestrator operator
go build ./apps/orchestrator/cmd/orchestrator

# Run backend tests
go test ./apps/vinculum-infra/...
go test ./apps/vinculum-agent/...
go test ./apps/orchestrator/...

# Render the app chart with local-dev values
helm template vinculum ./helm/vinculum -n vinculum -f helm/vinculum/values-dev.yaml

# Render a drone worker chart
helm template drone ./helm/drone -n vinculum-drones

# Render the orchestrator operator chart
helm template orchestrator ./helm/orchestrator -n vinculum-system
```

Current backend direction is Go for services and Vue for the future frontend.

The first concrete backend slice is `apps/vinculum-infra`, a small reconciliation service that talks to Keycloak and Forgejo over their APIs to establish base platform state.

The next backend slice is a single-container OpenCode drone runtime: one container will bundle the OpenCode CLI, a small Go control API, Forgejo CLI, mounted SSH credentials, and mounted markdown instructions so the orchestrator can spawn many isolated worker pods from the same image.

That runtime now exists in `apps/vinculum-agent` and is packaged by `helm/drone`. The container starts a local `opencode serve` process, exposes a Go HTTP API for `/run` and `/exec`, and is designed to receive mounted SSH keys plus instruction markdown through Kubernetes volumes.

The control plane direction is now a Go Kubernetes operator in `apps/orchestrator`. It introduces durable `Drone` resources for named worker identities, `Repository` resources for managed repository provisioning, `DroneRepositoryAccess` resources for granting agent access to repositories, `Requirement` resources for user-facing feature requests, `Task` resources for derived technical work, and `Review` resources for explicit review decisions. The operator binds runnable `Task`s to available `Drone`s, spawns worker `Job`s from the drone config, and tracks active work on each drone.

`Drone` now supports both inline and referenced configuration for instructions and provider auth: inline content is convenient for local testing and UI-driven workflows, while `ConfigMap`/`Secret` references remain the better fit for production-style setups.

When `Drone.spec.forgejo.autoProvision=true`, the operator provisions the Forgejo user, generates an SSH keypair, uploads the public key to Forgejo, stores the private key in a Kubernetes `Secret`, and exposes only references plus metadata in `Drone.status`.

`Repository` lets the operator create repositories in Vinculum Code. `DroneRepositoryAccess` grants drones collaborator access with explicit permissions.

The Hive UI and orchestrator API both support the same step-by-step workflow:

1. create a `Drone`
2. create a `Repository`
3. create a `DroneRepositoryAccess`
4. create a `Requirement` or a planned `Task`
5. assign a `Drone` to a planned `Task`
6. create a `Review` once the task is in review

The corresponding HTTP endpoints are:

- `POST /api/drones`
- `POST /api/repositories`
- `POST /api/accesses`
- `POST /api/requirements`
- `POST /api/reviews`
- `POST /api/tasks`
- `PATCH /api/tasks`
- `POST /api/task-drafts`
- `POST /api/requirement-drafts`

For Kubernetes deployment, `helm/vinculum` is now the umbrella chart for the full stack, while `helm/infrastructure`, `helm/orchestrator`, and `helm/drone` remain available as lower-level building blocks.

For local inner-loop development, `Tilt` builds `apps/vinculum-infra`, deploys both Helm charts, and forwards Vinculum Code plus Keycloak for browser access.

`apps/vinculum-infra` now also reconciles the Forgejo OIDC login source. It provisions the Keycloak realm and client over HTTP APIs, then updates Vinculum Code's auth source by execing `forgejo admin auth` in the Forgejo pod through Kubernetes RBAC.

The local auth flow is now external-first: Vinculum Code is configured for external registration through OIDC, internal web sign-in is disabled, and `vinculum-infra` seeds a bootstrap Keycloak user (`picard` by default) in the Forgejo admin group.

When `AUTO_BOOTSTRAP=true`, the service retries startup reconciliation with exponential backoff so it can wait for Keycloak and Forgejo to become reachable after fresh cluster deploys.

Vinculum Code's auth source is configured with the Keycloak OpenID Connect discovery document URL, not just the realm issuer, so the login flow can resolve the correct authorization endpoint.

OIDC config is split between internal service URLs and public browser URLs:

- `KEYCLOAK_BASE_URL` / `FORGEJO_BASE_URL` are cluster-internal addresses used by `vinculum-infra`
- `KEYCLOAK_ISSUER_URL` / `FORGEJO_PUBLIC_URL` are browser-facing addresses used in OIDC configuration
- `vinculum-infra` keeps the configured URLs and also adds localhost Forgejo dev URLs automatically so local browser auth can work with port-forwards

For local Docker Desktop development, `helm/vinculum/values-dev.yaml` keeps the Keycloak issuer cluster-internal so Forgejo login-source reconciliation works without ingress, while the Keycloak client still gets localhost Forgejo redirect URLs for browser-driven dev flows.

`helm/infrastructure/values-dev.yaml` makes Keycloak advertise `http://localhost:8080` as its frontend hostname in dev, while still allowing internal backchannel communication from pods. In production, users can override this with their real public domain in their own values file.

The local identity and code surfaces are branded in-cluster: Keycloak uses a Borg-inspired `vinculum` login theme, and the code forge is rebranded as `Vinculum Code` with a custom green theme, cube logo asset overrides, and a stripped-down footer.

In local auth UX defaults, `Vinculum Code` requires sign-in before showing the site, uses the external `Vinculum` OIDC button, and disables the legacy manual OpenID URI flow.

Image versioning is chart-driven:

- `helm/vinculum/Chart.yaml` provides the default app version
- first-party chart values leave the image tag empty so Helm falls back to `.Chart.AppVersion`
- `helm/vinculum/values-dev.yaml` pins the local dev image tag to `dev` for Tilt
- GitHub Actions also publishes `latest`, branch, `sha-*`, and the current chart app version tags to GHCR

---

## Roadmap

- [x] Single-container OpenCode drone runtime with Go control API
- [x] Initial orchestrator operator with Drone and TaskRun CRDs
- [ ] Core orchestrator task graph engine
- [ ] Coder drone with OpenCode integration
- [ ] Reviewer drone with PR feedback loop
- [ ] Forgejo webhook integration
- [ ] Ephemeral test environment provisioning
- [ ] Tester drone with automated QA
- [ ] SonarQube quality gate integration
- [ ] Web UI for project creation and requirements gathering
- [ ] AI-assisted requirements decomposition
- [ ] Argo workflow templates for full feature pipelines
- [ ] Custom Kubernetes operator (`CodingAgent` CRD)

---

## Contributing

Contributions welcome. See [`docs/contributing.md`](docs/contributing.md) for guidelines.

---

## License

TBD

---

*Resistance is futile. Your code will be assimilated.*
