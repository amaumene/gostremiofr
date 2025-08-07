package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/utils"
)

const (
	yggAPIBase         = "https://yggapi.eu"
	yggSearchEndpoint  = "/torrents"
	yggTorrentEndpoint = "/torrent"
	movieCategories    = "&category_id=2178&category_id=2181&category_id=2183"
	seriesCategories   = "&category_id=2179&category_id=2181&category_id=2182&category_id=2184"
	defaultPage        = 1
	defaultPerPage     = 100
	yggTimeout         = 30 * time.Second
)

// YGGProvider implements the TorrentProvider interface for YGG API.
type YGGProvider struct {
	httpClient *http.Client
	cache      Cache
}

// YGGTorrent represents a torrent from YGG API.
type YGGTorrent struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Size     int64  `json:"size"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Hash     string `json:"hash,omitempty"`
}

// YGGTorrentDetail represents detailed torrent information from YGG API.
type YGGTorrentDetail struct {
	ID   int    `json:"id"`
	Hash string `json:"hash"`
}

// NewYGGProvider creates a new YGG provider instance.
func NewYGGProvider() *YGGProvider {
	return &YGGProvider{
		httpClient: &http.Client{
			Timeout: yggTimeout,
		},
	}
}

// SetCache sets the cache for the YGG provider.
func (y *YGGProvider) SetCache(cache interface{}) {
	if c, ok := cache.(Cache); ok {
		y.cache = c
	}
}

// Search searches for torrents using YGG API.
func (y *YGGProvider) Search(options models.SearchOptions) (*models.SearchResults, error) {
	cacheKey := y.buildCacheKey(options)

	if cached := y.getCachedResults(cacheKey); cached != nil {
		return cached, nil
	}

	query := utils.BuildSearchQuery(options.Query, options.MediaType, options.Season, options.Episode, options.SpecificEpisode)
	torrents, err := y.fetchTorrents(y.buildAPIURL(query, options.MediaType))
	if err != nil {
		return nil, err
	}

	results := y.classifyTorrents(torrents, options)
	y.cacheResults(cacheKey, results)

	return results, nil
}

// buildCacheKey creates a cache key for the search options.
func (y *YGGProvider) buildCacheKey(options models.SearchOptions) string {
	return fmt.Sprintf("ygg_search:%s:%s:%d:%d", options.Query, options.MediaType, options.Season, options.Episode)
}

// getCachedResults retrieves cached search results.
func (y *YGGProvider) getCachedResults(cacheKey string) *models.SearchResults {
	if y.cache == nil {
		return nil
	}

	if cached, found := y.cache.Get(cacheKey); found {
		if results, ok := cached.(*models.SearchResults); ok {
			return results
		}
	}
	return nil
}

// cacheResults stores search results in cache.
func (y *YGGProvider) cacheResults(cacheKey string, results *models.SearchResults) {
	if y.cache != nil {
		y.cache.Set(cacheKey, results)
	}
}

// buildAPIURL constructs the YGG API URL with query parameters.
func (y *YGGProvider) buildAPIURL(query, mediaType string) string {
	// Query already has + for spaces from BuildSearchQuery, no need to escape
	categories := y.getCategoryParams(mediaType)
	return fmt.Sprintf("%s%s?q=%s&page=%d&per_page=%d%s",
		yggAPIBase, yggSearchEndpoint, query, defaultPage, defaultPerPage, categories)
}

// getCategoryParams returns category parameters for the given media type.
func (y *YGGProvider) getCategoryParams(mediaType string) string {
	switch mediaType {
	case "movie":
		return movieCategories
	case "series":
		return seriesCategories
	default:
		return ""
	}
}

// fetchTorrents makes HTTP request to YGG API and returns torrent list.
func (y *YGGProvider) fetchTorrents(apiURL string) ([]YGGTorrent, error) {
	// Debug log the API URL (this will be captured by the parent handler)
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search YGG: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YGG API returned status %d", resp.StatusCode)
	}

	return y.decodeTorrents(resp.Body)
}

// decodeTorrents reads and unmarshals JSON response body.
func (y *YGGProvider) decodeTorrents(body io.Reader) ([]YGGTorrent, error) {
	bytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read YGG response: %w", err)
	}

	// YGG API returns an array directly, not wrapped in an object
	var torrents []YGGTorrent
	if err := json.Unmarshal(bytes, &torrents); err != nil {
		return nil, fmt.Errorf("failed to decode YGG response: %w", err)
	}

	return torrents, nil
}

// classifyTorrents converts YGG torrents to SearchResults with proper classification.
func (y *YGGProvider) classifyTorrents(torrents []YGGTorrent, options models.SearchOptions) *models.SearchResults {
	results := &models.SearchResults{
		MovieTorrents:          []models.TorrentInfo{},
		CompleteSeriesTorrents: []models.TorrentInfo{},
		CompleteSeasonTorrents: []models.TorrentInfo{},
		EpisodeTorrents:        []models.TorrentInfo{},
	}

	for _, torrent := range torrents {
		info := y.buildTorrentInfo(torrent)
		y.classifyAndAdd(info, torrent, options, results)
	}

	return results
}

// buildTorrentInfo converts YGGTorrent to TorrentInfo.
func (y *YGGProvider) buildTorrentInfo(torrent YGGTorrent) models.TorrentInfo {
	return models.TorrentInfo{
		ID:       fmt.Sprintf("%d", torrent.ID),
		Title:    torrent.Title,
		Hash:     torrent.Hash,
		Source:   ProviderYGG,
		Size:     torrent.Size,
		Seeders:  torrent.Seeders,
		Leechers: torrent.Leechers,
	}
}

// classifyAndAdd classifies torrent and adds to appropriate result category.
func (y *YGGProvider) classifyAndAdd(info models.TorrentInfo, torrent YGGTorrent, options models.SearchOptions, results *models.SearchResults) {
	switch options.MediaType {
	case "movie":
		results.MovieTorrents = append(results.MovieTorrents, info)
	case "series":
		y.classifySeries(info, torrent, options, results)
	}
}

// classifySeries classifies series torrents by episode or season.
func (y *YGGProvider) classifySeries(info models.TorrentInfo, torrent YGGTorrent, options models.SearchOptions, results *models.SearchResults) {
	if options.Episode > 0 && utils.MatchesEpisode(torrent.Title, options.Season, options.Episode) {
		results.EpisodeTorrents = append(results.EpisodeTorrents, info)
		return
	}

	if options.Season > 0 && utils.MatchesSeason(torrent.Title, options.Season) {
		results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, info)
		return
	}

	results.EpisodeTorrents = append(results.EpisodeTorrents, info)
}


// GetTorrentHash fetches the torrent hash for a given ID from YGG API.
func (y *YGGProvider) GetTorrentHash(torrentID string) (string, error) {
	cacheKey := fmt.Sprintf("ygg_hash:%s", torrentID)

	if cached := y.getCachedHash(cacheKey); cached != "" {
		return cached, nil
	}

	hash, err := y.fetchTorrentHash(torrentID)
	if err != nil {
		return "", err
	}

	y.cacheHash(cacheKey, hash)
	return hash, nil
}

// getCachedHash retrieves cached torrent hash.
func (y *YGGProvider) getCachedHash(cacheKey string) string {
	if y.cache == nil {
		return ""
	}

	if cached, found := y.cache.Get(cacheKey); found {
		if hash, ok := cached.(string); ok {
			return hash
		}
	}
	return ""
}

// cacheHash stores torrent hash in cache.
func (y *YGGProvider) cacheHash(cacheKey, hash string) {
	if y.cache != nil && hash != "" {
		y.cache.Set(cacheKey, hash)
	}
}

// fetchTorrentHash makes HTTP request to get torrent hash.
func (y *YGGProvider) fetchTorrentHash(torrentID string) (string, error) {
	apiURL := fmt.Sprintf("%s%s/%s", yggAPIBase, yggTorrentEndpoint, torrentID)
	// Debug log the hash fetch URL (will be captured by parent handler)
	
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to get YGG hash: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YGG API returned status %d", resp.StatusCode)
	}

	return y.decodeTorrentHash(resp.Body)
}

// decodeTorrentHash reads and unmarshals torrent detail response.
func (y *YGGProvider) decodeTorrentHash(body io.Reader) (string, error) {
	bytes, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("failed to read YGG response: %w", err)
	}

	var detail YGGTorrentDetail
	if err := json.Unmarshal(bytes, &detail); err != nil {
		return "", fmt.Errorf("failed to decode YGG response: %w", err)
	}

	return detail.Hash, nil
}