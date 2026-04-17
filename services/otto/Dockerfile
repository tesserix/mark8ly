# Multi-stage Dockerfile for the otto support-chat service.
# Mirrors the mark8ly house style (auth-bff / marketplace-api).

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.19 AS server
RUN apk add --no-cache ca-certificates \
 && addgroup -g 1000 otto \
 && adduser -D -u 1000 -G otto otto
USER otto:otto
COPY --from=build /out/server /server
EXPOSE 8089
ENTRYPOINT ["/server"]
