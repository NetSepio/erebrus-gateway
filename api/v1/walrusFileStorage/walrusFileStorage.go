package walrusFileStorage

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/walrusFileStorage")
	{
		g.POST("", upsertWalrusStorage)
		g.DELETE("/:address", deleteWalrusStorage)
		g.DELETE("/:address/blob/:blob_id", deleteBlobID)
		g.GET("/:address", getAllWalrusStorage)
	}
}

func upsertWalrusStorage(c *gin.Context) {
	var walrus models.WalrusStorage
	if err := c.ShouldBindJSON(&walrus); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := dbconfig.GetDb()

	var existingWalrus models.WalrusStorage
	result := db.Where("wallet_address = ?", walrus.WalletAddress).First(&existingWalrus)

	if result.Error == nil {
		existingWalrus.FileBlobs = append(existingWalrus.FileBlobs, walrus.FileBlobs...)
		if err := db.Save(&existingWalrus).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update wallet"})
			return
		}
		c.JSON(http.StatusOK, existingWalrus)
	} else if result.Error == gorm.ErrRecordNotFound {
		if err := db.Create(&walrus).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})
			return
		}
		c.JSON(http.StatusCreated, walrus)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
	}
}

func deleteBlobID(c *gin.Context) {
	walletAddress := c.Param("address")
	blobID := c.Param("blob_id")
	db := dbconfig.GetDb()

	var walrus models.WalrusStorage
	if err := db.Where("wallet_address = ?", walletAddress).First(&walrus).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}

	index := -1
	for i, fileObj := range walrus.FileBlobs {
		if fileObj.BlobID == blobID {
			index = i
			break
		}
	}

	if index == -1 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Blob ID not found"})
		return
	}

	walrus.FileBlobs = append(walrus.FileBlobs[:index], walrus.FileBlobs[index+1:]...)

	if err := db.Save(&walrus).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update wallet"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Blob ID removed successfully"})
}

func deleteWalrusStorage(c *gin.Context) {
	address := c.Param("address")
	db := dbconfig.GetDb()

	var walrus models.WalrusStorage
	if err := db.Where("wallet_address = ?", address).First(&walrus).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}

	if err := db.Delete(&walrus).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete wallet"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Wallet deleted successfully"})
}

func getAllWalrusStorage(c *gin.Context) {
	address := c.Param("address")
	db := dbconfig.GetDb()

	var walrus models.WalrusStorage
	if err := db.Where("wallet_address = ?", address).First(&walrus).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve wallet"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"file_blobs": walrus.FileBlobs})
}
