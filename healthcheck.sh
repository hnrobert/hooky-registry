#!/bin/bash

# Combined Registry Deployment Script
# This script helps deploy the combined registry and webhook receiver image

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="docker-compose.combined.yaml"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if Docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker and try again."
        exit 1
    fi
}

# Function to create external network if it doesn't exist
create_network() {
    if ! docker network ls | grep -q "mach-network"; then
        print_status "Creating external network: mach-network"
        docker network create mach-network
        print_success "Network created successfully"
    else
        print_status "Network mach-network already exists"
    fi
}

# Function to deploy the combined service
deploy() {
    print_status "Deploying combined registry and webhook receiver..."
    
    check_docker
    create_network
    
    cd "$SCRIPT_DIR"
    
    # Pull latest image if using pre-built image
    if grep -q "^[[:space:]]*image:" "$COMPOSE_FILE"; then
        print_status "Pulling latest image..."
        docker-compose -f "$COMPOSE_FILE" pull
    fi
    
    # Start the service
    docker-compose -f "$COMPOSE_FILE" up -d
    
    print_success "Combined registry service deployed successfully!"
    print_status "Registry available at: http://localhost:5000"
    print_status "Webhook receiver available at: http://localhost:5001"
    print_status "Health check: http://localhost:5001/health"
}

# Function to stop the service
stop() {
    print_status "Stopping combined registry service..."
    cd "$SCRIPT_DIR"
    docker-compose -f "$COMPOSE_FILE" down
    print_success "Service stopped successfully!"
}

# Function to show logs
logs() {
    cd "$SCRIPT_DIR"
    docker-compose -f "$COMPOSE_FILE" logs -f
}

# Function to show status
status() {
    cd "$SCRIPT_DIR"
    docker-compose -f "$COMPOSE_FILE" ps
    
    print_status "Testing service health..."
    
    # Test registry
    if curl -s http://localhost:5000/v2/ >/dev/null; then
        print_success "Registry is healthy"
    else
        print_error "Registry is not responding"
    fi
    
    # Test webhook receiver
    if curl -s http://localhost:5001/health >/dev/null; then
        print_success "Webhook receiver is healthy"
    else
        print_error "Webhook receiver is not responding"
    fi
}

# Function to build local image
build() {
    print_status "Building combined registry image locally..."
    cd "$SCRIPT_DIR"
    docker build -f Dockerfile.combined -t registry-combined:local .
    print_success "Image built successfully as registry-combined:local"
}

# Function to show help
show_help() {
    echo "Combined Registry Management Script"
    echo ""
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  deploy    Deploy the combined registry and webhook receiver"
    echo "  stop      Stop the combined service"
    echo "  logs      Show service logs"
    echo "  status    Show service status and health"
    echo "  build     Build the combined image locally"
    echo "  help      Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 deploy    # Deploy the service"
    echo "  $0 logs      # Show live logs"
    echo "  $0 status    # Check service health"
}

# Main script logic
case "${1:-}" in
    deploy)
        deploy
    ;;
    stop)
        stop
    ;;
    logs)
        logs
    ;;
    status)
        status
    ;;
    build)
        build
    ;;
    help|--help|-h)
        show_help
    ;;
    *)
        print_error "Unknown command: ${1:-}"
        echo ""
        show_help
        exit 1
    ;;
esac