package p2pnode

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
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
	OfflineThreshold     = 5 * time.Minute
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
				if err := db.Debug().Model(&models.Node{}).Find(&nodes).Error; err != nil {
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

					// Safely unmarshal JSON data with error handling
					if node.SystemInfo != "" {
						err = json.Unmarshal([]byte(node.SystemInfo), &newOSInfo)
						if err != nil {
							log.Printf("Error unmarshaling newOSInfo from JSON: %v", err)
						}
					}

					if node.IpGeoData != "" && len(node.IpGeoData) > 0 {
						err = json.Unmarshal([]byte(node.IpGeoData), &newGeoAddress)
						if err != nil {
							log.Printf("Error unmarshaling newGeoAddress from JSON: %v", err)
							// Set default values if unmarshal fails
							newGeoAddress = models.IpGeoAddress{
								IpInfoCity:     "Unknown",
								IpInfoCountry:  "Unknown",
								IpInfoLocation: "Unknown",
								IpInfoOrg:      "Unknown",
								IpInfoPostal:   "Unknown",
								IpInfoTimezone: "Unknown",
							}
						}
					} else {
						// Set default values if IpGeoData is empty
						newGeoAddress = models.IpGeoAddress{
							IpInfoCity:     "Unknown",
							IpInfoCountry:  "Unknown",
							IpInfoLocation: "Unknown",
							IpInfoOrg:      "Unknown",
							IpInfoPostal:   "Unknown",
							IpInfoTimezone: "Unknown",
						}
					}

					if node.IpInfo != "" {
						err = json.Unmarshal([]byte(node.IpInfo), &newIPInfo)
						if err != nil {
							log.Printf("Error unmarshaling newIPInfo from JSON: %v", err)
						}
					}

					node.SystemInfo = models.ToJSON(newOSInfo)
					node.IpGeoData = models.ToJSON(newGeoAddress)
					node.IpInfo = models.ToJSON(newIPInfo)

					peerMultiAddr, err := multiaddr.NewMultiaddr(node.PeerAddress)
					if err != nil {
						logrus.Errorf("Invalid peer address for node %s: %v", node.PeerId, err)
						continue
					}

					peerInfo, err := peer.AddrInfoFromP2pAddr(peerMultiAddr)
					if err != nil {
						logrus.Errorf("Failed to get peer info for node %s: %v", node.PeerId, err)
						continue
					}

					isConnected := ha.Connect(ctx, *peerInfo) == nil
					var newStatus uint8
					var nodeStatus string

					if !isConnected {
						timeSinceLastPing := time.Since(nodeStates[node.PeerId].LastPing)
						// Only update status if within our monitoring window
						if timeSinceLastPing <= OfflineThreshold+time.Minute {
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

					// Update contract status only for peaq nodes
					logrus.Debugf("Chain: %s, Node: %s, status: %s", strings.ToLower(node.Chain), node.PeerId, nodeStatus)
					logrus.Debugf("newStatus: %d, nodeStates[node.PeerId].ContractStatus: %d", newStatus, nodeStates[node.PeerId].ContractStatus)

					if strings.ToLower(node.Chain) == "peaq" && newStatus != nodeStates[node.PeerId].ContractStatus {
						go func(peerId string, status uint8) {
							// Update contract status
							logrus.Info("Updating contract status for node", peerId)
							if err := updateNodeContractStatus(peerId, status); err != nil {
								logrus.Errorf("Failed to update contract status: %v", err)
								return
							}
							nodeStates[peerId].ContractStatus = status
						}(node.PeerId, newStatus)
					}

					// Update database status
					go func(n models.Node, status string) {
						logrus.Info("Updating node status in db for node", n.PeerId)
						n.Status = status
						if status == "active" {
							n.LastPing = time.Now().Unix()
						}
						if err := db.Debug().Save(&n).Error; err != nil {
							logrus.Errorf("Failed to update node: %v", err)
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
// func formatNodeId(peerId string) string {
// 	prefix := "did:netsepio:"
// 	if !strings.HasPrefix(peerId, prefix) {
// 		return prefix + peerId
// 	}
// 	return peerId
// }

func updateNodeContractStatus(nodeId string, status uint8) error {
	// Get node data from database to check chain
	db := dbconfig.GetDb()
	var node models.Node
	if err := db.Debug().Model(&models.Node{}).Where("peer_id = ?", nodeId).First(&node).Error; err != nil {
		return fmt.Errorf("failed to fetch node from db: %v", err)
	}
	
	// Only proceed if chain is peaq
	if strings.ToLower(node.Chain) != "peaq" {
		logrus.Infof("Skipping contract update for non-peaq node: %s (chain: %s)", nodeId, node.Chain)
		return nil
	}

	// formattedNodeId := formatNodeId(nodeId)
	formattedNodeId := nodeId


	// Load environment variables if not already loaded
	if os.Getenv("CONTRACT_ADDRESS") == "" {
		err := godotenv.Load()
		if err != nil {
			return fmt.Errorf("error loading .env file: %v", err)
		}
	}

	// Connect to the Ethereum client
	client, err := ethclient.Dial(os.Getenv("RPC_URL"))
	if err != nil {
		return fmt.Errorf("failed to connect to the Ethereum client: %v", err)
	}

	// Create a new instance of the contract
	contractAddress := common.HexToAddress(os.Getenv("CONTRACT_ADDRESS"))
	instance, err := contract.NewContract(contractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to instantiate contract: %v\n", err)
	}

	// Create auth options for the transaction
	privateKey, err := crypto.HexToECDSA(os.Getenv("PRIVATE_KEY"))
	if err != nil {
		return fmt.Errorf("failed to create private key: %v\n", err)
	}

	// Add retry mechanism for getting chain ID
	var chainID *big.Int
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		chainID, err = client.ChainID(context.Background())
		if err == nil {
			break
		}
		
		if i < maxRetries-1 {
			// Exponential backoff
			delay := time.Duration(1<<uint(i)) * time.Second
			logrus.Warnf("Failed to get chain ID, retrying in %v: %v", delay, err)
			time.Sleep(delay)
		}
	}
	
	if err != nil {
		return fmt.Errorf("failed to get chain ID after %d attempts: %v", maxRetries, err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return fmt.Errorf("failed to create transactor: %v", err)
	}

	// Get node details to fetch tokenId
	opts := &bind.CallOpts{
		From: auth.From,
		Context: context.Background(),
	}

	// Check if node exists before trying to update
	contractNode, err := instance.Nodes(opts, formattedNodeId)
	if err != nil {
		return fmt.Errorf("Failed to get node details from contract: %v", err)
	}
	
	// Check if tokenId is valid
	if contractNode.TokenId == nil || contractNode.TokenId.Cmp(big.NewInt(0)) == 0 {
		logrus.Warnf("Node %s exists in database but not in contract or has invalid token ID", formattedNodeId)
		// Instead of returning error, we'll try to register the node
		return fmt.Errorf("Invalid token ID for node %s", formattedNodeId)
	}

	// Set gas limit and price
	auth.GasLimit = 300000
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return fmt.Errorf("Failed to get gas price: %v", err)
	}
	// Increase gas price by 20% to ensure transaction goes through
	auth.GasPrice = new(big.Int).Mul(gasPrice, big.NewInt(120))
	auth.GasPrice = new(big.Int).Div(auth.GasPrice, big.NewInt(100))

	logrus.Infof("Updating node %s status to %d", formattedNodeId, status)
	
	// Update node status
	tx, err := instance.UpdateNodeStatus(auth, formattedNodeId, status)
	if err != nil {
		return fmt.Errorf("Failed to update node status: %v", err)
	}

	logrus.Infof("Status update transaction sent: %s", tx.Hash().Hex())

	// Wait for transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		return fmt.Errorf("Failed to wait for status update transaction: %v", err)
	}
	
	if receipt.Status == 0 {
		return fmt.Errorf("Status update transaction failed")
	}

	logrus.Infof("Status update transaction confirmed for node %s", formattedNodeId)

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

	// Create a new auth for the second transaction
	auth, err = bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		return fmt.Errorf("failed to create transactor for URI update: %v", err)
	}
	
	auth.GasLimit = 300000
	auth.GasPrice = new(big.Int).Mul(gasPrice, big.NewInt(120))
	auth.GasPrice = new(big.Int).Div(auth.GasPrice, big.NewInt(100))

	logrus.Infof("Updating token URI for node %s to %s", formattedNodeId, uri)
	
	// Update the token URI
	tx, err = instance.UpdateTokenURI(auth, contractNode.TokenId, uri)
	if err != nil {
		return fmt.Errorf("Failed to update token URI: %v", err)
	}

	logrus.Infof("URI update transaction sent: %s", tx.Hash().Hex())

	// Wait for transaction to be mined
	receipt, err = bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		return fmt.Errorf("Failed to wait for URI update transaction: %v", err)
	}
	
	if receipt.Status == 0 {
		return fmt.Errorf("URI update transaction failed")
	}

	logrus.Infof("Node %s status updated to %d and token URI updated to %s",
		formattedNodeId, status, uri)
	return nil
}
