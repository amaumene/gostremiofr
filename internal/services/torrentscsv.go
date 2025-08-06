package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)

const (
	// TorrentsCSV API configuration
	torrentsCSVAPIBase        = "https://torrents-csv.com"
	torrentsCSVSearchEndpoint = "/service/search"

	// Cache keys
	torrentsCSVGeneralCacheKey = "TORRENTSCSV"
	torrentsCSVEpisodeCacheKey = "TORRENTSCSV_EPISODE"

	// Rate limiting
	torrentsCSVRateLimit  = 5 // requests per second
	torrentsCSVCacheBurst = 2
)

type TorrentsCSV struct {
	*BaseTorrentService
	yearExtractor *yearExtractor
}

// yearExtractor handles year extraction from torrent titles
type yearExtractor struct {
	patterns []*regexp.Regexp
}

type TorrentsCSVTorrent struct {
	RowID        int64  `json:"rowid"`
	InfoHash     string `json:"infohash"`
	Name         string `json:"name"`
	SizeBytes    int64  `json:"size_bytes"`
	CreatedUnix  int64  `json:"created_unix"`
	Seeders      int    `json:"seeders"`
	Leechers     int    `json:"leechers"`
	Completed    int    `json:"completed"`
	ScrapedDate  int64  `json:"scraped_date"`
	Source       string `json:"-"` // Set at runtime, not from JSON
}

// TorrentsCSVResponse represents the API response
type TorrentsCSVResponse struct {
	Torrents []TorrentsCSVTorrent `json:"torrents"`
	Next     int64                `json:"next"`
}

func NewTorrentsCSV(db database.Database, cache *cache.LRUCache) *TorrentsCSV {
	return &TorrentsCSV{
		BaseTorrentService: NewBaseTorrentService(db, cache, torrentsCSVRateLimit, torrentsCSVCacheBurst),
		yearExtractor:      newYearExtractor(),
	}
}

// newYearExtractor creates a new year extractor with compiled regex patterns
func newYearExtractor() *yearExtractor {
	patterns := []string{
		`\((\d{4})\)`,   // (2022)
		`\[(\d{4})\]`,   // [2022]
		`\b(19\d{2})\b`, // 19xx
		`\b(20\d{2})\b`, // 20xx
	}

	compiled := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		compiled[i] = regexp.MustCompile(pattern)
	}

	return &yearExtractor{patterns: compiled}
}

func (t *TorrentsCSV) SetConfig(cfg *config.Config) {
	t.BaseTorrentService.SetConfig(cfg)
}

func (t *TorrentsCSV) SearchTorrents(query string, mediaType string, season, episode int) (*models.TorrentResults, error) {
	return t.performSearch(query, mediaType, season, episode, false)
}

// SearchTorrentsSpecificEpisode searches for a specific episode only
func (t *TorrentsCSV) SearchTorrentsSpecificEpisode(query string, mediaType string, season, episode int) (*models.TorrentResults, error) {
	return t.performSearch(query, mediaType, season, episode, true)
}

// performSearch executes the API call and processes results
func (t *TorrentsCSV) performSearch(query string, mediaType string, season, episode int, specificEpisode bool) (*models.TorrentResults, error) {
	// Check cache first
	cacheKey := t.getCacheKey(specificEpisode)
	if cached, found := t.GetCachedSearch(cacheKey, query, mediaType, season, episode); found {
		return cached, nil
	}

	// Rate limit the API call
	t.rateLimiter.Wait()

	// Build search query and API URL
	searchQuery := t.buildSearchQueryWithMode(query, mediaType, season, episode, specificEpisode)
	apiURL := t.buildAPIURL(searchQuery)

	// API call to search torrents

	// Fetch torrents from API
	torrents, err := t.fetchTorrents(apiURL)
	if err != nil {
		return nil, err
	}

	// Filter out movie packs/collections for movie searches
	if mediaType == "movie" {
		torrents = t.filterOutMoviePacks(torrents)
	}

	// Sort torrents by size (largest first) for better cache hit probability
	sort.Slice(torrents, func(i, j int) bool {
		return torrents[i].SizeBytes > torrents[j].SizeBytes
	})

	// Add source and log sample torrents
	t.processTorrentMetadata(torrents)
	// API call completed

	// Process and cache results
	results := t.processTorrents(torrents, mediaType, season, episode)
	t.CacheSearch(cacheKey, query, mediaType, season, episode, results)

	return results, nil
}

// getCacheKey returns the appropriate cache key based on search mode
func (t *TorrentsCSV) getCacheKey(specificEpisode bool) string {
	if specificEpisode {
		return torrentsCSVEpisodeCacheKey
	}
	return torrentsCSVGeneralCacheKey
}

// buildAPIURL constructs the TorrentsCSV API URL
func (t *TorrentsCSV) buildAPIURL(searchQuery string) string {
	// Replace spaces with + for proper query formatting
	query := strings.ReplaceAll(searchQuery, " ", "+")
	return fmt.Sprintf("%s%s?q=%s",
		torrentsCSVAPIBase, torrentsCSVSearchEndpoint, query)
}

// buildSearchQueryWithMode builds the search query using the base service method
func (t *TorrentsCSV) buildSearchQueryWithMode(query string, mediaType string, season, episode int, specificEpisode bool) string {
	return t.BaseTorrentService.BuildSearchQueryWithMode(query, mediaType, season, episode, specificEpisode)
}

// fetchTorrents makes the API call and returns the torrents
func (t *TorrentsCSV) fetchTorrents(apiURL string) ([]TorrentsCSVTorrent, error) {
	resp, err := t.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search TorrentsCSV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TorrentsCSV API error: status %d", resp.StatusCode)
	}

	var response TorrentsCSVResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode TorrentsCSV response: %w", err)
	}

	return response.Torrents, nil
}

// processTorrentMetadata adds source information to torrents
func (t *TorrentsCSV) processTorrentMetadata(torrents []TorrentsCSVTorrent) {
	for i := range torrents {
		torrents[i].Source = "TORRENTSCSV"
	}
}

// processTorrents converts TorrentsCSV torrents to generic format and processes them
func (t *TorrentsCSV) processTorrents(torrents []TorrentsCSVTorrent, mediaType string, season, episode int) *models.TorrentResults {
	year := t.extractYearFromTorrents(torrents, mediaType)
	genericTorrents := WrapTorrentsCSVTorrents(torrents)
	return t.BaseTorrentService.ProcessTorrents(genericTorrents, mediaType, season, episode, "TORRENTSCSV", year)
}

// extractYearFromTorrents extracts year from torrent names for movies
func (t *TorrentsCSV) extractYearFromTorrents(torrents []TorrentsCSVTorrent, mediaType string) int {
	if mediaType != "movie" {
		return 0
	}

	// Try to extract year from torrent names
	for _, torrent := range torrents {
		if year := t.yearExtractor.extractYear(torrent.Name); year > 0 {
			return year
		}
	}
	return 0
}

// extractYear extracts a year from a title using compiled regex patterns
func (e *yearExtractor) extractYear(title string) int {
	for _, pattern := range e.patterns {
		if matches := pattern.FindStringSubmatch(title); len(matches) > 1 {
			if year, err := strconv.Atoi(matches[1]); err == nil {
				return year
			}
		}
	}
	return 0
}

// filterOutMoviePacks removes movie collections/packs from results
func (t *TorrentsCSV) filterOutMoviePacks(torrents []TorrentsCSVTorrent) []TorrentsCSVTorrent {
	// Patterns that indicate movie packs/collections
	packPatterns := []string{
		"collection",
		"trilogy",
		"quadrilogy",
		"pentalogy", // Fixed: was "pentology"
		"hexalogy",
		"saga",
		"complete",
		"1-2", "1-3", "1-4", "1-5", "1-6", "1-7", "1-8", "1-9",
		"I-II", "I-III", "I-IV", "I-V", "I-VI",
		"duology",
		"anthology",
		"box set",
		"boxset",
		" pack",
		"movie series",
		"film series",
		"all parts",
		"all movies",
		"movies collection",
	}
	
	filtered := make([]TorrentsCSVTorrent, 0, len(torrents))
	for _, torrent := range torrents {
		nameLower := strings.ToLower(torrent.Name)
		
		// Check if name contains any pack pattern
		isPack := false
		for _, pattern := range packPatterns {
			if strings.Contains(nameLower, pattern) {
				isPack = true
				break
			}
		}
		
		// Also check for year ranges like 1996-2023 or (1996-2023) - without requiring parentheses
		yearRangePattern := regexp.MustCompile(`\d{4}\s*[-–—]\s*\d{4}`)
		if yearRangePattern.MatchString(torrent.Name) {
			isPack = true
		}
		
		if !isPack {
			filtered = append(filtered, torrent)
		}
	}
	
	return filtered
}

type TorrentsCSVTorrentWrapper struct {
	TorrentsCSVTorrent
}

func (t TorrentsCSVTorrentWrapper) GetID() string    { return strconv.FormatInt(t.RowID, 10) }
func (t TorrentsCSVTorrentWrapper) GetTitle() string { return t.Name }
func (t TorrentsCSVTorrentWrapper) GetHash() string {
	return strings.ToLower(t.InfoHash)
}
func (t TorrentsCSVTorrentWrapper) GetSource() string { return t.Source }
func (t TorrentsCSVTorrentWrapper) GetType() string   { return "" }
func (t TorrentsCSVTorrentWrapper) GetSeason() int    { return 0 }
func (t TorrentsCSVTorrentWrapper) GetEpisode() int   { return 0 }
func (t TorrentsCSVTorrentWrapper) GetSize() int64    { return t.SizeBytes }

func WrapTorrentsCSVTorrents(torrents []TorrentsCSVTorrent) []GenericTorrent {
	generic := make([]GenericTorrent, len(torrents))
	for i, torrent := range torrents {
		generic[i] = TorrentsCSVTorrentWrapper{torrent}
	}
	return generic
}
