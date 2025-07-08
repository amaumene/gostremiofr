package httputil

import (
	"net/http"
	"time"
)

// NewHTTPClient creates a new HTTP client with standard configuration
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

// NewDefaultHTTPClient creates a new HTTP client with 30 second timeout
func NewDefaultHTTPClient() *http.Client {
	return NewHTTPClient(30 * time.Second)
}