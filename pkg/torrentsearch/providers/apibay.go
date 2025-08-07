// Package providers contains torrent search provider implementations.
package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/utils"
)

const (
	apibayAPIBase        = "https://apibay.org"
	apibaySearchEndpoint = "/q.php"
	apibayVideoCategory  = "video"
	apibayTimeout        = 30 * time.Second
)

// Cache defines the interface for caching search results.
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
}

// ApiBayProvider implements the TorrentProvider interface for ApiBay API.
type ApiBayProvider struct {
	httpClient    *http.Client
	cache         Cache
	yearExtractor *yearExtractor
}

type yearExtractor struct {
	patterns []*regexp.Regexp
}

// ApiBayTorrent represents a torrent from ApiBay API.
type ApiBayTorrent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	InfoHash string `json:"info_hash"`
	Seeders  string `json:"seeders"`
	Leechers string `json:"leechers"`
	Size     string `json:"size"`
	Category string `json:"category"`
	Added    string `json:"added"`
	IMDB     string `json:"imdb"`
}

// ApiBayResponse represents the API response from ApiBay.
type ApiBayResponse []ApiBayTorrent

// NewApiBayProvider creates a new ApiBay provider instance.
func NewApiBayProvider() *ApiBayProvider {
	return &ApiBayProvider{
		httpClient: &http.Client{
			Timeout: apibayTimeout,
		},
		yearExtractor: newYearExtractor(),
	}
}

var yearPatterns = []string{
	`\((\d{4})\)`,   // (2022)
	`\[(\d{4})\]`,   // [2022]
	`\b(19\d{2})\b`, // 19xx
	`\b(20\d{2})\b`, // 20xx
}

// newYearExtractor creates a year extractor with compiled regex patterns.
func newYearExtractor() *yearExtractor {
	compiled := make([]*regexp.Regexp, len(yearPatterns))
	for i, pattern := range yearPatterns {
		compiled[i] = regexp.MustCompile(pattern)
	}
	return &yearExtractor{patterns: compiled}
}

// SetCache sets the cache for the ApiBay provider.
func (a *ApiBayProvider) SetCache(cache interface{}) {
	if c, ok := cache.(Cache); ok {
		a.cache = c
	}
}

// Search searches for torrents using ApiBay API.
func (a *ApiBayProvider) Search(options models.SearchOptions) (*models.SearchResults, error) {
	cacheKey := a.buildCacheKey(options)

	if cached := a.getCachedResults(cacheKey); cached != nil {
		return cached, nil
	}

	query := utils.BuildSearchQuery(options.Query, options.MediaType, options.Season, options.Episode, options.SpecificEpisode)
	torrents, err := a.fetchTorrents(a.buildAPIURL(query))
	if err != nil {
		return nil, err
	}

	results := a.classifyTorrents(torrents, options)
	a.cacheResults(cacheKey, results)

	return results, nil
}

// buildCacheKey creates a cache key for the search options.
func (a *ApiBayProvider) buildCacheKey(options models.SearchOptions) string {
	return fmt.Sprintf("apibay_search:%s:%s:%d:%d", options.Query, options.MediaType, options.Season, options.Episode)
}

// getCachedResults retrieves cached search results.
func (a *ApiBayProvider) getCachedResults(cacheKey string) *models.SearchResults {
	if a.cache == nil {
		return nil
	}

	if cached, found := a.cache.Get(cacheKey); found {
		if results, ok := cached.(*models.SearchResults); ok {
			return results
		}
	}
	return nil
}

// cacheResults stores search results in cache.
func (a *ApiBayProvider) cacheResults(cacheKey string, results *models.SearchResults) {
	if a.cache != nil {
		a.cache.Set(cacheKey, results)
	}
}

// buildAPIURL constructs the ApiBay API URL with query parameters.
func (a *ApiBayProvider) buildAPIURL(query string) string {
	encodedQuery := url.QueryEscape(query)
	return fmt.Sprintf("%s%s?q=%s&cat=%s",
		apibayAPIBase, apibaySearchEndpoint, encodedQuery, apibayVideoCategory)
}

// fetchTorrents makes HTTP request to ApiBay API and returns torrent list.
func (a *ApiBayProvider) fetchTorrents(apiURL string) ([]ApiBayTorrent, error) {
	resp, err := a.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search ApiBay: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ApiBay API returned status %d", resp.StatusCode)
	}

	return a.decodeTorrents(resp.Body)
}

// decodeTorrents reads and unmarshals JSON response body.
func (a *ApiBayProvider) decodeTorrents(body io.Reader) ([]ApiBayTorrent, error) {
	bytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ApiBay response: %w", err)
	}

	var torrents []ApiBayTorrent
	if err := json.Unmarshal(bytes, &torrents); err != nil {
		return nil, fmt.Errorf("failed to decode ApiBay response: %w", err)
	}

	// Check for "No results returned" response
	if len(torrents) == 1 && torrents[0].Name == "No results returned" && torrents[0].ID == "0" {
		return []ApiBayTorrent{}, nil
	}

	return torrents, nil
}

// classifyTorrents converts ApiBay torrents to SearchResults with proper classification.
func (a *ApiBayProvider) classifyTorrents(torrents []ApiBayTorrent, options models.SearchOptions) *models.SearchResults {
	results := &models.SearchResults{
		MovieTorrents:          []models.TorrentInfo{},
		CompleteSeriesTorrents: []models.TorrentInfo{},
		CompleteSeasonTorrents: []models.TorrentInfo{},
		EpisodeTorrents:        []models.TorrentInfo{},
	}

	for _, torrent := range torrents {
		// Skip invalid entries
		if torrent.ID == "0" || torrent.InfoHash == "0000000000000000000000000000000000000000" {
			continue
		}
		info := a.buildTorrentInfo(torrent)
		a.classifyAndAdd(info, torrent, options, results)
	}

	return results
}

// buildTorrentInfo converts ApiBayTorrent to TorrentInfo.
func (a *ApiBayProvider) buildTorrentInfo(torrent ApiBayTorrent) models.TorrentInfo {
	seeders, _ := strconv.Atoi(torrent.Seeders)
	leechers, _ := strconv.Atoi(torrent.Leechers)
	size, _ := strconv.ParseInt(torrent.Size, 10, 64)

	return models.TorrentInfo{
		ID:       torrent.ID,
		Title:    torrent.Name,
		Hash:     strings.ToLower(torrent.InfoHash),
		Source:   ProviderApiBay,
		Size:     size,
		Seeders:  seeders,
		Leechers: leechers,
	}
}

// classifyAndAdd classifies torrent and adds to appropriate result category.
func (a *ApiBayProvider) classifyAndAdd(info models.TorrentInfo, torrent ApiBayTorrent, options models.SearchOptions, results *models.SearchResults) {
	switch options.MediaType {
	case "movie":
		results.MovieTorrents = append(results.MovieTorrents, info)
	case "series":
		a.classifySeries(info, torrent, options, results)
	}
}

// classifySeries classifies series torrents by episode or season.
func (a *ApiBayProvider) classifySeries(info models.TorrentInfo, torrent ApiBayTorrent, options models.SearchOptions, results *models.SearchResults) {
	if options.Episode > 0 && utils.MatchesEpisode(torrent.Name, options.Season, options.Episode) {
		results.EpisodeTorrents = append(results.EpisodeTorrents, info)
		return
	}

	if options.Season > 0 && utils.MatchesSeason(torrent.Name, options.Season) {
		results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, info)
		return
	}

	results.EpisodeTorrents = append(results.EpisodeTorrents, info)
}

// GetTorrentHash returns the torrent hash for a given ID.
// ApiBay includes hashes in search results, so this method is not needed.
func (a *ApiBayProvider) GetTorrentHash(torrentID string) (string, error) {
	return "", fmt.Errorf("ApiBay provider: hash already included in search results")
}

// extractYear extracts year from torrent title using regex patterns.
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