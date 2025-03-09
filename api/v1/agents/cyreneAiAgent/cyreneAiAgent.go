package cyreneAiAgent

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func ApplyRoutes(router *gin.RouterGroup) {
	routes := router.Group("/cyrene_agents")
	{
		routes.POST("/", CreateCyreneAgent)
		routes.GET("/", GetCyreneAgents)
		routes.GET("/:id", GetCyreneAgentByID)
		routes.PUT("/:id", UpdateCyreneAgent)
		routes.DELETE("/:id", DeleteCyreneAgent)
	}
}

func CreateCyreneAgent(c *gin.Context) {
	var agent models.CyreneAIAgent
	db := dbconfig.GetDb()
	if err := c.ShouldBindJSON(&agent); err != nil {
		log.Error("Invalid JSON input: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON input", "details": err.Error()})
		return
	}
	if err := db.Create(&agent).Error; err != nil {
		log.Error("Failed to create agent: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create agent", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, agent)
}

func GetCyreneAgents(c *gin.Context) {
	var agents []models.CyreneAIAgent
	db := dbconfig.GetDb()
	if err := db.Find(&agents).Error; err != nil {
		log.Error("Failed to fetch agents: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agents", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agents)
}

func GetCyreneAgentByID(c *gin.Context) {
	id := c.Param("id")
	var agent models.CyreneAIAgent
	db := dbconfig.GetDb()
	if err := db.First(&agent, "id = ? ", id).Error; err != nil {
		log.Error("Agent not found: ", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agent)
}

func UpdateCyreneAgent(c *gin.Context) {
	id := c.Param("id")
	var agent models.CyreneAIAgent
	db := dbconfig.GetDb()
	if err := db.First(&agent, "id = ?", id).Error; err != nil {
		log.Error("Agent not found: ", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found", "details": err.Error()})
		return
	}
	var updatedAgent models.CyreneAIAgent
	if err := c.ShouldBindJSON(&updatedAgent); err != nil {
		log.Error("Invalid JSON input: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON input", "details": err.Error()})
		return
	}
	if err := db.Model(&agent).Updates(updatedAgent).Error; err != nil {
		log.Error("Failed to update agent: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update agent", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agent)
}

func DeleteCyreneAgent(c *gin.Context) {
	id := c.Param("id")
	db := dbconfig.GetDb()
	if err := db.Delete(&models.CyreneAIAgent{}, "id = ?", id).Error; err != nil {
		log.Error("Failed to delete agent: ", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete agent", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Agent deleted successfully"})
}
