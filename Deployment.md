# Deployment

This repository publishes Helm charts and container images through GitHub Actions.
The deployable artifact for users is the umbrella chart at `oci://ghcr.io/florianwenzel/helm/vinculum`.

## What exists

- Charts live in `helm/`:
  - `helm/vinculum` — umbrella chart: operator only
  - `helm/operator` — operator, CRDs, RBAC
  - `helm/agent` — optional long-lived agent pod (debug/bootstrap only)
- Chart metadata is versioned in `helm/*/Chart.yaml`.
- First-party runtime images are published to GHCR:
  - `ghcr.io/florianwenzel/vinculum-operator`
  - `ghcr.io/florianwenzel/vinculum-agent`
- The `vnclm` CLI (`apps/vnclm`) ships as source + GitHub release binaries. Install with `go install ./cmd/vnclm`. Release assets are built by the `publish-cli.yaml` workflow on tag push.

## Release flow

### 1. CI validation

File: `.github/workflows/ci.yaml`

- Runs Go tests for `apps/operator` and `apps/vinculum-agent`.
- Runs `helm dependency build helm/vinculum`.
- Lints all Helm charts.
- Renders the local charts with `helm template`.
- Packages `helm/vinculum` and renders the packaged tarball.

### 2. Chart publishing

File: `.github/workflows/publish-charts.yaml`

Triggers on pushes to `main` under `helm/**`, or manual dispatch.

1. Check out the repo.
2. `helm dependency build helm/vinculum`.
3. Log into GHCR with `GITHUB_TOKEN`.
4. Package these charts into `chart-repo/`:
   - `operator`
   - `agent`
   - `vinculum`
5. Generate a classic Helm repo index.
6. Push each chart to `oci://ghcr.io/florianwenzel/helm`.
7. Publish `chart-repo/` to GitHub Pages via `peaceiris/actions-gh-pages`.

### 3. Image publishing

File: `.github/workflows/publish-images.yaml`

- Runs on pushes to `main` and tag pushes matching `v*`.
- Builds and pushes two images: operator, agent.
- Tags: `latest` on default branch, `appVersion` from `helm/vinculum/Chart.yaml`, branch name, git tag, `sha-*`, and semver.

## How Helm resolves image tags

The umbrella chart leaves first-party image tags empty in `helm/vinculum/values.yaml`. Templates fall back to `.Chart.AppVersion`.

Operational meaning:
- Installing chart version `0.4.0` uses app images tagged `0.4.0` by default.
- Local dev overrides via `values-dev.yaml`.
- Chart `version` controls the package version.
- Chart `appVersion` controls the default first-party image tag.

## Published endpoints

OCI install source:

```bash
helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum --version 0.4.0 -n vinculum-system --create-namespace
```

Classic Helm repo source:

```bash
helm repo add vinculum https://florianwenzel.github.io/vinculum
helm repo update
helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace
```

GHCR namespace:
- charts: `ghcr.io/florianwenzel/helm/*`
- images: `ghcr.io/florianwenzel/*`

## Releasing

For a normal charted release:
1. Bump `version` and `appVersion` in `helm/vinculum/Chart.yaml`.
2. Bump dependent chart versions in `helm/operator/Chart.yaml` and `helm/agent/Chart.yaml`.
3. Keep umbrella dependency versions aligned with `helm/operator/Chart.yaml`.
4. Commit, merge to `main`.
5. Let GitHub Actions publish images and charts.

For app-only changes:
- Merging to `main` publishes fresh images.
- The existing chart keeps its `appVersion` until you bump it.

## Pre-release checks

```bash
helm dependency build helm/vinculum
helm lint helm/operator
helm lint helm/agent
helm lint helm/vinculum
helm template vinculum helm/vinculum --namespace vinculum-system
helm package helm/vinculum --destination /tmp/chart-packages
helm template vinculum /tmp/chart-packages/vinculum-*.tgz --namespace vinculum-system
```

## Notes for automation

- Do not manually edit generated tarballs in `chart-repo/`; the workflow recreates them.
- Do not manually update GitHub Pages artifacts; `publish-charts.yaml` owns them.
- Chart publication is path-filtered to `helm/**`; image publication runs on every `main` push and on `v*` tags.
- Keep `helm/vinculum/Chart.yaml` `version` and `appVersion` aligned for remote-installable releases.
