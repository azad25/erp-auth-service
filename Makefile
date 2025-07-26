# Auth Service Makefile
# 
# ⚠️  IMPORTANT: This module now uses the unified infrastructure approach.
# For full development environment, use: cd ../infrastructure && make dev-full-stack

.PHONY: help dev build test clean proto infra-dev infra-up infra-down infra-logs migrate standalone

# Default target
help:
	@echo "Auth Service Commands:"
	@echo ""
	@echo "🚀 Recommended Development (Unified Infrastructure):"
	@echo "  infra-dev    - Start full ERP development stack (infrastructure + all services)"
	@echo "  infra-up     - Start only infrastructure services"
	@echo "  infra-down   - Stop infrastructure services"
	@echo "  infra-logs   - Show infrastructure logs"
	@echo ""
	@echo "🔧 Local Development:"
	@echo "  dev          - Run auth service locally (requires infrastructure running)"
	@echo "  build        - Build the application"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo ""
	@echo "🐳 Standalone Development (Not Recommended):"
	@echo "  standalone      - Run auth service only (connects to external infrastructure)"
	@echo "  standalone-down - Stop standalone auth service"
	@echo ""
	@echo "🛠️  Utilities:"
	@echo "  proto        - Generate protobuf files"
	@echo "  migrate      - Run database migrations"
	@echo "  deps         - Install dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"

# ============================================================================
# UNIFIED INFRASTRUCTURE COMMANDS (Recommended)
# ============================================================================

# Start full ERP development stack (infrastructure + all services)
infra-dev:
	@echo "🚀 Starting full ERP development stack..."
	@echo "This includes: Infrastructure + Auth Service + Django Core + Frontend"
	cd ../infrastructure && make dev-full-stack

# Start only infrastructure services
infra-up:
	@echo "🏗️ Starting infrastructure services..."
	cd ../infrastructure && make dev-up

# Stop infrastructure services
infra-down:
	@echo "🛑 Stopping infrastructure services..."
	cd ../infrastructure && make dev-down

# Show infrastructure logs
infra-logs:
	@echo "📋 Showing infrastructure logs..."
	cd ../infrastructure && make logs

# ============================================================================
# LOCAL DEVELOPMENT COMMANDS
# ============================================================================

# Run auth service locally (requires infrastructure running)
dev:
	@echo "🔧 Starting auth service locally..."
	@echo "⚠️  Make sure infrastructure is running: make infra-up"
	go run .

# Build the application
build:
	@echo "🔨 Building auth service..."
	go build -o bin/auth-service .

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	rm -rf tmp/

# ============================================================================
# STANDALONE DEVELOPMENT (Not Recommended)
# ============================================================================

# Run auth service only (connects to external infrastructure)
standalone:
	@echo "🐳 Starting auth service in standalone mode..."
	@echo "⚠️  This connects to external infrastructure services"
	@echo "⚠️  Make sure infrastructure is running: make infra-up"
	docker-compose -f docker-compose.standalone.yml up --build

# Stop standalone service
standalone-down:
	@echo "🛑 Stopping standalone auth service..."
	docker-compose -f docker-compose.standalone.yml down

# ============================================================================
# UTILITIES
# ============================================================================

# Generate protobuf files
proto:
	@echo "🔧 Generating protobuf files..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/auth.proto

# Database migration (run after infrastructure is up)
migrate:
	@echo "🗄️ Running database migrations..."
	@echo "⚠️  Make sure infrastructure is running: make infra-up"
	go run . migrate

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	go mod download
	go mod tidy

# Format code
fmt:
	@echo "✨ Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "🔍 Linting code..."
	golangci-lint run

# Security scan
security:
	@echo "🔒 Running security scan..."
	gosec ./...

# Generate mocks for testing
mocks:
	@echo "🎭 Generating mocks..."
	mockgen -source=internal/handlers/auth.go -destination=mocks/auth_handler_mock.go

# Health check
health:
	@echo "🏥 Checking service health..."
	@curl -f http://localhost:8080/health || echo "❌ HTTP service not responding"
	@grpcurl -plaintext localhost:50051 auth.AuthService/HealthCheck || echo "❌ gRPC service not responding"

# Generate configuration for auth module
config:
	@echo "⚙️ Generating configuration for auth module..."
	cd ../infrastructure && make generate-config MODULE=auth ENV=development