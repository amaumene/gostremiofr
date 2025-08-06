package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)

const (
	// YGG API endpoints
	yggAPIBase         = "https://yggapi.eu"
	yggSearchEndpoint  = "/torrents"
	yggTorrentEndpoint = "/torrent"

	// YGG category IDs
	movieCategories  = "&category_id=2178&category_id=2181&category_id=2183"
	seriesCategories = "&category_id=2179&category_id=2181&category_id=2182&category_id=2184"

	// API parameters
	defaultPage              = 1
	defaultPerPage           = 100
	maxConcurrentHashFetches = 5
)

type YGG struct {
	*BaseTorrentService
	tmdbService     TMDBService
	titleTranslator *frenchTitleTranslator
}

// frenchTitleTranslator handles French title translation using TMDB
type frenchTitleTranslator struct {
	tmdbService TMDBService
	cache       *cache.LRUCache
}

type yggAPIConfig struct {
	baseURL    string
	page       int
	perPage    int
	categories string
}

func NewYGG(db database.Database, cache *cache.LRUCache, tmdbService TMDBService) *YGG {
	baseTorrentService := NewBaseTorrentService(db, cache, 10, 2)
	return &YGG{
		BaseTorrentService: baseTorrentService,
		tmdbService:        tmdbService,
		titleTranslator: &frenchTitleTranslator{
			tmdbService: tmdbService,
			cache:       cache,
		},
	}
}

func (y *YGG) SetConfig(cfg *config.Config) {
	y.BaseTorrentService.SetConfig(cfg)
}

// buildAPIURL constructs the YGG API URL with proper parameters
func (y *YGG) buildAPIURL(query, category string) string {
	encodedQuery := url.QueryEscape(query)
	categoryParams := y.mapMediaTypeToCategoryID(category)
	return fmt.Sprintf("%s%s?q=%s&page=%d&per_page=%d%s",
		yggAPIBase, yggSearchEndpoint, encodedQuery, defaultPage, defaultPerPage, categoryParams)
}

// getCategoryParams returns the appropriate category parameters for the content type
func (y *YGG) mapMediaTypeToCategoryID(category string) string {
	switch category {
	case "movie":
		return movieCategories
	case "series":
		return seriesCategories
	default:
		return ""
	}
}

// buildTorrentURL constructs the URL for fetching torrent details
func (y *YGG) buildTorrentURL(torrentID string) string {
	return fmt.Sprintf("%s%s/%s", yggAPIBase, yggTorrentEndpoint, torrentID)
}

func (y *YGG) translateToFrenchTitle(originalTitle string) string {
	return y.titleTranslator.translateTitle(originalTitle)
}

func (t *frenchTitleTranslator) translateTitle(originalTitle string) string {
	if t.tmdbService == nil {
		return originalTitle
	}

	result, err := t.searchTMDBForTitle(originalTitle)
	if err != nil {
		// Could not find French title, using original
		return originalTitle
	}

	return t.getFrenchTitle(originalTitle, result)
}

// extractTMDBID extracts the TMDB ID from a result ID string
func (t *frenchTitleTranslator) extractTMDBID(resultID string) string {
	if len(resultID) > 5 && resultID[:5] == "tmdb:" {
		return resultID[5:]
	}
	return ""
}

func (t *frenchTitleTranslator) fetchFrenchMetadata(mediaType, tmdbID string) (*models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb_french:%s:%s", mediaType, tmdbID)
	if data, found := t.cache.Get(cacheKey); found {
		if meta, ok := data.(*models.Meta); ok {
			return meta, nil
		}
	}

	apiKey := t.retrieveTMDBAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not available")
	}

	// Build API URL based on media type
	apiURL := t.buildTMDBURL(mediaType, tmdbID, apiKey)

	// Make API call
	meta, err := t.fetchMetadataFromAPI(apiURL, mediaType)
	if err != nil {
		return nil, err
	}

	// Cache the result
	t.cache.Set(cacheKey, meta)
	return meta, nil
}

// buildTMDBURL constructs the TMDB API URL for French metadata
func (t *frenchTitleTranslator) buildTMDBURL(mediaType, tmdbID, apiKey string) string {
	baseURL := "https://api.themoviedb.org/3"
	if mediaType == "movie" {
		return fmt.Sprintf("%s/movie/%s?api_key=%s&language=fr-FR", baseURL, tmdbID, apiKey)
	}
	return fmt.Sprintf("%s/tv/%s?api_key=%s&language=fr-FR", baseURL, tmdbID, apiKey)
}

// fetchFrenchMetadata makes the HTTP request and parses the response
func (t *frenchTitleTranslator) fetchMetadataFromAPI(apiURL, mediaType string) (*models.Meta, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch French metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var meta models.Meta
	if mediaType == "movie" {
		var movieDetails models.TMDBMovieDetails
		if err := json.NewDecoder(resp.Body).Decode(&movieDetails); err != nil {
			return nil, fmt.Errorf("failed to decode movie details: %w", err)
		}
		meta.Name = movieDetails.Title
		meta.Type = "movie"
	} else {
		var tvDetails models.TMDBTVDetails
		if err := json.NewDecoder(resp.Body).Decode(&tvDetails); err != nil {
			return nil, fmt.Errorf("failed to decode TV details: %w", err)
		}
		meta.Name = tvDetails.Name
		meta.Type = "series"
	}

	return &meta, nil
}

// getAPIKey gets the TMDB API key from the TMDB service
func (t *frenchTitleTranslator) retrieveTMDBAPIKey() string {
	if tmdb, ok := t.tmdbService.(*TMDB); ok {
		return tmdb.apiKey
	}
	return ""
}

func (y *YGG) buildSearchQuery(query string, category string, season, episode int) string {
	// For YGG (French torrent site), try to get French title for better results
	frenchQuery := y.translateToFrenchTitle(query)
	if frenchQuery != query {
		// Using French title for search
	}

	// Use the base method with the French-translated query
	return y.BaseTorrentService.BuildSearchQuery(frenchQuery, category, season, episode)
}

func (y *YGG) SearchTorrents(query string, category string, season, episode int) (*models.TorrentResults, error) {
	// Check cache using generic method
	if result, found := y.GetCachedSearch("YGG", query, category, season, episode); found {
		return result, nil
	}

	// Build the search query in consistent format
	searchQuery := y.buildSearchQuery(query, category, season, episode)

	// Use unified search method
	result, err := y.performSearch(searchQuery, category, season, episode, false)
	if err != nil {
		return nil, err
	}

	// Cache the result using generic method
	y.CacheSearch("YGG", query, category, season, episode, result)

	return result, nil
}

// SearchTorrentsSpecificEpisode searches for a specific episode using episode-specific query
func (y *YGG) SearchTorrentsSpecificEpisode(query string, category string, season, episode int) (*models.TorrentResults, error) {
	// Build episode-specific query
	frenchQuery := y.translateToFrenchTitle(query)
	if frenchQuery != query {
		// Using French title for specific episode search
	}

	// Try episode-specific query first
	searchQuery := y.BaseTorrentService.BuildSearchQueryWithMode(frenchQuery, category, season, episode, true)

	result, err := y.performSearch(searchQuery, category, season, episode, true)
	if err == nil && len(result.EpisodeTorrents) > 0 {
		return result, nil
	}

	// If episode-specific search fails, fall back to season search and filter
	// Episode-specific search failed, falling back to season search
	seasonQuery := y.BaseTorrentService.BuildSearchQueryWithMode(frenchQuery, category, season, episode, false)
	return y.performSearch(seasonQuery, category, season, episode, false)
}

// performSearch executes the API call and processes results
func (y *YGG) performSearch(searchQuery, category string, season, episode int, isSpecificEpisode bool) (*models.TorrentResults, error) {
	queryType := y.getQueryType(isSpecificEpisode)
	// Searching torrents

	torrents, err := y.searchAndFetchTorrents(searchQuery, category, queryType)
	if err != nil {
		return nil, err
	}

	results := y.processTorrents(torrents, category, season, episode)
	y.fetchHashesForResults(results, category, season, episode)

	return results, nil
}

func (y *YGG) getQueryType(isSpecificEpisode bool) string {
	if isSpecificEpisode {
		return "specific episode"
	}
	return "season"
}

func (y *YGG) searchAndFetchTorrents(searchQuery, category, queryType string) ([]models.YggTorrent, error) {
	y.rateLimiter.Wait()

	apiURL := y.buildAPIURL(searchQuery, category)
	// Making API call

	torrents, err := y.fetchTorrents(apiURL, queryType)
	if err != nil {
		return nil, err
	}

	// Received torrents from API
	y.addSourceToTorrents(torrents)
	
	return torrents, nil
}

func (y *YGG) addSourceToTorrents(torrents []models.YggTorrent) {
	for i := range torrents {
		torrents[i].Source = "YGG"
	}
}

func (y *YGG) processTorrents(torrents []models.YggTorrent, category string, season, episode int) *models.TorrentResults {
	return y.convertYggTorrentsToResults(torrents, category, season, episode)
}

func (y *YGG) fetchHashesForResults(results *models.TorrentResults, category string, season, episode int) {
	if category == "series" && season > 0 && episode > 0 {
		y.fetchHashesForEpisodeTorrents(results, season, episode)
	} else if len(results.EpisodeTorrents) > 0 {
		y.fetchHashesForTorrents(results.EpisodeTorrents)
	}
}

func (y *YGG) GetTorrentHash(torrentID string) (string, error) {
	// Check cache using generic method
	if hash, found := y.GetCachedHash("YGG", torrentID); found {
		return hash, nil
	}

	y.rateLimiter.Wait()

	apiURL := y.buildTorrentURL(torrentID)
	// Getting torrent hash from API

	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		// HTTP request failed
		return "", fmt.Errorf("failed to get torrent hash: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("YGG API returned status %d for torrent %s", resp.StatusCode, torrentID)
	}

	var result struct {
		Hash string `json:"hash"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Failed to decode JSON response
		return "", fmt.Errorf("failed to decode hash response: %w", err)
	}

	// Successfully retrieved hash

	// Cache the hash result using generic method
	y.CacheHash("YGG", torrentID, result.Hash)

	return result.Hash, nil
}

// fetchTorrents makes the API call and returns the torrents
func (y *YGG) fetchTorrents(apiURL, queryType string) ([]models.YggTorrent, error) {
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search YGG: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		// API returned non-200 status
		return []models.YggTorrent{}, nil
	}

	// Read the response body first to check if it's valid JSON
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read YGG response: %w", err)
	}

	// Check if response looks like an error message (starts with text)
	if len(body) > 0 && body[0] != '[' && body[0] != '{' {
		preview := string(body)
		if len(preview) > 100 {
			preview = preview[:100]
		}
		// API returned non-JSON response
		return []models.YggTorrent{}, nil
	}

	var torrents []models.YggTorrent
	if err := json.Unmarshal(body, &torrents); err != nil {
		// Failed to decode response, returning empty results
		return []models.YggTorrent{}, nil
	}

	return torrents, nil
}

// fetchHashesForEpisodeTorrents fetches hashes for torrents matching specific episodes
func (y *YGG) fetchHashesForEpisodeTorrents(results *models.TorrentResults, season, episode int) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	matchCount := 0

	// Searching for specific episode in torrents

	for i := range results.EpisodeTorrents {
		if y.BaseTorrentService.MatchesEpisode(results.EpisodeTorrents[i].Title, season, episode) {
			matchCount++
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				torrent := &results.EpisodeTorrents[index]
				// Fetching hash for episode match
				hash, err := y.GetTorrentHash(torrent.ID)

				mu.Lock()
				if err != nil {
					// Failed to fetch hash
				} else {
					torrent.Hash = hash
					// Hash fetched successfully
				}
				mu.Unlock()
			}(i)
		} else {
			// Torrent does not match episode
		}
	}

	if matchCount == 0 {
		// No torrents found matching episode
	} else {
		// Found torrents matching episode, fetching hashes
	}

	wg.Wait()
	// Completed hash fetching for episode
}

// fetchHashesForTorrents fetches hashes for a list of torrents with concurrency control
func (y *YGG) fetchHashesForTorrents(torrents []models.TorrentInfo) {
	if len(torrents) == 0 {
		return
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxConcurrentHashFetches)

	for i := range torrents {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			torrent := &torrents[idx]
			hash, err := y.GetTorrentHash(torrent.ID)
			if err != nil {
				// Failed to get hash for torrent
				return
			}
			torrent.Hash = hash
		}(i)
	}

	wg.Wait()
	// Completed hash fetching
}

func (y *YGG) convertYggTorrentsToResults(torrents []models.YggTorrent, category string, season, episode int) *models.TorrentResults {
	genericTorrents := WrapYggTorrents(torrents)
	return y.BaseTorrentService.ProcessTorrents(genericTorrents, category, season, episode, "YGG", 0)
}
