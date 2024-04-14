package models

type Node struct {
	Id                  string  `json:"id" gorm:"primaryKey"`
	HttpPort            string  `json:"httpPort"`
	Domain              string  `json:"domain"`
	Address             string  `json:"address"`
	Region              string  `json:"region"`
	Status              string  `json:"status"`
	DownloadSpeed       float64 `json:"downloadSpeed"`
	UploadSpeed         float64 `json:"uploadSpeed"`
	StartTimeStamp      int64   `json:"startTimeStamp"`
	LastPingedTimeStamp int64   `json:"lastPingedTimeStamp"`
}
