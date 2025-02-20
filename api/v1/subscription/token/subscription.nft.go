package token

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ApplyRoutesSubscriptionNft(r *gin.RouterGroup) {
	g := r.Group("/nft")
	{
		g.POST("", CreateSubscriptionNFT)
		g.GET("/all", GetSubscriptionNFTs)
		g.GET("/:id", GetSubscriptionNFT)
		g.PATCH("/:id", UpdateSubscriptionNFT)
		g.DELETE("/:id", DeleteSubscriptionNFT)

	}
}

// Create Subscription NFT
func CreateSubscriptionNFT(c *gin.Context) {
	var nft models.SubscriptionNFT

	if err := c.ShouldBindJSON(&nft); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	DB := dbconfig.GetDb()

	if err := DB.Create(&nft).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create NFT", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Subscription NFT created successfully", "nft": nft})
}

// Get All Subscription NFTs
func GetSubscriptionNFTs(c *gin.Context) {
	var nfts []models.SubscriptionNFT
	DB := dbconfig.GetDb()

	if err := DB.Find(&nfts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch NFTs", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nfts": nfts})
}

// Get Subscription NFT by ID
func GetSubscriptionNFT(c *gin.Context) {
	id := c.Param("id")
	var nft models.SubscriptionNFT
	DB := dbconfig.GetDb()

	// Validate UUID
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	if err := DB.First(&nft, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "NFT not found"})
		return
	}

	c.JSON(http.StatusOK, nft)
}

// Update Subscription NFT
func UpdateSubscriptionNFT(c *gin.Context) {
	id := c.Param("id")
	var nft models.SubscriptionNFT
	DB := dbconfig.GetDb()

	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	if err := DB.First(&nft, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "NFT not found"})
		return
	}

	if err := c.ShouldBindJSON(&nft); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := DB.Save(&nft).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update NFT", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subscription NFT updated successfully", "nft": nft})
}

// Delete Subscription NFT
func DeleteSubscriptionNFT(c *gin.Context) {
	id := c.Param("id")
	var nft models.SubscriptionNFT
	DB := dbconfig.GetDb()

	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	if err := DB.First(&nft, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "NFT not found"})
		return
	}

	if err := DB.Delete(&nft).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete NFT", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "NFT deleted successfully"})
}
