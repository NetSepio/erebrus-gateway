package p2pnode

import (
	"context"
	"time"

	p2pHost "github.com/TheLazarusNetwork/erebrus-gateway/app/p2p-Node/host"
	"github.com/TheLazarusNetwork/erebrus-gateway/app/p2p-Node/service"
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

	dht, err := p2pHost.NewDHT(ctx, ha, nil)
	if err != nil {
		panic(err)
	}
	go p2pHost.Discover(ctx, ha, dht)

	service.SubscribeTopics(ps, ha, ctx)

}
