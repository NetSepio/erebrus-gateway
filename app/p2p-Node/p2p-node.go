package p2pnode

import (
	"context"

	"github.com/TheLazarusNetwork/erebrus-gateway/app/p2p-Node/host"
)

func Init() {
	h := host.CreateHost()
	dht, err := host.NewDHT(context.TODO(), h, nil)
	if err != nil {
		panic(err)
	}

	// Setup global peer discovery over DiscoveryServiceTag.
	go host.Discover(context.TODO(), h, dht)
}
