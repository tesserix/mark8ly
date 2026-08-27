# Multi-stage Dockerfile for the otto support-chat service.
# Builds on tesserix company-owned base images:
#   * builder: base-go-builder (CGO_ENABLED=0 — static binaries)
#   * runtime: base-distroless-static (no shell, ca-certs + tzdata,
#              upstream gcr.io/distroless/static:nonroot)
# Go's runtime handles SIGTERM directly so tini is unnecessary; K8s
# securityContext overrides USER, mode 0755 lets any uid execute.

FROM ghcr.io/tesserix/base-go-builder@sha256:40fbd9b59ef20100c90213a5390332c48fa4ef7d5908223f4f088cb4ab476972 AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ghcr.io/tesserix/base-distroless-static@sha256:ccf26e72c62d53b69caae2ab73f3b059fe46785c0e5c18868e103b63c52a729f AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8089
CMD ["/server"]
