package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User model optimized for high-performance queries with proper indexing
type User struct {
	// Primary identifiers with optimized indexes
	ID             uuid.UUID `gorm:"type:uuid;primary_key;index:idx_user_id" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index:idx_user_org" json:"organization_id"`
	
	// Authentication fields with performance indexes
	Email        string `gorm:"unique;not null;size:255;index:idx_user_email" json:"email" validate:"required,email"`
	PasswordHash string `gorm:"not null;size:255" json:"-"`
	
	// Profile information
	FirstName string `gorm:"not null;size:100" json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `gorm:"not null;size:100" json:"last_name" validate:"required,min=1,max=100"`
	
	// Status and security with composite indexes
	IsActive       bool       `gorm:"default:true;index:idx_user_active" json:"is_active"`
	IsVerified     bool       `gorm:"default:false" json:"is_verified"`
	LastLoginAt    *time.Time `gorm:"index:idx_user_last_login" json:"last_login_at"`
	PasswordResetAt *time.Time `json:"password_reset_at"`
	
	// Two-factor authentication
	TwoFactorEnabled bool   `gorm:"default:false" json:"two_factor_enabled"`
	TwoFactorSecret  string `gorm:"size:255" json:"-"`
	BackupCodes      pq.StringArray `gorm:"type:text[]" json:"-"`
	
	// Security tracking
	FailedLoginAttempts int       `gorm:"default:0" json:"-"`
	LockedUntil        *time.Time `json:"-"`
	
	// Audit fields with time-based indexes
	CreatedAt time.Time `gorm:"index:idx_user_created" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	// Relationships (lazy loaded for performance)
	Organization Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	UserRoles    []UserRole  `gorm:"foreignKey:UserID" json:"user_roles,omitempty"`
	
	// Computed fields for caching
	FullName        string `gorm:"-" json:"full_name"`
	PermissionCache string `gorm:"-" json:"-"` // JSON serialized permissions
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hashedPassword)
	return nil
}

func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

func (u *User) GetFullName() string {
	return u.FirstName + " " + u.LastName
}

// IsLocked checks if the user account is currently locked
func (u *User) IsLocked() bool {
	return u.LockedUntil != nil && u.LockedUntil.After(time.Now())
}

// CanAttemptLogin checks if the user can attempt to login
func (u *User) CanAttemptLogin() bool {
	return u.IsActive && u.IsVerified && !u.IsLocked()
}

func (u *User) TableName() string {
	return "users"
}