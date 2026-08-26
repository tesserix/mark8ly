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
FROM ghcr.io/tesserix/base-go-builder@sha256:6dd108601b58dde3e6cee2b1bac01b113baa9180f4051983c4b69b76a1c8ec06 AS builder
USER 0:0
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" \
      -o /out/marketplace-api ./cmd/marketplace-api \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/refund-sweep-cron ./cmd/refund-sweep-cron

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-distroless-static@sha256:12c4a0a01927b1bd443d69f56923ec493d45cca2979a40d6aab8c255811d50c5 AS migrate
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
CMD ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries all three binaries — server (default ENTRYPOINT),
# the migrate CLI at /usr/local/bin/migrate, and the refund-sweep-cron at
# /usr/local/bin/refund-sweep-cron — so the Helm init container and the
# refund-sweep CronJob can override ENTRYPOINT to run migrations / the
# stuck-refund sweeper from the same image.
FROM ghcr.io/tesserix/base-distroless-static@sha256:12c4a0a01927b1bd443d69f56923ec493d45cca2979a40d6aab8c255811d50c5 AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
COPY --from=builder --chown=10001:10001 /out/refund-sweep-cron /usr/local/bin/refund-sweep-cron
EXPOSE 8091
CMD ["/usr/local/bin/marketplace-api"]
