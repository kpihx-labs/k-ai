# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/k-ai ./cmd/k-ai

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=builder /out/k-ai /app/k-ai
COPY config/config.yaml /app/config/config.yaml
COPY scripts/docker-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 8080
ENV K_AI_CONFIG_PATH=/app/config/config.yaml
ENV K_AI_DATA_DIR=/data
VOLUME ["/data"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["-config", "/app/config/config.yaml"]
