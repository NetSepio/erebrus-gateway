package apiv1

import (
	"github.com/NetSepio/erebrus-gateway/api/status"
	"github.com/NetSepio/erebrus-gateway/api/v1/client"
	"github.com/NetSepio/erebrus-gateway/api/v1/nodeOperatorForm"
	"github.com/NetSepio/erebrus-gateway/api/v1/nodes"
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
		nodeOperatorForm.ApplyRoutes(v1)
	}
}
