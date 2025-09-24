#!/bin/sh
set -e

# Start registry in background
registry_config="/etc/docker/registry/config.yml"
if [ ! -f "$registry_config" ]; then
    echo "Warning: registry config not found at $registry_config"
fi

echo "Starting registry..."
/entrypoint.sh /etc/docker/registry/config.yml &
REG_PID=$!

# Start webhook receiver
if [ -x "/usr/local/bin/webhook-receiver" ]; then
    echo "Starting webhook receiver..."
    /usr/local/bin/webhook-receiver &
    WEBHOOK_PID=$!
else
    echo "webhook-receiver binary not found or not executable"
fi

# Forward signals to children
term_handler() {
    echo "Termination signal received, stopping services..."
    if [ -n "$WEBHOOK_PID" ]; then
        kill -TERM "$WEBHOOK_PID" 2>/dev/null || true
        wait "$WEBHOOK_PID" || true
    fi
    if [ -n "$REG_PID" ]; then
        kill -TERM "$REG_PID" 2>/dev/null || true
        wait "$REG_PID" || true
    fi
    exit 0
}

trap 'term_handler' TERM INT

# Wait for children
wait
