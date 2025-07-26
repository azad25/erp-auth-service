package router

import (
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/events"
	"erp-auth-service/internal/handlers"
	"erp-auth-service/internal/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func Initialize(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, kafkaProducer *events.Producer) *gin.Engine {
	// Set Gin mode
	gin.SetMode(cfg.Server.GinMode)

	r := gin.Default()

	// CORS middleware
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = cfg.Server.AllowedOrigins
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	r.Use(cors.New(corsConfig))

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, redisClient, cfg, kafkaProducer)
	userHandler := handlers.NewUserHandler(db, redisClient, cfg, kafkaProducer)
	roleHandler := handlers.NewRoleHandler(db, redisClient, cfg, kafkaProducer)

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "auth-service"})
	})

	// API routes
	api := r.Group("/api/v1")
	{
		// Public routes (no authentication required)
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.POST("/forgot-password", authHandler.ForgotPassword)
			auth.POST("/reset-password", authHandler.ResetPassword)
		}

		// Protected routes (authentication required)
		protected := api.Group("/")
		protected.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
		protected.Use(middleware.TenantMiddleware())
		{
			// User management
			users := protected.Group("/users")
			{
				users.GET("/profile", userHandler.GetProfile)
				users.PUT("/profile", userHandler.UpdateProfile)
				users.POST("/change-password", userHandler.ChangePassword)
				users.POST("/enable-2fa", userHandler.Enable2FA)
				users.POST("/disable-2fa", userHandler.Disable2FA)
			}

			// Role and permission management
			roles := protected.Group("/roles")
			{
				roles.GET("/", roleHandler.GetRoles)
				roles.POST("/", roleHandler.CreateRole)
				roles.GET("/:id", roleHandler.GetRole)
				roles.PUT("/:id", roleHandler.UpdateRole)
				roles.DELETE("/:id", roleHandler.DeleteRole)
				roles.POST("/:id/permissions", roleHandler.AssignPermissions)
				roles.DELETE("/:id/permissions/:permissionId", roleHandler.RemovePermission)
			}

			// User role management
			userRoles := protected.Group("/user-roles")
			{
				userRoles.POST("/assign", roleHandler.AssignUserRole)
				userRoles.DELETE("/revoke", roleHandler.RevokeUserRole)
				userRoles.GET("/user/:userId", roleHandler.GetUserRoles)
			}

			// Permission management
			permissions := protected.Group("/permissions")
			{
				permissions.GET("/", roleHandler.GetPermissions)
			}

			// Organization management
			org := protected.Group("/organization")
			{
				org.GET("/", userHandler.GetOrganization)
				org.PUT("/", userHandler.UpdateOrganization)
			}
		}

		// Service-to-service routes (for internal communication)
		internal := api.Group("/internal")
		internal.Use(middleware.ServiceAuthMiddleware())
		{
			internal.POST("/validate-token", authHandler.ValidateToken)
			internal.GET("/user/:id", userHandler.GetUserByID)
			internal.POST("/check-permission", roleHandler.CheckPermission)
		}
	}

	return r
}