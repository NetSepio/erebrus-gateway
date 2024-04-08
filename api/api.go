package api

import (
	"github.com/NetSepio/erebrus-gateway/api/status"
	"github.com/NetSepio/erebrus-gateway/api/v1/client"
	"github.com/NetSepio/erebrus-gateway/api/v1/nodes"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		status.ApplyRoutes(api)
		client.ApplyRoutes(api)
		nodes.ApplyRoutes(api)
	}
}
