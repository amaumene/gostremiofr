// Package security provides security utilities for API key validation and handling.
package security

import (
	"crypto/subtle"
	"regexp"
	"strings"
)

const (
	// Default API key length constraints
	defaultMinLength = 8
	defaultMaxLength = 128
	
	// Service-specific key lengths
	allDebridMinLength = 16
	allDebridMaxLength = 40
	tmdbKeyLength      = 32
	
	// Masking constants
	maskedKeyShort   = "[***]"
	maskedKeyEmpty   = "[empty]"
	visibleChars     = 3
	minMaskableLength = 8
)

// Pre-compiled regular expressions for better performance
var (
	validKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	sanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	hexPattern      = regexp.MustCompile(`^[a-fA-F0-9]+$`)
)

// APIKeyValidator provides secure validation and handling of API keys.
// It implements constant-time comparison to prevent timing attacks.
type APIKeyValidator struct {
	minLength int
	maxLength int
}

// NewAPIKeyValidator creates a new API key validator with reasonable defaults.
// Default minimum length is 8 characters, maximum is 128 characters.
func NewAPIKeyValidator() *APIKeyValidator {
	return &APIKeyValidator{
		minLength: defaultMinLength,
		maxLength: defaultMaxLength,
	}
}

// ValidateAPIKey validates API key format and length.
// Returns true if the key meets length requirements and contains only alphanumeric characters, underscores, and hyphens.
func (v *APIKeyValidator) ValidateAPIKey(apiKey string) bool {
	if apiKey == "" {
		return false
	}

	keyLen := len(apiKey)
	if keyLen < v.minLength || keyLen > v.maxLength {
		return false
	}

	return validKeyPattern.MatchString(apiKey)
}

// SanitizeAPIKey removes dangerous characters and trims whitespace.
// Only alphanumeric characters, underscores, and hyphens are preserved.
func (v *APIKeyValidator) SanitizeAPIKey(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	return sanitizePattern.ReplaceAllString(apiKey, "")
}

// MaskAPIKey creates a masked version for logging.
// Shows only first and last 3 characters for keys longer than 8 characters.
// Shorter keys are completely masked for security.
func (v *APIKeyValidator) MaskAPIKey(apiKey string) string {
	if len(apiKey) == 0 {
		return maskedKeyEmpty
	}

	if len(apiKey) <= minMaskableLength {
		return maskedKeyShort
	}

	return apiKey[:visibleChars] + "..." + apiKey[len(apiKey)-visibleChars:]
}

// SecureCompare performs constant-time comparison of API keys to prevent timing attacks.
// Returns true if the keys are identical, false otherwise.
func (v *APIKeyValidator) SecureCompare(key1, key2 string) bool {
	return subtle.ConstantTimeCompare([]byte(key1), []byte(key2)) == 1
}

// IsValidAllDebridKey validates AllDebrid API key format specifically.
// AllDebrid keys must be between 16 and 40 characters.
func (v *APIKeyValidator) IsValidAllDebridKey(apiKey string) bool {
	if !v.ValidateAPIKey(apiKey) {
		return false
	}

	keyLen := len(apiKey)
	return keyLen >= allDebridMinLength && keyLen <= allDebridMaxLength
}

// IsValidTMDBKey validates TMDB API key format specifically.
// TMDB keys must be exactly 32 hexadecimal characters.
func (v *APIKeyValidator) IsValidTMDBKey(apiKey string) bool {
	if !v.ValidateAPIKey(apiKey) {
		return false
	}

	if len(apiKey) != tmdbKeyLength {
		return false
	}

	return hexPattern.MatchString(apiKey)
}
