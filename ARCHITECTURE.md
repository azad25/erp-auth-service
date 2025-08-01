# Auth Service Architecture

This document describes the enhanced clean architecture structure of the ERP Auth Service.

## Project Structure

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
│   ├── config/                    # Configuration management
│   ├── database/                  # Database connection and migration
│   ├── events/                    # Event publishing
│   ├── grpc/                      # gRPC server implementation
│   ├── handlers/                  # HTTP handlers (controllers)
│   ├── middleware/                # HTTP middleware
│   ├── models/                    # Database models (GORM)
│   ├── redis/                     # Redis client
│   ├── router/                    # HTTP routing
│   ├── seeder/                    # Database seeding
│   └── utils/                     # Utility functions
├── proto/                         # Protocol buffer definitions
└── main.go                        # Application entry point
```

## Clean Architecture Layers

### 1. Domain Layer (`internal/domain/`)
- **Entities**: Core business objects with business rules
- **Interfaces**: Contracts for repositories and services
- **No dependencies on external frameworks**

### 2. Application Layer (`internal/application/`)
- **DTOs**: Data transfer objects for API communication
- **Interfaces**: Contracts for handlers and middleware
- **Services**: Use cases and application business logic
- **Depends only on domain layer**

### 3. Infrastructure Layer (`internal/infrastructure/`)
- **Repositories**: Data access implementations
- **Interfaces**: Contracts for external services
- **Database, cache, and external service implementations**

### 4. Presentation Layer (`internal/handlers/`, `internal/middleware/`)
- **Handlers**: HTTP request/response handling
- **Middleware**: Cross-cutting concerns
- **Routing**: URL to handler mapping

## Key Design Principles

1. **Dependency Inversion**: High-level modules don't depend on low-level modules
2. **Interface Segregation**: Clients depend only on interfaces they use
3. **Single Responsibility**: Each component has one reason to change
4. **Open/Closed**: Open for extension, closed for modification

## Entity Relationships

- **User**: Core user entity with authentication and profile data
- **Organization**: Multi-tenant organization management
- **Permission**: Hierarchical permission system
- **Role**: Role-based access control
- **Token**: JWT token management with security tracking

## Interface Contracts

All layers communicate through well-defined interfaces:
- Repository interfaces for data access
- Service interfaces for business logic
- Handler interfaces for HTTP endpoints
- Middleware interfaces for cross-cutting concerns

This structure ensures testability, maintainability, and scalability while following clean architecture principles.