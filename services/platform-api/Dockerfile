# Multi-stage Dockerfile producing four targets: server, migrate, seed,
# backfill-vendors. Same source, same embedded migrations, no version skew.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
#
# Build context MUST be the monorepo root (mark8ly#720). platform-api does
# not import packages/platformauth yet — that lands in a follow-up — but its
# Dockerfile is switched to the root context here too so that follow-up is a
# pure code change, not another build-context migration. marketplace-api's
# Dockerfile needs the root context now because its go.mod already holds
# `replace github.com/mark8ly/platformauth => ../../packages/platformauth`,
# a sibling module a services/platform-api-scoped context can't reach.

FROM ghcr.io/tesserix/base-go-builder:20260919@sha256:f60d710e75afce7cffbcb63e1bbbf5dd7236d16bfff882ba050fb1f7ded3ed21 AS build
USER 0:0
WORKDIR /src

# go.mod/go.sum first (both modules) so `go mod download` layer-caches
# independently of source changes. The local replace target only needs to
# exist as a module — its own go.sum isn't consulted by the downloader.
COPY services/platform-api/go.mod services/platform-api/go.sum ./services/platform-api/
COPY packages/platformauth/go.mod packages/platformauth/go.sum ./packages/platformauth/
WORKDIR /src/services/platform-api
RUN go mod download

WORKDIR /src
COPY services/platform-api/ ./services/platform-api/
COPY packages/platformauth/ ./packages/platformauth/

WORKDIR /src/services/platform-api
RUN go build -trimpath -ldflags="-s -w" -o /out/server           ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate          ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/seed             ./cmd/seed \
 && go build -trimpath -ldflags="-s -w" -o /out/backfill-vendors ./cmd/backfill-vendors

# Single runtime stage carries all four binaries so K8s init/seed/Job
# overrides can pick the right entrypoint without pulling a second image.
# /backfill-vendors is the CLI surfaced by the marketplace-api error
# "tenant has no self-vendor; run platform-api's backfill-vendors CLI".
FROM ghcr.io/tesserix/base-distroless-static:20260912@sha256:98b997f450dc0c45cc6fbf4b717ae399c03b3312873d515e3e0438fc1f5d628b AS server
COPY --from=build --chown=10001:10001 /out/server           /server
COPY --from=build --chown=10001:10001 /out/migrate          /migrate
COPY --from=build --chown=10001:10001 /out/seed             /seed
COPY --from=build --chown=10001:10001 /out/backfill-vendors /backfill-vendors
EXPOSE 8086
CMD ["/server"]

# Legacy single-purpose stages retained for local docker-compose flows.
FROM ghcr.io/tesserix/base-distroless-static:20260912@sha256:98b997f450dc0c45cc6fbf4b717ae399c03b3312873d515e3e0438fc1f5d628b AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]

FROM ghcr.io/tesserix/base-distroless-static:20260912@sha256:98b997f450dc0c45cc6fbf4b717ae399c03b3312873d515e3e0438fc1f5d628b AS seed
COPY --from=build --chown=10001:10001 /out/seed /seed
CMD ["/seed"]
