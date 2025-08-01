package interfaces

import (
	"github.com/gin-gonic/gin"
)

// AuthMiddleware defines the interface for authentication middleware
type AuthMiddleware interface {
	RequireAuth() gin.HandlerFunc
	OptionalAuth() gin.HandlerFunc
	RequirePermission(resource, action string) gin.HandlerFunc
	RequireRole(roleName string) gin.HandlerFunc
	RequireOrganization() gin.HandlerFunc
}

// RateLimitMiddleware defines the interface for rate limiting middleware
type RateLimitMiddleware interface {
	RateLimit(requests int, window string) gin.HandlerFunc
	RateLimitByIP(requests int, window string) gin.HandlerFunc
	RateLimitByUser(requests int, window string) gin.HandlerFunc
}

// LoggingMiddleware defines the interface for logging middleware
type LoggingMiddleware interface {
	RequestLogger() gin.HandlerFunc
	ErrorLogger() gin.HandlerFunc
	AuditLogger() gin.HandlerFunc
}

// SecurityMiddleware defines the interface for security middleware
type SecurityMiddleware interface {
	CORS() gin.HandlerFunc
	SecurityHeaders() gin.HandlerFunc
	CSRFProtection() gin.HandlerFunc
	RequestSizeLimit(maxSize int64) gin.HandlerFunc
}

// ValidationMiddleware defines the interface for request validation middleware
type ValidationMiddleware interface {
	ValidateJSON(schema interface{}) gin.HandlerFunc
	ValidateQuery(schema interface{}) gin.HandlerFunc
	ValidateParams(schema interface{}) gin.HandlerFunc
	SanitizeInput() gin.HandlerFunc
}