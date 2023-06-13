package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	gateway()
}

// DiscoveryInterval is how often we search for other peers via the DHT.
const DiscoveryInterval = time.Second * 10

// DiscoveryServiceTag is used in our DHT advertisements to discover
// other peers.
const DiscoveryServiceTag = "erebrus"

func gateway() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create a new libp2p Host
	ha, err := makeBasicHost()
	if err != nil {
		log.Fatal(err)
	}
	fullAddr := getHostAddress(ha)
	log.Printf("I am %s\n", fullAddr)

	// Create a new PubSub service using the GossipSub router.
	ps, err := pubsub.NewGossipSub(ctx, ha)
	if err != nil {
		panic(err)
	}

	// Setup DHT with empty discovery peers so this will be a discovery peer for other
	// peers. This peer should run with a public ip address, otherwise change "nil" to
	// a list of peers to bootstrap with.
	dht, err := NewDHT(ctx, ha, nil)
	if err != nil {
		panic(err)
	}

	// Setup global peer discovery over DiscoveryServiceTag.
	go Discover(ctx, ha, dht, DiscoveryServiceTag)

	// Join a PubSub topic.
	// topicString := "status" // Change "UniversalPeer" to whatever you want!
	// topic, err := ps.Join(DiscoveryServiceTag + "/" + topicString)
	// if err != nil {
	// 	panic(err)
	// }
	topicString2 := "client" // Change "UniversalPeer" to whatever you want!
	topic2, err := ps.Join(DiscoveryServiceTag + "/" + topicString2)
	if err != nil {
		panic(err)
	}
	sub2, err := topic2.Subscribe()
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
			if msg.ReceivedFrom == ha.ID() {
				continue
			}
			fmt.Printf("[%s] , status is: %s", msg.ReceivedFrom, string(msg.Data))
			if err := topic2.Publish(ctx, []byte("heres a reply from masternode")); err != nil {
				panic(err)
			}
		}
	}()

	// Subscribe to the topic.
	// sub, err := topic.Subscribe()
	// if err != nil {
	// 	panic(err)
	// }
	// type status struct {
	// 	Status string
	// }
	// go func() {
	// 	for {
	// 		// Block until we recieve a new message.
	// 		msg, err := sub.Next(ctx)
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 		status := new(status)
	// 		err = json.Unmarshal(msg.Data, status)
	// 		if err != nil {
	// 			fmt.Println(err)
	// 		}
	// 		if msg.ReceivedFrom == ha.ID() {
	// 			continue
	// 		}
	// 		fmt.Printf("[%s] , status is: %s", msg.ReceivedFrom, string(msg.Data))
	// 		fmt.Println()

	// 		// if err := topic.Publish(ctx, []byte("heres a reply from masternode")); err != nil {
	// 		// 	panic(err)
	// 		// }
	// 	}

	// }()
	// wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Received signal, shutting down...")

}

// makeBasicHost creates a LibP2P host with a random peer ID listening on the
// given multiaddress. It won't encrypt the connection if insecure is true.
func makeBasicHost() (host.Host, error) {
	privk, err := LoadIdentity("identity.key")
	if err != nil {
		panic(err)
	}
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/9001"),
		libp2p.Identity(privk),
	}

	return libp2p.New(opts...)
}

func getHostAddress(ha host.Host) string {
	// Build host multiaddress
	hostAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s", ha.ID().Pretty()))

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addr := ha.Addrs()[0]
	return addr.Encapsulate(hostAddr).String()
}

// NewDHT attempts to connect to a bunch of bootstrap peers and returns a new DHT.
// If you don't have any bootstrapPeers, you can use dht.DefaultBootstrapPeers
// or an empty list.
func NewDHT(ctx context.Context, host host.Host, bootstrapPeers []multiaddr.Multiaddr) (*dht.IpfsDHT, error) {
	var options []dht.Option

	// if no bootstrap peers, make this peer act as a bootstraping node
	// other peers can use this peers ipfs address for peer discovery via dht
	if len(bootstrapPeers) == 0 {
		options = append(options, dht.Mode(dht.ModeServer))
	}

	// set our DiscoveryServiceTag as the protocol prefix so we can discover
	// peers we're interested in.
	options = append(options, dht.ProtocolPrefix("/"+DiscoveryServiceTag))

	kdht, err := dht.New(ctx, host, options...)
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
			if err := host.Connect(ctx, *peerinfo); err != nil {
				fmt.Printf("Error while connecting to node %q: %-v", peerinfo, err)
				fmt.Println()
			} else {
				fmt.Printf("Connection established with bootstrap node: %q", *peerinfo)
				fmt.Println()
			}
		}()
	}
	wg.Wait()

	return kdht, nil
}

// Search the DHT for peers, then connect to them.
func Discover(ctx context.Context, h host.Host, dht *dht.IpfsDHT, rendezvous string) {
	var routingDiscovery = routing.NewRoutingDiscovery(dht)

	// Advertise our addresses on rendezvous
	discovery.Advertise(ctx, routingDiscovery, rendezvous)

	// Search for peers every DiscoveryInterval
	ticker := time.NewTicker(DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:

			// Search for other peers advertising on rendezvous and
			// connect to them.
			peers, err := discovery.FindPeers(ctx, routingDiscovery, rendezvous)
			if err != nil {
				panic(err)
			}

			for _, p := range peers {
				if p.ID == h.ID() {
					continue
				}
				if h.Network().Connectedness(p.ID) != network.Connected {
					_, err = h.Network().DialPeer(ctx, p.ID)
					if err != nil {
						fmt.Printf("Failed to connect to peer (%s): %s", p.ID, err.Error())
						fmt.Println()
						continue
					}
					fmt.Println("Connected to peer", p.ID.Pretty())
				}
			}
		}
	}
}

// GenerateIdentity writes a new random private key to the given path.
func GenerateIdentity(path string) (crypto.PrivKey, error) {
	privk, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		return nil, err
	}

	bytes, err := crypto.MarshalPrivateKey(privk)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(path, bytes, 0400)

	return privk, err
}

// ReadIdentity reads a private key from the given path.
func ReadIdentity(path string) (crypto.PrivKey, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(bytes)
}

// LoadIdentity reads a private key from the given path and, if it does not
// exist, generates a new one.
func LoadIdentity(path string) (crypto.PrivKey, error) {
	if _, err := os.Stat(path); err == nil {
		return ReadIdentity(path)
	} else if os.IsNotExist(err) {
		fmt.Printf("Generating peer identity in %s\n", path)
		return GenerateIdentity(path)
	} else {
		return nil, err
	}
}
