# Multi-stage Dockerfile for the mcp-catalog MCP server.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder@sha256:4ec5ce5894e501cc24f3a81329a8d548661e234be00b1f68205171bc6d048d1a AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/mcp-catalog

FROM ghcr.io/tesserix/base-distroless-static@sha256:0d68ca8037f210e82e211017dbf49b54cde04c42a47baa5c5e429dee07b29c14 AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8765
CMD ["/server"]
