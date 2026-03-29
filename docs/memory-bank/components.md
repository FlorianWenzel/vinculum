# Components

Back to index: [`README.md`](README.md)

## Applications

### `apps/orchestrator`

Go operator and HTTP API.

Responsibilities:

- owns CRDs and reconciliation loops
- exposes `/api/*` endpoints
- schedules task work onto drones
- creates jobs, PRs, review flows, and merge actions

### `apps/vinculum-infra`

Bootstrap/reconciliation service for Keycloak and Forgejo.

Responsibilities:

- create and update Keycloak realm/client/bootstrap state
- configure Forgejo OIDC login source
- create shared org/bootstrap integration state

### `apps/vinculum-agent`

Containerized worker runtime.

Responsibilities:

- start `opencode serve`
- expose `/run` and `/exec`
- execute isolated task work in jobs

### `apps/hive-ui`

Current browser control surface.

Responsibilities today:

- read overview state
- create drones, repositories, accesses, and requirements
- surface major cluster objects at a basic level

### `apps/site`

Public website and docs entry surface.

## Helm charts

### `helm/vinculum`

Umbrella chart for the main deployable stack.

### `helm/infrastructure`

Shared platform dependencies and branding/config overlays.

### `helm/orchestrator`

Operator chart and CRD packaging.

### `helm/drone`

Worker runtime chart for drone execution.

## Supporting infrastructure

- Forgejo / Vinculum Code
- Keycloak
- PostgreSQL

## Related docs

- [`architecture.md`](architecture.md)
- [`api-and-resources.md`](api-and-resources.md)
- [`implementation-status.md`](implementation-status.md)
