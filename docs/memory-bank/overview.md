# Overview

Back to index: [`README.md`](README.md)

## What Vinculum is

Vinculum is a Kubernetes-native software delivery platform.

It combines:

- a Go orchestrator/operator
- a browser UI called Hive UI
- a containerized agent runtime built around OpenCode
- Forgejo for repositories and pull requests
- Keycloak for identity and OIDC
- Helm charts for packaging and deployment

The system is centered on durable resources rather than pure chat state. The main resources are drones, repositories, accesses, requirements, tasks, and reviews.

## What it currently does

At the current implementation level, Vinculum can:

- provision and reconcile core platform services
- provision Forgejo users for drones
- create managed repositories
- grant drones repository access
- sync repository-backed requirements
- create and run tasks via Kubernetes jobs
- create pull requests and attach review/test flows
- expose cluster state through an API and a basic UI

## What it does not fully do yet

Some high-level product ideas are only partial or still roadmap work:

- AI-assisted requirements gathering
- broad task-graph decomposition
- full tester/deployer behavior
- ephemeral preview environments
- full end-to-end autonomous release pipelines

See [`implementation-status.md`](implementation-status.md) for the detailed breakdown.

## Main runtime scope

Today the deployable stack consists of:

- PostgreSQL
- Keycloak
- Forgejo, branded as Vinculum Code
- `vinculum-infra`
- `orchestrator`
- `hive-ui`
- `website`

## Read next

- [`architecture.md`](architecture.md)
- [`components.md`](components.md)
- [`implementation-status.md`](implementation-status.md)
