package api

import (
	"encoding/json"
	"net/http"
	"sync"
	// "time"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			// Allow all connections by default
			return true
		},
	}

	subscribers = make(map[*websocket.Conn]bool)
	mutex       = &sync.Mutex{}
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/nodedwifi")
	{
		g.GET("/all", FetchAllNodeDwifi)
		g.GET("/stream", StreamNodeDwifi)
	}

	// Start the CheckForUpdates function in a separate goroutine
	go CheckForUpdates()
}

func FetchAllNodeDwifi(c *gin.Context) {
	db := dbconfig.GetDb()
	var nodeDwifis []models.NodeDwifi

	if err := db.Find(&nodeDwifis).Error; err != nil {
		logwrapper.Errorf("failed to get NodeDwifi from DB: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch NodeDwifi data"})
		return
	}

	var responses []models.NodeDwifiResponse

	for _, nd := range nodeDwifis {
		var deviceInfos []models.DeviceInfo
		if len(nd.Status) > 0 {
			err := json.Unmarshal([]byte(nd.Status), &deviceInfos)
			if err != nil {
				logwrapper.Errorf("failed to unmarshal NodeDwifi Status: %s", err)
				continue
			}
		}

		response := models.NodeDwifiResponse{
			ID:             nd.ID,
			Gateway:        nd.Gateway,
			CreatedAt:      nd.CreatedAt,
			UpdatedAt:      nd.UpdatedAt,
			Status:         deviceInfos,
			Password:       nd.Password,
			Location:       nd.Location,
			Price_per_min:  nd.Price_per_min,
			Wallet_address: nd.Wallet_address,
			Chain_name:     nd.Chain_name,
			Co_ordinates:   nd.Co_ordinates,
		}

		responses = append(responses, response)
	}

	c.JSON(http.StatusOK, gin.H{"data": responses})
}

func StreamNodeDwifi(c *gin.Context) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logwrapper.Errorf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	mutex.Lock()
	subscribers[conn] = true
	mutex.Unlock()

	// Listen for WebSocket closure
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			mutex.Lock()
			delete(subscribers, conn)
			mutex.Unlock()
			break
		}
	}
}

func CheckForUpdates() {
	// db := dbconfig.GetDb()
	// for {
	// 	var nodeDwifis []models.NodeDwifi
	// 	if err := db.Find(&nodeDwifis).Error; err != nil {
	// 		logwrapper.Errorf("Error fetching updates: %v", err)
	// 		continue
	// 	}

	// 	for _, nd := range nodeDwifis {
	// 		var deviceInfos []models.DeviceInfo
	// 		if len(nd.Status) > 0 {
	// 			err := json.Unmarshal([]byte(nd.Status), &deviceInfos)
	// 			if err != nil {
	// 				logwrapper.Errorf("failed to unmarshal NodeDwifi Status: %s", err)
	// 				continue
	// 			}
	// 		}

	// 		response := models.NodeDwifiResponse{
	// 			ID:             nd.ID,
	// 			Gateway:        nd.Gateway,
	// 			CreatedAt:      nd.CreatedAt,
	// 			UpdatedAt:      nd.UpdatedAt,
	// 			Status:         deviceInfos,
	// 			Password:       nd.Password,
	// 			Location:       nd.Location,
	// 			Price_per_min:  nd.Price_per_min,
	// 			Wallet_address: nd.Wallet_address,
	// 			Chain_name:     nd.Chain_name,
	// 			Co_ordinates:   nd.Co_ordinates,
	// 		}

	// 		mutex.Lock()
	// 		for conn := range subscribers {
	// 			err := conn.WriteJSON(response)
	// 			if err != nil {
	// 				logwrapper.Errorf("error writing to WebSocket: %v", err)
	// 				conn.Close()
	// 				delete(subscribers, conn)
	// 			}
	// 		}
	// 		mutex.Unlock()
	// 	}

	// 	time.Sleep(1 * time.Second)
	// }
}
