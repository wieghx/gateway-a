package model

import (
	"time"

	"gorm.io/gorm"
)

// UsageTrack tracks token usage for billing and quota enforcement
type UsageTrack struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	TenantID    uint           `gorm:"not null;index" json:"tenant_id"`
	Date        string         `gorm:"size:10;not null;index" json:"date"` // YYYY-MM-DD
	Model       string         `gorm:"size:100" json:"model"`
	PromptTokens int64          `gorm:"default:0" json:"prompt_tokens"`
	CompletionTokens int64      `gorm:"default:0" json:"completion_tokens"`
	TotalTokens int64           `gorm:"default:0" json:"total_tokens"`
	Cost        float64         `gorm:"default:0" json:"cost"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   gorm.DeletedAt  `gorm:"index" json:"-"`
}

// TableName specifies the table name
func (UsageTrack) TableName() string {
	return "usage_tracks"
}

// TenantUsage tracks cumulative usage for a tenant
type TenantUsage struct {
	TenantID     uint      `gorm:"primaryKey"`
	DailyTotal   int64     `gorm:"default:0"`
	MonthlyTotal int64     `gorm:"default:0"`
	LastUpdated  time.Time `json:"last_updated"`
}
