# Multi-stage Dockerfile for the otto support-chat service.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder:20261003@sha256:b54f68e360a9d2dd7ae6606e41de2e48bd601c3af8e155f4a4e44fcaa816c9d7 AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ghcr.io/tesserix/base-distroless-static:20261003@sha256:a48c2e47ff68bd42d63ab38e1c0304726961d58d893278830b7a1abbe8af8ee0 AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8089
CMD ["/server"]
