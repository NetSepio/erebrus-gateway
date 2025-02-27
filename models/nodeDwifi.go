package models

import (
	"time"
)

type NodeDwifi struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	Gateway        string    `json:"gateway"`
	Status         string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Password       string    `json:"password"`
	Location       string    `json:"location"`
	Price_per_min  string    `json:"price_per_min"`
	Wallet_address string    `json:"wallet_address"`
	Chain_name     string    `json:"chain_name"`
	Co_ordinates   string    `json:"co_ordinates"`
}

type NodeDwifiResponse struct {
	ID             uint         `json:"id"`
	Gateway        string       `json:"gateway"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Status         []DeviceInfo `json:"status"`
	Password       string       `json:"password"`
	Location       string       `json:"location"`
	Price_per_min  string       `json:"price_per_min"`
	Wallet_address string       `json:"wallet_address"`
	Chain_name     string       `json:"chain_name"`
	Co_ordinates   string       `json:"co_ordinates"`
}

type DeviceInfo struct {
	MACAddress         string        `json:"macAddress"`
	IPAddress          string        `json:"ipAddress"`
	ConnectedAt        time.Time     `json:"connectedAt"`
	TotalConnectedTime time.Duration `json:"totalConnectedTime"`
	Connected          bool          `json:"connected"`
	LastChecked        time.Time     `json:"lastChecked"`
	DefaultGateway     string        `json:"defaultGateway"`
	Manufacturer       string        `json:"manufacturer"`
	InterfaceName      string        `json:"interfaceName"`
	HostSSID           string        `json:"hostSSID"`
}
