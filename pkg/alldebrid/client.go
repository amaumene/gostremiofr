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

type Client struct {
	httpClient *http.Client
	baseURL    string
}

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

func (c *Client) UploadMagnet(apiKey string, magnetURLs []string) (*MagnetUploadResponse, error) {
	endpoint := fmt.Sprintf("%s/magnet/upload", c.baseURL)
	formData := c.buildMagnetFormData(apiKey, magnetURLs)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	var result MagnetUploadResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) buildMagnetFormData(apiKey string, magnetURLs []string) url.Values {
	formData := url.Values{}
	formData.Set("agent", "stremio")
	formData.Set("apikey", apiKey)
	for _, magnetURL := range magnetURLs {
		formData.Add("magnets[]", magnetURL)
	}
	return formData
}

func (c *Client) buildParams(apiKey string, extraParams map[string]string) url.Values {
	params := url.Values{}
	params.Set("agent", "stremio")
	params.Set("apikey", apiKey)
	for key, value := range extraParams {
		params.Set(key, value)
	}
	return params
}

func (c *Client) decodeResponse(resp *http.Response, result interface{}) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

func (c *Client) UnlockLink(apiKey, link string) (*LinkUnlockResponse, error) {
	endpoint := fmt.Sprintf("%s/link/unlock", c.baseURL)
	params := c.buildParams(apiKey, map[string]string{"link": link})
	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	var result LinkUnlockResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetMagnetFiles(apiKey, magnetID string) (*MagnetFilesResponse, error) {
	endpoint := fmt.Sprintf("%s/magnet/files", c.baseURL)
	params := c.buildParams(apiKey, map[string]string{"id": magnetID})
	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	var result MagnetFilesResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteMagnet(apiKey string, magnetID string) error {
	endpoint := fmt.Sprintf("%s/magnet/delete", c.baseURL)
	params := c.buildParams(apiKey, map[string]string{"id": magnetID})
	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	return c.validateDeleteResponse(resp)
}

func (c *Client) validateDeleteResponse(resp *http.Response) error {
	var result struct {
		Status string `json:"status"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := c.decodeResponse(resp, &result); err != nil {
		return err
	}

	if result.Status != "success" {
		if result.Error != nil {
			return fmt.Errorf("AllDebrid API error: %s - %s", result.Error.Code, result.Error.Message)
		}
		return fmt.Errorf("AllDebrid API error: %s", result.Status)
	}

	return nil
}
