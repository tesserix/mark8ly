# Multi-stage Dockerfile for the otto support-chat service.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder:20261022@sha256:52dfe62a2b3fac61f5cef9ef6c551810745c8f1308f314eb1f370c9f5e5f027e AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ghcr.io/tesserix/base-distroless-static:20261024@sha256:a0ada4bf3d50447e7bbc3bf95f872ade97dd85feba81e7b4facf8e9fab665594 AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8089
CMD ["/server"]
