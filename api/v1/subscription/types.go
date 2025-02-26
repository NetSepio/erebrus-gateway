package subscription

import "github.com/NetSepio/erebrus-gateway/models"

type SubscriptionResponse struct {
	Subscription *models.Subscription `json:"subscription,omitempty"`
	Status       string               `json:"status"`
}
