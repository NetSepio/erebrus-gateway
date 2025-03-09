package models

import (
	"github.com/google/uuid"
)

// NFTSubscriptionMintAddress represents the table structure
type NFTSubscriptionMintAddress struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	MintAddress string    `gorm:"type:varchar(255);unique;not null" json:"mint_address"`
}
