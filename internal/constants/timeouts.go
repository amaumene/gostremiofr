// Package constants defines timeout values and retry limits used throughout the application.
// These constants help maintain consistent behavior and make the codebase more maintainable.
package constants

import "time"

// Timeout constants for various operations
const (
	// Request timeout for the entire stream request
	RequestTimeout = 30 * time.Second

	// Search timeout for torrent provider searches
	SearchTimeout = 15 * time.Second

	// Retry delays for various operations
	MagnetCheckRetryDelay = 2 * time.Second
	MagnetReadyRetryDelay = 3 * time.Second

	// Maximum retry attempts
	MaxMagnetCheckAttempts = 2
)
