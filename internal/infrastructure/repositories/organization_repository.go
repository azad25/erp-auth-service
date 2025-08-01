package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// organizationRepository implements the OrganizationRepository interface
type organizationRepository struct {
	db     *gorm.DB
	readDB *gorm.DB // Read replica for read operations
}

// NewOrganizationRepository creates a new organization repository
func NewOrganizationRepository(db, readDB *gorm.DB) interfaces.OrganizationRepository {
	return &organizationRepository{
		db:     db,
		readDB: readDB,
	}
}

// GetByID retrieves an organization by ID
func (r *organizationRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	var org models.Organization
	err := r.readDB.WithContext(ctx).
		Where("id = ? AND is_active = ?", id, true).
		First(&org).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization by ID: %w", err)
	}
	
	return &org, nil
}

// GetByDomain retrieves an organization by domain
func (r *organizationRepository) GetByDomain(ctx context.Context, domain string) (*models.Organization, error) {
	var org models.Organization
	err := r.readDB.WithContext(ctx).
		Where("domain = ? AND is_active = ?", domain, true).
		First(&org).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization by domain: %w", err)
	}
	
	return &org, nil
}

// Create creates a new organization
func (r *organizationRepository) Create(ctx context.Context, org *models.Organization) error {
	err := r.db.WithContext(ctx).Create(org).Error
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}
	return nil
}

// Update updates an existing organization
func (r *organizationRepository) Update(ctx context.Context, org *models.Organization) error {
	err := r.db.WithContext(ctx).Save(org).Error
	if err != nil {
		return fmt.Errorf("failed to update organization: %w", err)
	}
	return nil
}

// Delete soft deletes an organization
func (r *organizationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Model(&models.Organization{}).
		Where("id = ?", id).
		Update("is_active", false).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	return nil
}

// GetActiveOrganizations retrieves active organizations with pagination
func (r *organizationRepository) GetActiveOrganizations(ctx context.Context, limit, offset int) ([]models.Organization, error) {
	var orgs []models.Organization
	
	err := r.readDB.WithContext(ctx).
		Where("is_active = ?", true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&orgs).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get active organizations: %w", err)
	}
	
	return orgs, nil
}

// GetOrganizationWithUsers retrieves organization with its users
func (r *organizationRepository) GetOrganizationWithUsers(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	var org models.Organization
	err := r.readDB.WithContext(ctx).
		Preload("Users", "is_active = ?", true).
		Where("id = ? AND is_active = ?", id, true).
		First(&org).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization with users: %w", err)
	}
	
	return &org, nil
}

// GetUserCount returns the number of active users in an organization
func (r *organizationRepository) GetUserCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int64
	err := r.readDB.WithContext(ctx).
		Model(&models.User{}).
		Where("organization_id = ? AND is_active = ?", orgID, true).
		Count(&count).Error
	
	if err != nil {
		return 0, fmt.Errorf("failed to get user count: %w", err)
	}
	
	return int(count), nil
}