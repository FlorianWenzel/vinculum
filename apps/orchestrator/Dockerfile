FROM golang:1.24.3 AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY apps/orchestrator/go.mod ./apps/orchestrator/go.mod
COPY apps/orchestrator ./apps/orchestrator

WORKDIR /src/apps/orchestrator
RUN GOWORK=off go mod tidy
RUN GOWORK=off CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/vinculum-orchestrator ./cmd/orchestrator

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/vinculum-orchestrator /usr/local/bin/vinculum-orchestrator

ENTRYPOINT ["/usr/local/bin/vinculum-orchestrator"]
