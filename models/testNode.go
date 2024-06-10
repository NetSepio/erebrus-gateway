package models

import (
	"encoding/json"
	"fmt"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
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
func CreateNode(node *TestNode) error {
	DB := dbconfig.GetDb()
	return DB.Create(node).Error
}

// GetNodeByID retrieves a node record from the database by ID
func GetNodeByID(id string) (*Node, error) {
	var node Node
	DB := dbconfig.GetDb()
	err := DB.First(&node, id).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// UpdateNode updates an existing node record in the database
func UpdateNode(node *Node) error {
	DB := dbconfig.GetDb()
	return DB.Save(node).Error
}

// DeleteNode deletes a node record from the database
func DeleteNode(id string) error {
	DB := dbconfig.GetDb()
	return DB.Delete(&Node{}, id).Error
}

func testMain() {
	// DB := dbconfig.GetDb()

	// Example: Create a new node with SystemInfo and IpInfo
	systemInfo := OSInfo{Name: "ExampleOS", Hostname: "localhost", Architecture: "x86_64", NumCPU: 4}
	ipInfo := IPInfo{IPv4Addresses: []string{"192.168.1.1", "192.168.1.2"}, IPv6Addresses: []string{"::1"}}
	newNode := &TestNode{
		Name:       "ExampleNode",
		SystemInfo: toJSON(systemInfo),
		IpInfo:     toJSON(ipInfo),
		// Populate other fields as needed
	}
	err := CreateNode(newNode)
	if err != nil {
		panic(err)
	}

	// Example: Retrieve the node by ID
	retrievedNode, err := GetNodeByID(newNode.PeerId)
	if err != nil {
		panic(err)
	}
	fmt.Println("Retrieved Node:", retrievedNode)

	// Example: Update the node
	retrievedNode.Name = "UpdatedNode"
	err = UpdateNode(retrievedNode)
	if err != nil {
		panic(err)
	}

	// Example: Delete the node
	err = DeleteNode(retrievedNode.PeerId)
	if err != nil {
		panic(err)
	}

}
