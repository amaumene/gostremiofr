// Package models defines data structures for torrent search operations.
package models

import (
	"time"

	"github.com/cehbz/torrentname"
)

// TorrentInfo represents detailed information about a torrent.
type TorrentInfo struct {
	ID               string
	Title            string
	Hash             string
	Source           string
	Size             int64
	Seeders          int
	Leechers         int
	Type             string
	Season           int
	Episode          int
	ConfidenceScore  float64
	ParsedInfo       *torrentname.TorrentInfo
}

// SearchResults contains categorized torrent search results from a single provider.
type SearchResults struct {
	CompleteSeriesTorrents []TorrentInfo
	CompleteSeasonTorrents []TorrentInfo
	EpisodeTorrents        []TorrentInfo
	MovieTorrents          []TorrentInfo
}

// CombinedSearchResults contains search results from multiple providers.
type CombinedSearchResults struct {
	Results   map[string]*SearchResults // Provider name -> results
	DebugInfo map[string]string         // Provider name -> debug info (e.g., API URLs)
}

// ParsedFileName contains parsed information from torrent file names.
type ParsedFileName struct {
	Resolution string
	Codec      string
	Source     string
}

// Priority defines priority scoring for torrent sorting.
type Priority struct {
	Resolution int
	Source     int
	Seeders    int
}

// SearchOptions defines parameters for torrent search operations.
type SearchOptions struct {
	Query           string
	MediaType       string
	Season          int
	Episode         int
	Year            int
	Language        string
	SpecificEpisode bool
	ResolutionFilter []string
	MaxResults      int
}

// TranslationOptions defines parameters for content translation.
type TranslationOptions struct {
	TargetLanguage string
	APIKey         string
	CacheTimeout   time.Duration
}

// SortOptions defines parameters for sorting torrent results.
type SortOptions struct {
	ByPriority     bool
	BySize         bool
	BySeeders      bool
	ResolutionOrder []string
}