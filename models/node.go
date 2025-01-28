package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type NodeResponse struct {
	Id                  string  `json:"id" gorm:"primaryKey"`
	Name                string  `json:"name"`
	HttpPort            string  `json:"httpPort"`
	Domain              string  `json:"domain"`
	NodeName            string  `json:"nodename"`
	Chain               string  `json:"chainName"`
	Address             string  `json:"address"`
	Region              string  `json:"region"`
	Status              string  `json:"status"`
	DownloadSpeed       float64 `json:"downloadSpeed"`
	UploadSpeed         float64 `json:"uploadSpeed"`
	StartTimeStamp      int64   `json:"startTimeStamp"`
	LastPingedTimeStamp int64   `json:"lastPingedTimeStamp"`
	WalletAddressSui    string  `json:"walletAddress"`
	WalletAddressSolana string  `json:"walletAddressSol"`
	IpInfoIP            string  `json:"ipinfoip"`
	IpInfoCity          string  `json:"ipinfocity"`
	IpInfoCountry       string  `json:"ipinfocountry"`
	IpInfoLocation      string  `json:"ipinfolocation"`
	IpInfoOrg           string  `json:"ipinfoorg"`
	IpInfoPostal        string  `json:"ipinfopostal"`
	IpInfoTimezone      string  `json:"ipinfotimezone"`
	TotalActiveDuration float64 `json:"totalUptime"`
	// TodayActiveDuration float64 `json:"todayUpTime"`
	UptimeUnit string `json:"upTimeUnit"`
}

func ToJSON(data interface{}) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

type Node struct {
	//using for db operation
	PeerId           string  `json:"peerId" gorm:"primaryKey"`
	Name             string  `json:"name"`
	HttpPort         string  `json:"httpPort"`
	Host             string  `json:"host"` //domain
	PeerAddress      string  `json:"peerAddress"`
	Region           string  `json:"region"`
	Status           string  `json:"status"` // offline 1, online 2, maintainance 3,block 4
	DownloadSpeed    float64 `json:"downloadSpeed"`
	UploadSpeed      float64 `json:"uploadSpeed"`
	RegistrationTime int64   `json:"registrationTime"` //StartTimeStamp
	LastPing         int64   `json:"lastPing"`
	Chain            string  `json:"chainName"`
	WalletAddress    string  `json:"walletAddress"`
	Version          string  `json:"version"`
	CodeHash         string  `json:"codeHash"`
	SystemInfo       string  `json:"systemInfo" gorm:"type:jsonb"`
	IpInfo           string  `json:"ipinfo" gorm:"type:jsonb"`
	IpGeoData        string  `json:"ipGeoData" gorm:"type:jsonb"`
	NodeType         string  `json:"nodeType"`
	NodeConfig       string  `json:"nodeConfig"`
}

type NodeAppends struct {
	PeerId           string  `json:"peerId" gorm:"primaryKey"`
	Name             string  `json:"name"`
	HttpPort         string  `json:"httpPort"`
	Host             string  `json:"host"` //domain
	PeerAddress      string  `json:"peerAddress"`
	Region           string  `json:"region"`
	Status           string  `json:"status"` // offline 1, online 2, maintainance 3,block 4
	DownloadSpeed    float64 `json:"downloadSpeed"`
	UploadSpeed      float64 `json:"uploadSpeed"`
	RegistrationTime int64   `json:"registrationTime"` //StartTimeStamp
	LastPing         int64   `json:"lastPing"`
	Chain            string  `json:"chain"`
	WalletAddress    string  `json:"walletAddress"`
	Version          string  `json:"version"`
	CodeHash         string  `json:"codeHash"`
	SystemInfo       OSInfo  `json:"systemInfo"`
	IpInfo           IPInfo  `json:"ipinfo"`
	NodeType         string  `json:"nodeType"`
	NodeConfig       string  `json:"nodeConfig"`
}

type OSInfo struct {
	Name         string // Name of the operating system
	Hostname     string // Hostname of the system
	Architecture string // Architecture of the system
	NumCPU       int    // Number of CPUs
}

// IPInfo struct to store IP information
type IPInfo struct {
	IPv4Addresses []string // IPv4 Addresses
	IPv6Addresses []string // IPv6 Addresses
}

type IpGeoAddress struct {
	IpInfoIP       string
	IpInfoCity     string
	IpInfoCountry  string
	IpInfoLocation string
	IpInfoOrg      string
	IpInfoPostal   string
	IpInfoTimezone string
}

type NodeActivity struct {
	PeerID              string     `json:"peerId" gorm:"primaryKey;type:varchar(255)"` // Ensure peer_id is indexed as a primary key
	StartTime           time.Time  `json:"startTime" gorm:"not null"`                  // Ensure StartTime is required
	EndTime             *time.Time `json:"endTime"`                                    // EndTime can be nil if the node is still active
	DurationSeconds     int        `json:"durationSeconds" gorm:"default:0"`           // Duration in seconds, default to 0
	LastActiveStartTime *time.Time `json:"lastActiveStartTime"`                        // Time when the node last became active (nullable)
}

// NodeLog represents the schema for the node_logs table
type NodeLog struct {
	ID        uuid.UUID `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"` // Use UUID as primary key
	PeerID    string    `gorm:"index;not null"`                                  // PeerID is not unique, but indexed for fast lookup
	Status    string    `gorm:"not null"`                                        // Status: active or inactive
	Timestamp time.Time `gorm:"not null"`                                        // Timestamp of the status change
}
