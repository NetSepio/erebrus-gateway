package p2pnode

import (
	"context"
	"fmt"
	"time"

	p2pHost "github.com/NetSepio/erebrus-gateway/app/p2p-Node/host"
	"github.com/NetSepio/erebrus-gateway/app/p2p-Node/service"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
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

	var nodes []models.Node

	err := db.Model(&models.Node{}).Find(&nodes).Error
	if err != nil {
		logrus.Error("failed to fetch nodes from db")
		return
	}
	for _, node := range nodes {
		// Parse multiaddress string
		addr, err := multiaddr.NewMultiaddr(node.Address)
		if err != nil {
			fmt.Printf("Error parsing multiaddress %s: %s\n", node.Address, err)
			continue // Skip to next address if parsing fails
		}

		// Append parsed multiaddress to the slice
		bootstrapPeers = append(bootstrapPeers, addr)
	}
	dht, err := p2pHost.NewDHT(ctx, ha, bootstrapPeers)
	if err != nil {
		logrus.Error("failed to init new dht")
		return
	}

	go p2pHost.Discover(ctx, ha, dht)

	go func() {
		for _, addr := range peerAddrs {
			peer, err := h.Peerstore().AddrInfo(addr)
			if err != nil {
				fmt.Printf("Failed to create AddrInfo for peer %s: %s\n", addr.String(), err)
				continue
			}

			// Attempt to connect to the peer
			_, err = h.Network().DialPeer(ctx, peer.ID)
			if err != nil {
				if err == network.ErrNoAddresses {
					fmt.Printf("Peer %s is unreachable\n", peer.ID.Pretty())
				} else {
					fmt.Printf("Failed to connect to peer %s: %s\n", peer.ID.Pretty(), err)
				}
				continue
			}

			// If connection is successful, the peer is alive
			fmt.Printf("Peer %s is alive\n", peer.ID.Pretty())

			// Close the connection
			h.Network().ClosePeer(peer.ID)
		}
	}()
	go service.SubscribeTopics(ps, ha, ctx)
}
