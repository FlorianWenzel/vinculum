# Deployment

Back to index: [`README.md`](README.md)

## Public install path

The public install path is the published Helm chart:

```bash
helm repo add vinculum https://florianwenzel.github.io/vinculum
helm repo update
helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace
```

OCI chart publication also exists; see [`../../Deployment.md`](../../Deployment.md).

## What the umbrella chart installs

The `helm/vinculum` chart installs:

- PostgreSQL
- Keycloak
- Forgejo
- `vinculum-infra`
- orchestrator and CRDs
- Hive UI
- website

## Local development path

Local inner-loop development uses Tilt.

That path is separate from the public install story and should not be confused with the public chart workflow.

## Image/version model

- `helm/vinculum/Chart.yaml` provides the default app version
- first-party image tags fall back to `.Chart.AppVersion` when unset
- GitHub Actions publishes image tags including `latest`, branch tags, `sha-*`, and the chart app version

## Deployment notes to keep in mind

- browser-facing hostnames should be provided through user values files
- local environment notes should stay out of public docs
- convergence claims should be phrased carefully unless verified by CI or real installs

## Related docs

- [`architecture.md`](architecture.md)
- [`implementation-status.md`](implementation-status.md)
- [`../../Deployment.md`](../../Deployment.md)
