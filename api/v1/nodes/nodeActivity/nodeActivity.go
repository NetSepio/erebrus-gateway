package nodeactivity

import (
	"errors"
	"math"
	"time"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"gorm.io/gorm"
)

func CalculateTotalAndTodayActiveDuration(peerID string) (totalDurationHr, todayDurationHr float64) {
	db := dbconfig.GetDb()
	now := time.Now()

	// Fetch all activities for the given peer
	var activities []models.NodeActivity
	if err := db.Where("peer_id = ?", peerID).Find(&activities).Error; err != nil {
		logwrapper.Errorf("failed to fetch activities for peer_id %s: %s", peerID, err)
		return
	}

	var (
		totalDuration int
		todayDuration int
	)

	for _, activity := range activities {
		// Calculate total active duration
		if activity.EndTime != nil {
			totalDuration += int(activity.EndTime.Sub(activity.StartTime).Seconds())
		} else {
			// If the node is still active, calculate the duration until now
			totalDuration += int(now.Sub(activity.StartTime).Seconds())
		}

		// Calculate today's active duration
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if activity.EndTime != nil {
			// If the activity occurred today and ended today
			if activity.StartTime.After(startOfDay) {
				todayDuration += int(activity.EndTime.Sub(activity.StartTime).Seconds())
			}
		} else {
			// If the node is still active and the activity is today
			if activity.StartTime.After(startOfDay) {
				todayDuration += int(now.Sub(activity.StartTime).Seconds())
			}
		}
	}

	// Convert totalDuration and todayDuration from seconds to hours
	// totalDurationHr = float64(totalDuration) / 3600
	// todayDurationHr = float64(todayDuration) / 3600

	// math.Round(i.TotalActiveDuration*100) / 100

	// float64(totalDuration) / 3600

	// float64(todayDuration) / 3600

	return math.Round((float64(totalDuration)/3600)*100) / 100, math.Round((float64(todayDuration)/3600)*100) / 100
}

func TrackNodeActivity(peerID string, isActive bool) {
	db := dbconfig.GetDb()
	now := time.Now()

	var activity models.NodeActivity
	// Try to fetch an existing record with NULL end_time (active node)
	err := db.Where("peer_id = ? AND end_time IS NULL", peerID).
		First(&activity).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logwrapper.Errorf("failed to fetch node activity: %s", err)
		return
	}

	if isActive {
		// If the node is active, check if an active record exists
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No active record, create a new one
			newActivity := models.NodeActivity{
				PeerID:    peerID,
				StartTime: now,
			}
			if err := db.Create(&newActivity).Error; err != nil {
				logwrapper.Errorf("failed to create new node activity: %s", err)
			}
		} else {
			// If node was already active, update the existing record
			logwrapper.Infof("Node with peer_id %s is already active", peerID)
			// Optionally, you can update any other fields if needed
		}
	} else {
		// If the node becomes inactive
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Update the activity record with the end time and duration
			duration := int(now.Sub(activity.StartTime).Seconds()) // Duration in seconds
			activity.EndTime = &now
			activity.DurationSeconds = duration

			if err := db.Save(&activity).Error; err != nil {
				logwrapper.Errorf("failed to update node activity: %s", err)
			}
		} else {
			// If there's no active record and the node is inactive, log the event
			logwrapper.Infof("No active record found for peer_id %s to mark as inactive", peerID)
		}
	}
}
