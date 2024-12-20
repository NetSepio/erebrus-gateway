package nodelogs

import (
	"context"
	"fmt"
	"time"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/redisconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	log "github.com/sirupsen/logrus"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

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
		fmt.Printf("peerID : %v, status : %v ", peerID, status)

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
