package models

import (
	"time"

	"github.com/google/uuid"
)

type PerksToken struct {
	ID              uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Blockchain      string    `gorm:"type:varchar(50);not null" json:"blockchain"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`
	Symbol          string    `gorm:"type:varchar(50);not null" json:"symbol"`
	ImageURI        string    `gorm:"type:text;not null" json:"imageUri"`
	ContractAddress string    `gorm:"type:varchar(100);not null" json:"contractAddress"`
	Quantity        int       `gorm:"not null" json:"quantity"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
