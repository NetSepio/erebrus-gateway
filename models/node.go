package models

type Node struct {
	Id                  string  `json:"id" gorm:"primaryKey"`
	Name                string  `json:"name"`
	HttpPort            string  `json:"httpPort"`
	Domain              string  `json:"domain"`
	NodeName            string  `json:"nodename"`
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
}
