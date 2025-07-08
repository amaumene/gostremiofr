// Package constants defines numerical limits and conversion factors.
package constants

// Limits and counts for various operations
const (
	// Number of concurrent goroutines for torrent searches
	TorrentSearchGoroutines = 2

	// Maximum number of episode torrents to log for debugging
	MaxEpisodeTorrentsToLog = 5

	// Conversion factors
	BytesToGB = 1024 * 1024 * 1024
)
