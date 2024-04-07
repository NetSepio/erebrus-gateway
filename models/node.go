package models

type Node struct {
	Id           string `json:"id" gorm:"primaryKey"`
	HttpPort     string `json:"httpPort"`
	Domain       string `json:"domain"`
	Address      string `json:"address"`
	Region       string `json:"region"`
	Status       string `json:"status"`
	Uptime       string `json:"uptime"`
	NetworkSpeed string `json:"networkSpeed"`
}
