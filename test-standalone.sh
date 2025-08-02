#!/bin/bash

# ============================================================================
# ERP Auth Service - Standalone Testing Script
# ============================================================================
# This script provides comprehensive testing capabilities for the auth service
# in standalone mode with all dependencies isolated.
#
# Usage:
#   ./test-standalone.sh [command] [options]
#
# Commands:
#   start       - Start all services
#   stop        - Stop all services
#   restart     - Restart all services
#   test        - Run all tests
#   test-unit   - Run unit tests only
#   test-integration - Run integration tests only
#   test-load   - Run load tests
#   logs        - Show service logs
#   health      - Check service health
#   clean       - Clean up all data
#   tools       - Start with management tools
#   help        - Show this help message
# ============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="docker-compose.standalone.yml"
SERVICE_NAME="auth-service"
HEALTH_ENDPOINT="http://localhost:8080/health"
GRPC_ENDPOINT="localhost:50051"

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_dependencies() {
    log_info "Checking dependencies..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed or not in PATH"
        exit 1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_warning "curl is not installed - health checks will be limited"
    fi
    
    log_success "Dependencies check passed"
}

wait_for_service() {
    local service_name=$1
    local max_attempts=30
    local attempt=1
    
    log_info "Waiting for $service_name to be healthy..."
    
    while [ $attempt -le $max_attempts ]; do
        if docker-compose -f $COMPOSE_FILE ps $service_name | grep -q "healthy\|Up"; then
            log_success "$service_name is ready"
            return 0
        fi
        
        log_info "Attempt $attempt/$max_attempts - waiting for $service_name..."
        sleep 5
        ((attempt++))
    done
    
    log_error "$service_name failed to become healthy within timeout"
    return 1
}

start_services() {
    log_info "Starting auth service and dependencies..."
    
    # Start core services
    docker-compose -f $COMPOSE_FILE up -d postgres-auth redis-auth kafka-auth
    
    # Wait for dependencies
    wait_for_service postgres-auth
    wait_for_service redis-auth
    wait_for_service kafka-auth
    
    # Start auth service
    docker-compose -f $COMPOSE_FILE up -d auth-service
    wait_for_service auth-service
    
    log_success "All services started successfully"
    show_service_info
}

start_with_tools() {
    log_info "Starting auth service with management tools..."
    
    # Start all services including tools
    docker-compose -f $COMPOSE_FILE --profile tools up -d
    
    # Wait for core services
    wait_for_service postgres-auth
    wait_for_service redis-auth
    wait_for_service kafka-auth
    wait_for_service auth-service
    
    log_success "All services and tools started successfully"
    show_service_info
    show_tools_info
}

stop_services() {
    log_info "Stopping all services..."
    docker-compose -f $COMPOSE_FILE down
    log_success "All services stopped"
}

restart_services() {
    log_info "Restarting services..."
    stop_services
    sleep 2
    start_services
}

clean_data() {
    log_warning "This will remove all data including databases and caches!"
    read -p "Are you sure? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cleaning up all data..."
        docker-compose -f $COMPOSE_FILE down -v
        docker-compose -f $COMPOSE_FILE rm -f
        log_success "All data cleaned up"
    else
        log_info "Cleanup cancelled"
    fi
}

show_logs() {
    local service=${1:-}
    if [ -n "$service" ]; then
        log_info "Showing logs for $service..."
        docker-compose -f $COMPOSE_FILE logs -f $service
    else
        log_info "Showing logs for all services..."
        docker-compose -f $COMPOSE_FILE logs -f
    fi
}

check_health() {
    log_info "Checking service health..."
    
    # Check if containers are running
    if ! docker-compose -f $COMPOSE_FILE ps | grep -q "Up"; then
        log_error "Services are not running. Start them first with: $0 start"
        return 1
    fi
    
    # Check HTTP health endpoint
    if command -v curl &> /dev/null; then
        if curl -f -s $HEALTH_ENDPOINT > /dev/null; then
            log_success "HTTP health check passed"
        else
            log_error "HTTP health check failed"
            return 1
        fi
    fi
    
    # Check gRPC health (if grpcurl is available)
    if command -v grpcurl &> /dev/null; then
        if grpcurl -plaintext $GRPC_ENDPOINT list > /dev/null 2>&1; then
            log_success "gRPC health check passed"
        else
            log_error "gRPC health check failed"
            return 1
        fi
    fi
    
    # Check database connectivity
    if docker-compose -f $COMPOSE_FILE exec -T postgres-auth pg_isready -U postgres -d erp_auth > /dev/null; then
        log_success "Database health check passed"
    else
        log_error "Database health check failed"
        return 1
    fi
    
    # Check Redis connectivity
    if docker-compose -f $COMPOSE_FILE exec -T redis-auth redis-cli -a redispassword ping | grep -q "PONG"; then
        log_success "Redis health check passed"
    else
        log_error "Redis health check failed"
        return 1
    fi
    
    log_success "All health checks passed"
}

run_unit_tests() {
    log_info "Running unit tests..."
    
    if ! docker-compose -f $COMPOSE_FILE ps auth-service | grep -q "Up"; then
        log_error "Auth service is not running. Start it first with: $0 start"
        return 1
    fi
    
    docker-compose -f $COMPOSE_FILE exec auth-service go test -v -race -cover ./...
    
    if [ $? -eq 0 ]; then
        log_success "Unit tests passed"
    else
        log_error "Unit tests failed"
        return 1
    fi
}

run_integration_tests() {
    log_info "Running integration tests..."
    
    if ! docker-compose -f $COMPOSE_FILE ps auth-service | grep -q "Up"; then
        log_error "Auth service is not running. Start it first with: $0 start"
        return 1
    fi
    
    docker-compose -f $COMPOSE_FILE exec auth-service go test -v -tags=integration ./tests/integration/...
    
    if [ $? -eq 0 ]; then
        log_success "Integration tests passed"
    else
        log_error "Integration tests failed"
        return 1
    fi
}

run_load_tests() {
    log_info "Running load tests..."
    
    if ! docker-compose -f $COMPOSE_FILE ps auth-service | grep -q "Up"; then
        log_error "Auth service is not running. Start it first with: $0 start"
        return 1
    fi
    
    # Start test runner container
    docker-compose -f $COMPOSE_FILE --profile testing up -d test-runner
    
    # Run load tests
    docker-compose -f $COMPOSE_FILE exec test-runner go test -v -tags=load -timeout=10m ./tests/load/...
    
    if [ $? -eq 0 ]; then
        log_success "Load tests passed"
    else
        log_error "Load tests failed"
        return 1
    fi
}

run_all_tests() {
    log_info "Running all tests..."
    
    run_unit_tests
    run_integration_tests
    run_load_tests
    
    log_success "All tests completed"
}

show_service_info() {
    echo
    log_info "Service Information:"
    echo "  Auth Service HTTP API: http://localhost:8080"
    echo "  Auth Service gRPC API: localhost:50051"
    echo "  Health Check: http://localhost:8080/health"
    echo "  Swagger UI: http://localhost:8080/swagger/index.html"
    echo "  Metrics: http://localhost:8080/metrics"
    echo "  Profiling: http://localhost:6060/debug/pprof/"
    echo
    echo "  PostgreSQL: localhost:5433 (user: postgres, password: postgres, db: erp_auth)"
    echo "  Redis: localhost:6380 (password: redispassword)"
    echo "  Kafka: localhost:9093"
    echo
}

show_tools_info() {
    log_info "Management Tools:"
    echo "  pgAdmin: http://localhost:8081 (admin@auth.local / admin)"
    echo "  Redis Commander: http://localhost:8082 (admin / admin)"
    echo "  Kafka UI: http://localhost:8083 (admin / admin)"
    echo
}

show_help() {
    echo "ERP Auth Service - Standalone Testing Script"
    echo
    echo "Usage: $0 [command] [options]"
    echo
    echo "Commands:"
    echo "  start              Start all services"
    echo "  stop               Stop all services"
    echo "  restart            Restart all services"
    echo "  test               Run all tests"
    echo "  test-unit          Run unit tests only"
    echo "  test-integration   Run integration tests only"
    echo "  test-load          Run load tests"
    echo "  logs [service]     Show service logs (optional: specific service)"
    echo "  health             Check service health"
    echo "  clean              Clean up all data (destructive)"
    echo "  tools              Start with management tools"
    echo "  help               Show this help message"
    echo
    echo "Examples:"
    echo "  $0 start           # Start all services"
    echo "  $0 tools           # Start with pgAdmin, Redis Commander, Kafka UI"
    echo "  $0 test            # Run all tests"
    echo "  $0 logs auth-service  # Show auth service logs"
    echo "  $0 health          # Check if all services are healthy"
    echo
}

# Main script logic
case "${1:-help}" in
    start)
        check_dependencies
        start_services
        ;;
    stop)
        stop_services
        ;;
    restart)
        check_dependencies
        restart_services
        ;;
    test)
        run_all_tests
        ;;
    test-unit)
        run_unit_tests
        ;;
    test-integration)
        run_integration_tests
        ;;
    test-load)
        run_load_tests
        ;;
    logs)
        show_logs $2
        ;;
    health)
        check_health
        ;;
    clean)
        clean_data
        ;;
    tools)
        check_dependencies
        start_with_tools
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        log_error "Unknown command: $1"
        echo
        show_help
        exit 1
        ;;
esac