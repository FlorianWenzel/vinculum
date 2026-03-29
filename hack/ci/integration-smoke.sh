#!/usr/bin/env bash

set -euo pipefail

namespace="vinculum-system"
release="vinculum"
overview_url="http://127.0.0.1:8084/api/overview"

wait_for_url() {
  local url="$1"
  local attempts="${2:-60}"
  local delay="${3:-5}"

  for _ in $(seq 1 "$attempts"); do
    if curl -fsS "$url" >/tmp/vinculum-overview.json; then
      return 0
    fi
    sleep "$delay"
  done

  return 1
}

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
if ! wait_for_url "$overview_url" 60 5; then
  echo "Failed to reach orchestrator overview endpoint"
  cat /tmp/vinculum-orchestrator-port-forward.log || true
  exit 1
fi

echo "Running Hive UI end-to-end smoke test"
npm --prefix apps/hive-ui ci
npm --prefix apps/hive-ui run e2e
