package models

import "time"

type Subscription struct {
	ID        uint      `gorm:"primary_key" json:"id,omitempty"`
	UserId    string    `json:"userId,omitempty"`
	Type      string    `json:"type,omitempty"`
	StartTime time.Time `json:"startTime,omitempty"`
	EndTime   time.Time `json:"endTime,omitempty"`
}
