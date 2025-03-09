package client

type Client struct {
	UUID                      string   `json:"UUID,omitempty"`
	Name                      string   `json:"Name" binding:"required"`
	Tags                      []string `json:"Tags,omitempty"`
	WalletAddress             string   `json:"WalletAddress,omitempty"`
	Enable                    bool     `json:"Enable,omitempty"`
	IgnorePersistentKeepalive bool     `json:"IgnorePersistentKeepalive,omitempty"`
	PublicKey                 string   `json:"PublicKey,omitempty"`
	PresharedKey              string   `json:"PresharedKey,omitempty"`
	AllowedIPs                []string `json:"AllowedIPs,omitempty"`
	Address                   []string `json:"Address,omitempty"`
	CreatedBy                 string   `json:"CreatedBy,omitempty"`
	UpdatedBy                 string   `json:"UpdatedBy,omitempty"`
	CreatedAt                 int64    `json:"CreatedAt,omitempty"`
	UpdatedAt                 int64    `json:"UpdatedAt,omitempty"`
	CollectionId              string   `json:"CollectionId,omitempty"`
}

type ClientRequest struct {
	Name         string `json:"name" binding:"required"`
	PublicKey    string `json:"publicKey" binding:"required"`
	PresharedKey string `json:"presharedKey" binding:"required"`
}

type Response struct {
	Status  int64             `json:"status,omitempty"`
	Success bool              `json:"success,omitempty"`
	Message string            `json:"message,omitempty"`
	Error   string            `json:"error,omitempty"`
	Client  *ClientResponse   `json:"client,omitempty"`
	Server  *Server           `json:"server,omitempty"`
	Clients []*ClientResponse `json:"clients,omitempty"`
}

type ClientResponse struct {
	UUID                      string   `json:"UUID,omitempty"`
	Name                      string   `json:"Name" binding:"required"`
	Tags                      []string `json:"Tags,omitempty"`
	WalletAddress             string   `json:"WalletAddress,omitempty"`
	Enable                    bool     `json:"Enable,omitempty"`
	IgnorePersistentKeepalive bool     `json:"IgnorePersistentKeepalive,omitempty"`
	PublicKey                 string   `json:"PublicKey,omitempty"`
	PresharedKey              string   `json:"PresharedKey,omitempty"`
	AllowedIPs                []string `json:"AllowedIPs,omitempty"`
	Address                   []string `json:"Address,omitempty"`
	CreatedBy                 string   `json:"CreatedBy,omitempty"`
	UpdatedBy                 string   `json:"UpdatedBy,omitempty"`
	CreatedAt                 int64    `json:"CreatedAt,omitempty"`
	UpdatedAt                 int64    `json:"UpdatedAt,omitempty"`
	ReceiveBytes              int64    `json:"ReceiveBytes,omitempty"`
	TransmitBytes             int64    `json:"TransmitBytes,omitempty"`
}

type Server struct {
	Address             []string `json:"Address,omitempty"`
	ListenPort          int64    `json:"ListenPort,omitempty"`
	Mtu                 int64    `json:"Mtu,omitempty"`
	PrivateKey          string   `json:"PrivateKey,omitempty"`
	PublicKey           string   `json:"PublicKey,omitempty"`
	Endpoint            string   `json:"Endpoint,omitempty"`
	PersistentKeepalive int64    `json:"PersistentKeepalive,omitempty"`
	DNS                 []string `json:"DNS,omitempty"`
	AllowedIPs          []string `json:"AllowedIPs,omitempty"`
	PreUp               string   `json:"PreUp,omitempty"`
	PostUp              string   `json:"PostUp,omitempty"`
	PreDown             string   `json:"PreDown,omitempty"`
	PostDown            string   `json:"PostDown,omitempty"`
	UpdatedBy           string   `json:"UpdatedBy,omitempty"`
	CreatedAt           int64    `json:"CreatedAt,omitempty"`
	UpdatedAt           int64    `json:"UpdatedAt,omitempty"`
}
