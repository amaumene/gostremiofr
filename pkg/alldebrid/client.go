package alldebrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/pkg/httputil"
)

// Client represents an AllDebrid API client
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new AllDebrid API client
func NewClient() *Client {
	return &Client{
		httpClient: httputil.NewHTTPClient(30 * time.Second),
		baseURL:    "https://api.alldebrid.com/v4",
	}
}

// MagnetUploadResponse represents the response from magnet upload endpoint
type MagnetUploadResponse struct {
	Status string `json:"status"`
	Data   struct {
		Magnets []struct {
			ID    int64         `json:"id"`
			Hash  string        `json:"hash"`
			Name  string        `json:"name"`
			Size  int64         `json:"size"`
			Ready bool          `json:"ready"`
			Files []interface{} `json:"files"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		} `json:"magnets"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// LinkUnlockResponse represents the response from link unlock endpoint
type LinkUnlockResponse struct {
	Status string `json:"status"`
	Data   struct {
		Link     string        `json:"link"`
		Host     string        `json:"host"`
		Filename string        `json:"filename"`
		Filesize int64         `json:"filesize"`
		ID       string        `json:"id"`
		Streams  []interface{} `json:"streams"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// MagnetFilesResponse represents the response from magnet files endpoint
type MagnetFilesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Magnets []struct {
			ID    int64  `json:"id"`
			Hash  string `json:"hash"`
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			Ready bool   `json:"ready"`
			Links []struct {
				Link     string `json:"link"`
				Filename string `json:"filename"`
				Size     int64  `json:"size"`
			} `json:"links"`
		} `json:"magnets"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// UploadMagnet uploads a magnet link to AllDebrid
func (c *Client) UploadMagnet(apiKey string, magnetURLs []string) (*MagnetUploadResponse, error) {
	endpoint := fmt.Sprintf("%s/magnet/upload", c.baseURL)

	formData := url.Values{}
	formData.Set("agent", "stremio")
	formData.Set("apikey", apiKey)

	// Add all magnet URLs
	for _, magnetURL := range magnetURLs {
		formData.Add("magnets[]", magnetURL)
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result MagnetUploadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UnlockLink unlocks a link to get direct download URL
func (c *Client) UnlockLink(apiKey, link string) (*LinkUnlockResponse, error) {
	endpoint := fmt.Sprintf("%s/link/unlock", c.baseURL)

	params := url.Values{}
	params.Set("agent", "stremio")
	params.Set("apikey", apiKey)
	params.Set("link", link)

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result LinkUnlockResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetMagnetFiles gets files for a magnet ID
func (c *Client) GetMagnetFiles(apiKey, magnetID string) (*MagnetFilesResponse, error) {
	endpoint := fmt.Sprintf("%s/magnet/files", c.baseURL)

	params := url.Values{}
	params.Set("agent", "stremio")
	params.Set("apikey", apiKey)
	params.Set("id", magnetID)

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result MagnetFilesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteMagnet deletes a magnet from AllDebrid
func (c *Client) DeleteMagnet(apiKey string, magnetID string) error {
	endpoint := fmt.Sprintf("%s/magnet/delete", c.baseURL)

	params := url.Values{}
	params.Set("agent", "stremio")
	params.Set("apikey", apiKey)
	params.Set("id", magnetID)

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var result struct {
		Status string `json:"status"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Status != "success" {
		if result.Error != nil {
			return fmt.Errorf("AllDebrid API error: %s - %s", result.Error.Code, result.Error.Message)
		}
		return fmt.Errorf("AllDebrid API error: %s", result.Status)
	}

	return nil
}
