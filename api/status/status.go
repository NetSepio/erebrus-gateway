package status

import (
	"net/http"

	"github.com/NetSepio/erebrus-gateway/app/p2p-Node/service"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/status")
	{
		g.GET("", GetStatus)
	}
}

func GetStatus(c *gin.Context) {
	length := len(service.Status_data)
	if length == 0 {
		c.JSON(http.StatusOK, gin.H{"data": "no data"})
		return
	}
	c.JSON(http.StatusOK, service.Status_data)
}
