package caddyservices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"log"
	"net/http"
)

func AddServiceInErebrusNode(requestData RequestData, url string) (APIResponse, error) {
	var (
		apiResponse      APIResponse
		errorApiResponse ErrorAPIResponse
	)

	// Marshal the data into JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		log.Printf("Error marshalling requestData: %v", err)
		return apiResponse, err
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return apiResponse, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return apiResponse, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return apiResponse, err
	}

	// Print the raw response body for debugging
	fmt.Println("Read the response body body:", string(body))

	// Try to unmarshal into APIResponse
	if err = json.Unmarshal(body, &apiResponse); err != nil {
		log.Printf("Error unmarshalling apiResponse: %v", err)

		// If unmarshalling into APIResponse fails, try unmarshalling into ErrorAPIResponse
		if err = json.Unmarshal(body, &errorApiResponse); err != nil {
			log.Printf("Error unmarshalling errorApiResponse: %v", err)
			return apiResponse, fmt.Errorf("unexpected error: %v", err)
		} else {
			// If we successfully unmarshalled ErrorAPIResponse, return the error message
			apiResponse.Status = errorApiResponse.Status
			apiResponse.Message.Name = errorApiResponse.Message
			return apiResponse, fmt.Errorf("Error: %v", errorApiResponse.Message)
		}
	}

	// If unmarshalling into APIResponse was successful, return it
	return apiResponse, nil
}

// FetchServices retrieves the list of services from the API
func FetchServices(url string) (GetAPIResponse, error) {
	var apiResponse GetAPIResponse

	// Send the GET request
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return apiResponse, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return apiResponse, err
	}

	// Print the raw response body for debugging
	fmt.Println("Read the response body body:", string(body))

	// Unmarshal the response body into the APIResponse struct
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		log.Printf("Error unmarshalling response: %v", err)
		return apiResponse, err
	}

	return apiResponse, nil
}

// FetchServiceDetails retrieves the details of a service by name from the API
func FetchServiceDetails(url string) (APIResponse, error) {
	var apiResponse APIResponse

	// Send the GET request
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return apiResponse, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return apiResponse, err
	}

	// Print the raw response body for debugging
	fmt.Println("Read the response body body:", string(body))

	// Unmarshal the response body into the APIResponse struct
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		log.Printf("Error unmarshalling response: %v", err)
		return apiResponse, err
	}

	return apiResponse, nil
}

// DeleteService sends a DELETE request to the API to remove a service
func DeleteService(url string) (DeleteAPIResponse, error) {
	var apiResponse DeleteAPIResponse

	// Create the DELETE request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return apiResponse, err
	}

	// Set the Content-Type header to application/json
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return apiResponse, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return apiResponse, err
	}

	// Print the raw response body for debugging
	fmt.Println("Read the response body body:", string(body))

	// Unmarshal the response body into the APIResponse struct
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		log.Printf("Error unmarshalling response: %v", err)
		return apiResponse, err
	}

	return apiResponse, nil
}
