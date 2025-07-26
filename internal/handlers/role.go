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

type RoleHandler struct {
	db            *gorm.DB
	redisClient   *redis.Client
	config        *config.Config
	kafkaProducer *events.Producer
}

type CreateRoleRequest struct {
	Name        string `json:"name" validate:"required,min=2,max=100"`
	Description string `json:"description"`
}

type AssignPermissionsRequest struct {
	PermissionIDs []uuid.UUID `json:"permission_ids" validate:"required"`
}

type AssignUserRoleRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required"`
	RoleID uuid.UUID `json:"role_id" validate:"required"`
}

type CheckPermissionRequest struct {
	UserID   uuid.UUID `json:"user_id" validate:"required"`
	Resource string    `json:"resource" validate:"required"`
	Action   string    `json:"action" validate:"required"`
}

func NewRoleHandler(db *gorm.DB, redisClient *redis.Client, config *config.Config, kafkaProducer *events.Producer) *RoleHandler {
	return &RoleHandler{
		db:            db,
		redisClient:   redisClient,
		config:        config,
		kafkaProducer: kafkaProducer,
	}
}

func (h *RoleHandler) GetRoles(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	var roles []models.Role
	if err := h.db.Where("organization_id = ?", organizationID).Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch roles"})
		return
	}

	c.JSON(http.StatusOK, roles)
}

func (h *RoleHandler) CreateRole(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if role name already exists in organization
	var existingRole models.Role
	if err := h.db.Where("organization_id = ? AND name = ?", organizationID, req.Name).First(&existingRole).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Role name already exists"})
		return
	}

	role := models.Role{
		OrganizationID: organizationID.(uuid.UUID),
		Name:           req.Name,
		Description:    req.Description,
		IsSystem:       false,
		IsActive:       true,
	}

	if err := h.db.Create(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create role"})
		return
	}

	c.JSON(http.StatusCreated, role)
}

func (h *RoleHandler) GetRole(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var role models.Role
	if err := h.db.Preload("RolePermissions.Permission").Where("id = ? AND organization_id = ?", roleID, organizationID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	c.JSON(http.StatusOK, role)
}

func (h *RoleHandler) UpdateRole(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var role models.Role
	if err := h.db.Where("id = ? AND organization_id = ?", roleID, organizationID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	// Don't allow updating system roles
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot update system role"})
		return
	}

	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role.Name = req.Name
	role.Description = req.Description

	if err := h.db.Save(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}

	c.JSON(http.StatusOK, role)
}

func (h *RoleHandler) DeleteRole(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var role models.Role
	if err := h.db.Where("id = ? AND organization_id = ?", roleID, organizationID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	// Don't allow deleting system roles
	if role.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete system role"})
		return
	}

	if err := h.db.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Role deleted successfully"})
}

func (h *RoleHandler) AssignPermissions(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	var req AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify role exists and belongs to organization
	var role models.Role
	if err := h.db.Where("id = ? AND organization_id = ?", roleID, organizationID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	// Remove existing permissions
	h.db.Where("role_id = ?", roleID).Delete(&models.RolePermission{})

	// Add new permissions
	for _, permissionID := range req.PermissionIDs {
		rolePermission := models.RolePermission{
			RoleID:       roleID,
			PermissionID: permissionID,
		}
		h.db.Create(&rolePermission)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permissions assigned successfully"})
}

func (h *RoleHandler) RemovePermission(c *gin.Context) {
	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role ID"})
		return
	}

	permissionIDStr := c.Param("permissionId")
	permissionID, err := uuid.Parse(permissionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid permission ID"})
		return
	}

	if err := h.db.Where("role_id = ? AND permission_id = ?", roleID, permissionID).Delete(&models.RolePermission{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove permission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permission removed successfully"})
}

func (h *RoleHandler) AssignUserRole(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	var req AssignUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify user and role belong to the same organization
	var user models.User
	if err := h.db.Where("id = ? AND organization_id = ?", req.UserID, organizationID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var role models.Role
	if err := h.db.Where("id = ? AND organization_id = ?", req.RoleID, organizationID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	// Check if user already has this role
	var existingUserRole models.UserRole
	if err := h.db.Where("user_id = ? AND role_id = ?", req.UserID, req.RoleID).First(&existingUserRole).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "User already has this role"})
		return
	}

	userRole := models.UserRole{
		UserID: req.UserID,
		RoleID: req.RoleID,
	}

	if err := h.db.Create(&userRole).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign role"})
		return
	}

	// Publish role assigned event
	if h.kafkaProducer != nil {
		ctx := context.Background()
		currentUserID, _ := c.Get("user_id")
		roleData := events.RoleAssignedData{
			UserID:         req.UserID,
			RoleID:         req.RoleID,
			RoleName:       role.Name,
			OrganizationID: organizationID.(uuid.UUID),
			AssignedBy:     currentUserID.(uuid.UUID),
		}
		h.kafkaProducer.PublishRoleAssigned(ctx, roleData, req.UserID, organizationID.(uuid.UUID), c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Role assigned successfully"})
}

func (h *RoleHandler) RevokeUserRole(c *gin.Context) {
	organizationID, exists := c.Get("organization_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Organization context not found"})
		return
	}

	var req AssignUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get role name for event
	var role models.Role
	if err := h.db.Where("id = ?", req.RoleID).First(&role).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		return
	}

	if err := h.db.Where("user_id = ? AND role_id = ?", req.UserID, req.RoleID).Delete(&models.UserRole{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke role"})
		return
	}

	// Publish role revoked event
	if h.kafkaProducer != nil {
		ctx := context.Background()
		currentUserID, _ := c.Get("user_id")
		roleData := events.RoleRevokedData{
			UserID:         req.UserID,
			RoleID:         req.RoleID,
			RoleName:       role.Name,
			OrganizationID: organizationID.(uuid.UUID),
			RevokedBy:      currentUserID.(uuid.UUID),
		}
		h.kafkaProducer.PublishRoleRevoked(ctx, roleData, req.UserID, organizationID.(uuid.UUID), c.ClientIP(), c.GetHeader("User-Agent"))
	}

	c.JSON(http.StatusOK, gin.H{"message": "Role revoked successfully"})
}

func (h *RoleHandler) GetUserRoles(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var userRoles []models.UserRole
	if err := h.db.Preload("Role").Where("user_id = ?", userID).Find(&userRoles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user roles"})
		return
	}

	c.JSON(http.StatusOK, userRoles)
}

func (h *RoleHandler) GetPermissions(c *gin.Context) {
	var permissions []models.Permission
	if err := h.db.Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch permissions"})
		return
	}

	c.JSON(http.StatusOK, permissions)
}

func (h *RoleHandler) CheckPermission(c *gin.Context) {
	var req CheckPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user roles
	var userRoles []models.UserRole
	if err := h.db.Preload("Role.RolePermissions.Permission").Where("user_id = ?", req.UserID).Find(&userRoles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user roles"})
		return
	}

	// Check if user has the required permission
	hasPermission := false
	for _, userRole := range userRoles {
		for _, rolePermission := range userRole.Role.RolePermissions {
			if rolePermission.Permission.Resource == req.Resource && rolePermission.Permission.Action == req.Action {
				hasPermission = true
				break
			}
		}
		if hasPermission {
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{"has_permission": hasPermission})
}