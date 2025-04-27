package agents

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"fmt"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/gin-gonic/gin"
	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/agents")
	{
		g.POST("/:server_domain", addAgent)
		g.GET("/:server_domain", getAgents)
		g.GET("/:server_domain/:agentId", getAgent)
		g.DELETE("/:server_domain/:agentId", deleteAgent)
		g.PATCH("/:server_domain/:agentId", manageAgent)
		
		g.GET("/wallet/:wallet_address", getAgentsByWalletAddress)

		configGroup := g.Group("/config")
		configGroup.Use(paseto.PASETO(false))
		configGroup.GET("/:agentId", getCharacterFileByAgentId)
	}
}

func addAgent(c *gin.Context) {
	// Get multipart form
	serverDomain := c.Param("server_domain")
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
		return
	}

	// Get the file
	files := form.File["character_file"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Character file is required"})
		return
	}

	// Get wallet address from form
	walletAddress := c.PostForm("wallet_address")
	if walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}

	// Read the character file content
	file, err := files[0].Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open character file"})
		return
	}
	defer file.Close()
	
	characterFileContent, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read character file"})
		return
	}
	
	// Reset file pointer for the next read
	file.Seek(0, 0)

	// Create new multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file
	part, err := writer.CreateFormFile("character_file", files[0].Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create form file"})
		return
	}
	io.Copy(part, file)

	// Add all form fields
	formFields := []string{"domain", "avatar_img", "cover_img", "voice_model", "organization"}
	for _, field := range formFields {
		value := c.PostForm(field)
		if value != "" {
			writer.WriteField(field, value)
		}
	}

	writer.Close()

	// Forward request to upstream service
	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1.0/agents", serverDomain), body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send request to server"})
		return
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	if resp.StatusCode == http.StatusOK {
		var agentResponse struct {
			Agent struct {
				ID           string   `json:"id"`
				Name         string   `json:"name"`
				Clients      []string `json:"clients"`
				Status       string   `json:"status"`
				AvatarImg    string   `json:"avatar_img"`
				CoverImg     string   `json:"cover_img"`
				VoiceModel   string   `json:"voice_model"`
				Organization string   `json:"organization"`
			} `json:"agent"`
			Domain string `json:"domain"`
		}

		if err := json.Unmarshal(respBody, &agentResponse); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
			return
		}

		// Convert clients array to string for database storage
		clientsStr := ""
		if len(agentResponse.Agent.Clients) > 0 {
			clientsBytes, err := json.Marshal(agentResponse.Agent.Clients)
			if err == nil {
				clientsStr = string(clientsBytes)
			}
		}

		// Create agent record for database
		agent := models.Agent{
			ID:             agentResponse.Agent.ID,
			Name:           agentResponse.Agent.Name,
			Clients:        clientsStr,
			Status:         agentResponse.Agent.Status,
			AvatarImg:      agentResponse.Agent.AvatarImg,
			CoverImg:       agentResponse.Agent.CoverImg,
			VoiceModel:     agentResponse.Agent.VoiceModel,
			Organization:   agentResponse.Agent.Organization,
			WalletAddress:  walletAddress,
			ServerDomain:   serverDomain,
			Domain:         agentResponse.Domain,
			CharacterFile:  string(characterFileContent),
		}

		// Store in database
		db := dbconfig.GetDb()
		if err := db.Create(&agent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store agent in database"})
			return
		}

		// Parse the response as a generic map to modify it
		var responseMap map[string]interface{}
		if err := json.Unmarshal(respBody, &responseMap); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
			return
		}

		// Move domain inside agent object
		if agentMap, ok := responseMap["agent"].(map[string]interface{}); ok {
			if domain, ok := responseMap["domain"].(string); ok {
				agentMap["domain"] = domain
				delete(responseMap, "domain")
			}
			// Add server_domain to agent object
			agentMap["server_domain"] = serverDomain
		}

		// Convert back to JSON
		updatedResponse, err := json.Marshal(responseMap)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process response"})
			return
		}

		c.Header("Content-Type", "application/json")
		c.Writer.WriteHeader(resp.StatusCode)
		c.Writer.Write(updatedResponse)
		return
	}

	c.Header("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(respBody)
}

func getAgent(c *gin.Context) {
	serverDomain := c.Param("server_domain")
	agentId := c.Param("agentId")
	
	// Forward request to upstream service
	resp, err := http.Get(fmt.Sprintf("https://%s/api/v1.0/agents/%s", serverDomain, agentId))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agent"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

func deleteAgent(c *gin.Context) {
	serverDomain := c.Param("server_domain")
	agentId := c.Param("agentId")

	// Forward request to upstream service
	req, err := http.NewRequest("DELETE", fmt.Sprintf("https://%s/api/v1.0/agents/%s", serverDomain, agentId), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send request to server"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	if resp.StatusCode == http.StatusOK {
		// Delete from database using Unscoped to perform a hard delete
		db := dbconfig.GetDb()
		if err := db.Unscoped().Where("id = ?", agentId).Delete(&models.Agent{}).Error; err != nil {
			fmt.Printf("Error deleting agent from database: %v\n", err)
		}
	}

	c.Header("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(respBody)
}

func manageAgent(c *gin.Context) {
	serverDomain := c.Param("server_domain")
	agentId := c.Param("agentId")
	action := c.Query("action")

	// Forward request to upstream service
	req, err := http.NewRequest("PATCH", fmt.Sprintf("https://%s/api/v1.0/agents/manage/%s?action=%s", serverDomain, agentId, action), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to manage agent"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	if resp.StatusCode == http.StatusOK {
		// Update agent status in database if action is pause/resume
		if action == "pause" || action == "resume" {
			db := dbconfig.GetDb()
			status := "active"
			if action == "pause" {
				status = "inactive"
			}
			if err := db.Model(&models.Agent{}).Where("id = ?", agentId).Update("status", status).Error; err != nil {
				fmt.Printf("Error updating agent status in database: %v\n", err)
			}
		}	
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

func getAgents(c *gin.Context) {
	serverDomain := c.Param("server_domain")
	// Forward request to upstream service	
	resp, err := http.Get(fmt.Sprintf("https://%s/api/v1.0/agents", serverDomain))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agents"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

func getAgentsByWalletAddress(c *gin.Context) {
	walletAddress := c.Param("wallet_address")
	if walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}

	var agents []models.Agent
	db := dbconfig.GetDb()
	if err := db.Where("wallet_address = ?", walletAddress).Find(&agents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query agents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func getCharacterFileByAgentId(c *gin.Context) {
	agentId := c.Param("agentId")
	
	// Get wallet address from the token context
	walletAddress, exists := c.Get(paseto.CTX_WALLET_ADDRESS)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	
	walletAddressStr, ok := walletAddress.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid wallet address format in token"})
		return
	}
	
	if walletAddressStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}
	
	if agentId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Agent ID is required"})
		return
	}
	
	var agent models.Agent
	db := dbconfig.GetDb()
	if err := db.Where("wallet_address = ? AND id = ?", walletAddressStr, agentId).First(&agent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found for your wallet address"})
		return
	}
	
	// Return the character file data
	c.JSON(http.StatusOK, gin.H{
		"agent_id": agent.ID,
		"name": agent.Name,
		"character_file": agent.CharacterFile,
	})
}
