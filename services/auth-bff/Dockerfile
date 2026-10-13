# Multi-stage Dockerfile producing two targets: server and migrate.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, no package manager,
#              ca-certs + tzdata + nonroot uid 65532 from upstream
#              gcr.io/distroless/static:nonroot)
#
# Go binaries are PID 1 directly — Go's runtime handles SIGTERM via
# signal.Notify and reaps any goroutine-spawned children, so tini is
# unnecessary. K8s securityContext overrides the image's USER, so the
# binaries are world-executable (mode 0755 from `go build`).

FROM ghcr.io/tesserix/base-go-builder:20261011@sha256:03f7dc939802b567835fce13a09edb89bb9904918dd5edd0025022f1976f4c89 AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Single runtime stage carries both binaries so the K8s init pattern
# can override ENTRYPOINT to /migrate without pulling a second image.
FROM ghcr.io/tesserix/base-distroless-static:20261011@sha256:489f033f622bfac077447cfe7e8d9dbec9678429fa717e1bb74bfe770f9e61ba AS server
COPY --from=build --chown=10001:10001 /out/server /server
COPY --from=build --chown=10001:10001 /out/migrate /migrate
EXPOSE 8080
CMD ["/server"]

# Legacy single-purpose stage retained for local docker-compose flows
# that build with --target migrate.
FROM ghcr.io/tesserix/base-distroless-static:20261011@sha256:489f033f622bfac077447cfe7e8d9dbec9678429fa717e1bb74bfe770f9e61ba AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]
