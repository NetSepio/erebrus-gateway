package token

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ApplyRoutesSubscriptionToken(r *gin.RouterGroup) {
	g := r.Group("/token")
	{
		g.POST("", CreateSubscriptionToken)
		g.GET("/all", GetSubscriptionTokens)
		g.GET("/:id", GetSubscriptionToken)
		g.PATCH("/:id", UpdateSubscriptionToken)
		g.DELETE("/:id", DeleteSubscriptionToken)

	}
}

// Create Subscription Token
func CreateSubscriptionToken(c *gin.Context) {
	var token models.SubscriptionToken
	if err := c.ShouldBindJSON(&token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	DB := dbconfig.GetDb()

	tx := DB.Create(&token)
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}
	c.JSON(http.StatusCreated, token)
}

// Get All Subscription Tokens
func GetSubscriptionTokens(c *gin.Context) {
	var tokens []models.SubscriptionToken
	DB := dbconfig.GetDb()

	tx := DB.Find(&tokens)
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, tokens)
}

// Get Subscription Token by ID
func GetSubscriptionToken(c *gin.Context) {
	idStr := c.Param("id")

	// Convert string to UUID
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format", "info": err.Error()})
		return
	}

	var token models.SubscriptionToken
	DB := dbconfig.GetDb()

	// Use UUID in the query
	if err := DB.First(&token, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found", "info": err.Error()})
		return
	}

	c.JSON(http.StatusOK, token)
}

// Update Subscription Token
func UpdateSubscriptionToken(c *gin.Context) {
	idStr := c.Param("id")
	var token models.SubscriptionToken

	// Convert string to UUID
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format", "info": err.Error()})
		return
	}

	DB := dbconfig.GetDb()

	if err := DB.First(&token, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found", "info": err.Error()})
		return
	}

	if err := c.ShouldBindJSON(&token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "info": err.Error()})
		return
	}

	tx := DB.Save(&token)
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update token", "info": tx.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, token)
}

// Delete Subscription Token
func DeleteSubscriptionToken(c *gin.Context) {
	id := c.Param("id")
	var token models.SubscriptionToken

	DB := dbconfig.GetDb()

	if err := DB.First(&token, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found", "info": err.Error()})
		return
	}

	tx := DB.Delete(&token)
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete token", "info": tx.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Token deleted successfully"})
}
