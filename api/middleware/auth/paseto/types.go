package paseto

type GenericAuthHeaders struct {
	Authorization string
}
type AuthenticateTokenPayload struct {
	UserId        string `json:"userId"`
	WalletAddress string `json:"walletAddress"`
}

type AuthenticateTokenResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Payload struct {
		UserID        string `json:"userId"`
		WalletAddress string `json:"walletAddress"`
	} `json:"payload"`
}
