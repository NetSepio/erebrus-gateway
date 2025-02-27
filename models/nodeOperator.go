package models

type FormData struct {
	ID            uint   `gorm:"primaryKey"`
	Email         string `json:"email" binding:"required"`
	TelegramID    string `json:"telegram_id"`
	Name          string `json:"name" binding:"required"`
	RanNodeBefore bool   `json:"ran_node_before"`
	ProjectName   string `json:"project_name"`
	ProjectID     string `json:"project_id"`
	WalletAddress string `json:"wallet_address"`
	TwitterID     string `json:"twitter_id"`
	Region        string `json:"region"`
	Chain         string `json:"chain"`
}
