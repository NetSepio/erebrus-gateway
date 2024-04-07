package authenticate

type AuthenticateRequest struct {
	FlowId    string `json:"flowId" binding:"required"`
	Signature string `json:"signature" binding:"required,hexadecimal,startswith=0x"`
	PubKey    string `json:"pubKey" binding:"required,hexadecimal,startswith=0x"`
}

type AuthenticatePayload struct {
	Token  string `json:"token"`
	UserId string `json:"userId"`
}
