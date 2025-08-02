# ERP Auth Service - Developer Documentation

## Table of Contents
1. [Architecture Overview](#architecture-overview)
2. [Service Components](#service-components)
3. [Development Commands](#development-commands)
4. [Testing Strategy](#testing-strategy)
5. [Task Completion Summary](#task-completion-summary)
6. [API Documentation](#api-documentation)
7. [Request Flow Diagrams](#request-flow-diagrams)
8. [OpenAPI 3.0 Specifications](#openapi-30-specifications)
9. [Developer Codebase Guide](#developer-codebase-guide)
10. [Security Features](#security-features)
11. [Performance Optimizations](#performance-optimizations)

## Architecture Overview

The ERP Auth Service is built using Clean Architecture principles with a layered approach:

```
┌─────────────────────────────────────────────────────────────┐
│                    Presentation Layer                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ HTTP/REST   │  │ gRPC Server │  │ GraphQL Resolver    │ │
│  │ Handlers    │  │             │  │                     │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                   Application Layer                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ Auth        │  │ Token       │  │ Permission          │ │
│  │ Service     │  │ Service     │  │ Service             │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Domain Layer                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ Entities    │  │ Value       │  │ Domain              │ │
│  │             │  │ Objects     │  │ Services            │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                 Infrastructure Layer                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ Database    │  │ Cache       │  │ External            │ │
│  │ Repositories│  │ (Redis)     │  │ Services            │ │
│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### Project Structure

```
erp-auth-service/
├── cmd/                           # Application entry points
│   └── seed/                      # Database seeding utility
├── internal/                      # Private application code
│   ├── application/               # Application layer (use cases)
│   │   ├── dto/                   # Data Transfer Objects
│   │   ├── interfaces/            # Application interfaces
│   │   └── services/              # Application services (use cases)
│   ├── domain/                    # Domain layer (business logic)
│   │   ├── entities/              # Domain entities
│   │   └── interfaces/            # Domain interfaces
│   ├── infrastructure/            # Infrastructure layer
│   │   ├── interfaces/            # Infrastructure interfaces
│   │   └── repositories/          # Data access implementations
│   ├── cache/                     # Caching infrastructure
│   ├── config/                    # Configuration management
│   ├── database/                  # Database connection and migration
│   ├── errors/                    # Error handling system
│   ├── events/                    # Event publishing
│   ├── grpc/                      # gRPC server implementation
│   ├── handlers/                  # HTTP handlers (controllers)
│   ├── middleware/                # HTTP middleware
│   ├── models/                    # Database models (GORM)
│   ├── redis/                     # Redis client
│   └── utils/                     # Utility functions
├── proto/                         # Protocol buffer definitions
└── main.go                        # Application entry point
```##
 Service Components

### Core Services

#### 1. Authentication Service (`AuthService`)
- **Purpose**: Handles user authentication, registration, and password management
- **Location**: `internal/application/services/auth_service.go`
- **Key Features**:
  - Multi-factor authentication support
  - Rate limiting and brute force protection
  - Password strength validation
  - Account lockout mechanisms
  - Security context tracking
  - Worker pool optimization for CPU-intensive operations

#### 2. Token Service (`TokenService`)
- **Purpose**: JWT token generation, validation, and lifecycle management
- **Location**: `internal/application/services/token_service.go`
- **Key Features**:
  - Access and refresh token generation
  - Token rotation and revocation
  - Blacklist management with Redis
  - Key rotation support
  - Token introspection
  - Sub-10ms validation performance

#### 3. Permission Service (`PermissionService`)
- **Purpose**: Role-based access control (RBAC) and permission evaluation
- **Location**: `internal/application/services/permission_service.go`
- **Key Features**:
  - Hierarchical role management
  - Bulk permission evaluation
  - Permission caching with intelligent invalidation
  - Dynamic permission assignment
  - Scope-based permissions
  - Sub-15ms permission checking

### Infrastructure Components

#### 1. Cache Management
- **Location**: `internal/cache/`
- **Components**:
  - **BigCache**: In-memory L1 cache for ultra-fast access
  - **Redis Cache**: Distributed L2 cache with clustering support
  - **Cache Manager**: Unified cache interface with multiple strategies
  - **Cache Warmer**: Proactive cache population for frequently accessed data
  - **Cache Invalidator**: Pattern-based and event-driven cache invalidation

#### 2. Database Layer
- **Location**: `internal/database/` and `internal/infrastructure/repositories/`
- **Features**:
  - **PostgreSQL**: Primary data store with ACID compliance
  - **Connection Pooling**: Optimized database connection management
  - **Migration System**: Version-controlled database schema management
  - **Repository Pattern**: Clean data access abstraction
  - **Query Optimization**: Indexed queries and performance monitoring

#### 3. Event System
- **Location**: `internal/events/`
- **Components**:
  - **Event Publisher**: Asynchronous event notification system
  - **Persistent Queue**: Local event storage for Kafka unavailability
  - **Enhanced Producer**: High-throughput event publishing with batching
  - **Event Types**: User lifecycle, authentication, and security events

## Development Commands

### Build Commands

```bash
# Build the service
go build -o bin/auth-service ./main.go

# Build with race detection
go build -race -o bin/auth-service ./main.go

# Build for production (optimized)
go build -ldflags="-w -s" -o bin/auth-service ./main.go

# Cross-platform builds
GOOS=linux GOARCH=amd64 go build -o bin/auth-service-linux ./main.go
GOOS=windows GOARCH=amd64 go build -o bin/auth-service.exe ./main.go
```

### Development Commands

```bash
# Run in development mode
go run ./main.go

# Run with hot reload (using air)
air

# Run with specific config
go run ./main.go -config=./configs/dev.yaml

# Run with environment variables
ENV=development go run ./main.go

# Generate code (mocks, protobuf, etc.)
go generate ./...

# Format code
go fmt ./...

# Lint code
golangci-lint run

# Security scan
gosec ./...
```

### Database Commands

```bash
# Run migrations
go run ./cmd/migrate up

# Rollback migrations
go run ./cmd/migrate down

# Create new migration
go run ./cmd/migrate create -ext sql -dir migrations -seq <migration_name>

# Check migration status
go run ./cmd/migrate version

# Force migration version
go run ./cmd/migrate force <version>

# Seed database
go run ./cmd/seed/main.go

# Reset database
go run ./cmd/migrate drop && go run ./cmd/migrate up
```

### Docker Commands

```bash
# Build Docker image
docker build -t erp-auth-service .

# Run with Docker Compose (from infrastructure directory)
cd ../erp-suite-infrastructure
make dev-full-stack

# Run specific services
docker-compose up -d postgres redis

# View logs
docker-compose logs -f auth-service

# Scale service
docker-compose up -d --scale auth-service=3

# Clean up
docker-compose down -v
```## 
Testing Strategy

### Unit Tests

```bash
# Run all unit tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Run tests with race detection
go test -race ./...

# Run specific test package
go test ./internal/application/services/...

# Run specific test
go test -run TestAuthService_Authenticate ./internal/application/services/

# Run tests with verbose output
go test -v ./...

# Run tests with timeout
go test -timeout 30s ./...

# Run comprehensive test suite
./run_comprehensive_tests.sh
```

### Integration Tests

```bash
# Run integration tests
go test -tags=integration ./...

# Run integration tests with test database
TEST_DB_URL=postgres://test:test@localhost/auth_test go test -tags=integration ./...

# Run API integration tests
go test -tags=api ./tests/integration/

# Run database integration tests
go test -tags=database ./tests/integration/

# Run cache integration tests
go test -tags=cache ./tests/integration/

# Run gRPC integration tests
go test ./internal/grpc/server_integration_test.go
```

### End-to-End Tests

```bash
# Run E2E tests
go test -tags=e2e ./tests/e2e/

# Run E2E tests with specific environment
E2E_BASE_URL=http://localhost:8080 go test -tags=e2e ./tests/e2e/

# Run E2E tests with cleanup
go test -tags=e2e -cleanup ./tests/e2e/
```

### Benchmark Tests

```bash
# Run benchmark tests
go test -bench=. ./...

# Run specific benchmarks
go test -bench=BenchmarkAuthentication ./internal/application/services/

# Run benchmarks with memory profiling
go test -bench=. -benchmem ./...

# Run benchmarks with CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./...

# Run benchmarks with custom duration
go test -bench=. -benchtime=10s ./...

# Run token service benchmarks
go test -bench=. ./internal/application/services/token_service_benchmark_test.go
```

### Coverage Commands

```bash
# Generate coverage for all packages
go test -coverprofile=coverage.out ./...

# View coverage summary
go tool cover -func=coverage.out

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Coverage by package
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep -E "(total|internal/)"

# Detailed coverage analysis
go test -covermode=count -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Coverage with exclusions
go test -coverprofile=coverage.out $(go list ./... | grep -v /vendor/ | grep -v /mocks/)
```