FROM golang:1.24.3 AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY apps/vinculum-infra/go.mod ./apps/vinculum-infra/go.mod
COPY apps/vinculum-infra ./apps/vinculum-infra

WORKDIR /src/apps/vinculum-infra
RUN GOWORK=off CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/vinculum-infra ./cmd/vinculum-infra

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/vinculum-infra /usr/local/bin/vinculum-infra

EXPOSE 8081

ENTRYPOINT ["/usr/local/bin/vinculum-infra"]
