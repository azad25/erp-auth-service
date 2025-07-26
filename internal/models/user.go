package models

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	Email          string    `gorm:"unique;not null;size:255" json:"email" validate:"required,email"`
	PasswordHash   string    `gorm:"not null;size:255" json:"-"`
	FirstName      string    `gorm:"not null;size:100" json:"first_name" validate:"required,min=1,max=100"`
	LastName       string    `gorm:"not null;size:100" json:"last_name" validate:"required,min=1,max=100"`
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	IsVerified     bool      `gorm:"default:false" json:"is_verified"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Two-Factor Authentication
	TwoFactorEnabled bool   `gorm:"default:false" json:"two_factor_enabled"`
	TwoFactorSecret  string `gorm:"size:255" json:"-"`

	// Relationships
	Organization Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	UserRoles    []UserRole  `gorm:"foreignKey:UserID" json:"user_roles,omitempty"`
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

func (u *User) TableName() string {
	return "users"
}