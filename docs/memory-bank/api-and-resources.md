# API And Resources

Back to index: [`README.md`](README.md)

## Main HTTP endpoints

The orchestrator exposes endpoints including:

- `GET /api/overview`
- `POST /api/drones`
- `POST /api/repositories`
- `POST /api/accesses`
- `POST /api/requirements`
- `POST /api/tasks`
- `PATCH /api/tasks`
- `POST /api/reviews`
- `POST /api/task-drafts`
- `POST /api/requirement-drafts`

These are implemented in `apps/orchestrator/cmd/orchestrator/main.go`.

## Main Kubernetes resources

The current resource model includes:

- `Drone`
- `Repository`
- `DroneRepositoryAccess`
- `Requirement`
- `Task`
- `Review`

These types live under `apps/orchestrator/api/v1alpha1`.

## Resource intent

### `Drone`

Represents a named worker identity and execution configuration.

Supports:

- inline or referenced instructions
- inline or referenced provider auth
- optional Forgejo auto-provisioning

### `Repository`

Represents an operator-managed repository in Forgejo.

### `DroneRepositoryAccess`

Represents explicit collaborator access from a drone to a repository.

### `Requirement`

Represents user-facing product intent, backed by markdown in a repository.

### `Task`

Represents derived technical work that can be scheduled and executed.

### `Review`

Represents explicit review state and decisions around task output.

## Related docs

- [`workflows.md`](workflows.md)
- [`implementation-status.md`](implementation-status.md)
