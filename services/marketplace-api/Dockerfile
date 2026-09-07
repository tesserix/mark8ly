# syntax=docker/dockerfile:1
#
# Mark8ly marketplace-api Dockerfile.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM so tini is unnecessary; K8s
# securityContext overrides USER and binaries are mode 0755.
#
# Build context MUST be the monorepo root (mark8ly#720): marketplace-api's
# go.mod holds `replace github.com/mark8ly/platformauth =>
# ../../packages/platformauth`, a sibling module outside a
# services/marketplace-api-scoped context. platform-api's Dockerfile follows
# the same pattern for the same reason.

# ─── builder ────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-go-builder@sha256:1d52f1321953c29971c261ccc7cd212f2ab84008321b88113ccff88330dd7d1f AS builder
USER 0:0
WORKDIR /src

# go.mod/go.sum first (both modules) so `go mod download` layer-caches
# independently of source changes. The local replace target only needs to
# exist as a module — its own go.sum isn't consulted by the downloader.
COPY services/marketplace-api/go.mod services/marketplace-api/go.sum ./services/marketplace-api/
COPY packages/platformauth/go.mod packages/platformauth/go.sum ./packages/platformauth/
WORKDIR /src/services/marketplace-api
RUN go mod download

WORKDIR /src
COPY services/marketplace-api/ ./services/marketplace-api/
COPY packages/platformauth/ ./packages/platformauth/

WORKDIR /src/services/marketplace-api
RUN go build -trimpath -ldflags="-s -w" \
      -o /out/marketplace-api ./cmd/marketplace-api \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/refund-sweep-cron ./cmd/refund-sweep-cron \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/stock-hold-sweep-cron ./cmd/stock-hold-sweep-cron \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/webhook-delivery-prune-cron ./cmd/webhook-delivery-prune-cron \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/carrier-secrets-backfill ./cmd/carrier-secrets-backfill

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-distroless-static@sha256:db09c7f5c9fde6eb1217a323d6e0158ba6339123859cb8ffc113320352a99598 AS migrate
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
FROM ghcr.io/tesserix/base-distroless-static@sha256:db09c7f5c9fde6eb1217a323d6e0158ba6339123859cb8ffc113320352a99598 AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
COPY --from=builder --chown=10001:10001 /out/refund-sweep-cron /usr/local/bin/refund-sweep-cron
COPY --from=builder --chown=10001:10001 /out/stock-hold-sweep-cron /usr/local/bin/stock-hold-sweep-cron
COPY --from=builder --chown=10001:10001 /out/webhook-delivery-prune-cron /usr/local/bin/webhook-delivery-prune-cron
# carrier-secrets-backfill is a ONE-OFF migration tool, not a CronJob: it is
# run by hand as a Job when credentials need moving from GCP Secret Manager to
# OpenBao (mark8ly#319 phase 3). It defaults to --dry-run and refuses to run
# unless SHIPPING_SECRET_STORE=bao. Shipped in the image so it runs in-cluster
# under the real ServiceAccount, rather than from an operator's laptop with
# production credentials port-forwarded to it.
COPY --from=builder --chown=10001:10001 /out/carrier-secrets-backfill /usr/local/bin/carrier-secrets-backfill
EXPOSE 8091
CMD ["/usr/local/bin/marketplace-api"]
