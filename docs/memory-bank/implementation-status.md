# Implementation Status

Back to index: [`README.md`](README.md)

Yes, this belongs in the memory bank.

For LLM-oriented docs, an explicit implementation-status file is useful because it separates:

- shipped behavior
- partial behavior
- roadmap intent

without forcing every other document to repeat those caveats.

## Implemented

- umbrella Helm deployment for the main platform stack
- orchestrator API and CRDs for drones, repositories, accesses, requirements, tasks, and reviews
- drone runtime container with OpenCode wrapper and `/run` plus `/exec`
- Forgejo user provisioning for drones
- repository creation and drone access grants
- requirement parsing and requirement-backed task creation
- task job execution, PR creation, verification jobs, review hooks, and merge handling
- Keycloak and Forgejo bootstrap/reconciliation in `vinculum-infra`
- published images and charts through GitHub Actions
- a real but still limited Hive UI

## Partial

- coder / reviewer / tester workflow as a full autonomous loop
- Hive UI parity with the API surface
- task decomposition beyond simple derived/dependent flows
- verification and test automation as a complete tester subsystem
- remote install convergence claims beyond what charts and CI strongly imply

## Missing Or Not Yet Implemented

- AI-assisted requirements gathering
- broad DAG-based task graph engine
- ephemeral preview environments
- deployer-managed production release flow
- human escalation workflow for ambiguity
- full end-to-end path from requirement to deployed production software with minimal human intervention

## Guidance For Future Docs

- say "implemented" only when there is code, config, or workflow evidence
- say "partial" when a slice exists but the README/product story is broader than the implementation
- say "planned" or "roadmap" for aspirational features

## Related docs

- [`overview.md`](overview.md)
- [`workflows.md`](workflows.md)
- [`roadmap.md`](roadmap.md)
