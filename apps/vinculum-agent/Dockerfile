FROM golang:1.24.3 AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY apps/vinculum-agent/go.mod ./apps/vinculum-agent/go.mod
COPY apps/vinculum-agent ./apps/vinculum-agent

WORKDIR /src/apps/vinculum-agent
RUN GOWORK=off CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/vinculum-agent ./cmd/vinculum-agent
RUN GOWORK=off go install code.gitea.io/tea@v1.3.3

FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    WORK_DIR=/workspace \
    INSTRUCTIONS_DIR=/instructions \
    SERVER_ADDR=:8090 \
    OPENCODE_HOST=127.0.0.1 \
    OPENCODE_PORT=4096 \
    HOME=/home/agent

RUN apt-get update && apt-get install -y --no-install-recommends git openssh-client curl ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npm install -g opencode-ai
RUN useradd -m -u 10001 -s /bin/bash agent

COPY --from=build /out/vinculum-agent /usr/local/bin/vinculum-agent
COPY --from=build /go/bin/tea /usr/local/bin/tea

RUN mkdir -p /workspace /instructions /home/agent/.ssh /home/agent/.local/share/opencode && chown -R agent:agent /workspace /instructions /home/agent

USER agent
WORKDIR /workspace

EXPOSE 8090 4096

ENTRYPOINT ["/usr/local/bin/vinculum-agent"]
