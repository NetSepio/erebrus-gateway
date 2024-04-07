package paseto

type GenericAuthHeaders struct {
	Authorization string
}
type AuthenticateTokenPayload struct {
	UserId        string `json:"userId"`
	WalletAddress string `json:"walletAddress"`
}
