package caddyservices

import (
	"encoding/json"
	"fmt"
)

// Unmarshal the response

type RequestData struct {
	Name      string `json:"name"`
	IpAddress string `json:"ipAddress"`
	Port      string `json:"port"`
}

type ResponseMessage struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	IpAddress string `json:"ipAddress"`
	Port      string `json:"port"`
	Domain    string `json:"domain"`
	CreatedAt string `json:"createdAt"`
}

type APIResponse struct {
	Message ResponseMessage `json:"message"`
	Status  int             `json:"status"`
}

type ErrorAPIResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// RequestPayload represents the structure for POST requests
type RequestPayload struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Port      string `json:"port"`
}

type MessageUnion struct {
	String   string
	Response *ResponseMessage
}

type Service struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	IpAddress string `json:"ipAddress"`
	Port      string `json:"port"`
	Domain    string `json:"domain"`
	CreatedAt string `json:"createdAt"`
}

type DeleteAPIResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

type GetAPIResponse struct {
	Services []Service `json:"services"`
}

// type ServiceDetails struct {
// 	Name      string `json:"name"`
// 	Type      string `json:"type"`
// 	Port      string `json:"port"`
// 	Domain    string `json:"domain"`
// 	Status    string `json:"status"`
// 	CreatedAt string `json:"createdAt"`
// }

func (mu *MessageUnion) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as a string
	if err := json.Unmarshal(data, &mu.String); err == nil {
		return nil
	}
	// If it's not a string, try unmarshalling as a ResponseMessage
	var response ResponseMessage
	if err := json.Unmarshal(data, &response); err == nil {
		mu.Response = &response
		return nil
	}
	return fmt.Errorf("invalid message format")
}
