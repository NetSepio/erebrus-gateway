package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"gorm.io/gorm"
)

const DiscoveryServiceTag = "erebrus"

type status struct {
	Status string
}

func NewService(h host.Host, ctx context.Context) *pubsub.PubSub {
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}
	return ps
}

var Status_data []*Status
var StatusData map[string]*Status

func SubscribeTopics(ps *pubsub.PubSub, h host.Host, ctx context.Context) {
	// Initialize StatusData map
	StatusData = make(map[string]*Status)
	topicString := "status"
	topic, err := ps.Join(DiscoveryServiceTag + "/" + topicString)
	if err != nil {
		panic(err)
	}
	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			// Block until we recieve a new message.
			msg, err := sub.Next(ctx)
			if err != nil {
				panic(err)
			}
			fmt.Println("recieved")
			if msg.ReceivedFrom == h.ID() {
				continue
			}
			var node *models.Node
			if err := json.Unmarshal(msg.Data, &node); err != nil {
				panic(err)
			}
			db := dbconfig.GetDb()

			err = CreateOrUpdate(db, node)
			if err != nil {
				logwrapper.Error("failed to update db: ", err.Error())
			}
			if err := topic.Publish(ctx, []byte("Gateway recieved the node information")); err != nil {
				panic(err)
			}
			fmt.Println("here already")

			topic.EventHandler()
		}
	}()
	// topicString2 := "client"
	// topic2, err := ps.Join(DiscoveryServiceTag + "/" + topicString2)
	// if err != nil {
	// 	panic(err)
	// }

	// sub2, err := topic2.Subscribe()
	// if err != nil {
	// 	panic(err)
	// }

	// go func() {
	// 	for {
	// 		// Block until we recieve a new message.
	// 		msg, err := sub2.Next(ctx)
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 		if msg.ReceivedFrom == h.ID() {
	// 			continue
	// 		}
	// 		fmt.Printf("[%s] , status isz: %s", msg.ReceivedFrom, string(msg.Data))
	// 		if err := topic2.Publish(ctx, []byte("heres a reply from client")); err != nil {
	// 			panic(err)
	// 		}
	// 	}
	// }()

}

func CreateOrUpdate(db *gorm.DB, node *models.Node) error {
	var model models.Node

	result := db.Model(&models.Node{}).Where("id = ?", node.Id)
	if result.RowsAffected != 0 {
		//exists, update
		return db.Model(&model).Updates(node).Error
	} else {
		//create
		return db.Create(node).Error
	}
}
