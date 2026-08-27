# Multi-stage Dockerfile producing four targets: server, migrate, seed,
# backfill-vendors. Same source, same embedded migrations, no version skew.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)

FROM ghcr.io/tesserix/base-go-builder@sha256:40fbd9b59ef20100c90213a5390332c48fa4ef7d5908223f4f088cb4ab476972 AS build
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
FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS server
COPY --from=build --chown=10001:10001 /out/server           /server
COPY --from=build --chown=10001:10001 /out/migrate          /migrate
COPY --from=build --chown=10001:10001 /out/seed             /seed
COPY --from=build --chown=10001:10001 /out/backfill-vendors /backfill-vendors
EXPOSE 8086
CMD ["/server"]

# Legacy single-purpose stages retained for local docker-compose flows.
FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]

FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS seed
COPY --from=build --chown=10001:10001 /out/seed /seed
CMD ["/seed"]
