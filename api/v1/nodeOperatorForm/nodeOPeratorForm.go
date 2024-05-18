package nodeoperatorform

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/gateway/config/dbconfig"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Nodeoperatorform() {
	// Get the database connection
	db := dbconfig.GetDb()
	if db.Error != nil {
		panic(db.Error)
	}

	// Initialize Gin router
	router := gin.Default()

	// Enable CORS
	router.Use(cors.Default())

	// Handle form submission
	router.POST("/submit", func(c *gin.Context) {
		var formData models.FormData
		if err := c.ShouldBindJSON(&formData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Save the form data to the database
		result := db.Create(&formData)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Form data saved successfully"})
	})

	// GET API to retrieve form data
	router.GET("/formdata", func(c *gin.Context) {
		var formData []models.FormData
		result := db.Find(&formData)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
			return
		}

		c.JSON(http.StatusOK, formData)
	})

	// Start the server

}
