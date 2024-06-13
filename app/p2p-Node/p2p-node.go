package p2pnode

import (
	"context"
	"encoding/json"
	"log"
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

	ticker := time.NewTicker(10 * time.Second)
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

				// fmt.Println("nodes : ", len(nodes))

				for _, node := range nodes {

					var (
						newOSInfo     models.OSInfo
						newGeoAddress models.IpGeoAddress
						newIPInfo     models.IPInfo
					)

					err = json.Unmarshal([]byte(node.SystemInfo), &newOSInfo)
					if err != nil {
						log.Printf("Error unmarshaling newOSInfo from JSON: %v", err)
					}

					if len(node.IpGeoData) > 0 {
						// fmt.Println("node.IpGeoData : ", node.IpGeoData)
						err = json.Unmarshal([]byte(node.IpGeoData), &newGeoAddress)
						if err != nil {
							log.Printf("Error unmarshaling newGeoAddress from JSON : %v", err)
						}
					} else {
						// IP := "150.129.168.46"
						City := "Test"
						// Region := "Maharashtra"
						Country := "Test"
						Location := "Test"
						Organization := "Test"
						Postal := "Test"
						Timezone := "Test"

						newGeoAddress.IpInfoCity = City
						newGeoAddress.IpInfoCountry = Country
						newGeoAddress.IpInfoLocation = Location
						newGeoAddress.IpInfoOrg = Organization
						newGeoAddress.IpInfoPostal = Postal
						newGeoAddress.IpInfoTimezone = Timezone
					}
					err = json.Unmarshal([]byte(node.IpInfo), &newIPInfo)
					if err != nil {
						log.Printf("Error unmarshaling newGeoAddress from JSON p2p-node.go: %v", err)
					}

					node.SystemInfo = models.ToJSON(newOSInfo)
					node.IpGeoData = models.ToJSON(newGeoAddress)
					node.IpInfo = models.ToJSON(newIPInfo)

					// fmt.Printf("%+v\n", node.IpGeoData)

					peerMultiAddr, err := multiaddr.NewMultiaddr(node.PeerAddress)
					if err != nil {
						continue
					}
					peerInfo, err := peer.AddrInfoFromP2pAddr(peerMultiAddr)
					if err != nil {
						logrus.Error(err)
						continue
						// log.Println(node)
					}
					// Attempt to connect to the peer
					if err := ha.Connect(ctx, *peerInfo); err != nil {
						node.Status = "inactive"
						if err := db.Model(&models.Node{}).Where("peer_id = ?", node.PeerId).Save(&node).Error; err != nil {
							logrus.Error("failed to update node: ", err.Error())
							continue
						}
						lastPingTime := time.Unix(node.LastPing, 0)
						duration := time.Since(lastPingTime)
						threshold := 48 * time.Hour
						if duration > threshold {
							if err := db.Where("peer_id = ?", node.PeerId).Delete(&models.Node{}).Error; err != nil {
								logrus.Error("failed to delete nodes: ", err.Error())
								continue
							}
						}
					} else {
						node.Status = "active"
						node.LastPing = time.Now().Unix()
						if err := db.Model(&models.Node{}).Where("peer_id = ?", node.PeerId).Save(&node).Error; err != nil {
							logrus.Error("failed to update node: ", err.Error())
							continue
						}
					}
				}

			case <-quit:
				ticker.Stop()
				return
			}

		}
	}()

	go service.SubscribeTopics(ps, ha, ctx)
}
