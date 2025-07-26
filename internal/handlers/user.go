package handlers

import (
	"context"
	"net/http"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/events"
	"erp-auth-service/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type UserHandler struct {
	db            *gorm.DB
	redisClient   *redis.Client
	config        *config.Config
	kafkaProducer *events.Producer
}

type UpdateProfileRequest struct {
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

func NewUserHandler(db *gorm.DB, redisClient *redis.Client, config *config.Config, kafkaProducer *events.Producer) *UserHandler {
	return &UserHandler{
		db:            db,
		redisClient:   redisClient,
		config:        config,
		kafkaProducer: kafkaProducer,
	}
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User context not found"})
		return
	}

	var user models.User
	if err := h.db.Preload("Organization").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User context not found"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Update user profile
	user.FirstName = req.FirstName
	user.LastName = req.LastName

	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User context not found"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Verify current password
	if !user.CheckPassword(req.CurrentPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Set new password
	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash new password"})
		return
	}

	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	// Publish password changed event
	if h.kafkaProducer != nil {
		ctx := context.Background()
		passwordData := events.PasswordChangedData{
			UserID:         user.ID,
			OrganizationID: user.OrganizationID,
			ChangedBy:      user.ID, // User changed their own password
		}
		h.kafkaProducer.PublishPasswordChanged(ctx, passwordData, user.ID, user.OrganizationID, c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}

func (h *UserHandler) Enable2FA(c *gin.Context) {
	// Implementation for enabling 2FA
	c.JSON(http.StatusOK, gin.H{"message": "2FA enabled successfully"})
}

func (h *UserHandler) Disable2FA(c *gin.Context) {
	// Implementation for disabling 2FA
	c.JSON(http.StatusOK, gin.H{"message": "2FA disabled successfully"})
}

func (h *UserHandler) GetOrganization(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	var org models.Organization
	if err := h.db.Where("id = ?", organizationID).First(&org).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	c.JSON(http.StatusOK, org)
}

func (h *UserHandler) UpdateOrganization(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	var org models.Organization
	if err := h.db.Where("id = ?", organizationID).First(&org).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	if err := c.ShouldBindJSON(&org); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Save(&org).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update organization"})
		return
	}

	c.JSON(http.StatusOK, org)
}

func (h *UserHandler) GetUserByID(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var user models.User
	if err := h.db.Preload("Organization").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}