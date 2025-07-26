# Auth Service - ERP Suite Authentication & Authorization

This is the authentication and authorization microservice for the ERP Suite, built with Go and providing both HTTP REST API and gRPC interfaces.

## 🏗️ Architecture Overview

The Auth Service is part of the unified ERP Suite infrastructure and provides:
- User authentication (login/logout)
- JWT token management (access & refresh tokens)
- Role-based access control (RBAC)
- Multi-tenant user management
- gRPC API for inter-service communication
- HTTP REST API for frontend integration

## 🚀 Quick Start (Recommended)

### Using Unified Infrastructure

The **recommended** way to develop this service is using the unified infrastructure approach:

```bash
# Start the complete ERP development environment
cd ../infrastructure
make dev-full-stack

# This starts:
# ✅ All infrastructure services (PostgreSQL, Redis, Kafka, etc.)
# ✅ Auth Service (this service) - ports 8080/50051
# ✅ Django Core Gateway - port 8000  
# ✅ Next.js Frontend - port 3000
# ✅ All monitoring and development tools
```

**Service Endpoints:**
- HTTP API: http://localhost:8080
- gRPC API: localhost:50051
- Health Check: http://localhost:8080/health

## 🔧 Local Development

### Prerequisites

- Go 1.21+
- Infrastructure services running (see above)

### Running Locally

```bash
# 1. Start infrastructure services first
cd ../infrastructure
make dev-up

# 2. Return to auth-module and run locally
cd ../auth-module
make dev

# Or build and run
make build
./bin/auth-service
```

### Available Commands

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

## 📊 Service Configuration

### Environment Variables

The service uses the following configuration (automatically provided by infrastructure):

```bash
# Database (PostgreSQL)
DB_HOST=postgres
DB_PORT=5432
DB_NAME=erp_auth
DB_USER=postgres
DB_PASSWORD=postgres

# Cache (Redis)
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=redispassword

# Messaging (Kafka)
KAFKA_BROKERS=kafka:29092
KAFKA_TOPIC=auth-events

# Server
HTTP_PORT=8080
GRPC_PORT=50051
```

### Configuration Generation

Generate environment-specific configuration:

```bash
# Generate development config
make config

# Or from infrastructure directory
cd ../infrastructure
make generate-config MODULE=auth ENV=development
```

## 🔌 API Endpoints

### HTTP REST API (Port 8080)

```bash
# Authentication
POST   /api/v1/auth/login
POST   /api/v1/auth/logout
POST   /api/v1/auth/refresh
GET    /api/v1/auth/me

# User Management
GET    /api/v1/users
POST   /api/v1/users
GET    /api/v1/users/:id
PUT    /api/v1/users/:id
DELETE /api/v1/users/:id

# Role Management
GET    /api/v1/roles
POST   /api/v1/roles
GET    /api/v1/roles/:id
PUT    /api/v1/roles/:id
DELETE /api/v1/roles/:id

# Health Check
GET    /health
```

### gRPC API (Port 50051)

```protobuf
service AuthService {
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
  rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

## 🗄️ Database Schema

The service uses PostgreSQL with the following main tables:

- `users` - User accounts and profiles
- `roles` - Role definitions and permissions
- `user_roles` - User-role assignments
- `organizations` - Multi-tenant organization data
- `sessions` - Active user sessions

### Running Migrations

```bash
# Make sure infrastructure is running
make infra-up

# Run migrations
make migrate
```

## 🔄 Integration with Other Services

### Django Core Gateway

The Django Core Gateway communicates with this service via gRPC for:
- Token validation
- User authentication
- Permission checks

### Frontend Integration

The Next.js frontend communicates via HTTP REST API through the Django Core Gateway.

### Event Publishing

The service publishes events to Kafka for:
- User login/logout events
- User creation/updates
- Role changes
- Security events

## 🧪 Testing

```bash
# Run all tests
make test

# Run specific test package
go test -v ./internal/handlers/...

# Run tests with coverage
go test -v -cover ./...

# Generate test mocks
make mocks
```

## 🐳 Docker Development

### Using Full Stack (Recommended)

```bash
cd ../infrastructure
make dev-full-stack
```

### Standalone Mode (Not Recommended)

```bash
# Only if you need to test auth service in isolation
make standalone
```

## 📋 Health Checks

```bash
# Check service health
make health

# Manual health checks
curl http://localhost:8080/health
grpcurl -plaintext localhost:50051 auth.AuthService/HealthCheck
```

## 🔒 Security Features

- JWT token-based authentication
- Refresh token rotation
- Password hashing with bcrypt
- Rate limiting on authentication endpoints
- CORS protection
- SQL injection prevention
- Input validation and sanitization

## 📚 Development Workflow

1. **Start Infrastructure**: `cd ../infrastructure && make dev-full-stack`
2. **Make Changes**: Edit code in the auth-module
3. **Test Changes**: Service auto-reloads in development mode
4. **Run Tests**: `make test`
5. **Check Health**: `make health`

## 🚨 Troubleshooting

### Service Won't Start

```bash
# Check if infrastructure is running
cd ../infrastructure
make status

# Check logs
make infra-logs

# Restart infrastructure
make dev-down && make dev-full-stack
```

### Database Connection Issues

```bash
# Check PostgreSQL status
cd ../infrastructure
make logs-postgres

# Run migrations
cd ../auth-module
make migrate
```

### gRPC Connection Issues

```bash
# Check if gRPC port is correct (50051)
make health

# Check service logs
cd ../infrastructure
make logs | grep auth-service
```

## 📖 Additional Resources

- [ERP Suite Infrastructure Documentation](../infrastructure/README.md)
- [API Documentation](./docs/api.md)
- [gRPC Protocol Documentation](./proto/auth.proto)
- [Database Schema](./docs/schema.md)

## 🤝 Contributing

1. Follow the unified infrastructure approach
2. Use `make fmt` and `make lint` before committing
3. Add tests for new features
4. Update documentation as needed
5. Test with full stack: `make infra-dev`