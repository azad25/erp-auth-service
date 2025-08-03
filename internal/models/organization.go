package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Organization struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;index:idx_org_id" json:"id"`
	Name      string    `gorm:"not null;size:255;index:idx_org_name" json:"name" validate:"required,min=2,max=255"`
	Domain    string    `gorm:"unique;not null;size:255;index:idx_org_domain" json:"domain" validate:"required,fqdn"`
	Settings  Settings  `gorm:"type:jsonb" json:"settings"`
	IsActive  bool      `gorm:"default:true;index:idx_org_active" json:"is_active"`
	CreatedAt time.Time `gorm:"index:idx_org_created" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships (lazy loaded for performance)
	Users []User `gorm:"foreignKey:OrganizationID" json:"users,omitempty"`
	Roles []Role `gorm:"foreignKey:OrganizationID" json:"roles,omitempty"`
}

type Settings struct {
	Timezone         string            `json:"timezone"`
	DateFormat       string            `json:"date_format"`
	Currency         string            `json:"currency"`
	Language         string            `json:"language"`
	TwoFactorEnabled bool              `json:"two_factor_enabled"`
	SessionTimeout   int               `json:"session_timeout"`
	CustomFields     map[string]string `json:"custom_fields"`
}

// Value implements the driver.Valuer interface for database storage
func (s Settings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for database retrieval
func (s *Settings) Scan(value interface{}) error {
	if value == nil {
		*s = Settings{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into Settings", value)
	}

	return json.Unmarshal(bytes, s)
}

func (o *Organization) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}

func (o *Organization) TableName() string {
	return "organizations"
}