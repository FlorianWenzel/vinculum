#!/usr/bin/env bash

set -euo pipefail

namespace="vinculum-system"
release="vinculum"
overview_url="http://127.0.0.1:8084/api/overview"

cleanup() {
  jobs -p | xargs -r kill >/dev/null 2>&1 || true
}

trap cleanup EXIT

echo "Building chart dependencies"
helm dependency build helm/infrastructure
helm dependency build helm/vinculum

echo "Installing Vinculum integration stack"
helm upgrade --install "$release" helm/vinculum \
  -n "$namespace" \
  --create-namespace \
  -f helm/vinculum/values-ci.yaml \
  --set vinculumInfra.image.repository=vinculum-infra \
  --set vinculumInfra.image.tag=e2e \
  --set orchestrator.orchestrator.image.repository=vinculum-orchestrator \
  --set orchestrator.orchestrator.image.tag=e2e

echo "Waiting for statefulsets"
while IFS= read -r statefulset; do
  [ -n "$statefulset" ] || continue
  kubectl rollout status -n "$namespace" "$statefulset" --timeout=15m
done < <(kubectl get statefulsets -n "$namespace" -o name)

echo "Waiting for deployments"
while IFS= read -r deployment; do
  [ -n "$deployment" ] || continue
  kubectl rollout status -n "$namespace" "$deployment" --timeout=15m
done < <(kubectl get deployments -n "$namespace" -o name)

echo "Waiting for all pods to become Ready"
kubectl wait --for=condition=Ready pod --all -n "$namespace" --timeout=15m

echo "Port-forwarding orchestrator API"
kubectl port-forward -n "$namespace" svc/vinculum-orchestrator 8084:8084 >/tmp/vinculum-orchestrator-port-forward.log 2>&1 &

echo "Waiting for orchestrator overview endpoint"
for _ in $(seq 1 60); do
  if curl -fsS "$overview_url" >/tmp/vinculum-overview.json; then
    break
  fi
  sleep 5
done

curl -fsS "$overview_url" >/tmp/vinculum-overview.json

echo "Running Hive UI end-to-end smoke test"
npm --prefix apps/hive-ui ci
npm --prefix apps/hive-ui run e2e
