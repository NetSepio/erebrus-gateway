package p2pnode

import (
	"context"
	"fmt"
	"time"

	p2pHost "github.com/NetSepio/erebrus-gateway/app/p2p-Node/host"
	"github.com/NetSepio/erebrus-gateway/app/p2p-Node/service"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

// DiscoveryInterval is how often we search for other peers via the DHT.
const DiscoveryInterval = time.Second * 10

// DiscoveryServiceTag is used in our DHT advertisements to discover
// other peers.
const DiscoveryServiceTag = "erebrus"

func Init() {
	ctx, _ := context.WithCancel(context.Background())

	ha := p2pHost.CreateHost()
	ps := service.NewService(ha, ctx)

	bootstrapPeers := []multiaddr.Multiaddr{}
	db := dbconfig.GetDb()

	dht, err := p2pHost.NewDHT(ctx, ha, bootstrapPeers)
	if err != nil {
		logrus.Error("failed to init new dht")
		return
	}

	go p2pHost.Discover(ctx, ha, dht)

	ticker := time.NewTicker(1 * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				var nodes []models.Node

				err := db.Model(&models.Node{}).Find(&nodes).Error
				if err != nil {
					logrus.Error("failed to fetch nodes from db")
					return
				}
				for _, node := range nodes {
					peerMultiAddr, err := multiaddr.NewMultiaddr(node.Address)
					if err != nil {
						fmt.Printf("Failed to create multiAddr for peer %s: %s\n", node.Address, err)
						continue
					}
					peerInfo, err := peer.AddrInfoFromP2pAddr(peerMultiAddr)
					if err != nil {
						panic(err)
					}
					// Attempt to connect to the peer
					if err := ha.Connect(ctx, *peerInfo); err != nil {
						fmt.Printf("Failed to connect to peer %s: %s\n", peerInfo.ID.String(), err)
						if err := db.Model(&models.Node{}).Delete(node).Error; err != nil {
							logrus.Error("failed to delete node: ", err.Error())
							continue
						}
					} else {
						fmt.Printf("Connected to peer %s\n", peerInfo.ID.String())
					}
				}

			case <-quit:
				ticker.Stop()
				return
			}
			fmt.Println("refresh")

		}
	}()

	go service.SubscribeTopics(ps, ha, ctx)
}
