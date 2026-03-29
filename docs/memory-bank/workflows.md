# Workflows

Back to index: [`README.md`](README.md)

## Current implemented workflow

The implemented workflow is best understood as a resource-driven operator loop, not yet a fully autonomous product pipeline.

Typical flow today:

1. create a `Drone`
2. create a `Repository`
3. create a `DroneRepositoryAccess`
4. create a `Requirement` or planned `Task`
5. assign a drone to a runnable task
6. run worker job(s)
7. create PR / verification / review artifacts
8. merge approved work when conditions pass

## Requirement flow

Requirements are repository-backed markdown files.

The operator can:

- parse them
- sync metadata into Kubernetes resources
- derive tasks from them

## Task execution flow

Tasks can:

- bind to drones
- spawn Kubernetes jobs
- create pull requests
- trigger verification jobs
- trigger review flows
- merge approved PRs

## Review flow

Review resources exist and are part of the current control-plane model.

What is present:

- explicit review resources
- review-triggered logic in the operator
- PR-oriented merge handling

What is not fully present:

- a rich iterative reviewer UX in Hive UI
- a fully generalized autonomous multi-agent feedback loop

## Workflow claims to treat carefully

These ideas exist in product direction but are not fully implemented:

- AI-assisted requirements gathering
- complete DAG-based task decomposition
- tester-managed ephemeral environments
- deployer-managed production rollout
- minimal-human-intervention path from requirement to production

See [`implementation-status.md`](implementation-status.md).
