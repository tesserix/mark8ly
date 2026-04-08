# Multi-stage Dockerfile producing two targets: server and migrate.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

# Single runtime stage carrying both binaries so the K8s init pattern can
# run /migrate as a command override on the same image.
FROM alpine:3.19 AS server
RUN apk add --no-cache ca-certificates
COPY --from=build /out/server /server
COPY --from=build /out/migrate /migrate
EXPOSE 8080
ENTRYPOINT ["/server"]

# Legacy single-purpose stage retained for local docker-compose flows.
FROM alpine:3.19 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=build /out/migrate /migrate
ENTRYPOINT ["/migrate"]
