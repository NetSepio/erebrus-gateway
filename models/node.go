package models

import "gorm.io/gorm"

// type Node struct {
// 	Id                  string  `json:"id" gorm:"primaryKey"`
// 	Name                string  `json:"name"`
// 	HttpPort            string  `json:"httpPort"`
// 	Domain              string  `json:"domain"`
// 	NodeName            string  `json:"nodename"`
// 	Address             string  `json:"address"`
// 	Region              string  `json:"region"`
// 	Status              string  `json:"status"`
// 	DownloadSpeed       float64 `json:"downloadSpeed"`
// 	UploadSpeed         float64 `json:"uploadSpeed"`
// 	StartTimeStamp      int64   `json:"startTimeStamp"`
// 	LastPingedTimeStamp int64   `json:"lastPingedTimeStamp"`
// 	WalletAddressSui    string  `json:"walletAddress"`
// 	WalletAddressSolana string  `json:"walletAddressSol"`
// 	IpInfoIP            string  `json:"ipinfoip"`
// 	IpInfoCity          string  `json:"ipinfocity"`
// 	IpInfoCountry       string  `json:"ipinfocountry"`
// 	IpInfoLocation      string  `json:"ipinfolocation"`
// 	IpInfoOrg           string  `json:"ipinfoorg"`
// 	IpInfoPostal        string  `json:"ipinfopostal"`
// 	IpInfoTimezone      string  `json:"ipinfotimezone"`
// }

type Node struct {
	gorm.Config
	PeerId           string        `json:"peerId" gorm:"primaryKey"`
	Name             string        `json:"name"`
	HttpPort         string        `json:"httpPort"`
	Host             string        `json:"host"` //domain
	PeerAddress      string        `json:"peerAddress"`
	Region           string        `json:"region"`
	Status           string        `json:"status"` // offline 1, online 2, maintainance 3,block 4
	DownloadSpeed    float64       `json:"downloadSpeed"`
	UploadSpeed      float64       `json:"uploadSpeed"`
	RegistrationTime int64         `json:"registrationTime"` //StartTimeStamp
	LastPing         int64         `json:"lastPing"`
	Chain            string        `json:"chain"`
	WalletAddress    string        `json:"walletAddress"`
	Version          string        `json:"version"`
	CodeHash         string        `json:"codeHash"`
	SystemInfo       []interface{} `json:"systemInfo"`
	IpInfo           []interface{} `json:"ipinfo"`
}
