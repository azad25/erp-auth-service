# ERP Auth Service - Technical Documentation

## Table of Contents
1. [Architecture Overview](#architecture-overview)
2. [Service Components](#service-components)
3. [Development Commands](#development-commands)
4. [Testing Strategy](#testing-strategy)
5. [Task Completion Summary](#task-completion-summary)
6. [API Documentation](#api-documentation)
7. [Security Features](#security-features)
8. [Performance Optimizations](#performance-optimizations)
9. [Deployment & Infrastructure](#deployment--infrastructure)

## Architecture Overview

The ERP Auth Service is built using Clean Architecture principles with a layered approach, providing authentication and authorization services for the entire ERP Suite.

### Clean Architecture Layers

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
```

### Key Architectural Principles

1. **Clean Architecture**: Clear separation of concerns with dependency inversion
2. **Domain-Driven Design**: Rich domain models with business logic encapsulation
3. **CQRS Pattern**: Command and Query separation for better scalability
4. **Event-Driven Architecture**: Asynchronous event publishing for loose coupling
5. **Microservice Architecture**: Independent, deployable service units

## Service Components

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

### Security Components

#### 1. Error Handling System
- **Location**: `internal/errors/`
- **Components**:
  - **Structured Errors**: Following Google's error model
  - **Circuit Breaker**: Service isolation and failure protection
  - **Retry Mechanisms**: Exponential backoff with jitter
  - **Recovery Strategies**: Graceful degradation and fallback mechanisms
  - **Error Logging**: Comprehensive audit trails and alerting

#### 2. Rate Limiting & Security
- **Features**:
  - **Multi-level Protection**: IP, user, and endpoint-based rate limiting
  - **Sliding Window**: Advanced rate limiting algorithms
  - **Dynamic Thresholds**: Adaptive rate limiting based on system load
  - **Bypass Mechanisms**: Whitelist support for trusted sources

#### 3. Encryption & Hashing
- **Features**:
  - **Password Hashing**: bcrypt with configurable cost factors
  - **Token Encryption**: AES-256 encryption for sensitive tokens
  - **Data Encryption**: Field-level encryption for PII data
  - **Key Management**: Secure key rotation and storage

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
```

### Makefile Commands

```bash
# Unified Infrastructure (Recommended)
make infra-dev      # Start full ERP stack
make infra-up       # Start infrastructure only
make infra-down     # Stop infrastructure
make infra-logs     # View infrastructure logs

# Local Development
make dev            # Run service locally
make build          # Build binary
make test           # Run tests
make clean          # Clean artifacts

# Utilities
make proto          # Generate protobuf files
make migrate        # Run database migrations
make config         # Generate configuration
make health         # Check service health
make deps           # Install dependencies
make fmt            # Format code
make lint           # Lint code
```

## Testing Strategy

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

### Test Data Management

```bash
# Generate test data
go run ./scripts/generate-test-data.go

# Clean test data
go run ./scripts/clean-test-data.go

# Load test fixtures
go run ./scripts/load-fixtures.go

# Validate test data
go run ./scripts/validate-test-data.go
```

## Task Completion Summary

### ✅ Task 1: Set up enhanced project structure and core interfaces
**Status**: Completed
**Summary**: Successfully restructured the authentication service using Clean Architecture principles with clear separation of concerns. Implemented layered architecture with domain, application, infrastructure, and presentation layers.

**Key Achievements**:
- Clean Architecture implementation with dependency inversion
- Domain-driven design with rich domain models
- Proper separation of business logic from infrastructure concerns
- Modular service structure for better maintainability
- Configuration management with environment-specific settings
- Structured logging with zap logger and correlation IDs

### ✅ Task 2: Implement high-performance database layer with connection pooling
**Status**: Completed
**Summary**: Built optimized database layer with connection pooling, repository pattern, and migration system.

**Key Achievements**:
- Optimized database models with proper indexing strategies
- Repository pattern with connection pooling and read replicas
- Database migration system with version control
- Materialized views for complex permission queries
- Comprehensive unit tests for repository layer

### ✅ Task 3: Build Redis-based caching infrastructure
**Status**: Completed
**Summary**: Implemented comprehensive caching system with multi-level caching, intelligent invalidation, and distributed synchronization.

**Key Achievements**:
- Multi-level caching with BigCache (L1) and Redis cluster (L2)
- Cache manager with write-through and write-back strategies
- Cache warming and intelligent invalidation mechanisms
- Distributed cache synchronization across service instances
- Unit tests for caching layer with mock Redis

### ✅ Task 4: Create enhanced JWT token service with security features
**Status**: Completed
**Summary**: Built comprehensive JWT token management system with advanced features including token rotation, blacklisting, and multi-key support.

**Key Achievements**:
- JWT token generation with key rotation support
- Token validation with Redis-based blacklisting
- Token refresh mechanism with automatic cleanup
- Memory pooling for high-throughput token operations
- Comprehensive token security tests including edge cases
- Sub-10ms token validation performance

### ✅ Task 5: Build user authentication service with concurrency optimization
**Status**: Completed
**Summary**: Implemented comprehensive authentication system with security features and performance optimizations.

**Key Achievements**:
- User authentication with bcrypt password hashing
- Rate limiting and brute force protection mechanisms
- User registration with organization creation workflow
- Password change functionality with token revocation
- Two-factor authentication support with backup codes
- Unit tests for authentication flows and security measures

### ✅ Task 6: Implement permission service with hierarchical RBAC
**Status**: Completed
**Summary**: Built sophisticated RBAC system with hierarchical roles, dynamic permissions, and scope-based access control.

**Key Achievements**:
- Permission evaluation engine with caching optimization
- Role-based access control with inheritance support
- Bulk permission checking for performance optimization
- Permission cache warming based on usage patterns
- Comprehensive tests for permission evaluation scenarios
- Sub-15ms permission checking performance

### ✅ Task 7: Build Kafka event publishing system
**Status**: Completed
**Summary**: Implemented comprehensive event-driven system with reliable event publishing, persistence, and processing capabilities.

**Key Achievements**:
- Asynchronous event publisher with connection pooling
- Event schemas for authentication and user lifecycle events
- Event queuing with local persistence for Kafka unavailability
- Event publishing with retry mechanisms and dead letter queues
- Integration tests for event publishing scenarios

### ✅ Task 8: Create enhanced gRPC server with interceptor chain
**Status**: Completed
**Summary**: Built high-performance gRPC server with comprehensive interceptor chain and advanced features.

**Key Achievements**:
- gRPC server with connection pooling and load balancing
- Comprehensive interceptor chain for security, logging, and metrics
- Rate limiting interceptor with Redis-based counters
- Circuit breaker pattern for external dependencies
- Graceful shutdown handling with connection draining

### ✅ Task 9: Implement core gRPC service methods with performance optimization
**Status**: Completed
**Summary**: Created core gRPC service methods with performance optimization and intelligent caching strategies.

**Key Achievements**:
- ValidateToken method with sub-10ms response time optimization
- Authenticate method with concurrent user lookup and validation
- CheckPermission method with intelligent caching strategies
- GetUser method with preloaded relationships and caching
- RefreshToken and RevokeToken methods with atomic operations
- Performance monitoring and metrics collection

### ✅ Task 10: Add advanced gRPC methods for user and organization management
**Status**: Completed
**Summary**: Implemented advanced gRPC methods for user and organization management with transaction management and bulk operations.

**Key Achievements**:
- CreateUser method with transaction management
- UpdateUser method with cache invalidation
- ChangePassword method with security event publishing
- CreateOrganization method with initial role setup
- Bulk operations for high-throughput scenarios (BulkCreateUsers, BulkUpdateUsers, BulkCheckPermissions)
- Comprehensive input validation and error handling

### ✅ Task 12: Create worker pool system for CPU-intensive operations
**Status**: Completed
**Summary**: Implemented worker pool pattern for CPU-intensive operations with background job processing.

**Key Achievements**:
- Worker pool pattern for password hashing operations
- Background job processing for cache warming and cleanup
- Async processing for non-critical operations like audit logging
- Graceful worker shutdown with job completion
- Tests for worker pool behavior under various load conditions

### ✅ Task 13: Implement comprehensive error handling and recovery
**Status**: Completed
**Summary**: Built comprehensive error handling and recovery system following Google's error model with circuit breaker patterns and retry mechanisms.

**Key Achievements**:
- Structured error system following Google's error model
- Circuit breaker implementation for external dependencies
- Retry mechanisms with exponential backoff and jitter
- Error recovery strategies for partial system failures
- Comprehensive error logging and alerting
- Integration with gRPC server and interceptors

### 🔄 Task 11: Build comprehensive monitoring and observability system
**Status**: In Progress
**Summary**: Implementing enterprise-grade monitoring and observability system with metrics collection, alerting, and performance monitoring.

**Planned Achievements**:
- Prometheus metrics collection for all service operations
- Distributed tracing with Jaeger for request flow visibility
- Health check endpoints with dependency status reporting
- Structured logging with correlation IDs and performance metrics
- Custom metrics for business logic monitoring

### 🔄 Task 14: Build extensive unit test suite with high coverage
**Status**: In Progress
**Summary**: Creating comprehensive unit test suite with high coverage, property-based testing, and performance benchmarks.

**Planned Achievements**:
- Unit tests for all service methods with table-driven tests
- Mock objects for external dependencies
- Property-based testing for security-critical functions
- Benchmark tests for performance-critical code paths
- Minimum 85% code coverage with meaningful tests

## API Documentation

### gRPC API (Port 50051)

#### Core Authentication Methods

##### ValidateToken
```protobuf
rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
```
- **Purpose**: Validate JWT token and return user context
- **Performance**: Sub-10ms response time with caching
- **Features**: Token blacklist checking, user context extraction

##### Authenticate
```protobuf
rpc Authenticate(AuthenticateRequest) returns (AuthenticateResponse);
```
- **Purpose**: User authentication with email/password
- **Features**: 2FA support, rate limiting, security context tracking

##### RefreshToken
```protobuf
rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
```
- **Purpose**: Refresh access token using refresh token
- **Features**: Atomic token rotation, old token revocation

##### RevokeToken
```protobuf
rpc RevokeToken(RevokeTokenRequest) returns (RevokeTokenResponse);
```
- **Purpose**: Revoke access or refresh token
- **Features**: Blacklist management, cache invalidation

#### Permission Management

##### CheckPermission
```protobuf
rpc CheckPermission(CheckPermissionRequest) returns (CheckPermissionResponse);
```
- **Purpose**: Check user permission for resource/action
- **Performance**: Sub-15ms response time with intelligent caching
- **Features**: Hierarchical RBAC, scope-based permissions

##### BulkCheckPermissions
```protobuf
rpc BulkCheckPermissions(BulkCheckPermissionsRequest) returns (BulkCheckPermissionsResponse);
```
- **Purpose**: Check multiple permissions concurrently
- **Features**: Concurrent processing, individual result tracking

#### User Management

##### GetUser
```protobuf
rpc GetUser(GetUserRequest) returns (GetUserResponse);
```
- **Purpose**: Get user details with relationships
- **Features**: Preloaded relationships, multi-level caching

##### CreateUser
```protobuf
rpc CreateUser(CreateUserRequest) returns (CreateUserResponse);
```
- **Purpose**: Create new user account
- **Features**: Transaction management, role assignment, event publishing

##### UpdateUser
```protobuf
rpc UpdateUser(UpdateUserRequest) returns (UpdateUserResponse);
```
- **Purpose**: Update user profile and roles
- **Features**: Selective updates, cache invalidation, event publishing

##### ChangePassword
```protobuf
rpc ChangePassword(ChangePasswordRequest) returns (ChangePasswordResponse);
```
- **Purpose**: Change user password securely
- **Features**: Current password verification, token revocation, security events

##### BulkCreateUsers
```protobuf
rpc BulkCreateUsers(BulkCreateUsersRequest) returns (BulkCreateUsersResponse);
```
- **Purpose**: Create multiple users in batches
- **Features**: Batch processing, error isolation, performance optimization

#### Organization Management

##### CreateOrganization
```protobuf
rpc CreateOrganization(CreateOrganizationRequest) returns (CreateOrganizationResponse);
```
- **Purpose**: Create new organization with admin user
- **Features**: Transaction management, initial role setup, token generation

### HTTP REST API (Port 8080)

#### Authentication Endpoints

##### POST /api/v1/auth/login
Authenticate user with email and password.

**Request Body**:
```json
{
  "email": "user@example.com",
  "password": "securePassword123",
  "mfa_code": "123456",
  "remember_me": true
}
```

**Response**:
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_at": "2024-01-01T12:00:00Z",
    "token_type": "Bearer",
    "user": {
      "id": "uuid",
      "email": "user@example.com",
      "first_name": "John",
      "last_name": "Doe"
    }
  }
}
```

##### POST /api/v1/auth/register
Register new user account.

##### POST /api/v1/auth/refresh
Refresh access token using refresh token.

##### POST /api/v1/auth/logout
Logout user and invalidate tokens.

##### POST /api/v1/auth/forgot-password
Initiate password reset process.

##### POST /api/v1/auth/reset-password
Reset password using reset token.

#### Health Check

##### GET /health
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z",
  "version": "1.0.0",
  "dependencies": {
    "database": "healthy",
    "redis": "healthy",
    "kafka": "healthy"
  }
}
```

## Security Features

### Authentication Security
- **Password Hashing**: bcrypt with configurable cost factors
- **Account Lockout**: Automatic lockout after failed attempts
- **Session Management**: Secure session handling with timeout
- **MFA Support**: Multiple authentication factors for enhanced security
- **Rate Limiting**: Protection against brute force attacks

### Token Security
- **JWT Signing**: RSA and HMAC signing algorithms
- **Token Rotation**: Automatic token refresh and rotation
- **Token Blacklisting**: Immediate token revocation capability
- **Short-lived Tokens**: Configurable token expiration times
- **Key Rotation**: Support for zero-downtime key rotation

### API Security
- **Rate Limiting**: Comprehensive rate limiting at multiple levels
- **Input Validation**: Strict input validation and sanitization
- **CORS Protection**: Configurable CORS policies
- **Security Headers**: Comprehensive security header implementation
- **Circuit Breakers**: Protection against cascade failures

### Data Protection
- **Encryption at Rest**: Database field-level encryption
- **Encryption in Transit**: TLS 1.3 for all communications
- **PII Protection**: Special handling for personally identifiable information
- **Audit Logging**: Complete audit trail of all security events

## Performance Optimizations

### Caching Strategy
- **Multi-level Caching**: L1 (BigCache) + L2 (Redis) caching
- **Cache Warming**: Proactive cache population for frequently accessed data
- **Intelligent Invalidation**: Event-driven cache invalidation
- **Cache Monitoring**: Performance metrics and optimization

### Database Optimization
- **Connection Pooling**: Optimized database connection management
- **Query Optimization**: Indexed queries and query analysis
- **Read Replicas**: Read/write splitting for better performance
- **Database Monitoring**: Query performance tracking

### Application Performance
- **Worker Pools**: Concurrent processing with worker pools
- **Async Processing**: Non-blocking operations where possible
- **Memory Management**: Efficient memory usage and garbage collection
- **Performance Profiling**: Continuous performance monitoring

### Scalability Features
- **Horizontal Scaling**: Stateless design for easy scaling
- **Load Balancing**: Support for multiple load balancing strategies
- **Circuit Breakers**: Fault tolerance and resilience patterns
- **Auto-scaling**: Kubernetes-based automatic scaling

## Deployment & Infrastructure

### Docker Configuration
- **Multi-stage Build**: Optimized Docker images
- **Health Checks**: Container health monitoring
- **Resource Limits**: CPU and memory constraints
- **Security**: Non-root user execution

### Kubernetes Deployment
- **Deployment Manifests**: Production-ready Kubernetes configurations
- **ConfigMaps**: Environment-specific configuration
- **Secrets**: Secure credential management
- **Service Discovery**: Kubernetes service integration
- **Horizontal Pod Autoscaling**: Automatic scaling based on metrics

### Monitoring & Observability
- **Prometheus Metrics**: Comprehensive metrics collection
- **Grafana Dashboards**: Visual monitoring and alerting
- **Distributed Tracing**: Request flow visibility with Jaeger
- **Structured Logging**: JSON-formatted logs with correlation IDs
- **Health Checks**: Multi-level health monitoring

### Configuration Management
- **Environment Variables**: 12-factor app configuration
- **Configuration Files**: YAML-based configuration
- **Hot Reloading**: Dynamic configuration updates
- **Validation**: Configuration schema validation
- **Secrets Management**: Secure configuration storage

---

## Getting Started

### Prerequisites
- Go 1.23 or higher
- PostgreSQL 13 or higher
- Redis 6 or higher
- Docker and Docker Compose (for containerized development)

### Quick Start (Recommended)

```bash
# Start the complete ERP development environment
cd ../erp-suite-infrastructure
make dev-full-stack

# This starts:
# ✅ All infrastructure services (PostgreSQL, Redis, Kafka, etc.)
# ✅ Auth Service (this service) - ports 8080/50051
# ✅ Django Core Gateway - port 8000  
# ✅ Next.js Frontend - port 3000
# ✅ All monitoring and development tools
```

### Local Development

```bash
# 1. Start infrastructure services first
cd ../erp-suite-infrastructure
make dev-up

# 2. Return to auth service and run locally
cd ../erp-auth-service
make dev

# Or build and run
make build
./bin/auth-service
```

### Development Workflow
1. **Start Infrastructure**: `cd ../erp-suite-infrastructure && make dev-full-stack`
2. **Make Changes**: Edit code in the auth service
3. **Test Changes**: Service auto-reloads in development mode
4. **Run Tests**: `make test`
5. **Check Health**: `make health`

For more detailed information, refer to the individual component documentation and the main README.md file.