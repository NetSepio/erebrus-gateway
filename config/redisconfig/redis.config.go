package redisconfig

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

var (
	RedisClient *redis.Client
	Ctx         = context.Background()
)

func ConnectRedis() *redis.Client {

	address := os.Getenv("REDIS_ADDRESS")
	password := os.Getenv("REDIS_PASSWORD")

	RedisClient = redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password, // No password set
		DB:       0,        // Use default DB
		Protocol: 2,        // Connection protocol
	})
	return RedisClient
}

func RedisConnection() {
	r := ConnectRedis()
	if r.Ping(Ctx).Err() != nil {
		logrus.Fatal(r.Ping(Ctx).Err())
	} else {
		logrus.Infoln("REDIS CONNECTED SUCCESSFULLY")

		// Function to clear Redis data every hour
		go func(redisClient *redis.Client) {
			ticker := time.NewTicker(1 * time.Hour) // Runs every 1 hour
			defer ticker.Stop()

			ctx := context.Background()

			for range ticker.C {
				err := redisClient.FlushDB(ctx).Err() // Use FlushAll() to clear all databases
				if err != nil {
					fmt.Println("Error clearing Redis:", err)
				} else {
					fmt.Println("Redis data cleared successfully at", time.Now())
				}
			}
		}(r)
	}
}

func DecodeB64(message string) (retour string) {
	base64Text := make([]byte, base64.StdEncoding.DecodedLen(len(message)))
	base64.StdEncoding.Decode(base64Text, []byte(message))
	// fmt.Printf("base64: %s\n", base64Text)
	return string(base64Text)
}
