package models

type Erebrus struct {
	UUID          string `gorm:"primary_key" json:"UUID"`
	Name          string `json:"name"`
	WalletAddress string `json:"walletAddress"`
	UserId        string `json:"userId,omitempty"`
	Region        string `json:"region"`
	NodeId        string `json:"nodeId"`
	Domain        string `json:"domain"`
	CollectionId  string `json:"collectionId"`
}
