package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	p2pHost "github.com/TheLazarusNetwork/erebrus-gateway/app/p2p-Node/host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

const DiscoveryServiceTag = "erebrus"

type status struct {
	Status string
}

func Init(h host.Host, ctx context.Context) {
	ps := NewService(h, ctx)
	dht, err := p2pHost.NewDHT(ctx, h, nil)
	if err != nil {
		panic(err)
	}

	// Setup global peer discovery over DiscoveryServiceTag.
	go p2pHost.Discover(ctx, h, dht)
	SubscribeTopics(ps, h, ctx)
}
func NewService(h host.Host, ctx context.Context) *pubsub.PubSub {
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}
	return ps
}

func SubscribeTopics(ps *pubsub.PubSub, h host.Host, ctx context.Context) {
	//Topic status
	topicString := "status"
	topic, err := ps.Join(DiscoveryServiceTag + "/" + topicString)
	fmt.Println(topic)
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
			fmt.Println(msg)
			if err != nil {
				panic(err)
			}
			if msg.ReceivedFrom == h.ID() {
				continue
			}
			st := new(status)
			err = json.Unmarshal(msg.Data, st)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("[from %s] , status is: %s", msg.ReceivedFrom, st.Status)
			fmt.Println()
			// if err := topic.Publish(context.TODO(), []byte("from masternode: got status")); err != nil {
			// 	panic(err)
			// }
		}
	}()
	//Topic status end

	//Topic status
	topicString2 := "client"
	topic2, err := ps.Join(DiscoveryServiceTag + "/" + topicString2)
	if err != nil {
		panic(err)
	}
	sub2, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			// Block until we recieve a new message.
			msg, err := sub2.Next(ctx)
			if err != nil {
				panic(err)
			}
			// if msg.ReceivedFrom == h.ID() {
			// 	continue
			// }
			fmt.Printf("[%s] , status is: %s", msg.ReceivedFrom, string(msg.Data))
		}
	}()
	//Topic status end
	SendMessage([]byte("some string"), topic2, ctx)

	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")
}

// send bytes through the topic
func SendMessage(messageBytes []byte, topic *pubsub.Topic, ctx context.Context) {
	if err := topic.Publish(ctx, messageBytes); err != nil {
		panic(err)
	}
	fmt.Println("sent msg")
}
