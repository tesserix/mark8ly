# Multi-stage Dockerfile for the otto support-chat service.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder:20261113@sha256:63086000b8d00b95ff9312080d4d1f0d64475039ff81db67a2cda5db70c23d48 AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ghcr.io/tesserix/base-distroless-static:20261113@sha256:bd65dd61664f24e0e70b80881b5e75559ff0d98b1d99a2d2f61365b0dcf828d2 AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8089
CMD ["/server"]
