package model

import (
	"time"

	"gorm.io/gorm"
)

// APIKey represents an API key for a tenant
type APIKey struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	TenantID   uint           `gorm:"not null;index" json:"tenant_id"`
	KeyHash    string         `gorm:"size:255;uniqueIndex;not null" json:"-"`
	Name       string         `gorm:"size:255" json:"name"`
	Active     bool           `gorm:"default:true" json:"active"`
	Permissions string         `gorm:"type:text" json:"permissions"` // JSON string of permissions
	CreatedAt  time.Time      `json:"created_at"`
	ExpiresAt  *time.Time     `json:"expires_at"`
	LastUsedAt *time.Time     `json:"last_used_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name
func (APIKey) TableName() string {
	return "api_keys"
}
