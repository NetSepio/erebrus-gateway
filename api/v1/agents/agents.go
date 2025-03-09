package agents

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"fmt"

	"github.com/gin-gonic/gin"
)

// ... existing code ...
func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/agents")
	{
		g.POST(":server_domain", addAgent)
		g.GET(":server_domain", getAgents)
		g.GET(":server_domain/:agentId", getAgent)
		g.DELETE(":server_domain/:agentId", deleteAgent)
		g.PATCH(":server_domain/:agentId", manageAgent)
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

	// Create new multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file
	file, err := files[0].Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process file"})
		return
	}
	defer file.Close()

	part, err := writer.CreateFormFile("character_file", files[0].Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create form file"})
		return
	}
	io.Copy(part, file)

	// Add all form fields
	formFields := []string{"domain", "avatar_img", "cover_img", "voice_model"}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send request"})
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	// Forward the response status and body
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
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
	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://%s/api/v1.0/agents/%s", serverDomain, agentId), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete agent"})
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