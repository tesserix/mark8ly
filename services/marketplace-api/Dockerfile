# syntax=docker/dockerfile:1
#
# Mark8ly marketplace-api Dockerfile.
# Builds on tesserix company-owned base images — non-root uid 10001,
# tini PID 1, ca-certs + tzdata + wget/curl, and weekly upstream CVE
# patches are all baked in the base.

# ─── builder ────────────────────────────────────────────────────────────
FROM ghcr.io/tesserix/base-go-builder:latest AS builder
USER 0:0
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" \
      -o /out/marketplace-api ./cmd/marketplace-api \
 && go build -trimpath -ldflags="-s -w" \
      -o /out/migrate ./cmd/migrate

# ─── migrate ────────────────────────────────────────────────────────────
# Standalone image used by the K8s Job pattern.
FROM ghcr.io/tesserix/base-alpine-runtime:latest AS migrate
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
CMD ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries BOTH binaries — server (default CMD) and the
# migrate CLI at /usr/local/bin/migrate — so the Helm init container
# can override ENTRYPOINT to run migrations from the same image.
FROM ghcr.io/tesserix/base-alpine-runtime:latest AS runtime
COPY --from=builder --chown=10001:10001 /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder --chown=10001:10001 /out/migrate /usr/local/bin/migrate
EXPOSE 8091
# base-alpine-runtime's ENTRYPOINT is already `tini --`.
CMD ["/usr/local/bin/marketplace-api"]
