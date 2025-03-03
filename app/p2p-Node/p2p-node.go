package p2pnode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	nodelogs "github.com/NetSepio/erebrus-gateway/api/v1/nodes/nodeLogs"
	p2pHost "github.com/NetSepio/erebrus-gateway/app/p2p-Node/host"
	"github.com/NetSepio/erebrus-gateway/app/p2p-Node/service"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/contract"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

// DiscoveryInterval is how often we search for other peers via the DHT.
const DiscoveryInterval = time.Second * 10

// DiscoveryServiceTag is used in our DHT advertisements to discover
// other peers.
const DiscoveryServiceTag = "erebrus"

// Node status constants matching the contract's enum
const (
	StatusOffline     uint8 = 0
	StatusOnline      uint8 = 1
	StatusMaintenance uint8 = 2
	StatusDeactivated uint8 = 3
)

// Time thresholds for status changes
const (
	MaintenanceThreshold = 2 * time.Minute
	OfflineThreshold    = 5 * time.Minute
)

// OnlineURI, MaintenanceURI, and OfflineURI are constants for token URIs
const (
	OnlineURI      = "ipfs://bafkreiczwfmevybanlj73w3v2smos2qgoxsfigonmmki4aoftcgike45sq"
	MaintenanceURI = "ipfs://bafybeibil3zpj6povthugmrpwdvhgehrfpbhgkabltrrtwwfijvuguopka"
	OfflineURI     = "ipfs://bafybeicetdyf7ocbdflobb7dkw5lvwejpa6ny3x55ht4pf2cmyedgarxmu"
)

// NodeStateTracker keeps track of node states to minimize contract calls
type NodeStateTracker struct {
	ContractStatus uint8
	LastPing       time.Time
}

// Global map to track node states
var nodeStates = make(map[string]*NodeStateTracker)

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
				if err := db.Model(&models.Node{}).Find(&nodes).Error; err != nil {
					logrus.Error("failed to fetch nodes from db")
					continue
				}

				for _, node := range nodes {
					if _, exists := nodeStates[node.PeerId]; !exists {
						nodeStates[node.PeerId] = &NodeStateTracker{
							ContractStatus: StatusOffline,
							LastPing:       time.Now(),
						}
						continue // Skip first iteration for new nodes
					}

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
						err = json.Unmarshal([]byte(node.IpGeoData), &newGeoAddress)
						if err != nil {
							log.Printf("Error unmarshaling newGeoAddress from JSON : %v", err)
						}
					} else {
						City := "Test"
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

					peerMultiAddr, err := multiaddr.NewMultiaddr(node.PeerAddress)
					if err != nil {
						continue
					}
					
					peerInfo, err := peer.AddrInfoFromP2pAddr(peerMultiAddr)
					if err != nil {
						logrus.Error(err)
						continue
					}

					isConnected := ha.Connect(ctx, *peerInfo) == nil
					var newStatus uint8
					var nodeStatus string

					if !isConnected {
						timeSinceLastPing := time.Since(nodeStates[node.PeerId].LastPing)
						// Only update status if within our monitoring window
						if timeSinceLastPing <= OfflineThreshold + time.Minute {
							if timeSinceLastPing > OfflineThreshold {
								newStatus = StatusOffline
								nodeStatus = "inactive"
							} else if timeSinceLastPing > MaintenanceThreshold {
								newStatus = StatusMaintenance
								nodeStatus = "inactive"
							} else {
								continue
							}
						} else {
							continue // Skip nodes that have been offline for too long
						}
					} else {
						newStatus = StatusOnline
						nodeStatus = "active"
						nodeStates[node.PeerId].LastPing = time.Now()
					}

					// Only update contract for peaq nodes and if status has changed
					if strings.ToLower(node.Chain) == "peaq" && newStatus != nodeStates[node.PeerId].ContractStatus {
						go func(peerId string, status uint8) {
							if err := updateNodeContractStatus(peerId, status); err != nil {
								logrus.Error("failed to update contract status: ", err.Error())
								return
							}
							nodeStates[peerId].ContractStatus = status
						}(node.PeerId, newStatus)
					}

					// Update database status
					go func(n models.Node, status string) {
						n.Status = status
						if status == "active" {
							n.LastPing = time.Now().Unix()
						}
						if err := db.Save(&n).Error; err != nil {
							logrus.Error("failed to update node: ", err.Error())
						}
						nodelogs.LogNodeStatus(n.PeerId, status)
					}(node, nodeStatus)
				}

			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	go service.SubscribeTopics(ps, ha, ctx)
}

// formatNodeId adds the "did:netsepio:" prefix to the peer ID if not present
func formatNodeId(peerId string) string {
	prefix := "did:netsepio:"
	if !strings.HasPrefix(peerId, prefix) {
		return prefix + peerId
	}
	return peerId
}

func updateNodeContractStatus(nodeId string, status uint8) error {
	formattedNodeId := formatNodeId(nodeId)
	
	// Load environment variables if not already loaded
	if os.Getenv("CONTRACT_ADDRESS") == "" {
		err := godotenv.Load()
		if err != nil {
			return fmt.Errorf("Error loading .env file: %v", err)
		}
	}

	// Connect to the Ethereum client
	client, err := ethclient.Dial(os.Getenv("RPC_URL"))
	if err != nil {
		return fmt.Errorf("Failed to connect to the Ethereum client: %v", err)
	}

	// Create a new instance of the contract
	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	instance, err := contract.NewContract(contractAddress, client)
	if err != nil {
		return fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	// Create auth options for the transaction
	privateKey, err := crypto.HexToECDSA(os.Getenv("PRIVATE_KEY"))
	if err != nil {
		return fmt.Errorf("Failed to create private key: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return fmt.Errorf("Failed to get chain ID: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return fmt.Errorf("Failed to create transactor: %v", err)
	}

	// Get node details to fetch tokenId
	opts := &bind.CallOpts{
		From: auth.From,
	}
	
	node, err := instance.Nodes(opts, formattedNodeId)
	if err != nil {
		return fmt.Errorf("Failed to get node details: %v", err)
	}

	// Update node status
	tx, err := instance.UpdateNodeStatus(auth, formattedNodeId, status)
	if err != nil {
		return fmt.Errorf("Failed to update node status: %v", err)
	}

	// Get the appropriate URI based on status
	var uri string
	switch status {
	case StatusOnline:
		uri = OnlineURI
	case StatusMaintenance:
		uri = MaintenanceURI
	case StatusOffline:
		uri = OfflineURI
	default:
		return fmt.Errorf("Invalid status for URI update")
	}

	// Update the token URI
	tx, err = instance.UpdateTokenURI(auth, node.TokenId, uri)
	if err != nil {
		return fmt.Errorf("Failed to update token URI: %v", err)
	}

	logrus.Infof("Node %s status updated to %d and token URI updated to %s. Transaction hash: %s", 
		formattedNodeId, status, uri, tx.Hash().Hex())
	return nil
}
