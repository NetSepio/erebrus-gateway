package nodelogs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/redisconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/TheLazarusNetwork/go-helpers/httpo"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/active-nodes")
	{
		g.GET("", GetActiveNodesHandler)
		// g.GET("/:status", FetchAllNodesByStatus)
		// g.GET("/status_wallet_address/:status/:wallet_address", FetchAllNodesByStatusAndWalletAddress)
		// g.GET("/nodes_details_by_wallet_adddress_and_chain", HandlerGetNodesByChainAndWallet())

	}
}

// GetActiveNodesHandler retrieves active nodes within a specific time range
func GetActiveNodesHandler(c *gin.Context) {
	peer_id := c.Query("peer_id")
	if len(peer_id) == 0 {
		httpo.NewSuccessResponse(http.StatusBadRequest, "Please pass the peer_id").SendD(c)
		return
	}
	start_time := c.Query("start_time")
	end_time := c.Query("end_time")
	var (
		startTime time.Time
		endTime   time.Time
		err       error
	)

	if len(start_time) == 0 || len(end_time) == 0 {
		endTime = time.Now()
		startTime = endTime.AddDate(0, 0, -30)
	} else {
		startTime, err = time.Parse(time.RFC3339, start_time)
		if err != nil {
			httpo.NewSuccessResponse(http.StatusBadRequest, "Invalid start_time format").SendD(c)
			return
		}
		endTime, err = time.Parse(time.RFC3339, end_time)
		if err != nil {
			httpo.NewSuccessResponse(http.StatusBadRequest, "Invalid end_time format").SendD(c)
			return
		}
	}

	activeNodes, err := GetTotalActiveDuration(peer_id, startTime, endTime)
	if err != nil {
		httpo.NewSuccessResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	httpo.NewSuccessResponseP(http.StatusOK, "active_nodes", activeNodes).SendD(c)
}

// LogNodeStatus logs the status change of a node
func LogNodeStatus(peerID string, status string) error {
	// Initialize the database and Redis client
	db := dbconfig.GetDb()
	Ctx := context.Background()
	RedisClient := redisconfig.ConnectRedis()

	// Create cache key for Redis
	cacheKey := fmt.Sprintf("node:%s:status", peerID)

	// Check if the status is available in Redis
	cachedStatus, err := RedisClient.Get(Ctx, cacheKey).Result()

	if err == redis.Nil {
		fmt.Printf("peerID : %v, status : %v \n", peerID, status)

		// If Redis does not have data for the PeerID, check the database
		var nodeLog models.NodeLog
		err := db.Where("peer_id = ?", peerID).Order("timestamp desc").First(&nodeLog).Error
		if err == gorm.ErrRecordNotFound {
			// If no entry exists in DB, insert the status change in the database
			nodeLog := models.NodeLog{
				ID:        uuid.New(), // Ensure that UUID is generated for the new log
				PeerID:    peerID,
				Status:    status,
				Timestamp: time.Now(),
			}
			if err := db.Create(&nodeLog).Error; err != nil {
				return err
			}

			// Set the status in Redis
			RedisClient.Set(Ctx, cacheKey, status, time.Hour*1) // Cache status for 1 hour
			return nil
		} else if err == nil {
			// If the node log exists but status is different, create a new log
			if nodeLog.Status != status {
				nodeLog := models.NodeLog{
					ID:        uuid.New(), // Ensure UUID is generated for the new log
					PeerID:    peerID,
					Status:    status,
					Timestamp: time.Now(),
				}
				if err := db.Create(&nodeLog).Error; err != nil {
					return err
				}
				// Set the status in Redis
				RedisClient.Set(Ctx, cacheKey, status, time.Hour*1) // Cache status for 1 hour
				return nil
			}
		} else {
			// Return any other errors encountered while querying the database
			return err
		}
	} else if err == nil && cachedStatus != status {
		// If status in Redis is different from the database, create a new log
		nodeLog := models.NodeLog{
			ID:        uuid.New(), // Ensure UUID is generated for the new log
			PeerID:    peerID,
			Status:    status,
			Timestamp: time.Now(),
		}
		if err := db.Create(&nodeLog).Error; err != nil {
			return err
		}

		// Update the status in Redis cache
		RedisClient.Set(Ctx, cacheKey, status, time.Hour*1) // Cache status for 1 hour
		return nil
	}

	// If status in Redis already matches, just update the Redis cache timestamp
	if err == nil && cachedStatus == status {
		log.Info("Data alread exists", peerID, status)
		// Reset the cache expiration time
		RedisClient.Set(Ctx, cacheKey, status, time.Hour*1)
		return nil
	}

	return nil
}

// GetActiveDuration fetches the active duration of a node between two timestamps
func GetActiveDuration(peerID string, startTime, endTime time.Time) (time.Duration, error) {
	db := dbconfig.GetDb()
	var logs []models.NodeLog
	if err := db.Where("peer_id = ? AND timestamp BETWEEN ? AND ?", peerID, startTime, endTime).
		Order("timestamp").
		Find(&logs).Error; err != nil {
		return 0, err
	}

	// Calculate total active duration
	var totalDuration time.Duration
	var lastActiveTime *time.Time
	for _, log := range logs {
		if log.Status == "active" {
			lastActiveTime = &log.Timestamp
		} else if log.Status == "inactive" && lastActiveTime != nil {
			totalDuration += log.Timestamp.Sub(*lastActiveTime)
			lastActiveTime = nil
		}
	}

	return totalDuration, nil
}

// GetActiveNodes fetches all nodes that were active during a specific time range
func GetActiveNodes(startTime, endTime time.Time) ([]string, error) {
	db := dbconfig.GetDb()
	var logs []models.NodeLog
	if err := db.Where("status = 'active' AND timestamp BETWEEN ? AND ?", startTime, endTime).
		Group("peer_id").
		Find(&logs).Error; err != nil {
		return nil, err
	}

	var activeNodes []string
	for _, log := range logs {
		activeNodes = append(activeNodes, log.PeerID)
	}

	return activeNodes, nil
}

// GetTotalActiveDuration calculates the total duration (in seconds) that a node with a given PeerID was active
func GetTotalActiveDuration(peerID string, startTime time.Time, endTime time.Time) (int64, error) {
	// Initialize the database client
	db := dbconfig.GetDb()

	// Retrieve all status logs for the given peerID within the specified date range
	var nodeLogs []models.NodeLog
	err := db.Where("peer_id = ? AND timestamp BETWEEN ? AND ?", peerID, startTime, endTime).
		Order("timestamp ASC").Find(&nodeLogs).Error
	if err != nil {
		return 0, err
	}

	// Initialize variables to track the total active duration
	var totalActiveDuration int64
	var lastStatus string
	var lastTimestamp time.Time

	// If there is only one log entry with status 'active', calculate the duration from that log entry to the current time
	if len(nodeLogs) == 1 && nodeLogs[0].Status == "active" {
		duration := time.Now().Sub(nodeLogs[0].Timestamp).Seconds()
		if duration > 0 {
			totalActiveDuration = int64(duration)
		}
		return totalActiveDuration, nil
	}

	// Iterate through the logs to calculate the total active duration
	for _, log := range nodeLogs {
		// Skip if the status is the same as the last status
		if lastStatus == log.Status {
			continue
		}

		// If the last status was "active", calculate the duration until the current timestamp
		if lastStatus == "active" && !lastTimestamp.IsZero() {
			duration := log.Timestamp.Sub(lastTimestamp).Seconds()
			if duration > 0 {
				totalActiveDuration += int64(duration)
			}
		}

		// Update the last status and timestamp
		lastStatus = log.Status
		lastTimestamp = log.Timestamp
	}

	// Return the total active duration in seconds
	return totalActiveDuration, nil
}
