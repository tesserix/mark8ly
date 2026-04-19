# Multi-stage Dockerfile producing three targets: server, migrate, seed.
# Same source, same embedded migrations, no version skew.
# Builds on tesserix company-owned base images.

FROM ghcr.io/tesserix/base-go-builder:latest AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server  ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/seed    ./cmd/seed

# Single runtime stage carries all three binaries so the K8s init+seed
# pattern can override ENTRYPOINT without pulling a second image.
FROM ghcr.io/tesserix/base-alpine-runtime:latest AS server
COPY --from=build --chown=10001:10001 /out/server  /server
COPY --from=build --chown=10001:10001 /out/migrate /migrate
COPY --from=build --chown=10001:10001 /out/seed    /seed
EXPOSE 8086
# base-alpine-runtime's ENTRYPOINT is already `tini --`.
CMD ["/server"]

# Legacy single-purpose stages retained for local docker-compose flows.
FROM ghcr.io/tesserix/base-alpine-runtime:latest AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]

FROM ghcr.io/tesserix/base-alpine-runtime:latest AS seed
COPY --from=build --chown=10001:10001 /out/seed /seed
CMD ["/seed"]
