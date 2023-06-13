package service

import (
	"context"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

const DiscoveryServiceTag = "erebrus"

func NewService(h host.Host) *pubsub.PubSub {
	ps, err := pubsub.NewGossipSub(context.TODO(), h)
	if err != nil {
		panic(err)
	}
	return ps
}

func SubscribeTopics(ps *pubsub.PubSub, h host.Host) {
	//Topic status
	topicString := "status" // Change "UniversalPeer" to whatever you want!
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
			msg, err := sub.Next(context.TODO())
			if err != nil {
				panic(err)
			}
			if msg.ReceivedFrom == h.ID() {
				continue
			}
			fmt.Printf("[%s] , status is: %s", msg.ReceivedFrom, string(msg.Data))
			if err := topic.Publish(context.TODO(), []byte("heres a reply from masternode")); err != nil {
				panic(err)
			}
		}
	}()
	//Topic status end

}

// send bytes through the topic
func SendMessage(messageBytes []byte, topic *pubsub.Topic) {
	if err := topic.Publish(context.TODO(), messageBytes); err != nil {
		panic(err)
	}
}
