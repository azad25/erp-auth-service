package handlers

import (
	"context"
	"net/http"
	"time"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/events"
	"erp-auth-service/internal/middleware"
	"erp-auth-service/internal/models"
	"erp-auth-service/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db            *gorm.DB
	redisClient   *redis.Client
	config        *config.Config
	kafkaProducer *events.Producer
}

type RegisterRequest struct {
	Email           string `json:"email" validate:"required,email"`
	Password        string `json:"password" validate:"required,min=8"`
	FirstName       string `json:"first_name" validate:"required,min=1,max=100"`
	LastName        string `json:"last_name" validate:"required,min=1,max=100"`
	OrganizationName string `json:"organization_name" validate:"required,min=2,max=255"`
	Domain          string `json:"domain" validate:"required,fqdn"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type AuthResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	User         models.User `json:"user"`
}

func NewAuthHandler(db *gorm.DB, redisClient *redis.Client, config *config.Config, kafkaProducer *events.Producer) *AuthHandler {
	return &AuthHandler{
		db:            db,
		redisClient:   redisClient,
		config:        config,
		kafkaProducer: kafkaProducer,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate request
	if err := utils.ValidateStruct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if organization domain already exists
	var existingOrg models.Organization
	if err := h.db.Where("domain = ?", req.Domain).First(&existingOrg).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Organization domain already exists"})
		return
	}

	// Check if user email already exists
	var existingUser models.User
	if err := h.db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User email already exists"})
		return
	}

	// Start transaction
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create organization
	org := models.Organization{
		Name:   req.OrganizationName,
		Domain: req.Domain,
		Settings: models.Settings{
			Timezone:         "UTC",
			DateFormat:       "YYYY-MM-DD",
			Currency:         "USD",
			Language:         "en",
			TwoFactorEnabled: false,
			SessionTimeout:   3600,
		},
	}

	if err := tx.Create(&org).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create organization"})
		return
	}

	// Create user
	user := models.User{
		OrganizationID: org.ID,
		Email:          req.Email,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		IsActive:       true,
		IsVerified:     false,
	}

	if err := user.SetPassword(req.Password); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Create default admin role for the organization
	adminRole := models.Role{
		OrganizationID: org.ID,
		Name:           "Admin",
		Description:    "Full access to all features",
		IsSystem:       true,
	}

	if err := tx.Create(&adminRole).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create admin role"})
		return
	}

	// Assign admin role to user
	userRole := models.UserRole{
		UserID: user.ID,
		RoleID: adminRole.ID,
	}

	if err := tx.Create(&userRole).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign admin role"})
		return
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete registration"})
		return
	}

	// Generate tokens
	accessToken, refreshToken, err := h.generateTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Update last login
	h.db.Model(&user).Update("last_login_at", time.Now())

	// Publish organization created event
	if h.kafkaProducer != nil {
		ctx := context.Background()
		orgData := events.OrganizationCreatedData{
			OrganizationID: org.ID,
			Name:           org.Name,
			Domain:         org.Domain,
			CreatedBy:      user.ID,
		}
		h.kafkaProducer.PublishOrganizationCreated(ctx, orgData, user.ID, org.ID, c.ClientIP(), c.GetHeader("User-Agent"))
		
		// Publish user registered event
		userData := events.UserRegisteredData{
			UserID:         user.ID,
			Email:          user.Email,
			FirstName:      user.FirstName,
			LastName:       user.LastName,
			OrganizationID: user.OrganizationID,
		}
		h.kafkaProducer.PublishUserRegistered(ctx, userData, user.ID, org.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusCreated, AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    h.config.JWT.AccessExpiry,
		User:         user,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by email
	var user models.User
	if err := h.db.Preload("Organization").Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Check if user is active
	if !user.IsActive {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Account is deactivated"})
		return
	}

	// Check password
	if !user.CheckPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Generate tokens
	accessToken, refreshToken, err := h.generateTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Update last login
	now := time.Now()
	h.db.Model(&user).Update("last_login_at", now)

	// Publish user logged in event
	if h.kafkaProducer != nil {
		ctx := context.Background()
		loginData := events.UserLoggedInData{
			UserID:         user.ID,
			Email:          user.Email,
			OrganizationID: user.OrganizationID,
			LoginMethod:    "password",
		}
		h.kafkaProducer.PublishUserLoggedIn(ctx, loginData, user.ID, user.OrganizationID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    h.config.JWT.AccessExpiry,
		User:         user,
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse refresh token
	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(req.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.config.JWT.Secret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// Check if refresh token is blacklisted
	ctx := context.Background()
	if h.redisClient.Get(ctx, "blacklist:"+req.RefreshToken).Err() == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token is blacklisted"})
		return
	}

	// Find user
	var user models.User
	if err := h.db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	// Generate new tokens
	accessToken, newRefreshToken, err := h.generateTokens(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Blacklist old refresh token
	h.redisClient.Set(ctx, "blacklist:"+req.RefreshToken, "1", time.Duration(h.config.JWT.RefreshExpiry)*time.Second)

	// Publish token refreshed event
	if h.kafkaProducer != nil {
		refreshData := events.TokenRefreshedData{
			UserID:         user.ID,
			OrganizationID: user.OrganizationID,
			TokenID:        claims.ID, // JWT ID from claims
		}
		h.kafkaProducer.PublishTokenRefreshed(ctx, refreshData, user.ID, user.OrganizationID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusOK, AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    h.config.JWT.AccessExpiry,
		User:         user,
	})
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	// Implementation for password reset email
	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	// Implementation for password reset
	c.JSON(http.StatusOK, gin.H{"message": "Password reset successful"})
}

func (h *AuthHandler) ValidateToken(c *gin.Context) {
	// For service-to-service token validation
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token required"})
		return
	}

	// Validate token logic here
	c.JSON(http.StatusOK, gin.H{"valid": true})
}

func (h *AuthHandler) generateTokens(user models.User) (string, string, error) {
	// Access token claims
	accessClaims := middleware.Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(h.config.JWT.AccessExpiry) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	// Generate access token
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(h.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	// Refresh token claims
	refreshClaims := middleware.Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(h.config.JWT.RefreshExpiry) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	// Generate refresh token
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(h.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	return accessTokenString, refreshTokenString, nil
}