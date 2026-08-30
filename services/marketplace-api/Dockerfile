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
FROM ghcr.io/tesserix/base-go-builder@sha256:40fbd9b59ef20100c90213a5390332c48fa4ef7d5908223f4f088cb4ab476972 AS builder
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
      -o /out/stock-hold-sweep-cron ./cmd/stock-hold-sweep-cron

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS migrate
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
CMD ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries every binary — server (default ENTRYPOINT), the
# migrate CLI at /usr/local/bin/migrate, the refund-sweep-cron and the
# stock-hold-sweep-cron — so the Helm init container and the CronJobs can
# override ENTRYPOINT and run migrations / the stuck-refund sweeper / the
# expired-hold sweeper from the same image.
FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
COPY --from=builder --chown=10001:10001 /out/refund-sweep-cron /usr/local/bin/refund-sweep-cron
COPY --from=builder --chown=10001:10001 /out/stock-hold-sweep-cron /usr/local/bin/stock-hold-sweep-cron
EXPOSE 8091
CMD ["/usr/local/bin/marketplace-api"]
