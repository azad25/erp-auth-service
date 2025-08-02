# ERP Auth Service - Standalone Testing Guide

This guide provides comprehensive instructions for testing the ERP Auth Service independently using Docker Compose with isolated dependencies.

## 🎯 Overview

The standalone testing environment provides:
- **Isolated Dependencies**: PostgreSQL, Redis, and Kafka running in separate containers
- **Management Tools**: pgAdmin, Redis Commander, and Kafka UI for debugging
- **Comprehensive Testing**: Unit, integration, and load testing capabilities
- **Development Tools**: Hot reload, profiling, and metrics endpoints
- **Easy Setup**: Single command to start the entire testing environment

## 📋 Prerequisites

- Docker 20.10+ and Docker Compose 2.0+
- Go 1.21+ (for local development)
- curl (for health checks)
- grpcurl (optional, for gRPC testing)

## 🚀 Quick Start

### 1. Start the Testing Environment

```bash
# Start all services
./test-standalone.sh start

# Or start with management tools
./test-standalone.sh tools
```

### 2. Verify Services are Running

```bash
# Check health of all services
./test-standalone.sh health

# View service logs
./test-standalone.sh logs
```

### 3. Run Tests

```bash
# Run all tests
./test-standalone.sh test

# Run specific test types
./test-standalone.sh test-unit
./test-standalone.sh test-integration
./test-standalone.sh test-load
```

### 4. Stop Services

```bash
# Stop all services
./test-standalone.sh stop

# Clean up all data (destructive)
./test-standalone.sh clean
```

## 🔧 Service Configuration

### Port Mappings

| Service | Internal Port | External Port | Purpose |
|---------|---------------|---------------|---------|
| Auth Service HTTP | 8080 | 8080 | REST API |
| Auth Service gRPC | 50051 | 50051 | gRPC API |
| PostgreSQL | 5432 | 5433 | Database |
| Redis | 6379 | 6380 | Cache |
| Kafka | 29092 | 9093 | Event Streaming |
| pgAdmin | 80 | 8081 | DB Management |
| Redis Commander | 8081 | 8082 | Redis Management |
| Kafka UI | 8080 | 8083 | Kafka Management |
| Profiling | 6060 | 6060 | Go pprof |

### Environment Variables

The standalone environment uses the following configuration:

```bash
# Database
DB_HOST=postgres-auth
DB_PORT=5432
DB_NAME=erp_auth
DB_USER=postgres
DB_PASSWORD=postgres

# Redis
REDIS_HOST=redis-auth
REDIS_PORT=6379
REDIS_PASSWORD=redispassword

# Kafka
KAFKA_BROKERS=kafka-auth:29092
KAFKA_TOPIC=auth-events

# JWT
JWT_SECRET=test-jwt-secret-key-for-standalone-testing
JWT_ACCESS_EXPIRY=3600
JWT_REFRESH_EXPIRY=604800
```

## 🧪 Testing Capabilities

### Unit Tests

```bash
# Run unit tests with coverage
./test-standalone.sh test-unit

# Or run directly in container
docker-compose -f docker-compose.standalone.yml exec auth-service go test -v -race -cover ./...
```

### Integration Tests

```bash
# Run integration tests
./test-standalone.sh test-integration

# Run specific integration test
docker-compose -f docker-compose.standalone.yml exec auth-service go test -v -tags=integration ./tests/integration/auth_test.go
```

### Load Tests

```bash
# Run load tests
./test-standalone.sh test-load

# Run with custom parameters
docker-compose -f docker-compose.standalone.yml exec test-runner go test -v -tags=load -timeout=10m ./tests/load/...
```

### Manual Testing

#### HTTP API Testing

```bash
# Health check
curl http://localhost:8080/health

# Register user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!",
    "first_name": "Test",
    "last_name": "User"
  }'

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!"
  }'

# Validate token
curl -X POST http://localhost:8080/api/v1/auth/validate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{
    "token": "YOUR_JWT_TOKEN"
  }'
```

#### gRPC API Testing

```bash
# List available services (requires grpcurl)
grpcurl -plaintext localhost:50051 list

# Test authentication
grpcurl -plaintext -d '{
  "email": "test@example.com",
  "password": "SecurePass123!"
}' localhost:50051 auth.AuthService/Authenticate

# Test token validation
grpcurl -plaintext -d '{
  "token": "YOUR_JWT_TOKEN"
}' localhost:50051 auth.AuthService/ValidateToken
```

## 🛠 Management Tools

### pgAdmin (Database Management)

- **URL**: http://localhost:8081
- **Username**: admin@auth.local
- **Password**: admin
- **Server**: Already configured to connect to postgres-auth

### Redis Commander (Cache Management)

- **URL**: http://localhost:8082
- **Username**: admin
- **Password**: admin
- **Features**: Browse keys, execute commands, monitor performance

### Kafka UI (Event Stream Management)

- **URL**: http://localhost:8083
- **Username**: admin
- **Password**: admin
- **Features**: View topics, messages, consumer groups, cluster health

## 📊 Monitoring and Debugging

### Application Metrics

```bash
# Prometheus metrics
curl http://localhost:8080/metrics

# Health check with details
curl http://localhost:8080/health
```

### Profiling

```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine profile
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

### Log Analysis

```bash
# View all logs
./test-standalone.sh logs

# View specific service logs
./test-standalone.sh logs auth-service
./test-standalone.sh logs postgres-auth
./test-standalone.sh logs redis-auth
./test-standalone.sh logs kafka-auth

# Follow logs in real-time
docker-compose -f docker-compose.standalone.yml logs -f auth-service
```

## 🔍 Troubleshooting

### Common Issues

#### Services Won't Start

```bash
# Check Docker daemon
docker info

# Check port conflicts
netstat -tulpn | grep -E ':(8080|5433|6380|9093)'

# Check container logs
docker-compose -f docker-compose.standalone.yml logs
```

#### Database Connection Issues

```bash
# Test database connectivity
docker-compose -f docker-compose.standalone.yml exec postgres-auth pg_isready -U postgres -d erp_auth

# Check database logs
docker-compose -f docker-compose.standalone.yml logs postgres-auth

# Connect to database directly
docker-compose -f docker-compose.standalone.yml exec postgres-auth psql -U postgres -d erp_auth
```

#### Redis Connection Issues

```bash
# Test Redis connectivity
docker-compose -f docker-compose.standalone.yml exec redis-auth redis-cli -a redispassword ping

# Check Redis logs
docker-compose -f docker-compose.standalone.yml logs redis-auth

# Connect to Redis directly
docker-compose -f docker-compose.standalone.yml exec redis-auth redis-cli -a redispassword
```

#### Kafka Connection Issues

```bash
# Check Kafka health
docker-compose -f docker-compose.standalone.yml exec kafka-auth kafka-broker-api-versions --bootstrap-server localhost:29092

# List topics
docker-compose -f docker-compose.standalone.yml exec kafka-auth kafka-topics --bootstrap-server localhost:29092 --list

# Check Kafka logs
docker-compose -f docker-compose.standalone.yml logs kafka-auth
```

### Performance Issues

#### Memory Usage

```bash
# Check container memory usage
docker stats

# Optimize memory settings in docker-compose.standalone.yml
# Adjust KAFKA_HEAP_OPTS, deploy.resources.limits, etc.
```

#### Slow Startup

```bash
# Check dependency health checks
docker-compose -f docker-compose.standalone.yml ps

# Increase health check intervals if needed
# Modify healthcheck settings in docker-compose.standalone.yml
```

## 🔄 Development Workflow

### 1. Code Changes

```bash
# Start services with hot reload
./test-standalone.sh start

# Make code changes - they will be automatically reloaded
# Check logs to see reload messages
./test-standalone.sh logs auth-service
```

### 2. Testing Changes

```bash
# Run unit tests after changes
./test-standalone.sh test-unit

# Run integration tests
./test-standalone.sh test-integration

# Check health after changes
./test-standalone.sh health
```

### 3. Database Changes

```bash
# Apply migrations
docker-compose -f docker-compose.standalone.yml exec auth-service go run ./cmd/migrate up

# Seed test data
docker-compose -f docker-compose.standalone.yml exec auth-service go run ./cmd/seed

# Reset database if needed
./test-standalone.sh clean
./test-standalone.sh start
```

## 📝 Configuration Customization

### Custom Environment Variables

Create a `.env.standalone` file:

```bash
# Custom JWT secret
JWT_SECRET=my-custom-secret

# Custom database settings
DB_NAME=my_custom_auth_db
DB_USER=my_user
DB_PASSWORD=my_password

# Custom Redis settings
REDIS_PASSWORD=my_redis_password

# Custom Kafka settings
KAFKA_TOPIC=my-auth-events
```

Then use it:

```bash
docker-compose -f docker-compose.standalone.yml --env-file .env.standalone up -d
```

### Resource Limits

Modify the `deploy.resources` sections in `docker-compose.standalone.yml`:

```yaml
deploy:
  resources:
    limits:
      memory: 1G      # Increase memory limit
      cpus: '2.0'     # Increase CPU limit
    reservations:
      memory: 512M    # Increase memory reservation
      cpus: '1.0'     # Increase CPU reservation
```

## 🚀 Advanced Usage

### Load Testing with Custom Parameters

```bash
# Run load test with custom parameters
docker-compose -f docker-compose.standalone.yml exec test-runner go test -v -tags=load \
  -ldflags="-X main.concurrency=100 -X main.duration=5m" \
  ./tests/load/auth_load_test.go
```

### Continuous Integration

```bash
#!/bin/bash
# CI script example

set -e

# Start services
./test-standalone.sh start

# Wait for services to be ready
./test-standalone.sh health

# Run all tests
./test-standalone.sh test

# Generate coverage report
docker-compose -f docker-compose.standalone.yml exec auth-service \
  go test -coverprofile=coverage.out ./...

# Clean up
./test-standalone.sh stop
```

### Multi-Environment Testing

```bash
# Test against different database versions
docker-compose -f docker-compose.standalone.yml \
  -f docker-compose.postgres14.yml up -d

# Test with different Redis configurations
docker-compose -f docker-compose.standalone.yml \
  -f docker-compose.redis-cluster.yml up -d
```

## 📚 Additional Resources

- [Auth Service API Documentation](./API_DOCUMENTATION.md)
- [Development Documentation](./DEV_DOCS.md)
- [Architecture Documentation](./ARCHITECTURE.md)
- [Docker Compose Reference](https://docs.docker.com/compose/)
- [Go Testing Documentation](https://golang.org/pkg/testing/)

## 🤝 Contributing

When contributing to the auth service:

1. Always test changes using the standalone environment
2. Ensure all tests pass before submitting PRs
3. Add new tests for new functionality
4. Update documentation as needed
5. Use the provided testing script for consistency

## 📞 Support

For issues with the standalone testing environment:

1. Check the troubleshooting section above
2. Review service logs using `./test-standalone.sh logs`
3. Verify health status using `./test-standalone.sh health`
4. Check Docker and Docker Compose versions
5. Ensure no port conflicts with other services