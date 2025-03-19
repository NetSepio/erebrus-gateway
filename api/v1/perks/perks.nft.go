package perks

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ApplyRoutesPerksNFT(r *gin.RouterGroup) {
	g := r.Group("/nft")
	{
		g.POST("", CreatePerkNFT)
		g.GET("", GetPerkNFTs)
		g.GET("/:id", GetPerkNFT)
		g.PATCH("/:id", UpdatePerkNFT)
		g.DELETE("/:id", DeletePerkNFT)

	}
}

// Create Perks NFT
func CreatePerkNFT(c *gin.Context) {
	var nft models.PerkNFT

	if err := c.ShouldBindJSON(&nft); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	DB := dbconfig.GetDb()

	if err := DB.Create(&nft).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create NFT", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Perks NFT created successfully", "nft": nft})
}

// Get All Perks NFTs
func GetPerkNFTs(c *gin.Context) {
	var nfts []models.PerkNFT
	DB := dbconfig.GetDb()

	if err := DB.Find(&nfts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch NFTs", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nfts": nfts})
}

// Get Perks NFT by ID
func GetPerkNFT(c *gin.Context) {
	id := c.Param("id")
	var nft models.PerkNFT
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

// Update Perks NFT
func UpdatePerkNFT(c *gin.Context) {
	id := c.Param("id")
	var nft models.PerkNFT
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

	c.JSON(http.StatusOK, gin.H{"message": "Perks NFT updated successfully", "nft": nft})
}

// Delete Perks NFT
func DeletePerkNFT(c *gin.Context) {
	id := c.Param("id")
	var nft models.PerkNFT
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
