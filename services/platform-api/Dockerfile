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
FROM alpine:3.19 AS server
RUN apk add --no-cache ca-certificates
COPY --from=build /out/server /server
COPY --from=build /out/migrate /migrate
COPY --from=build /out/seed /seed
EXPOSE 8086
ENTRYPOINT ["/server"]

# Legacy single-purpose stages retained for local docker-compose flows that
# build with --target migrate / --target seed.
FROM alpine:3.19 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=build /out/migrate /migrate
ENTRYPOINT ["/migrate"]

FROM alpine:3.19 AS seed
RUN apk add --no-cache ca-certificates
COPY --from=build /out/seed /seed
ENTRYPOINT ["/seed"]
