package client

import "github.com/gin-gonic/gin"

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/status")
	{
		g.GET("", GetStatus)
	}
}
