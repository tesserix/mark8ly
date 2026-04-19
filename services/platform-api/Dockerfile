# Multi-stage Dockerfile producing two targets: server and migrate.
# Same source, same embedded migrations, no version skew.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 go build -o /out/seed ./cmd/seed

# Single runtime stage carrying all three binaries so the K8s init+seed
# pattern can run /migrate and /seed as command overrides on the same image.
# Runs as unprivileged user 10001 so the server can't escalate on an RCE.
FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 -S app \
    && adduser  -u 10001 -S app -G app
COPY --from=build /out/server /server
COPY --from=build /out/migrate /migrate
COPY --from=build /out/seed /seed
USER 10001:10001
EXPOSE 8086
ENTRYPOINT ["/server"]

# Legacy single-purpose stages retained for local docker-compose flows that
# build with --target migrate / --target seed.
FROM alpine:3.22 AS migrate
RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 -S app \
    && adduser  -u 10001 -S app -G app
COPY --from=build /out/migrate /migrate
USER 10001:10001
ENTRYPOINT ["/migrate"]

FROM alpine:3.22 AS seed
RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 -S app \
    && adduser  -u 10001 -S app -G app
COPY --from=build /out/seed /seed
USER 10001:10001
ENTRYPOINT ["/seed"]
