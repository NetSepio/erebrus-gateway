package client

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/httpo"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/erebrus")
	{
		g.Use(paseto.PASETO(false))
		g.POST("/client/:regionId", RegisterClient)
		g.GET("/clients", GetAllClients)
		g.DELETE("/client/:uuid", DeleteClient)
		g.PUT("/client/:uuid/blobId", UpdateClientBlobId)
		g.GET("/client/:uuid/blobId", GetClientBlobId)
		// g.GET("/config/:region/:uuid", GetConfig)
		// g.GET("/clients/node/:nodeId", GetClientsByNode)
	}
	r.GET("/erebrus/clients/node/:nodeId", GetClientsByNode)
}
func RegisterClient(c *gin.Context) {
	region_id := c.Param("regionId")
	db := dbconfig.GetDb()
	walletAddress := c.GetString(paseto.CTX_WALLET_ADDRESS)
	userId := c.GetString(paseto.CTX_USER_ID)
	// var count int64
	// err := db.Model(&models.Erebrus{}).Where("wallet_address = ?", walletAddress).Find(&models.Erebrus{}).Count(&count).Error
	// if err != nil {
	// 	logwrapper.Errorf("failed to fetch data from database: %s", err)
	// 	httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
	// 	return
	// }

	// if count >= 3 {
	// 	logwrapper.Error("Can't create more clients, maximum 3 allowed")
	// 	httpo.NewErrorResponse(http.StatusBadRequest, "Can't create more clients, maximum 3 allowed").SendD(c)
	// 	return
	// }
	var node *models.Node
	if err := db.Model(&models.Node{}).Where("peer_id = ?", region_id).First(&node).Error; err != nil {
		logwrapper.Errorf("failed to get node: %s", err)
		httpo.NewErrorResponse(http.StatusBadRequest, err.Error()).SendD(c)
		return
	}

	var req ClientRequest

	err := c.BindJSON(&req)
	if err != nil {
		logwrapper.Errorf("failed to bind JSON: %s", err)
		httpo.NewErrorResponse(http.StatusBadRequest, err.Error()).SendD(c)
		return
	}
	client := &http.Client{}
	data := Client{
		Name:         req.Name,
		Enable:       true,
		PresharedKey: req.PresharedKey,
		AllowedIPs:   []string{"0.0.0.0/0", "::/0"},
		Address:      []string{"10.0.0.0/24"},
		CreatedBy:    walletAddress,
		PublicKey:    req.PublicKey,
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		logwrapper.Errorf("failed to Marshal data: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	contractReq, err := http.NewRequest(http.MethodPost, node.Host+"/api/v1.0/client", bytes.NewReader(dataBytes))
	if err != nil {
		logwrapper.Errorf("failed to create	 request: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	resp, err := client.Do(contractReq)
	if err != nil {
		logwrapper.Errorf("failed to perform request: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logwrapper.Errorf("failed to read response: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	reqBody := new(Response)
	if err := json.Unmarshal(body, reqBody); err != nil {
		logwrapper.Errorf("failed to unmarshal response: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	dbEntry := models.Erebrus{
		UUID:          reqBody.Client.UUID,
		Name:          reqBody.Client.Name,
		WalletAddress: walletAddress,
		NodeId:        node.PeerId,
		Region:        node.Region,
		Domain:        node.Host,
		UserId:        userId,
		Chain:         node.Chain,
		// CollectionId:  req.CollectionId,
	}
	if err := db.Create(&dbEntry).Error; err != nil {
		logwrapper.Errorf("failed to create database entry: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	httpo.NewSuccessResponseP(200, "VPN client created successfully", gin.H{"client": reqBody.Client, "serverAddress": reqBody.Server.Address, "serverPublicKey": reqBody.Server.PublicKey, "endpoint": reqBody.Server.Endpoint}).SendD(c)
}

func GetClient(c *gin.Context) {
	uuid := c.Param("uuid")
	db := dbconfig.GetDb()

	var cl *models.Erebrus
	if err := db.Model(&models.Erebrus{}).Where("UUID = ?", uuid).First(&cl).Error; err != nil {
		logwrapper.Errorf("failed to fetch data from database: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	resp, err := http.Get(cl.Domain + "/api/v1.0/client/" + uuid)
	if err != nil {
		logwrapper.Errorf("failed to create	 request: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logwrapper.Errorf("failed to read response: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	resBody := new(Response)
	if err := json.Unmarshal(body, resBody); err != nil {
		logwrapper.Errorf("failed to unmarshal response: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	httpo.NewSuccessResponseP(200, "VPN client fetched successfully", resBody.Client).SendD(c)
}

func DeleteClient(c *gin.Context) {
	uuid := c.Param("uuid")
	db := dbconfig.GetDb()

	var cl *models.Erebrus
	err := db.Model(&models.Erebrus{}).Where("UUID = ?", uuid).First(&cl).Error
	if err != nil {
		logwrapper.Errorf("failed to fetch data from database: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	client := &http.Client{}
	contractReq, err := http.NewRequest(http.MethodDelete, cl.Domain+"/api/v1.0/client", bytes.NewReader(nil))
	if err != nil {
		logwrapper.Errorf("failed to create	 request: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	resp, err := client.Do(contractReq)
	if err != nil {
		logwrapper.Errorf("failed to perform request: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	defer resp.Body.Close()

	if err := db.Delete(cl).Error; err != nil {
		logwrapper.Errorf("failed to delete data from database: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	httpo.NewSuccessResponse(200, "VPN client deletes successfully").SendD(c)
}

func GetConfig(c *gin.Context) {
	uuid := c.Param("uuid")
	db := dbconfig.GetDb()

	var cl *models.Erebrus
	err := db.Model(&models.Erebrus{}).Where("UUID = ?", uuid).First(&cl).Error
	if err != nil {
		logwrapper.Errorf("failed to fetch data from database: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	resp, err := http.Get(cl.Domain + "/api/v1.0/client/" + uuid + "/config")
	if err != nil {
		logwrapper.Errorf("failed to create	request: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	defer resp.Body.Close()

	c.Header("Content-Disposition", "attachment; filename="+cl.Name+".conf")
	c.Header("Content-Type", resp.Header.Get("Content-Type"))

	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Writer.WriteHeader(200)
}

func GetClientsByRegion(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	region := c.Param("region")

	db := dbconfig.GetDb()
	var clients *[]models.Erebrus
	db.Model(&models.Erebrus{}).Where("user_id = ? and region = ?", userId, region).Find(&clients)

	httpo.NewSuccessResponseP(200, "VPN client fetched successfully", clients).SendD(c)
}
func GetClientsByCollectionRegion(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	region := c.Param("region")
	collection_id := c.Param("collection_id")

	db := dbconfig.GetDb()
	var clients *[]models.Erebrus
	db.Model(&models.Erebrus{}).Where("user_id = ? and region = ? and collection_id = ?", userId, region, collection_id).Find(&clients)

	httpo.NewSuccessResponseP(200, "VPN clients fetched successfully", clients).SendD(c)
}
func GetAllClients(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)

	region := c.Query("region")
	// collectionID := c.Query("collection_id")

	db := dbconfig.GetDb()
	query := db.Model(&models.Erebrus{}).Where("user_id = ?", userId)

	if region != "" {
		query = query.Where("region = ?", region)
	}
	// if collectionID != "" {
	// 	query = query.Where("collection_id = ?", collectionID)
	// }

	var clients *[]models.Erebrus
	query.Find(&clients)

	httpo.NewSuccessResponseP(200, "VPN client fetched successfully", clients).SendD(c)
}

func GetClientsByCollectionId(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	collection_id := c.Param("collection_id")

	db := dbconfig.GetDb()
	var clients *[]models.Erebrus
	db.Model(&models.Erebrus{}).Where("user_id = ? and collection_id = ?", userId, collection_id).Find(&clients)

	httpo.NewSuccessResponseP(200, "VPN clients fetched successfully", clients).SendD(c)
}

func GetClientsByNode(c *gin.Context) {
	nodeId := c.Param("nodeId")
	db := dbconfig.GetDb()

	var clients []models.Erebrus
	err := db.Model(&models.Erebrus{}).Where("node_id = ?", nodeId).Find(&clients).Error
	if err != nil {
		logwrapper.Errorf("failed to fetch clients from database: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	// Ensure that an empty slice is sent if no clients are found
	if clients == nil {
		clients = []models.Erebrus{}
	}

	httpo.NewSuccessResponseP(200, "VPN clients fetched successfully", clients).SendD(c)
}

func UpdateClientBlobId(c *gin.Context) {
	clientUUID := c.Param("uuid")
	db := dbconfig.GetDb()

	var req struct {
		BlobId string `json:"blobId" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		logwrapper.Errorf("failed to bind JSON: %s", err)
		httpo.NewErrorResponse(http.StatusBadRequest, err.Error()).SendD(c)
		return
	}

	// Validate UUID
	if _, err := uuid.Parse(clientUUID); err != nil {
		httpo.NewErrorResponse(http.StatusBadRequest, "Invalid UUID").SendD(c)
		return
	}

	// Update the client with the new blobId
	result := db.Model(&models.Erebrus{}).Where("UUID = ?", clientUUID).Update("blob_id", req.BlobId)
	if result.Error != nil {
		logwrapper.Errorf("failed to update client blobId: %s", result.Error)
		httpo.NewErrorResponse(http.StatusInternalServerError, result.Error.Error()).SendD(c)
		return
	}

	if result.RowsAffected == 0 {
		httpo.NewErrorResponse(http.StatusNotFound, "Client not found").SendD(c)
		return
	}

	httpo.NewSuccessResponse(200, "Client blobId updated successfully").SendD(c)
}

func GetClientBlobId(c *gin.Context) {
	clientUUID := c.Param("uuid")
	db := dbconfig.GetDb()

	// Validate UUID
	if _, err := uuid.Parse(clientUUID); err != nil {
		httpo.NewErrorResponse(http.StatusBadRequest, "Invalid UUID").SendD(c)
		return
	}

	var client models.Erebrus
	result := db.Model(&models.Erebrus{}).Select("blob_id").Where("UUID = ?", clientUUID).First(&client)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			httpo.NewErrorResponse(http.StatusNotFound, "Client not found").SendD(c)
		} else {
			logwrapper.Errorf("failed to fetch client blobId: %s", result.Error)
			httpo.NewErrorResponse(http.StatusInternalServerError, result.Error.Error()).SendD(c)
		}
		return
	}

	if client.BlobId == "" {
		httpo.NewErrorResponse(http.StatusNotFound, "BlobId not set for this client").SendD(c)
		return
	}

	httpo.NewSuccessResponseP(200, "Client blobId fetched successfully", gin.H{"blobId": client.BlobId}).SendD(c)
}

func ClientDelete() {
	logwrapper.Log.Infoln("🚀 Starting ClientDelete process")

	db := dbconfig.GetDb().Debug()
	var results []models.Erebrus

	// Calculate the time 24 hours ago
	cutoff := time.Now().Add(-24 * time.Hour)
	logwrapper.Log.Infoln("🕒 Cutoff time for deletion: %s\n", cutoff)

	// Fetch records older than 24 hours with name 'app'
	logwrapper.Log.Infoln("🔍 Fetching clients eligible for auto-delete")
	if err := db.Where("created_at < ? AND LOWER(name) = ?", cutoff, "app").
		Find(&results).Error; err != nil {
		logwrapper.Errorf("❌ Failed to fetch clients for auto-delete: %s", err)
		return
	}

	logwrapper.Infof("📋 Number of clients found for deletion: %d\n", len(results))

	if len(results) > 0 {
		for _, v := range results {
			logwrapper.Infof("👤 Processing client - UUID: %s, Domain: %s", v.UUID, v.Domain)

			url := fmt.Sprintf("%s/api/v1.0/client/%s", v.Domain, v.UUID)
			logwrapper.Infof("🌐 DELETE request URL: %s", url)

			urlReq, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(nil))
			if err != nil {
				logwrapper.Errorf("⚙️ Error creating DELETE request: %s", err)
				continue
			}

			client := &http.Client{}
			logwrapper.Info("📡 Sending DELETE request")
			resp, err := client.Do(urlReq)
			if err != nil {
				logwrapper.Errorf("🚫 Error making DELETE request: %s\n", err)
				continue
			}
			defer resp.Body.Close()

			logwrapper.Infof("📬 Received response - Status: %s\n", resp.Status)

			if resp.StatusCode == http.StatusOK {
				logwrapper.Infof("✅ DELETE request successful for UUID: %s. Deleting from database...\n", v.UUID)
				if err := db.Delete(&v).Error; err != nil {
					logwrapper.Errorf("🛑 Failed to delete client from database: %s\n", err)
					continue
				}
				logwrapper.Infof("🗑️ Successfully deleted client UUID: %s from database\n", v.UUID)
			} else {
				logwrapper.Warnf("⚠️ DELETE request failed for UUID: %s with status code: %d\n", v.UUID, resp.StatusCode)
			}
		}
	} else {
		logwrapper.Info("ℹ️ No clients found for auto-delete")
	}

	logwrapper.Info("🏁 ClientDelete process completed\n")
}

func AutoClientDelete() {
	// Run the function once at startup if needed
	ClientDelete()

	// Set up a ticker to run every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run the function every hour in a goroutine
	go func() {
		for range ticker.C {
			ClientDelete()
		}
	}()

	// Keep the main function running
	select {}
}

func deleteRecords(db *sql.DB) {
	// SQL query to delete the records
	query := `
		DELETE FROM erebrus
		WHERE domain IN (
			SELECT DISTINCT host
			FROM erebrus
			JOIN nodes ON nodes.peer_id != erebrus.node_id
		);
	`

	// Execute the query
	_, err := db.Exec(query)
	if err != nil {
		log.Fatalf("Error executing DELETE query: %v", err)
	} else {
		log.Println("Records deleted successfully")
	}
}

func runEverySunday(db *sql.DB) {
	for {
		// Check if today is Sunday
		now := time.Now()
		if now.Weekday() == time.Sunday {
			// Call the deleteRecords function to perform the delete
			deleteRecords(db)

			// Sleep for 24 hours to avoid running it multiple times on the same Sunday
			time.Sleep(24 * time.Hour)
		} else {
			// Sleep for 1 hour and check again if it's Sunday
			time.Sleep(time.Hour)
		}
	}
}
