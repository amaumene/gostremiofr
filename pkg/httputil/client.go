// Package httputil provides HTTP client utilities with standard configurations.
package httputil

import (
	"net/http"
	"time"
)

const (
	// Default timeout for HTTP requests
	defaultTimeout = 30 * time.Second
	
	// Transport configuration constants
	maxIdleConns        = 10
	maxIdleConnsPerHost = 2
	idleConnTimeout     = 30 * time.Second
)

// NewHTTPClient creates a new HTTP client with the specified timeout.
// The client is configured with connection pooling and idle connection management.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			IdleConnTimeout:     idleConnTimeout,
		},
	}
}

// NewDefaultHTTPClient creates a new HTTP client with default 30 second timeout.
// This is suitable for most API calls and web requests.
func NewDefaultHTTPClient() *http.Client {
	return NewHTTPClient(defaultTimeout)
}
