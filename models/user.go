package models

import "time"

type User struct {
	UserId            string         `gorm:"primary_key" json:"userId,omitempty"`
	Name              string         `json:"name,omitempty"`
	WalletAddress     *string        `json:"walletAddress,omitempty"`
	Discord           string         `json:"discord"`
	Twitter           string         `json:"twitter"`
	FlowIds           []FlowId       `gorm:"foreignkey:UserId" json:"-"`
	ProfilePictureUrl string         `json:"profilePictureUrl,omitempty"`
	Country           string         `json:"country,omitempty"`
	Feedbacks         []UserFeedback `gorm:"foreignkey:UserId" json:"userFeedbacks"`
	EmailId           *string        `json:"emailId,omitempty"`
}

type TStripePiType string

type UserStripePi struct {
	Id           string        `gorm:"primary_key" json:"id,omitempty"`
	UserId       string        `json:"userId,omitempty"`
	StripePiId   string        `json:"stripePiId,omitempty"`
	StripePiType TStripePiType `json:"stripePiType,omitempty"`
	CreatedAt    time.Time     `json:"createdAt,omitempty"`
}

var Erebrus111NFT TStripePiType = "Erebrus111NFT"
