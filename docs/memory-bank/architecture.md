# Architecture

Back to index: [`README.md`](README.md)

## Top-level view

Vinculum has four major layers:

1. browser surfaces
2. control plane and APIs
3. worker execution
4. platform dependencies

## Browser surfaces

- `apps/site` is the public-facing site
- `apps/hive-ui` is the current browser control surface

Hive UI reads orchestrator state and exposes creation flows for key resources.

## Control plane

The main control-plane services are:

- `apps/orchestrator` - Kubernetes operator plus HTTP API
- `apps/vinculum-infra` - bootstrap and reconciliation for Keycloak and Forgejo

The orchestrator owns the main custom resources and runtime scheduling behavior.

`vinculum-infra` configures realm/client/bootstrap state in Keycloak and OIDC/auth-source state in Forgejo.

## Worker execution

The worker runtime lives in `apps/vinculum-agent`.

It is a Go control service wrapped around OpenCode. The orchestrator creates Kubernetes jobs that run this runtime with mounted configuration, auth material, and repository access.

## Platform dependencies

- PostgreSQL stores shared state for platform services
- Keycloak provides identity and OIDC
- Forgejo provides repositories, pull requests, and git access

## Packaging

Helm charts live under `helm/`:

- `helm/drone`
- `helm/infrastructure`
- `helm/orchestrator`
- `helm/vinculum`

The umbrella chart in `helm/vinculum` installs the main stack.

## Important architectural constraints

- the resource model is durable and Kubernetes-backed
- image/version defaults are chart-driven
- public deployment and local development use different paths
- some README-level product language is ahead of the current codebase

See also:

- [`components.md`](components.md)
- [`api-and-resources.md`](api-and-resources.md)
- [`deployment.md`](deployment.md)
