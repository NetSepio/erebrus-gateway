package models

import (
	"time"
)

type NodeDwifi struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Gateway   string    `json:"gateway"`
	Status    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NodeDwifiResponse struct {
	ID        uint         `json:"id"`
	Gateway   string       `json:"gateway"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Status    []DeviceInfo `json:"status"`
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
