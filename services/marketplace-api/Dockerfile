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
FROM ghcr.io/tesserix/base-go-builder@sha256:4ec5ce5894e501cc24f3a81329a8d548661e234be00b1f68205171bc6d048d1a AS builder
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
      -o /out/refund-sweep-cron ./cmd/refund-sweep-cron \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/stock-hold-sweep-cron ./cmd/stock-hold-sweep-cron \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/webhook-delivery-prune-cron ./cmd/webhook-delivery-prune-cron

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-distroless-static@sha256:0d68ca8037f210e82e211017dbf49b54cde04c42a47baa5c5e429dee07b29c14 AS migrate
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
CMD ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries every binary — server (default ENTRYPOINT), the
# migrate CLI at /usr/local/bin/migrate, the refund-sweep-cron, the
# stock-hold-sweep-cron and the webhook-delivery-prune-cron — so the Helm
# init container and the CronJobs can override ENTRYPOINT and run
# migrations / the stuck-refund sweeper / the expired-hold sweeper / the
# outbound-delivery pruner from the same image.
#
# webhook-delivery-prune-cron prunes webhook_deliveries (OUTBOUND merchant
# webhooks). It is NOT internal/webhookprune, which prunes webhook_events
# (the inbound provider log) in-process inside marketplace-api.
FROM ghcr.io/tesserix/base-distroless-static@sha256:0d68ca8037f210e82e211017dbf49b54cde04c42a47baa5c5e429dee07b29c14 AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
COPY --from=builder --chown=10001:10001 /out/refund-sweep-cron /usr/local/bin/refund-sweep-cron
COPY --from=builder --chown=10001:10001 /out/stock-hold-sweep-cron /usr/local/bin/stock-hold-sweep-cron
COPY --from=builder --chown=10001:10001 /out/webhook-delivery-prune-cron /usr/local/bin/webhook-delivery-prune-cron
EXPOSE 8091
CMD ["/usr/local/bin/marketplace-api"]
