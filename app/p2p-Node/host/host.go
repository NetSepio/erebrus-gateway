package host

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/NetSepio/erebrus-gateway/app/p2p-Node/pkey"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

// DiscoveryServiceTag is used in our DHT advertisements to discover
// other peers.
const DiscoveryServiceTag = "erebrus"

// DiscoveryInterval is how often we search for other peers via the DHT.
const DiscoveryInterval = time.Second * 10

func getHostAddress(ha host.Host) string {
	// Build host multiaddress
	hostAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s", ha.ID()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := ha.Addrs()[0]
	return addr.Encapsulate(hostAddr).String()
}

func CreateHost() host.Host {
	privk, err := pkey.LoadIdentity("identity.key")
	if err != nil {
		log.Fatal(err)
	}
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/9001"),
		libp2p.Identity(privk),
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		log.Fatal(err)
	}

	//fullAddr := getHostAddress(host)
	log.Printf("I am %s\n", host.Addrs())
	log.Printf("I am %s\n", getHostAddress(host))

	return host
}

// NewDHT attempts to connect to a bunch of bootstrap peers and returns a new DHT.
// If you don't have any bootstrapPeers, you can use dht.DefaultBootstrapPeers
// or an empty list.
func NewDHT(ctx context.Context, host host.Host, bootstrapPeers []multiaddr.Multiaddr) (*dht.IpfsDHT, error) {
	kdht, err := dht.New(ctx, host)
	if err != nil {
		return nil, err
	}

	if err = kdht.Bootstrap(ctx); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	// loop through bootstrapPeers (if any), and attempt to connect to them
	for _, peerAddr := range bootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)

		wg.Add(1)
		go func() {
			defer wg.Done()
			host.Connect(ctx, *peerinfo)
		}()
	}
	wg.Wait()

	return kdht, nil
}

func Discover(ctx context.Context, h host.Host, dht *dht.IpfsDHT) {
	var routingDiscovery = routing.NewRoutingDiscovery(dht)

	// Advertise our addresses on rendezvous
	discovery.Advertise(ctx, routingDiscovery, DiscoveryServiceTag)
	// // Search for peers every DiscoveryInterval
	// ticker := time.NewTicker(DiscoveryInterval)
	// quit := make(chan struct{})
	// fmt.Println("ok")

	// go func() {
	// 	for {
	// 		select {
	// 		case <-ticker.C:
	// 			fmt.Println("ok")
	// 			// Search for other peers advertising on rendezvous and
	// 			// connect to them.
	// 			peers, err := discovery.FindPeers(ctx, routingDiscovery, DiscoveryServiceTag)
	// 			if err != nil {
	// 				panic(err)
	// 			}

	// 			for _, p := range peers {
	// 				if p.ID == h.ID() {
	// 					fmt.Println("self")
	// 					continue
	// 				}
	// 				fmt.Println("here")
	// 				if h.Network().Connectedness(p.ID) != network.Connected {
	// 					_, err = h.Network().DialPeer(ctx, p.ID)
	// 					if err != nil {
	// 						fmt.Printf("Failed to connect to peer (%s): %s", p.ID, err.Error())

	// 						fmt.Println()
	// 						continue
	// 					}
	// 					fmt.Println("Connected to peer", p.ID.String())
	// 				}
	// 			}
	// 		case <-quit:
	// 			ticker.Stop()
	// 			return
	// 		}

	// 	}
	// }()

}
