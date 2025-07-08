// Package errors defines custom error types for better error handling and debugging.
// StreamError provides context-aware error reporting with type classification.
package errors

import (
	"fmt"
)

// StreamError represents errors that occur during stream processing
type StreamError struct {
	Type    string
	Message string
	Cause   error
}

func (e *StreamError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *StreamError) Unwrap() error {
	return e.Cause
}

// Error type constants
const (
	ErrorTypeConfigurationInvalid = "CONFIGURATION_INVALID"
	ErrorTypeAPIKeyMissing        = "API_KEY_MISSING"
	ErrorTypeTMDBFailure          = "TMDB_FAILURE"
	ErrorTypeTorrentSearchFailed  = "TORRENT_SEARCH_FAILED"
	ErrorTypeMagnetProcessFailed  = "MAGNET_PROCESS_FAILED"
	ErrorTypeTimeout              = "TIMEOUT"
	ErrorTypeInvalidID            = "INVALID_ID"
)

// NewStreamError creates a new StreamError
func NewStreamError(errorType, message string, cause error) *StreamError {
	return &StreamError{
		Type:    errorType,
		Message: message,
		Cause:   cause,
	}
}

// NewConfigurationError creates a configuration-related error
func NewConfigurationError(message string, cause error) *StreamError {
	return NewStreamError(ErrorTypeConfigurationInvalid, message, cause)
}

// NewAPIKeyMissingError creates an API key missing error
func NewAPIKeyMissingError(service string) *StreamError {
	return NewStreamError(ErrorTypeAPIKeyMissing, fmt.Sprintf("API key missing for %s", service), nil)
}

// NewTMDBError creates a TMDB-related error
func NewTMDBError(message string, cause error) *StreamError {
	return NewStreamError(ErrorTypeTMDBFailure, message, cause)
}

// NewTorrentSearchError creates a torrent search error
func NewTorrentSearchError(message string, cause error) *StreamError {
	return NewStreamError(ErrorTypeTorrentSearchFailed, message, cause)
}

// NewMagnetProcessError creates a magnet processing error
func NewMagnetProcessError(message string, cause error) *StreamError {
	return NewStreamError(ErrorTypeMagnetProcessFailed, message, cause)
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(operation string) *StreamError {
	return NewStreamError(ErrorTypeTimeout, fmt.Sprintf("Operation timeout: %s", operation), nil)
}

// NewInvalidIDError creates an invalid ID error
func NewInvalidIDError(id string) *StreamError {
	return NewStreamError(ErrorTypeInvalidID, fmt.Sprintf("Invalid ID format: %s", id), nil)
}
