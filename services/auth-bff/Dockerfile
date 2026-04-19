# Multi-stage Dockerfile producing two targets: server and migrate.
# Builds on tesserix company-owned base images so the non-root uid,
# tini, ca-certs, tzdata and weekly upstream CVE patches come from the
# base — this file only describes app-specific build + copy.

FROM ghcr.io/tesserix/base-go-builder:latest AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Single runtime stage carries both binaries so the K8s init pattern
# can override ENTRYPOINT to /migrate without pulling a second image.
FROM ghcr.io/tesserix/base-alpine-runtime:latest AS server
COPY --from=build --chown=10001:10001 /out/server /server
COPY --from=build --chown=10001:10001 /out/migrate /migrate
EXPOSE 8080
# base-alpine-runtime's ENTRYPOINT is already `tini --`.
CMD ["/server"]

# Legacy single-purpose stage retained for local docker-compose flows
# that build with --target migrate.
FROM ghcr.io/tesserix/base-alpine-runtime:latest AS migrate
COPY --from=build --chown=10001:10001 /out/migrate /migrate
CMD ["/migrate"]
