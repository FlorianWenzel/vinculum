FROM --platform=$BUILDPLATFORM golang:1.24.3 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY apps/operator/go.mod ./apps/operator/go.mod
COPY apps/operator ./apps/operator

WORKDIR /src/apps/operator
RUN GOWORK=off go mod tidy
RUN GOWORK=off CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOHOSTARCH)} go build -o /out/vinculum-operator ./cmd/operator

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/vinculum-operator /usr/local/bin/vinculum-operator

ENTRYPOINT ["/usr/local/bin/vinculum-operator"]
