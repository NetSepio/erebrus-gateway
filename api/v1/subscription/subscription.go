package subscription

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
	"github.com/NetSepio/erebrus-gateway/api/v1/subscription/token"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/envconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/httpo"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/paymentintent"
	"github.com/stripe/stripe-go/v76/webhook"
	"gorm.io/gorm"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/subscription")
	{
		g.POST("webhook", HandleWebhook)
		token.ApplyRoutesSubscriptionMint(g)
		g.Use(paseto.PASETO(false))
		g.POST("/trial", TrialSubscription)
		g.POST("/create-payment", CreatePaymentIntent)
		g.GET("", CheckSubscription)
		g.POST("/custom_duration", SubscriptionForCustomDuration)

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
	err := db.Where("user_id = ?", userId).Order("end_time DESC").First(&subscription).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			res := SubscriptionResponse{
				Status: "notFound",
			}
			c.JSON(http.StatusOK, res)
		}
		logwrapper.Errorf("Error fetching subscriptions: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	var status = "expired"
	if time.Now().Before(subscription.EndTime) {
		status = "active"
	}
	res := SubscriptionResponse{
		Subscription: subscription,
		Status:       status,
	}
	c.JSON(http.StatusOK, res)
}

func CreatePaymentIntent(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	db := dbconfig.GetDb()
	params := &stripe.PaymentIntentParams{
		Amount:      stripe.Int64(1099),
		Currency:    stripe.String(string(stripe.CurrencyUSD)),
		Description: stripe.String("Payment to purchase vpn subscription"),
	}
	pi, err := paymentintent.New(params)
	if err != nil {
		logwrapper.Errorf("failed to create new payment intent: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	// insert in above table
	err = db.Create(&models.UserStripePi{
		Id:           uuid.NewString(),
		UserId:       userId,
		StripePiId:   pi.ID,
		StripePiType: models.Erebrus111NFT,
	}).Error
	if err != nil {
		logwrapper.Errorf("failed to insert into users_stripe_pi: %v", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, "internal server error").SendD(c)
		return
	}

	httpo.NewSuccessResponseP(200, "Created new charge", gin.H{"clientSecret": pi.ClientSecret}).SendD(c)
}

func HandleWebhook(c *gin.Context) {
	db := dbconfig.GetDb()
	const MaxBodyBytes = int64(65536)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBodyBytes)
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logwrapper.Errorf("Error reading request body: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	event, err := webhook.ConstructEvent(payload, c.GetHeader("Stripe-Signature"), envconfig.EnvVars.STRIPE_WEBHOOK_SECRET)

	if err != nil {
		logwrapper.Errorf("Error verifying webhook signature: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	switch event.Type {
	case stripe.EventTypePaymentIntentSucceeded:
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			c.Status(http.StatusInternalServerError)
			return
		}

		var userStripePi models.UserStripePi
		if err := db.Where("stripe_pi_id = ?", paymentIntent.ID).First(&userStripePi).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				//warn and return success
				logwrapper.Warnf("No user found with stripe_pi_id: %v", err)
				c.JSON(http.StatusOK, gin.H{"status": "received"})
				return
			}
			logwrapper.Errorf("Error getting user with stripe_pi_id: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}

		var user models.User
		if err := db.Where("user_id = ?", userStripePi.UserId).First(&user).Error; err != nil {
			logwrapper.Errorf("Error getting user with user_id: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		subscription := models.Subscription{
			UserId:    user.UserId,
			StartTime: time.Now(),
			EndTime:   time.Now().AddDate(0, 3, 0),
		}

		if err = db.Model(models.Subscription{}).Create(&subscription).Error; err != nil {
			logwrapper.Errorf("Error creating subscription: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}

	case stripe.EventTypePaymentIntentCanceled:
		err := HandleCanceledOrFailedPaymentIntent(event.Data.Raw)
		if err != nil {
			logwrapper.Errorf("Error handling canceled payment intent: %v", err)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "received"})
	}
	c.JSON(http.StatusOK, gin.H{"status": "recieved"})
}

func HandleCanceledOrFailedPaymentIntent(eventDataRaw json.RawMessage) error {
	var paymentIntent stripe.PaymentIntent
	err := json.Unmarshal(eventDataRaw, &paymentIntent)
	if err != nil {
		return fmt.Errorf("error parsing webhook JSON: %w", err)
	}

	return nil
}

func SubscriptionForCustomDuration(c *gin.Context) {

	var (
		oneMonth   = time.Now().AddDate(0, 1, 0)
		threeMonth = time.Now().AddDate(0, 3, 0)
		oneYear    = time.Now().AddDate(1, 0, 0)
	)

	// Map duration type to end time

	var request struct {
		DurationType int `json:"duration_type"`
	}
	err := c.BindJSON(&request)
	if err != nil {
		httpo.NewErrorResponse(http.StatusBadRequest, fmt.Sprintf("payload is invalid %s", err)).SendD(c)
		return
	}

	var endTime time.Time
	switch request.DurationType {
	case 1:
		endTime = oneMonth
	case 2:
		endTime = threeMonth
	case 3:
		endTime = oneYear
	default:
		httpo.NewErrorResponse(http.StatusBadRequest, "invalid duration type").SendD(c)
		return
	}

	userId := c.GetString(paseto.CTX_USER_ID)

	// Check if there is already an active trial subscription for the user
	var existingSubscription models.Subscription
	db := dbconfig.GetDb()
	if err := db.Where("user_id = ? AND type = ? AND end_time > ?", userId, "trial", time.Now()).First(&existingSubscription).Error; err == nil {
		// There is already an active trial subscription for the user
		c.JSON(http.StatusBadRequest, gin.H{"error": "You already have an active trial subscription"})
		return
	}

	// Create a new trial subscription
	subscription := models.Subscription{
		UserId:    userId,
		StartTime: time.Now(),
		EndTime:   endTime,
		Type:      "TrialSubscription",
	}

	// Save the new trial subscription to the database
	if err := db.Model(models.Subscription{}).Create(&subscription).Error; err != nil {
		logwrapper.Errorf("Error creating subscription: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "subscription created"})
}
