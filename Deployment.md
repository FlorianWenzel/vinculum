# Deployment

This repository publishes a Helm chart and container images through GitHub Actions.
The deployable artifact for users is the chart at `oci://ghcr.io/florianwenzel/helm/vinculum`.

## What exists

- Single chart at `helm/vinculum` — operator, CRDs, RBAC.
- Chart metadata is versioned in `helm/vinculum/Chart.yaml`.
- First-party runtime images are published to GHCR:
  - `ghcr.io/florianwenzel/vinculum-operator`
  - `ghcr.io/florianwenzel/vinculum-agent`
- The `vnclm` CLI (`apps/vnclm`) ships as source + GitHub release binaries. Release assets are built by `publish-cli.yaml` on tag push.

## Release flow

### 1. CI validation

File: `.github/workflows/ci.yaml`

- Runs Go tests for `apps/operator` and `apps/vinculum-agent`.
- Lints + renders `helm/vinculum`.
- Packages `helm/vinculum` and renders the packaged tarball.

### 2. Chart publishing

File: `.github/workflows/publish-charts.yaml`

Triggers on pushes to `main` under `helm/**`, or manual dispatch.

1. Check out the repo.
2. Log into GHCR with `GITHUB_TOKEN`.
3. Package `helm/vinculum` into `chart-repo/`.
4. Push chart to `oci://ghcr.io/florianwenzel/helm`.

### 3. Image publishing

File: `.github/workflows/publish-images.yaml`

- Runs on pushes to `main` and tag pushes matching `v*`.
- Builds and pushes two images: operator, agent.
- Tags: `latest` on default branch, branch name, git tag, `sha-*`, and semver. The `:<appVersion>` (e.g. `:0.5.3`) tag is produced **only by `v*` tag pushes** — main pushes don't claim it, to prevent two builds racing the same tag.

### 4. CLI publishing

File: `.github/workflows/publish-cli.yaml`

- Runs on tag pushes matching `v*`.
- Cross-builds `vnclm` for linux/darwin/windows × amd64/arm64, attaches binaries + sha256 files to the matching GitHub release.

## How Helm resolves image tags

The chart leaves first-party image tags empty in `helm/vinculum/values.yaml`. Templates fall back to `.Chart.AppVersion`.

Operational meaning:
- Installing chart version `0.5.3` uses app images tagged `0.5.3` by default.
- Chart `version` controls the package version.
- Chart `appVersion` controls the default first-party image tag.
- Image tags `:<semver>` are produced **only by `v*` tag pushes**. Main pushes
  build `:latest` + `:sha-<short>` + `:main`, but not the appVersion tag.
  Prevents the cache hazard where two builds (main + tag) clobber the same
  `:X.Y.Z` and kubelet keeps the first one it cached.

## Upgrading an existing install

`helm upgrade --install` is idempotent on the chart, but agent pods with
`spec.image` pinned to a fixed tag (e.g. `:tilt-dev` for local dev, or a
hardcoded `:0.5.3`) won't auto-track when the chart's appVersion bumps:

1. **Recommended** — leave `Agent.spec.image` empty in your manifests so
   the operator falls back to its `AGENT_DEFAULT_IMAGE` env (the chart's
   `defaultAgentImage`). Then `helm upgrade` plus an operator restart
   propagates to every agent's next reconcile.
2. **Pinned-image** — `kubectl patch agent <name> --type=merge \
   -p '{"spec":{"image":"ghcr.io/florianwenzel/vinculum-agent:0.5.3"}}'`
   then `kubectl rollout restart deploy/agent-<name>`.

`imagePullPolicy: IfNotPresent` (the chart default) means nodes that
cached a tag don't re-pull. If you suspect a stale-tag situation,
pin by digest:

```bash
kubectl set image deploy/vinculum-operator \
  operator=ghcr.io/florianwenzel/vinculum-operator@sha256:<digest>
```

## Published endpoints

```bash
helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum --version 0.5.3 -n vinculum-system --create-namespace
```

GHCR namespace:
- chart: `ghcr.io/florianwenzel/helm/vinculum`
- images: `ghcr.io/florianwenzel/vinculum-operator`, `ghcr.io/florianwenzel/vinculum-agent`

## Releasing

1. Bump `version` and `appVersion` in `helm/vinculum/Chart.yaml` (keep them aligned).
2. Commit, merge to `main` — images + chart publish.
3. Tag the commit `vX.Y.Z` and push the tag — CLI binaries publish to a GitHub release, images get the tag.

For app-only changes: merging to `main` publishes fresh images at `:latest` + `:sha-<short>` + `:main`. The immutable `:<appVersion>` tag is not republished until you push a new `v*` tag.

## Pre-release checks

```bash
helm lint helm/vinculum
helm template vinculum helm/vinculum --namespace vinculum-system
helm package helm/vinculum --destination /tmp/chart-packages
helm template vinculum /tmp/chart-packages/vinculum-*.tgz --namespace vinculum-system
```

## Notes for automation

- Do not manually edit generated tarballs in `chart-repo/`; the workflow recreates them.
- Chart publication is path-filtered to `helm/**`; image publication runs on every `main` push and on `v*` tags; CLI publication runs only on `v*` tags.
