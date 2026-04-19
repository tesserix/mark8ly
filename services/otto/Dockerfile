# Multi-stage Dockerfile for the otto support-chat service.
# Builds on tesserix company-owned base images — non-root uid 10001,
# tini PID 1, ca-certs, tzdata and weekly upstream CVE patches come
# from the base, so this file only carries the app-specific build +
# copy chain.

FROM ghcr.io/tesserix/base-go-builder:latest AS build
USER 0:0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ghcr.io/tesserix/base-alpine-runtime:latest AS server
COPY --from=build --chown=10001:10001 /out/server /server
EXPOSE 8089
# base-alpine-runtime's ENTRYPOINT is already `tini --`.
CMD ["/server"]
