package api

import "github.com/gin-gonic/gin"

// ok writes a success JSON payload.
func ok(c *gin.Context, code int, obj any) {
	c.JSON(code, obj)
}

// fail writes a uniform error envelope and aborts.
func fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}
