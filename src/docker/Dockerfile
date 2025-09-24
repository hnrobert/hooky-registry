# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy entire app directory
COPY app/ ./

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o webhook-receiver .

# Production stage - based on registry image
FROM registry:2

# Install runtime dependencies
RUN apk update && apk --no-cache add curl

# Copy the webhook receiver binary
COPY --from=builder /app/webhook-receiver /usr/local/bin/

# Copy registry configuration (kept as a default, docker-compose may mount another)
COPY config/config.yml /etc/docker/registry/config.yml

# Copy entrypoint script
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Expose both registry (5000) and webhook (5001) ports
EXPOSE 5000 5001

# Health check for both services
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:5000/v2/ && curl -f http://localhost:5001/health || exit 1

# Use our entrypoint to manage both registry and webhook
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
