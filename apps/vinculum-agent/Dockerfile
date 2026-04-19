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

FROM debian:bookworm-slim

ARG TARGETARCH
ARG CRUSH_VERSION=0.60.0

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

RUN useradd -m -u 10001 -s /bin/bash agent

COPY --from=build /out/vinculum-agent /usr/local/bin/vinculum-agent

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
