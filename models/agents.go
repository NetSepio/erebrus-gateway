package models

import (
	"gorm.io/gorm"
)

type Agent struct {
	gorm.Model
	ID            string   `json:"id" gorm:"uniqueIndex"`
	Name          string   `json:"name"`
	Clients       []string `json:"clients" gorm:"-"`
	ClientsJSON   string   `json:"-"`
	Status        string   `json:"status"`
	AvatarImg     string   `json:"avatar_img"`
	CoverImg      string   `json:"cover_img"`
	VoiceModel    string   `json:"voice_model"`
	Organization  string   `json:"organization"`
	WalletAddress string   `json:"wallet_address" gorm:"index"`
	ServerDomain  string   `json:"server_domain"`
}

// BeforeSave converts the Clients slice to JSON string for storage
func (a *Agent) BeforeSave(tx *gorm.DB) error {
	if len(a.Clients) > 0 {
		clientsJSON, err := json.Marshal(a.Clients)
		if err != nil {
			return err
		}
		a.ClientsJSON = string(clientsJSON)
	}
	return nil
}

// AfterFind converts the JSON string back to Clients slice
func (a *Agent) AfterFind(tx *gorm.DB) error {
	if a.ClientsJSON != "" {
		return json.Unmarshal([]byte(a.ClientsJSON), &a.Clients)
	}
	return nil
}