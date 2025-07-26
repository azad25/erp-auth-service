package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	jwt.RegisteredClaims
}

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Set user context
		c.Set("user_id", claims.UserID)
		c.Set("organization_id", claims.OrganizationID)
		c.Set("email", claims.Email)

		c.Next()
	}
}

func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, exists := c.Get("organization_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context required"})
			c.Abort()
			return
		}

		// Set tenant context for database queries
		c.Set("tenant_id", organizationID)
		c.Next()
	}
}

func ServiceAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For service-to-service communication
		// This would typically use a different authentication mechanism
		// like API keys or service tokens
		serviceKey := c.GetHeader("X-Service-Key")
		if serviceKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Service key required"})
			c.Abort()
			return
		}

		// Validate service key (implement your service key validation logic)
		if !isValidServiceKey(serviceKey) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid service key"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func isValidServiceKey(key string) bool {
	// Implement service key validation logic
	// This could check against a database or environment variable
	return key == "internal-service-key" // Placeholder
}