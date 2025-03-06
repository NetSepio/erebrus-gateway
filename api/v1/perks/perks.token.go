package perks

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ApplyRoutesPerksToken(r *gin.RouterGroup) {
	g := r.Group("/token")
	{
		g.POST("", CreatePerksToken)
		g.GET("", GetPerksTokens)
		g.GET("/:id", GetPerksToken)
		g.PATCH("/:id", UpdatePerksToken)
		g.DELETE("/:id", DeletePerksToken)

	}
}

// Create Perks Token
func CreatePerksToken(c *gin.Context) {
	var token models.PerksToken
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

// Get All Perks Tokens
func GetPerksTokens(c *gin.Context) {
	var tokens []models.PerksToken
	DB := dbconfig.GetDb()

	tx := DB.Find(&tokens)
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": tx.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, tokens)
}

// Get Perks Token by ID
func GetPerksToken(c *gin.Context) {
	idStr := c.Param("id")

	// Convert string to UUID
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format", "info": err.Error()})
		return
	}

	var token models.PerksToken
	DB := dbconfig.GetDb()

	// Use UUID in the query
	if err := DB.First(&token, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found", "info": err.Error()})
		return
	}

	c.JSON(http.StatusOK, token)
}

// Update Perks Token
func UpdatePerksToken(c *gin.Context) {
	idStr := c.Param("id")
	var token models.PerksToken

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

// Delete Perks Token
func DeletePerksToken(c *gin.Context) {
	id := c.Param("id")
	var token models.PerksToken

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
