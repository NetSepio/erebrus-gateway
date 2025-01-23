package cyrene

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/httpo"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
)

func baseURL(c *gin.Context) (string, int, error) {
	walletAddress := c.GetString(paseto.CTX_WALLET_ADDRESS)
	if len(walletAddress) == 0 {
		return "", http.StatusNotFound, errors.New("invalid wallet address : walletAddress is empty")
	}
	db := dbconfig.GetDb()
	var node models.Node

	err := db.Where("wallet_address = ?", walletAddress).First(&node).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return "", http.StatusNotFound, errors.New("failed to get wallet details : node not found")
		} else {
			return "", http.StatusInternalServerError, fmt.Errorf("error fetching node for wallet address %s: %v", walletAddress, err)

		}
	}
	fmt.Println()
	fmt.Println("node host : ", node.Host)
	fmt.Println()

	return node.Host, http.StatusOK, nil
}

func AddDeployeToNode(c *gin.Context) {
	// Retrieve the file from the form
	file, err := c.FormFile("character_file")
	if err != nil {
		httpo.NewErrorResponse(http.StatusBadRequest, fmt.Sprintf("Error: %v\n", "File is required")).SendD(c)
		return
	}

	// Retrieve the domain from the form
	domain := c.PostForm("domain")
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Domain is required"})
		httpo.NewErrorResponse(http.StatusBadRequest, fmt.Sprintf("Error: %v\n", "Domain is required")).SendD(c)
		return
	}

	// Save the file to a temporary location
	tempFilePath := fmt.Sprintf("./%s", file.Filename)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		// add logwrapper
		logwrapper.Errorf("Failed to save file: %v\n", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, fmt.Sprintf("Failed to save file: %v\n", err)).SendD(c)
		return
	}
	defer os.Remove(tempFilePath) // Clean up the temporary file

	baseURL, statusCode, err := baseURL(c)
	if err != nil && len(baseURL) == 0 {
		logwrapper.Errorf("failed to get node details by wallet address %s: %v\n", err, err)
		httpo.NewErrorResponse(statusCode, fmt.Sprintf("failed to get node details by wallet address: %v\n", err)).SendD(c)
		return
	}

	// Call the external API
	externalAPIURL := baseURL + "/api/v1.0/agents"
	response, err := uploadFileToExternalAPI(tempFilePath, domain, externalAPIURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return the response from the external API
	c.JSON(http.StatusOK, gin.H{"response": response})
}
