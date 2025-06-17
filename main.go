package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/NetSepio/erebrus-gateway/api"
	"github.com/NetSepio/erebrus-gateway/api/v1/client"
	"github.com/NetSepio/erebrus-gateway/app"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/redisconfig"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load()
	redisconfig.RedisConnection()
}

func main() {

	logwrapper.Init()
	app.Init()
	ginApp := gin.Default()
	if err := dbconfig.DbMigrations(); err != nil {
		logwrapper.Errorf("Error mirating to database: %v", err)
	}

	if os.Getenv("GIN_MODE") == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// cors middleware
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = []string{"Authorization", "Content-Type"}
	ginApp.Use(cors.New(config))

	// adding health check
	ginApp.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "pong"})
	})

	ginApp.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"status": 404, "message": "Invalid Endpoint Request"})
	})
	go client.AutoClientDelete()

	api.ApplyRoutes(ginApp)
	ginApp.Run(":" + os.Getenv("HTTP_PORT"))

	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")
}
