# Multi-stage Dockerfile for the otto support-chat service.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder@sha256:196c74ed0c05329dbe72c4277b7c983ab32c31279769e09bee9c9539e89e2b4a AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ghcr.io/tesserix/base-distroless-static@sha256:2fea7cf18950a0c9e6a08d0260643003d05eb43e66e09ec4d06562e3b71024e8 AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8089
CMD ["/server"]
