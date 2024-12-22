package redisconfig

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
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

	if len(address) == 0 || len(password) == 0 {
		count++
		address = DecodeB64("MzUuMjA5LjE5OC4xNjo2Mzc5")
		// password = DecodeB64("STkwNjFVNjI4NlFKQ1BOME0=")
		password = "I9061U6286QJCPN0M"

		fmt.Printf("address : %v and password : %v \n", address, password)
	}
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
	r := ConnectRedis()
	if r.Ping(Ctx).Err() != nil {
		logrus.Println("Redis port : ", testAddress)
		logrus.Println("Redis password : ", testPassword)
		logrus.Fatal(r.Ping(Ctx).Err())
	} else {
		if count != 0 {
			logrus.Println("Using the inbuild redis ")
		}
		logrus.Infoln("REDIS CONNECTED SUCCESSFULLY")
	}
}

func DecodeB64(message string) (retour string) {
	base64Text := make([]byte, base64.StdEncoding.DecodedLen(len(message)))
	base64.StdEncoding.Decode(base64Text, []byte(message))
	// fmt.Printf("base64: %s\n", base64Text)
	return string(base64Text)
}
