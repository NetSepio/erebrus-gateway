package apiv1

import (
	"github.com/NetSepio/erebrus-gateway/api/v1/client"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.RouterGroup) {
	v1 := r.Group("/v1.0")
	{
		client.ApplyRoutes(v1)
	}
}
