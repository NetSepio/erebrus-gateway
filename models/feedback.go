package models

import "time"

type UserFeedback struct {
	UserId    string    `json:"-" gorm:"primary_key"`
	Feedback  string    `json:"feedback" binding:"required" gorm:"primary_key"`
	Rating    int       `json:"rating" binding:"gte=1,lte=5" gorm:"primary_key"`
	CreatedAt time.Time `json:"-"`
}
