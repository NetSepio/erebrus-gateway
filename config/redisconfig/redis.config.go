package redisconfig

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

var (
	RedisClient *redis.Client
	Ctx         = context.Background()
)

func ConnectRedis() *redis.Client {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDRESS"),
		Password: os.Getenv("REDIS_PASSWORD"), // No password set
		DB:       0,                           // Use default DB
		Protocol: 2,                           // Connection protocol
	})
	return RedisClient
}

func RedisConnection() {
	r := ConnectRedis()
	if r.Ping(Ctx).Err() != nil {
		logrus.Println("Redis port : ", os.Getenv("REDIS_PORT"))
		logrus.Println("Redis password : ", os.Getenv("REDIS_PASSWORD"))
		logrus.Fatal(r.Ping(Ctx).Err())
	} else {
		logrus.Infoln("REDIS CONNECTED SUCCESSFULLY")
	}
}
