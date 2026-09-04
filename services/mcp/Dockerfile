# Multi-stage Dockerfile for the mcp-catalog MCP server.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder@sha256:1d52f1321953c29971c261ccc7cd212f2ab84008321b88113ccff88330dd7d1f AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/mcp-catalog

FROM ghcr.io/tesserix/base-distroless-static@sha256:db09c7f5c9fde6eb1217a323d6e0158ba6339123859cb8ffc113320352a99598 AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8765
CMD ["/server"]
