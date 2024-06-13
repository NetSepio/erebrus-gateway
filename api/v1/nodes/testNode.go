package nodes

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
)

func toJSON(data interface{}) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

// Helper function to convert JSON string to struct
func fromJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

// CreateNode creates a new node record in the database
func CreateNode(node *models.Node) error {
	DB := dbconfig.GetDb()
	return DB.Create(node).Error
}

// GetNodeByID retrieves a node record from the database by ID
func GetNodeByID(id string) (models.Node, error) {
	var (
		node  models.Node
		v     models.Node
		nodes models.Node
	)
	DB := dbconfig.GetDb()
	err := DB.First(&node, "peer_id = ?", id).Error
	if err != nil {
		return v, err
	}
	// Unmarshal SystemInfo into OSInfo struct
	var osInfo models.OSInfo
	err = json.Unmarshal([]byte(node.SystemInfo), &osInfo)
	if err != nil {
		fmt.Println("Error unmarshaling SystemInfo:", err)
		return nodes, nil
	}

	// Unmarshal IpInfo into IPInfo struct
	var ipInfo models.IPInfo
	err = json.Unmarshal([]byte(node.IpInfo), &ipInfo)
	if err != nil {
		fmt.Println("Error unmarshaling IpInfo:", err)
		return nodes, nil
	}

	// Create Node struct from Node struct
	nodes = models.Node{
		PeerId:           node.PeerId,
		Name:             node.Name,
		HttpPort:         node.HttpPort,
		Host:             node.Host,
		PeerAddress:      node.PeerAddress,
		Region:           node.Region,
		Status:           node.Status,
		DownloadSpeed:    node.DownloadSpeed,
		UploadSpeed:      node.UploadSpeed,
		RegistrationTime: node.RegistrationTime,
		LastPing:         node.LastPing,
		Chain:            node.Chain,
		WalletAddress:    node.WalletAddress,
		Version:          node.Version,
		CodeHash:         node.CodeHash,
		SystemInfo:       fmt.Sprintf("%+v\n", osInfo),
		IpInfo:           fmt.Sprintf("%+v\n", ipInfo),
	}

	// Print Node struct
	fmt.Printf("%+v\n", nodes)
	return nodes, nil
}

// UpdateNode updates an existing node record in the database
func UpdateNode(node *models.Node) error {
	DB := dbconfig.GetDb()
	return DB.Save(node).Error
}

// DeleteNode deletes a node record from the database
func DeleteNode(id string) error {
	DB := dbconfig.GetDb()
	return DB.Delete(&models.Node{}, "where peer_id = ?",id).Error
}

func getRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func TestMain() {
	// DB := dbconfig.GetDb()

	// Example: Create a new node with SystemInfo and IpInfo
	systemInfo := models.OSInfo{Name: "ExampleOS", Hostname: "localhost", Architecture: "x86_64", NumCPU: 4}
	ipInfo := models.IPInfo{IPv4Addresses: []string{"192.168.1.1", "192.168.1.2"}, IPv6Addresses: []string{"::1"}}
	newNode := &models.Node{
		PeerId:           getRandomString(10),
		Name:             getRandomString(5),
		HttpPort:         fmt.Sprintf("%d", rand.Intn(65535)),
		Host:             fmt.Sprintf("host%d.example.com", rand.Intn(100)),
		PeerAddress:      fmt.Sprintf("192.168.1.%d", rand.Intn(255)),
		Region:           getRandomString(5),
		Status:           fmt.Sprintf("%d", rand.Intn(4)+1),
		DownloadSpeed:    rand.Float64() * 100,
		UploadSpeed:      rand.Float64() * 100,
		RegistrationTime: time.Now().Unix(),
		LastPing:         time.Now().Unix(),
		Chain:            getRandomString(5),
		WalletAddress:    getRandomString(34),
		Version:          fmt.Sprintf("v%d.%d.%d", rand.Intn(10), rand.Intn(10), rand.Intn(10)),
		CodeHash:         getRandomString(64),
		SystemInfo:       toJSON(systemInfo),
		IpInfo:           toJSON(ipInfo),
		// Populate other fields as needed
	}
	err := CreateNode(newNode)
	if err != nil {
		panic(err)
	}

}

func GetNodes() {
	newNode := models.Node{}
	newNode.PeerId = "MWqenTWG5k"
	retrievedNode, err := GetNodeByID(newNode.PeerId)
	if err != nil {
		panic(err)
	}

	fmt.Println("Retrieved Node:")
	fmt.Sprintf("%+v\n", retrievedNode)

}
func Update() {
	// Example: Update the node
	newNode := &models.Node{}
	newNode.Name = "UpdatedNode"
	err := UpdateNode(newNode)
	if err != nil {
		panic(err)
	}
}
func Delete() {
	// Example: Update the node
	retrievedNode := &models.Node{}
	// Example: Delete the node
	err := DeleteNode(retrievedNode.PeerId)
	if err != nil {
		panic(err)
	}
}
