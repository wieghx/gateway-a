package model

import (
	"time"

	"gorm.io/gorm"
)

// Tenant represents a tenant (customer/organization)
type Tenant struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Slug        string         `gorm:"size:100;uniqueIndex;not null" json:"slug"`
	APIKey      string         `gorm:"size:255;uniqueIndex;not null" json:"api_key"`
	Email       string         `gorm:"size:255" json:"email"`
	Active      bool           `gorm:"default:true" json:"active"`
	QuotaDaily  int64          `gorm:"default:0" json:"quota_daily"`
	QuotaMonthly int64         `gorm:"default:0" json:"quota_monthly"`
	RateLimit   int            `gorm:"default:100" json:"rate_limit"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name
func (Tenant) TableName() string {
	return "tenants"
}
