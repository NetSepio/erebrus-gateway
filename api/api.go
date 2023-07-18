package api

import (
	"github.com/TheLazarusNetwork/erebrus-gateway/api/status"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		status.ApplyRoutes(api)
	}
}
