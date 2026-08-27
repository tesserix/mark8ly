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

FROM ghcr.io/tesserix/base-go-builder@sha256:40fbd9b59ef20100c90213a5390332c48fa4ef7d5908223f4f088cb4ab476972 AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Single runtime stage carries both binaries so the K8s init pattern
# can override ENTRYPOINT to /migrate without pulling a second image.
FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS server
COPY --from=build --chown=10001:10001 /out/server /server
COPY --from=build --chown=10001:10001 /out/migrate /migrate
EXPOSE 8080
CMD ["/server"]

# Legacy single-purpose stage retained for local docker-compose flows
# that build with --target migrate.
FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]
