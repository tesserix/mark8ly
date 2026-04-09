# syntax=docker/dockerfile:1

# ─── builder ────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/marketplace-api ./cmd/marketplace-api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ─── migrate ────────────────────────────────────────────────────────────
FROM alpine:3.19 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/migrate /usr/local/bin/migrate
ENTRYPOINT ["/usr/local/bin/migrate"]

# ─── runtime ────────────────────────────────────────────────────────────
# Runtime image carries BOTH binaries — the server (default ENTRYPOINT)
# and the migrate CLI at /usr/local/bin/migrate. The Helm chart's init
# container overrides the entrypoint to call /usr/local/bin/migrate.
# Mirrors platform-api's pattern of one image, two binaries.
FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /out/marketplace-api /usr/local/bin/marketplace-api
COPY --from=builder /out/migrate /usr/local/bin/migrate
EXPOSE 8091
ENTRYPOINT ["/usr/local/bin/marketplace-api"]
