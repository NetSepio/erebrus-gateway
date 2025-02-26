package redisconfig

import (
	"context"
	"encoding/base64"
	"os"

	"github.com/redis/go-redis/v9"
	// "github.com/sirupsen/logrus"
)

var (
	RedisClient  *redis.Client
	Ctx          = context.Background()
	count        int
	testAddress  string
	testPassword string
)

func ConnectRedis() *redis.Client {

	address := os.Getenv("REDIS_ADDRESS")
	password := os.Getenv("REDIS_PASSWORD")

	
	testAddress = address
	testPassword = password

	RedisClient = redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password, // No password set
		DB:       0,        // Use default DB
		Protocol: 2,        // Connection protocol
	})
	return RedisClient
}

func RedisConnection() {
	// r := ConnectRedis()
	// if r.Ping(Ctx).Err() != nil {
	// 	logrus.Println("Redis port : ", testAddress)
	// 	logrus.Println("Redis password : ", testPassword)
	// 	logrus.Fatal(r.Ping(Ctx).Err())
	// } else {
	// 	if count != 0 {
	// 		logrus.Println("Using the inbuild redis ")
	// 	}
	// 	logrus.Infoln("REDIS CONNECTED SUCCESSFULLY")
	// }
}

func DecodeB64(message string) (retour string) {
	base64Text := make([]byte, base64.StdEncoding.DecodedLen(len(message)))
	base64.StdEncoding.Decode(base64Text, []byte(message))
	// fmt.Printf("base64: %s\n", base64Text)
	return string(base64Text)
}
