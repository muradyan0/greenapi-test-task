package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed templates/*
var templates embed.FS

//go:embed static/*
var staticFiles embed.FS

type SettingsRequest struct {
	IDInstance       string `json:"idInstance"`
	APITokenInstance string `json:"apiTokenInstance"`
}

type SettingsResponse struct {
	URL      string      `json:"url"`
	Response interface{} `json:"response"`
	Status   int         `json:"status"`
	Time     string      `json:"time"`
}

func main() {
	// Set up routes
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/api/get-settings", settingsHandler)
	http.HandleFunc("/api/get-state", stateHandler)
	http.HandleFunc("/api/send-message", sendMessageHandler)
	http.HandleFunc("/api/send-file", sendFileHandler)
	http.Handle("/static/", http.FileServer(http.FS(staticFiles)))

	// Start server
	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(templates, "templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func makeAPIRequest(url string) (map[string]interface{}, int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("request creation failed: %w", err)
	}

	req.Header = http.Header{
		"Accept":          {"application/json"},
		"Content-Type":    {"application/json"},
		"Accept-Language": {"en-US"},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request execution failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode >= 400 {
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("api error: %s", string(errorBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("json decode failed: %w", err)
	}

	return result, resp.StatusCode, nil
}

func settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Construct the API URL
	apiUrl := fmt.Sprintf("https://1103.api.green-api.com/waInstance%s/getSettings/%s",
		url.PathEscape(req.IDInstance),
		url.PathEscape(req.APITokenInstance))

	// In a real app, you would make the actual GET request here
	// For this example, we'll simulate a response
	apiResponse, statusCode, err := makeAPIRequest(apiUrl)
	if err != nil {
		log.Printf("API request failed after retries: %v", err)
		http.Error(w, "Failed to communicate with WhatsApp API", http.StatusBadGateway)
		return
	}

	// Check for API-specific errors
	if statusCode != http.StatusOK {
		if apiError, ok := apiResponse["error"]; ok {
			http.Error(w, fmt.Sprintf("WhatsApp API error: %v", apiError), statusCode)
			return
		}
	}

	response := SettingsResponse{
		URL:      apiUrl,
		Response: apiResponse,
		Status:   200,
		Time:     time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func stateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON body
	var requestBody struct {
		IDInstance       string `json:"idInstance"`
		APITokenInstance string `json:"apiTokenInstance"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Construct the API URL for getStateInstance
	apiUrl := fmt.Sprintf("https://api.green-api.com/waInstance%s/getStateInstance/%s",
		url.PathEscape(requestBody.IDInstance),
		url.PathEscape(requestBody.APITokenInstance))

	// Make the actual HTTP request
	startTime := time.Now()
	apiResponse, statusCode, err := makeAPIRequest(apiUrl)
	if err != nil {
		http.Error(w, fmt.Sprintf("API request failed: %v", err), http.StatusBadGateway)
		return
	}

	// Prepare our response
	response := map[string]interface{}{
		"url": apiUrl,
		"requestBody": map[string]string{
			"idInstance":       requestBody.IDInstance,
			"apiTokenInstance": "••••••••", // Mask sensitive data
		},
		"response":    apiResponse,
		"statusCode":  statusCode,
		"processedAt": time.Now().Format(time.RFC3339),
		"requestTime": time.Since(startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON body
	var requestBody struct {
		IDInstance       string `json:"idInstance"`
		APITokenInstance string `json:"apiTokenInstance"`
		PhoneNumber      string `json:"phoneNumber"`
		MessageText      string `json:"messageText"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate phone number (simple validation)
	if len(requestBody.PhoneNumber) < 11 {
		http.Error(w, "Phone number too short", http.StatusBadRequest)
		return
	}

	// Construct the API URL
	apiUrl := fmt.Sprintf("https://api.green-api.com/waInstance%s/sendMessage/%s",
		url.PathEscape(requestBody.IDInstance),
		url.PathEscape(requestBody.APITokenInstance))

	// Prepare request payload
	payload := map[string]interface{}{
		"chatId":  fmt.Sprintf("%s@c.us", requestBody.PhoneNumber),
		"message": requestBody.MessageText,
	}

	// Make the API request
	startTime := time.Now()
	apiResponse, statusCode, err := makeAPIRequestWithPayload(apiUrl, payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("API request failed: %v", err), http.StatusBadGateway)
		return
	}

	// Prepare our response
	response := map[string]interface{}{
		"url": apiUrl,
		"requestBody": map[string]interface{}{
			"phoneNumber":      requestBody.PhoneNumber,
			"message":          requestBody.MessageText,
			"idInstance":       requestBody.IDInstance,
			"apiTokenInstance": "••••••••", // Mask sensitive data
		},
		"response":    apiResponse,
		"statusCode":  statusCode,
		"processedAt": time.Now().Format(time.RFC3339),
		"requestTime": time.Since(startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func makeAPIRequestWithPayload(url string, payload interface{}) (map[string]interface{}, int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Marshal payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result, resp.StatusCode, nil
}

func getFilename(url string) string {
	// Remove query parameters and fragments
	cleanURL := strings.Split(url, "?")[0]
	cleanURL = strings.Split(cleanURL, "#")[0]

	// Split by slashes
	parts := strings.Split(cleanURL, "/")

	// Get last non-empty part
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}

	return ""
}

func sendFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON body
	var requestBody struct {
		IDInstance       string `json:"idInstance"`
		APITokenInstance string `json:"apiTokenInstance"`
		PhoneNumber      string `json:"phoneNumber"`
		FileUrl          string `json:"fileUrl"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if requestBody.FileUrl == "" {
		http.Error(w, "File URL is required", http.StatusBadRequest)
		return
	}

	if _, err := url.ParseRequestURI(requestBody.FileUrl); err != nil {
		http.Error(w, "Invalid file URL", http.StatusBadRequest)
		return
	}

	// Construct the API URL
	apiUrl := fmt.Sprintf("https://api.green-api.com/waInstance%s/sendFileByUrl/%s",
		url.PathEscape(requestBody.IDInstance),
		url.PathEscape(requestBody.APITokenInstance))

	// Prepare request payload
	payload := map[string]interface{}{
		"chatId":   fmt.Sprintf("%s@c.us", requestBody.PhoneNumber),
		"urlFile":  requestBody.FileUrl,
		"fileName": getFilename(requestBody.FileUrl),
	}

	// Make the API request
	startTime := time.Now()
	apiResponse, statusCode, err := makeAPIRequestWithPayload(apiUrl, payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("API request failed: %v", err), http.StatusBadGateway)
		return
	}

	// Prepare our response
	response := map[string]interface{}{
		"url": apiUrl,
		"requestBody": map[string]interface{}{
			"phoneNumber":      requestBody.PhoneNumber,
			"fileUrl":          requestBody.FileUrl,
			"idInstance":       requestBody.IDInstance,
			"apiTokenInstance": "••••••••", // Mask sensitive data
		},
		"response":    apiResponse,
		"statusCode":  statusCode,
		"processedAt": time.Now().Format(time.RFC3339),
		"requestTime": time.Since(startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
