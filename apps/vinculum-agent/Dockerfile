FROM --platform=$BUILDPLATFORM golang:1.24.3 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY apps/vinculum-agent/go.mod ./apps/vinculum-agent/go.mod
WORKDIR /src/apps/vinculum-agent
RUN GOWORK=off go mod download

WORKDIR /src
COPY apps/vinculum-agent ./apps/vinculum-agent
WORKDIR /src/apps/vinculum-agent
RUN GOWORK=off CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOHOSTARCH)} go build -o /out/vinculum-agent ./cmd/vinculum-agent

# vnclm-mcp: stdio MCP server that exposes the operator API to crush sessions
# in orchestrator-mode pods. Baked into the same image so the existing
# MCPServer/crush.json plumbing can launch it via command: vnclm-mcp.
WORKDIR /src
COPY apps/vnclm-mcp ./apps/vnclm-mcp
WORKDIR /src/apps/vnclm-mcp
RUN GOWORK=off CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOHOSTARCH)} go build -o /out/vnclm-mcp ./cmd/vnclm-mcp

FROM debian:bookworm-slim

ARG TARGETARCH
ARG CRUSH_VERSION=0.69.1
ARG GH_VERSION=2.92.0
ARG KUBECTL_VERSION=1.31.4

ENV DEBIAN_FRONTEND=noninteractive \
    WORK_DIR=/workspace \
    INSTRUCTIONS_DIR=/instructions \
    SERVER_ADDR=:8090 \
    HOME=/home/agent \
    XDG_CONFIG_HOME=/home/agent/.config \
    XDG_DATA_HOME=/home/agent/.local/share

RUN apt-get update && apt-get install -y --no-install-recommends \
      git openssh-client curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
      amd64) crush_arch=x86_64 ;; \
      arm64) crush_arch=arm64 ;; \
      *) echo "unsupported arch ${TARGETARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/charmbracelet/crush/releases/download/v${CRUSH_VERSION}/crush_${CRUSH_VERSION}_Linux_${crush_arch}.tar.gz" \
      | tar -xz -C /tmp; \
    mv "/tmp/crush_${CRUSH_VERSION}_Linux_${crush_arch}/crush" /usr/local/bin/crush; \
    rm -rf /tmp/crush_*; \
    chmod +x /usr/local/bin/crush; \
    crush --version

# gh CLI — gives coding agents a typed, batteries-included surface for
# GitHub operations (issues, PRs, reviews) instead of hand-rolling curl.
# Auth: gh reads GITHUB_TOKEN / GH_TOKEN from env. Both are populated
# automatically when an Agent has gitCredentials.tokenSecretRef set.
RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
      amd64) gh_arch=amd64 ;; \
      arm64) gh_arch=arm64 ;; \
      *) echo "unsupported arch ${TARGETARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.tar.gz" \
      | tar -xz -C /tmp; \
    mv "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /usr/local/bin/gh; \
    rm -rf /tmp/gh_*; \
    chmod +x /usr/local/bin/gh; \
    gh --version

# kubectl — for the devops drone to deploy + roll workloads on the
# target cluster after merge. Auth comes from a kubeconfig file the
# operator mounts via Agent.spec.mounts (typically at ~/.kube/config).
RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
      amd64) k_arch=amd64 ;; \
      arm64) k_arch=arm64 ;; \
      *) echo "unsupported arch ${TARGETARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/kubectl \
      "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${k_arch}/kubectl"; \
    chmod +x /usr/local/bin/kubectl; \
    kubectl version --client

RUN useradd -m -u 10001 -s /bin/bash agent

COPY --from=build /out/vinculum-agent /usr/local/bin/vinculum-agent
COPY --from=build /out/vnclm-mcp /usr/local/bin/vnclm-mcp

RUN mkdir -p /workspace /instructions \
      /home/agent/.ssh \
      /home/agent/.config/crush \
      /home/agent/.local/share/crush \
      /home/agent/.cache/crush \
    && chown -R agent:agent /workspace /instructions /home/agent

USER agent
WORKDIR /workspace

EXPOSE 8090

ENTRYPOINT ["/usr/local/bin/vinculum-agent"]
