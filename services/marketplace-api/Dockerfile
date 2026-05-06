# syntax=docker/dockerfile:1
#
# Mark8ly marketplace-api Dockerfile.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM so tini is unnecessary; K8s
# securityContext overrides USER and binaries are mode 0755.

# ─── builder ────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-go-builder:latest AS builder
USER 0:0
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" \
      -o /out/marketplace-api ./cmd/marketplace-api \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/migrate ./cmd/migrate

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-distroless-static:latest AS migrate
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
CMD ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries BOTH binaries — server (default ENTRYPOINT) and the
# migrate CLI at /usr/local/bin/migrate — so the Helm init container
# can override ENTRYPOINT to run migrations from the same image.
FROM ghcr.io/tesserix/base-distroless-static:latest AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
EXPOSE 8091
CMD ["/usr/local/bin/marketplace-api"]
