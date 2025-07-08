package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)

const (
	// Apibay API configuration
	apibayAPIBase        = "https://apibay.org"
	apibaySearchEndpoint = "/q.php"
	apibayVideoCategory  = "video"

	// Cache keys
	apibayGeneralCacheKey = "APIBAY"
	apibayEpisodeCacheKey = "APIBAY_EPISODE"

	// Rate limiting
	apibayRateLimit  = 5 // requests per second
	apibayCacheBurst = 2

	// Logging
	apibayLogLimit = 5 // number of torrents to log for debugging
)

type Apibay struct {
	*BaseTorrentService
	yearExtractor *yearExtractor
}

// yearExtractor handles year extraction from torrent titles
type yearExtractor struct {
	patterns []*regexp.Regexp
}

type ApibayTorrent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	InfoHash string `json:"info_hash"`
	Seeders  string `json:"seeders"`
	Leechers string `json:"leechers"`
	Size     string `json:"size"`
	Category string `json:"category"`
	Added    string `json:"added"`
	IMDB     string `json:"imdb"`
	Source   string `json:"-"` // Set at runtime, not from JSON
}

// Response is an array of torrents
type ApibayResponse []ApibayTorrent

func NewApibay(db database.Database, cache *cache.LRUCache) *Apibay {
	return &Apibay{
		BaseTorrentService: NewBaseTorrentService(db, cache, apibayRateLimit, apibayCacheBurst),
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

func (a *Apibay) SetConfig(cfg *config.Config) {
	a.BaseTorrentService.SetConfig(cfg)
}

func (a *Apibay) SearchTorrents(query string, mediaType string, season, episode int) (*models.TorrentResults, error) {
	return a.performSearch(query, mediaType, season, episode, false)
}

// SearchTorrentsSpecificEpisode searches for a specific episode only
func (a *Apibay) SearchTorrentsSpecificEpisode(query string, mediaType string, season, episode int) (*models.TorrentResults, error) {
	return a.performSearch(query, mediaType, season, episode, true)
}

// performSearch executes the API call and processes results
func (a *Apibay) performSearch(query string, mediaType string, season, episode int, specificEpisode bool) (*models.TorrentResults, error) {
	// Check cache first
	cacheKey := a.getCacheKey(specificEpisode)
	if cached, found := a.GetCachedSearch(cacheKey, query, mediaType, season, episode); found {
		return cached, nil
	}

	// Rate limit the API call
	a.rateLimiter.Wait()

	// Build search query and API URL
	searchQuery := a.buildSearchQueryWithMode(query, mediaType, season, episode, specificEpisode)
	apiURL := a.buildAPIURL(searchQuery)

	a.logger.Infof("[APIBAY] API call to search torrents - URL: %s", apiURL)

	// Fetch torrents from API
	torrents, err := a.fetchTorrents(apiURL)
	if err != nil {
		return nil, err
	}

	// Add source and log sample torrents
	a.processTorrentMetadata(torrents)
	a.logger.Infof("[APIBAY] API call completed - found %d torrents for query: %s", len(torrents), query)

	// Process and cache results
	results := a.processTorrents(torrents, mediaType, season, episode)
	a.CacheSearch(cacheKey, query, mediaType, season, episode, results)

	return results, nil
}

// getCacheKey returns the appropriate cache key based on search mode
func (a *Apibay) getCacheKey(specificEpisode bool) string {
	if specificEpisode {
		return apibayEpisodeCacheKey
	}
	return apibayGeneralCacheKey
}

// buildAPIURL constructs the Apibay API URL
func (a *Apibay) buildAPIURL(searchQuery string) string {
	return fmt.Sprintf("%s%s?q=%s&cat=%s",
		apibayAPIBase, apibaySearchEndpoint, url.QueryEscape(searchQuery), apibayVideoCategory)
}

// buildSearchQueryWithMode builds the search query using the base service method
func (a *Apibay) buildSearchQueryWithMode(query string, mediaType string, season, episode int, specificEpisode bool) string {
	return a.BaseTorrentService.BuildSearchQueryWithMode(query, mediaType, season, episode, specificEpisode)
}

// fetchTorrents makes the API call and returns the torrents
func (a *Apibay) fetchTorrents(apiURL string) ([]ApibayTorrent, error) {
	resp, err := a.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search Apibay: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Apibay API error: status %d", resp.StatusCode)
	}

	var torrents []ApibayTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("failed to decode Apibay response: %w", err)
	}

	return torrents, nil
}

// processTorrentMetadata adds source information and logs sample torrents
func (a *Apibay) processTorrentMetadata(torrents []ApibayTorrent) {
	for i := range torrents {
		torrents[i].Source = "APIBAY"
		// Log first few torrents for debugging
		if i < apibayLogLimit {
			a.logger.Infof("[APIBAY] torrent %d: %s (hash: %s, seeders: %s)",
				i+1, torrents[i].Name, torrents[i].InfoHash, torrents[i].Seeders)
		}
	}
}

// processTorrents converts Apibay torrents to generic format and processes them
func (a *Apibay) processTorrents(torrents []ApibayTorrent, mediaType string, season, episode int) *models.TorrentResults {
	year := a.extractYearFromTorrents(torrents, mediaType)
	genericTorrents := WrapApibayTorrents(torrents)
	return a.BaseTorrentService.ProcessTorrents(genericTorrents, mediaType, season, episode, "APIBAY", year)
}

// extractYearFromTorrents extracts year from torrent names for movies
func (a *Apibay) extractYearFromTorrents(torrents []ApibayTorrent, mediaType string) int {
	if mediaType != "movie" {
		return 0
	}

	// Try to extract year from torrent names
	for _, torrent := range torrents {
		if year := a.yearExtractor.extractYear(torrent.Name); year > 0 {
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

// Wrapper type that implements GenericTorrent interface
type ApibayTorrentWrapper struct {
	ApibayTorrent
}

func (a ApibayTorrentWrapper) GetID() string    { return a.ID }
func (a ApibayTorrentWrapper) GetTitle() string { return a.Name }
func (a ApibayTorrentWrapper) GetHash() string {
	// Apibay returns uppercase hashes, convert to lowercase for consistency
	return strings.ToLower(a.InfoHash)
}
func (a ApibayTorrentWrapper) GetSource() string   { return a.Source }
func (a ApibayTorrentWrapper) GetLanguage() string { return "" } // Parse from title
func (a ApibayTorrentWrapper) GetType() string     { return "" } // Determine from title
func (a ApibayTorrentWrapper) GetSeason() int      { return 0 }  // Parse from title
func (a ApibayTorrentWrapper) GetEpisode() int     { return 0 }  // Parse from title
func (a ApibayTorrentWrapper) GetSize() int64 {
	if size, err := strconv.ParseInt(a.Size, 10, 64); err == nil {
		return size
	}
	return 0
}

// Helper function to convert slice to GenericTorrent slice
func WrapApibayTorrents(torrents []ApibayTorrent) []GenericTorrent {
	generic := make([]GenericTorrent, len(torrents))
	for i, torrent := range torrents {
		generic[i] = ApibayTorrentWrapper{torrent}
	}
	return generic
}
