package apiv1

import (
	"github.com/NetSepio/erebrus-gateway/api/status"
	"github.com/NetSepio/erebrus-gateway/api/v1/client"
	nodedwifi "github.com/NetSepio/erebrus-gateway/api/v1/nodeDwifi"

	"github.com/NetSepio/erebrus-gateway/api/v1/nodes"
	"github.com/NetSepio/erebrus-gateway/api/v1/registerDwifi"
	"github.com/NetSepio/erebrus-gateway/api/v1/subscription"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.RouterGroup) {
	v1 := r.Group("/v1.0")
	{
		status.ApplyRoutes(v1)
		client.ApplyRoutes(v1)
		nodes.ApplyRoutes(v1)
		subscription.ApplyRoutes(v1)

		registerDwifi.ApplyRoutes(v1)
		nodedwifi.ApplyRoutes(v1)
		walrus.ApplyRoutes(v1)
		services.ApplyRoutes(v1)
	}
}
