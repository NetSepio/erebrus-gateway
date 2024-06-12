package models

type NodeResponse struct {
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

// type Node struct {
// 	PeerId           string  `json:"peerId" gorm:"primaryKey"`
// 	Name             string  `json:"name"`
// 	HttpPort         string  `json:"httpPort"`
// 	Host             string  `json:"host"` //domain
// 	PeerAddress      string  `json:"peerAddress"`
// 	Region           string  `json:"region"`
// 	Status           string  `json:"status"` // offline 1, online 2, maintainance 3,block 4
// 	DownloadSpeed    float64 `json:"downloadSpeed"`
// 	UploadSpeed      float64 `json:"uploadSpeed"`
// 	RegistrationTime int64   `json:"registrationTime"` //StartTimeStamp
// 	LastPing         int64   `json:"lastPing"`
// 	Chain            string  `json:"chain"`
// 	WalletAddress    string  `json:"walletAddress"`
// 	Version          string  `json:"version"`
// 	CodeHash         string  `json:"codeHash"`
// 	SystemInfo       OSInfo  `json:"systemInfo"`
// 	IpInfo           IPInfo  `json:"ipinfo"`
// }

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
	Chain            string  `json:"chain"`
	WalletAddress    string  `json:"walletAddress"`
	Version          string  `json:"version"`
	CodeHash         string  `json:"codeHash"`
	SystemInfo       string  `json:"systemInfo" gorm:"type:jsonb"`
	IpInfo           string  `json:"ipinfo" gorm:"type:jsonb"`
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
