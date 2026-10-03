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
FROM ghcr.io/tesserix/base-go-builder:20260926@sha256:7ac7a7ccf1c0038f3a28621c64c0b53e6f14c23cae1e05026047cb05429279d2 AS builder
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
      -o /out/personalisation-upload-sweep-cron ./cmd/personalisation-upload-sweep-cron \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/carrier-secrets-backfill ./cmd/carrier-secrets-backfill \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/backfill-trial-start ./cmd/backfill-trial-start \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/webhook-replay ./cmd/webhook-replay

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-distroless-static:20260926@sha256:29dff281f52c6e95f5c5a2f2224c9ee99c0c7aa6326620cb1a2c56a14c7d970f AS migrate
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
CMD ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries every binary — server (default ENTRYPOINT), the
# migrate CLI at /usr/local/bin/migrate, the refund-sweep-cron, the
# stock-hold-sweep-cron, the webhook-delivery-prune-cron and the
# personalisation-upload-sweep-cron — so the Helm init container and the
# CronJobs can override ENTRYPOINT and run migrations / the stuck-refund
# sweeper / the expired-hold sweeper / the outbound-delivery pruner / the
# abandoned-artwork reaper from the same image.
#
# webhook-delivery-prune-cron prunes webhook_deliveries (OUTBOUND merchant
# webhooks). It is NOT internal/webhookprune, which prunes webhook_events
# (the inbound provider log) in-process inside marketplace-api.
FROM ghcr.io/tesserix/base-distroless-static:20260926@sha256:29dff281f52c6e95f5c5a2f2224c9ee99c0c7aa6326620cb1a2c56a14c7d970f AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
COPY --from=builder --chown=10001:10001 /out/refund-sweep-cron /usr/local/bin/refund-sweep-cron
COPY --from=builder --chown=10001:10001 /out/stock-hold-sweep-cron /usr/local/bin/stock-hold-sweep-cron
COPY --from=builder --chown=10001:10001 /out/webhook-delivery-prune-cron /usr/local/bin/webhook-delivery-prune-cron
# personalisation-upload-sweep-cron reaps buyer artwork that was uploaded to
# a cart and never checked out (#963). It MUST stay app-level rather than
# become a GCS lifecycle rule: a claimed order's artwork is the very same
# object under the very same buyer-uploads/ key — nothing is copied at
# checkout — so an age-based bucket rule would delete the artwork of paid
# orders. Only the database knows which objects are claimed.
COPY --from=builder --chown=10001:10001 /out/personalisation-upload-sweep-cron /usr/local/bin/personalisation-upload-sweep-cron
# carrier-secrets-backfill is a ONE-OFF migration tool, not a CronJob: it is
# run by hand as a Job when credentials need moving from GCP Secret Manager to
# OpenBao (mark8ly#319 phase 3). It defaults to --dry-run and refuses to run
# unless SHIPPING_SECRET_STORE=bao. Shipped in the image so it runs in-cluster
# under the real ServiceAccount, rather than from an operator's laptop with
# production credentials port-forwarded to it.
COPY --from=builder --chown=10001:10001 /out/carrier-secrets-backfill /usr/local/bin/carrier-secrets-backfill
# backfill-trial-start is the same shape of tool: a ONE-OFF, run by hand as a
# Job to give stores created before #828 the subscription row — and therefore
# the trial clock — that nothing created for them at signup (mark8ly#827).
#
# Shipped for the same reason as the one above, and for a sharper one: it was
# written, merged and left out of this image, so the command existed and could
# not be run anywhere the data lives. It reports what it would write and needs
# -apply to write anything.
COPY --from=builder --chown=10001:10001 /out/backfill-trial-start /usr/local/bin/backfill-trial-start
# webhook-replay repeats the mistake recorded directly above, which is why it
# is here: it shipped in #905 and was left out of this image, so the tool that
# recovers stuck Stripe events could not be run anywhere the events live.
#
# It puts events that exhausted their retries back in front of the resolver by
# clearing manual_review_required. It applies nothing itself — the orphan cron
# dispatches them on its next pass, under the usual advisory lock — and -list
# changes nothing at all. Shipped for the same reason as the two above: it
# needs the database, so it belongs in-cluster under the real ServiceAccount
# rather than on a laptop with production credentials forwarded to it.
COPY --from=builder --chown=10001:10001 /out/webhook-replay /usr/local/bin/webhook-replay
EXPOSE 8091
CMD ["/usr/local/bin/marketplace-api"]
