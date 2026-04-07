# Multi-stage Dockerfile producing two targets: server and migrate.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

FROM alpine:3.19 AS server
RUN apk add --no-cache ca-certificates
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]

FROM alpine:3.19 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=build /out/migrate /migrate
ENTRYPOINT ["/migrate"]
