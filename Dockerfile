# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./internal/web/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nfc-time-tracker-server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl restic su-exec \
    && adduser -D -u 1000 app
WORKDIR /app
COPY --from=build /out/nfc-time-tracker-server /usr/local/bin/nfc-time-tracker-server
COPY config.docker.yaml /app/config.yaml
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
ENV NFC_CONFIG_PATH=/app/config.yaml \
    NFC_BACKUP_TARGET_PATH=/backup
EXPOSE 8080
VOLUME ["/data", "/backup"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8080/api/v1/health >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/nfc-time-tracker-server", "/app/config.yaml"]
