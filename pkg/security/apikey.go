package security

import (
	"crypto/subtle"
	"regexp"
	"strings"
)

// APIKeyValidator provides secure validation and handling of API keys
type APIKeyValidator struct {
	minLength int
	maxLength int
}

// NewAPIKeyValidator creates a new API key validator with reasonable defaults
func NewAPIKeyValidator() *APIKeyValidator {
	return &APIKeyValidator{
		minLength: 8,
		maxLength: 128,
	}
}

// ValidateAPIKey validates API key format and length
func (v *APIKeyValidator) ValidateAPIKey(apiKey string) bool {
	if apiKey == "" {
		return false
	}
	
	// Check length constraints
	if len(apiKey) < v.minLength || len(apiKey) > v.maxLength {
		return false
	}
	
	// Check for basic alphanumeric characters (common for API keys)
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return validPattern.MatchString(apiKey)
}

// SanitizeAPIKey removes dangerous characters and trims whitespace
func (v *APIKeyValidator) SanitizeAPIKey(apiKey string) string {
	// Remove whitespace
	apiKey = strings.TrimSpace(apiKey)
	
	// Remove any potentially dangerous characters for URL/header injection
	// Keep only alphanumeric, hyphens, and underscores which are safe for most APIs
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return reg.ReplaceAllString(apiKey, "")
}

// MaskAPIKey creates a masked version for logging (shows only first/last few chars)
func (v *APIKeyValidator) MaskAPIKey(apiKey string) string {
	if len(apiKey) == 0 {
		return "[empty]"
	}
	
	if len(apiKey) <= 8 {
		return "[***]"
	}
	
	// Show first 3 and last 3 characters
	return apiKey[:3] + "..." + apiKey[len(apiKey)-3:]
}

// SecureCompare performs constant-time comparison of API keys to prevent timing attacks
func (v *APIKeyValidator) SecureCompare(key1, key2 string) bool {
	return subtle.ConstantTimeCompare([]byte(key1), []byte(key2)) == 1
}

// IsValidAllDebridKey validates AllDebrid API key format specifically
func (v *APIKeyValidator) IsValidAllDebridKey(apiKey string) bool {
	if !v.ValidateAPIKey(apiKey) {
		return false
	}
	
	// AllDebrid keys are typically 20-32 characters
	return len(apiKey) >= 16 && len(apiKey) <= 40
}

// IsValidTMDBKey validates TMDB API key format specifically  
func (v *APIKeyValidator) IsValidTMDBKey(apiKey string) bool {
	if !v.ValidateAPIKey(apiKey) {
		return false
	}
	
	// TMDB keys are typically 32 characters hexadecimal
	if len(apiKey) != 32 {
		return false
	}
	
	hexPattern := regexp.MustCompile(`^[a-fA-F0-9]+$`)
	return hexPattern.MatchString(apiKey)
}