package models

import (
	"time"

	"github.com/google/uuid"
)

type CyreneAIAgent struct {
	ID            uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	DID           string    `gorm:"uniqueIndex" json:"did"`
	Name          string    `json:"name"`
	AgentMetadata string    `json:"agentMetadata"`
	NFTMetadata   string    `json:"nftMetadata"`
	Domain        string    `json:"domain"`
	Status        string    `json:"status"`
	Owner         string    `json:"owner"`
	Node          string    `json:"node"`
	Region        string    `json:"region"`
	CreatedAt     time.Time `gorm:"type:timestamptz;default:now()" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"type:timestamptz;default:now()" json:"updatedAt"`
}
