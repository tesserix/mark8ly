# Multi-stage Dockerfile producing two targets: server and migrate.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

# Single runtime stage carrying both binaries so the K8s init pattern can
# run /migrate as a command override on the same image. Runs as an
# unprivileged user (uid 10001) so an RCE in the service can't write to
# root-owned paths or exec root-only syscalls.
FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 -S app \
    && adduser  -u 10001 -S app -G app
COPY --from=build /out/server /server
COPY --from=build /out/migrate /migrate
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/server"]

# Legacy single-purpose stage retained for local docker-compose flows.
FROM alpine:3.22 AS migrate
RUN apk add --no-cache ca-certificates \
    && addgroup -g 10001 -S app \
    && adduser  -u 10001 -S app -G app
COPY --from=build /out/migrate /migrate
USER 10001:10001
ENTRYPOINT ["/migrate"]
