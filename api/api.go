package api

import (
	apiv1 "github.com/NetSepio/erebrus-gateway/api/v1"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		apiv1.ApplyRoutes(api)
	}
}
