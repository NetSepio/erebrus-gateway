package models

type Erebrus struct {
	UUID          string `gorm:"primary_key" json:"UUID"`
	Name          string `json:"name"`
	WalletAddress string `json:"walletAddress"`
	Region        string `json:"region"`
	CollectionId  string `json:"collectionId"`
}
