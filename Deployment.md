# Deployment

This repository publishes Helm charts and container images through GitHub Actions.
The deployable artifact for users is the umbrella chart at `oci://ghcr.io/florianwenzel/helm/vinculum`.

## What exists

- Charts live in `helm/`:
  - `helm/vinculum`: umbrella chart for the full stack
  - `helm/infrastructure`: PostgreSQL, Keycloak, Forgejo
  - `helm/orchestrator`: operator and CRDs
  - `helm/drone`: worker runtime chart
- Chart metadata is versioned in `helm/*/Chart.yaml`.
- First-party runtime images are published to GHCR:
  - `ghcr.io/florianwenzel/vinculum-orchestrator`
  - `ghcr.io/florianwenzel/vinculum-agent`
  - `ghcr.io/florianwenzel/vinculum-infra`
  - `ghcr.io/florianwenzel/vinculum-hive-ui`

## Release flow

There are three relevant workflows.

### 1. CI validation

File: `.github/workflows/ci.yaml`

Triggers:
- pull requests
- pushes to `main`

What it does:
- runs Go tests for `apps/orchestrator` and `apps/vinculum-infra`
- builds the Hive UI
- runs `helm dependency build` for `helm/infrastructure` and `helm/vinculum`
- lints all Helm charts
- renders the local charts with `helm template`
- packages `helm/vinculum` and renders the packaged tarball to verify the remote-install path

This workflow does not publish anything. It only proves the charts can be built, packaged, and rendered.

### 2. Chart publishing

File: `.github/workflows/publish-charts.yaml`

Triggers:
- pushes to `main` when anything under `helm/**` changes
- manual `workflow_dispatch`

What it does exactly:
1. checks out the repo
2. computes `owner_lc` and `repo_name` from the GitHub repository metadata
3. installs Helm
4. adds the Bitnami repository
5. runs `helm dependency build helm/infrastructure`
6. runs `helm dependency build helm/vinculum`
7. logs into `ghcr.io` with `helm registry login` using `GITHUB_TOKEN`
8. packages these charts into `chart-repo/`:
   - `infrastructure`
   - `orchestrator`
   - `drone`
   - `vinculum`
9. generates a classic Helm repo index with:
   - base URL `https://florianwenzel.github.io/vinculum`
10. pushes each packaged chart to the OCI registry namespace:
   - `oci://ghcr.io/florianwenzel/helm`
11. publishes `chart-repo/` to GitHub Pages via `peaceiris/actions-gh-pages`

Result:
- OCI charts are available from GHCR
- the same packaged charts are also available as a GitHub Pages Helm repository

Important behavior:
- chart publication only runs from `main`
- a Helm chart is published when chart files change, not when app code changes
- the published chart version comes from each chart's `version` field in `Chart.yaml`

### 3. Image publishing

File: `.github/workflows/publish-images.yaml`

Triggers:
- pushes to `main`
- pushes of tags matching `v*`
- manual `workflow_dispatch`

What it does exactly:
1. checks out the repo
2. computes `owner_lc`
3. reads `appVersion` from `helm/vinculum/Chart.yaml`
4. sets up QEMU and Docker Buildx
5. logs into GHCR with `GITHUB_TOKEN`
6. builds and pushes four images from the matrix

Published image tags come from `docker/metadata-action`:
- `latest` on the default branch
- the current umbrella chart `appVersion` on the default branch
- branch tags
- git tag refs
- `sha-*`
- semver tags when the git tag matches semver

Important behavior:
- image publishing is driven by Git history events, not by `helm/**` path filtering
- the `appVersion` tag for images is taken from `helm/vinculum/Chart.yaml`, so that file is the central version source for the shipped stack

## How Helm resolves image tags

The umbrella chart intentionally leaves first-party image tags empty in `helm/vinculum/values.yaml`.
Templates then fall back to `.Chart.AppVersion`.

Examples:
- `helm/vinculum/templates/deployment.yaml` uses `.Values.vinculumInfra.image.tag` and falls back to `.Chart.AppVersion`
- `helm/vinculum/templates/hive-ui-deployment.yaml` uses `.Values.hiveUI.image.tag` and falls back to `.Chart.AppVersion`

Operational meaning:
- if no override is supplied, installing chart version `0.2.0` uses app images tagged `0.2.0`
- local dev can override tags, for example with `values-dev.yaml`
- chart `version` controls the package version users install
- chart `appVersion` controls the default first-party image tag users deploy

## Published endpoints

OCI install source:

```bash
helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum --version 0.2.0 -n vinculum-system --create-namespace
```

Classic Helm repo source:

```bash
helm repo add vinculum https://florianwenzel.github.io/vinculum
helm repo update
helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace
```

The GHCR namespace used by this repo is:

- charts: `ghcr.io/florianwenzel/helm/*`
- images: `ghcr.io/florianwenzel/*`

## Exact packaging layout

When `publish-charts.yaml` runs, it produces tarballs equivalent to:

- `chart-repo/infrastructure-<version>.tgz`
- `chart-repo/orchestrator-<version>.tgz`
- `chart-repo/drone-<version>.tgz`
- `chart-repo/vinculum-<version>.tgz`
- `chart-repo/index.yaml`

Those same tarballs are:
- pushed to GHCR with `helm push`
- copied to the GitHub Pages branch for traditional Helm installs

## What an agent should change for a release

For a normal charted release:
1. update `version` and `appVersion` in `helm/vinculum/Chart.yaml`
2. update dependent chart versions if needed in:
   - `helm/infrastructure/Chart.yaml`
   - `helm/orchestrator/Chart.yaml`
   - `helm/drone/Chart.yaml`
3. if umbrella dependencies changed, keep `helm/vinculum/Chart.yaml` dependency versions aligned
4. commit to a branch, merge to `main`
5. let GitHub Actions publish images and charts

For app-only changes where the packaged chart version should not move yet:
- merging to `main` still publishes fresh images
- the existing chart will keep pointing at whatever `appVersion` it already declares unless values override image tags explicitly

## What an agent should verify before changing deployment files

- If changing chart dependencies, run:

```bash
helm dependency build helm/infrastructure
helm dependency build helm/vinculum
```

- If changing chart templates or values, run:

```bash
helm lint helm/infrastructure
helm lint helm/orchestrator
helm lint helm/drone
helm lint helm/vinculum
helm template vinculum helm/vinculum --namespace vinculum-system
helm package helm/vinculum --destination /tmp/chart-packages
helm template vinculum /tmp/chart-packages/vinculum-*.tgz --namespace vinculum-system
```

These are the same checks the CI workflow performs.

## Notes for automation

- Do not manually edit generated chart tarballs in `chart-repo/`; the workflow recreates them.
- Do not manually update GitHub Pages artifacts; `publish-charts.yaml` owns them.
- Do not assume chart publication happens for app-only changes; it is path-filtered to `helm/**`.
- Do not assume image publication happens only on tags; it also runs on every push to `main`.
- If you need a remote-installable release where chart version and default image version match, keep `helm/vinculum/Chart.yaml` `version` and `appVersion` aligned.
