package subscription

import (
	"net/http"
	"time"

	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/subscription")
	{
		g.Use(paseto.PASETO(false))
		g.POST("/trial", TrialSubscription)
	}
}

func TrialSubscription(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	subscription := models.Subscription{
		UserId:    userId,
		StartTime: time.Now(),
		EndTime:   time.Now().AddDate(0, 0, 7),
		Type:      "trial",
	}
	db := dbconfig.GetDb()
	if err := db.Model(models.Subscription{}).Create(&subscription).Error; err != nil {
		logwrapper.Errorf("Error creating subscription: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "subscription created"})
}
func CheckSubscription(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)

	db := dbconfig.GetDb()
	var subscription *models.Subscription
	err := db.Where("userId = ?", userId).First(&subscription).Error
	if err != nil {
		logwrapper.Errorf("Error fetching subscriptions: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	var status = "expired"
	if time.Now().Before(subscription.EndTime) {
		status = "active"
	}

	c.JSON(http.StatusOK, gin.H{"data": subscription, "status": status})
}
