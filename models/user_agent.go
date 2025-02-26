package models

import (
	"github.com/google/uuid"
	// "gorm.io/gorm"
)

type UserAgent struct {
	ID            uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primary_key" json:"id"`
	WalletAddress string    `gorm:"not null" json:"wallet_address"`
	AgentID       string    `gorm:"not null" json:"agent_id"`
	ServerDomain  string    `gorm:"not null" json:"server_domain"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	CreatedAt     int64     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     int64     `gorm:"autoUpdateTime" json:"updated_at"`

	User User `gorm:"foreignkey:UserID" json:"-"`
} 