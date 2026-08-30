# Multi-stage Dockerfile producing four targets: server, migrate, seed,
# backfill-vendors. Same source, same embedded migrations, no version skew.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)

FROM ghcr.io/tesserix/base-go-builder@sha256:4ec5ce5894e501cc24f3a81329a8d548661e234be00b1f68205171bc6d048d1a AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server           ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate          ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/seed             ./cmd/seed \
 && go build -trimpath -ldflags="-s -w" -o /out/backfill-vendors ./cmd/backfill-vendors

# Single runtime stage carries all four binaries so K8s init/seed/Job
# overrides can pick the right entrypoint without pulling a second image.
# /backfill-vendors is the CLI surfaced by the marketplace-api error
# "tenant has no self-vendor; run platform-api's backfill-vendors CLI".
FROM ghcr.io/tesserix/base-distroless-static@sha256:0d68ca8037f210e82e211017dbf49b54cde04c42a47baa5c5e429dee07b29c14 AS server
COPY --from=build --chown=10001:10001 /out/server           /server
COPY --from=build --chown=10001:10001 /out/migrate          /migrate
COPY --from=build --chown=10001:10001 /out/seed             /seed
COPY --from=build --chown=10001:10001 /out/backfill-vendors /backfill-vendors
EXPOSE 8086
CMD ["/server"]

# Legacy single-purpose stages retained for local docker-compose flows.
FROM ghcr.io/tesserix/base-distroless-static@sha256:0d68ca8037f210e82e211017dbf49b54cde04c42a47baa5c5e429dee07b29c14 AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]

FROM ghcr.io/tesserix/base-distroless-static@sha256:0d68ca8037f210e82e211017dbf49b54cde04c42a47baa5c5e429dee07b29c14 AS seed
COPY --from=build --chown=10001:10001 /out/seed /seed
CMD ["/seed"]
