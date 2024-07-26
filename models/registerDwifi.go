package models

import "gorm.io/gorm"

type WifiNode struct {
	gorm.Model
	NodeName      string `json:"node_name"`
	Location      string `json:"location"`
	WifiPassword  string `json:"wifi_password"`
	WalletAddress string `json:"wallet_address"`
	NodeType      string `json:"node_type"` //The type of WiFi node (e.g., public, private, business).
	Ssid          string `json:"ssid"`
}
