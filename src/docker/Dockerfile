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

# Install dependencies for webhook receiver and supervisor
RUN apk update && apk --no-cache add ca-certificates curl supervisor

# Copy the webhook receiver binary
COPY --from=builder /app/webhook-receiver /usr/local/bin/

# Copy supervisor configuration
COPY config/supervisord.conf /etc/supervisor/supervisord.conf

# Create log directory
RUN mkdir -p /var/log/supervisor

# Expose both registry (5000) and webhook (5001) ports
EXPOSE 5000 5001

# Health check for both services
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:5000/v2/ && curl -f http://localhost:5001/health || exit 1

# Override entrypoint to use supervisor
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf"]
