package cyrene

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// uploadFileToExternalAPI forwards the file and domain to the external API.
func uploadFileToExternalAPI(filePath, domain, url string) (string, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening file: %v", err)
		return "", err
	}
	defer file.Close()

	// Create a buffer to store the form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add the file to the form
	fileWriter, err := writer.CreateFormFile("character_file", filepath.Base(filePath))
	if err != nil {
		log.Printf("Error creating form file: %v", err)
		return "", err
	}
	_, err = io.Copy(fileWriter, file)
	if err != nil {
		log.Printf("Error copying file data: %v", err)
		return "", err
	}

	// Add the domain field to the form
	err = writer.WriteField("domain", domain)
	if err != nil {
		log.Printf("Error adding domain field: %v", err)
		return "", err
	}

	// Close the writer to finalize the form data
	err = writer.Close()
	if err != nil {
		log.Printf("Error closing writer: %v", err)
		return "", err
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", url, &requestBody)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return "", err
	}

	// Set the Content-Type header to match the multipart data
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		return "", err
	}

	// Return the response as a string
	return string(body), nil
}
