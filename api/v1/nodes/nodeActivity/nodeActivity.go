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

// func CalculateTotalAndTodayActiveDuration(peerID string) (totalDurationHr, todayDurationHr float64) {
// 	db := dbconfig.GetDb()
// 	now := time.Now()

// 	// Fetch all activities for the given peer
// 	var activities []models.NodeActivity
// 	if err := db.Where("peer_id = ?", peerID).Find(&activities).Error; err != nil {
// 		logwrapper.Errorf("failed to fetch activities for peer_id %s: %s", peerID, err)
// 		return
// 	}

// 	var (
// 		totalDuration int
// 		todayDuration int
// 	)

// 	for _, activity := range activities {
// 		// Calculate total active duration
// 		if activity.EndTime != nil {
// 			totalDuration += int(activity.EndTime.Sub(activity.StartTime).Seconds())
// 		} else {
// 			// If the node is still active, calculate the duration until now
// 			totalDuration += int(now.Sub(activity.StartTime).Seconds())
// 		}

// 		// Calculate today's active duration
// 		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
// 		if activity.EndTime != nil {
// 			// If the activity occurred today and ended today
// 			if activity.StartTime.After(startOfDay) {
// 				todayDuration += int(activity.EndTime.Sub(activity.StartTime).Seconds())
// 			}
// 		} else {
// 			// If the node is still active and the activity is today
// 			if activity.StartTime.After(startOfDay) {
// 				todayDuration += int(now.Sub(activity.StartTime).Seconds())
// 			}
// 		}

// 		// If todayDuration is still zero, calculate it as the duration from the start of today to now
// 		if todayDuration == 0 {
// 			startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
// 			todayDuration = int(now.Sub(startOfDay).Seconds())
// 		}
// 	}

// 	return math.Round((float64(totalDuration)/3600)*100) / 100, math.Round((float64(todayDuration)/3600)*100) / 100
// }

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

	// Get the start of today (midnight)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for _, activity := range activities {
		// Calculate total active duration
		if activity.EndTime != nil {
			totalDuration += int(activity.EndTime.Sub(activity.StartTime).Seconds())
		} else {
			// If the node is still active, calculate the duration until now
			totalDuration += int(now.Sub(activity.StartTime).Seconds())
		}

		// Calculate today's active duration
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

	// If todayDuration is still zero, calculate it as the duration from the start of today to now
	if todayDuration == 0 {
		todayDuration = int(now.Sub(startOfDay).Seconds())
	}

	// Convert seconds to hours and round to two decimal places
	totalDurationHr = math.Round((float64(totalDuration)/3600)*100) / 100
	todayDurationHr = math.Round((float64(todayDuration)/3600)*100) / 100

	return totalDurationHr, todayDurationHr
}

func TrackNodeActivity(peerID string, isActive bool) {
	db := dbconfig.GetDb()
	now := time.Now()

	var activity models.NodeActivity
	// Try to fetch an existing record with NULL end_time (active node)
	err := db.Where("peer_id = ?", peerID).
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
				PeerID:              peerID,
				StartTime:           now,
				LastActiveStartTime: &now, // Track when the node became active
			}
			if err := db.Create(&newActivity).Error; err != nil {
				logwrapper.Errorf("failed to create new node activity: %s", err)
			}
		} else {
			// If node was already active, continue the duration
			logwrapper.Infof("Node with peer_id %s is already active", peerID)
			// Update the LastActiveStartTime if required
			if activity.LastActiveStartTime == nil {
				activity.LastActiveStartTime = &now
				if err := db.Save(&activity).Error; err != nil {
					logwrapper.Errorf("failed to update node activity: %s", err)
				}
			}
		}
	} else {
		// If the node becomes inactive
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			// Update the activity record with the end time and duration
			duration := int(now.Sub(activity.StartTime).Seconds()) // Duration in seconds
			activity.EndTime = &now
			activity.DurationSeconds += duration // Add to total duration

			// Reset LastActiveStartTime when the node is inactive
			activity.LastActiveStartTime = nil

			if err := db.Save(&activity).Error; err != nil {
				logwrapper.Errorf("failed to update node activity: %s", err)
			}
		} else {
			// If there's no active record and the node is inactive, log the event
			logwrapper.Infof("No active record found for peer_id %s to mark as inactive", peerID)
		}
	}
}
