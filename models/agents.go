package models

import (
	"time"
)

type Agent struct {
	ID             string    `json:"id" gorm:"primaryKey"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreaeTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	Name           string    `json:"name"`
	Clients        string    `json:"clients"` 
	Status         string    `json:"status"`
	AvatarImg      string    `json:"avatar_img"`
	CoverImg       string    `json:"cover_img"`
	VoiceModel     string    `json:"voice_model"`
	Organization   string    `json:"organization"`
	WalletAddress  string    `json:"wallet_address" gorm:"index"`
	ServerDomain   string    `json:"server_domain"`
	Domain         string    `json:"domain"`
	CharacterFile  string    `json:"character_file"` 
}

