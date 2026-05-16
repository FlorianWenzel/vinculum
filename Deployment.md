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
- Tags: `latest` on default branch, `appVersion` from `helm/vinculum/Chart.yaml`, branch name, git tag, `sha-*`, and semver.

### 4. CLI publishing

File: `.github/workflows/publish-cli.yaml`

- Runs on tag pushes matching `v*`.
- Cross-builds `vnclm` for linux/darwin/windows × amd64/arm64, attaches binaries + sha256 files to the matching GitHub release.

## How Helm resolves image tags

The chart leaves first-party image tags empty in `helm/vinculum/values.yaml`. Templates fall back to `.Chart.AppVersion`.

Operational meaning:
- Installing chart version `0.5.0` uses app images tagged `0.5.0` by default.
- Chart `version` controls the package version.
- Chart `appVersion` controls the default first-party image tag.

## Published endpoints

```bash
helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum --version 0.5.0 -n vinculum-system --create-namespace
```

GHCR namespace:
- chart: `ghcr.io/florianwenzel/helm/vinculum`
- images: `ghcr.io/florianwenzel/vinculum-operator`, `ghcr.io/florianwenzel/vinculum-agent`

## Releasing

1. Bump `version` and `appVersion` in `helm/vinculum/Chart.yaml` (keep them aligned).
2. Commit, merge to `main` — images + chart publish.
3. Tag the commit `vX.Y.Z` and push the tag — CLI binaries publish to a GitHub release, images get the tag.

For app-only changes: merging to `main` publishes fresh images under the existing `appVersion` (plus `latest`, `sha-*`).

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
