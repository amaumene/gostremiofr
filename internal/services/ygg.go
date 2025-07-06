package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)


type YGG struct {
	*BaseTorrentService
	tmdbService TMDBService
}

func NewYGG(db database.Database, cache *cache.LRUCache, tmdbService TMDBService) *YGG {
	return &YGG{
		BaseTorrentService: NewBaseTorrentService(db, cache, 10, 2),
		tmdbService:        tmdbService,
	}
}

func (y *YGG) SetConfig(cfg *config.Config) {
	y.BaseTorrentService.SetConfig(cfg)
}

// getFrenchTitle gets the French title for a given title by making a TMDB search
func (y *YGG) getFrenchTitle(originalTitle string) string {
	if y.tmdbService == nil {
		return originalTitle
	}
	
	// Try to find the movie/series via TMDB search
	searchResults, err := y.tmdbService.SearchMulti(originalTitle, 1)
	if err != nil || len(searchResults) == 0 {
		y.logger.Debugf("[YGG] could not find French title for '%s', using original", originalTitle)
		return originalTitle
	}
	
	// Get the first result and fetch its French metadata
	result := searchResults[0]
	
	// Extract TMDB ID from result ID (format: "tmdb:12345")
	tmdbID := ""
	if len(result.ID) > 5 && result.ID[:5] == "tmdb:" {
		tmdbID = result.ID[5:]
	} else {
		return originalTitle
	}
	
	// Get French metadata
	frenchMeta, err := y.getFrenchMetadata(result.Type, tmdbID)
	if err != nil {
		y.logger.Debugf("[YGG] could not get French metadata for '%s', using original", originalTitle)
		return originalTitle
	}
	
	// Return French title if different from original
	if frenchMeta.Name != originalTitle && frenchMeta.Name != "" {
		y.logger.Debugf("[YGG] using French title '%s' instead of '%s'", frenchMeta.Name, originalTitle)
		return frenchMeta.Name
	}
	
	return originalTitle
}

// getFrenchMetadata makes a specific TMDB API call with French language
func (y *YGG) getFrenchMetadata(mediaType, tmdbID string) (*models.Meta, error) {
	// Make a direct API call with French language parameter
	cacheKey := fmt.Sprintf("tmdb_french:%s:%s", mediaType, tmdbID)
	
	if data, found := y.cache.Get(cacheKey); found {
		meta := data.(*models.Meta)
		return meta, nil
	}
	
	y.rateLimiter.Wait()
	
	var apiURL string
	if mediaType == "movie" {
		apiURL = fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s&language=fr-FR", tmdbID, y.getAPIKey())
	} else {
		apiURL = fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s&language=fr-FR", tmdbID, y.getAPIKey())
	}
	
	resp, err := y.httpClient.Get(apiURL)
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
	
	y.cache.Set(cacheKey, &meta)
	return &meta, nil
}

// getAPIKey gets the TMDB API key from the TMDB service
func (y *YGG) getAPIKey() string {
	// This is a bit of a hack, but we need to access the API key from TMDB service
	// In a real implementation, we'd pass this through properly
	if tmdb, ok := y.tmdbService.(*TMDB); ok {
		return tmdb.apiKey
	}
	return ""
}

func (y *YGG) SearchTorrents(query string, category string, season, episode int) (*models.TorrentResults, error) {
	// Check cache using generic method
	if result, found := y.GetCachedSearch("YGG", query, category, season, episode); found {
		return result, nil
	}
	
	// For YGG (French torrent site), try to get French title for better results
	frenchQuery := y.getFrenchTitle(query)
	if frenchQuery != query {
		y.logger.Debugf("[YGG] using French title for search: '%s' -> '%s'", query, frenchQuery)
	}
	
	// Rate limit and fetch from YGG API
	y.rateLimiter.Wait()
	
	encodedQuery := url.QueryEscape(frenchQuery)
	
	// Set category IDs based on content type
	var categoryParams string
	if category == "movie" {
		categoryParams = "&category_id=2178&category_id=2181&category_id=2183"
	} else if category == "series" {
		categoryParams = "&category_id=2179&category_id=2181&category_id=2182&category_id=2184"
	}
	
	apiURL := fmt.Sprintf("https://yggapi.eu/torrents?q=%s&page=1&per_page=100%s", encodedQuery, categoryParams)
	y.logger.Debugf("[YGG] searching torrents: %s", apiURL)
	
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search YGG: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YGG API returned status %d", resp.StatusCode)
	}
	
	var torrents []models.YggTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("failed to decode YGG response: %w", err)
	}
	
	y.logger.Infof("[YGG] found %d torrents for query: %s", len(torrents), query)
	
	// For series episodes, fetch hash only for torrents matching the specific episode
	if category == "series" && season > 0 && episode > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		matchCount := 0
		
		y.logger.Infof("[YGG] searching for S%dE%d in %d torrents", season, episode, len(torrents))
		
		for i := range torrents {
			torrents[i].Source = "YGG"
			
			// Check if this torrent matches the specific episode
			if y.BaseTorrentService.MatchesEpisode(torrents[i].Title, season, episode) {
				matchCount++
				wg.Add(1)
				go func(index int, torrent models.YggTorrent) {
					defer wg.Done()
					
					y.logger.Debugf("[YGG] fetching hash for episode match - title: %s", torrent.Title)
					startTime := time.Now()
					hash, err := y.GetTorrentHash(fmt.Sprintf("%d", torrent.ID))
					duration := time.Since(startTime)
					
					mu.Lock()
					if err != nil {
						y.logger.Errorf("[YGG] failed to fetch hash for torrent %d after %v: %v", torrent.ID, duration, err)
					} else {
						torrents[index].Hash = hash
						y.logger.Infof("[YGG] hash fetched successfully in %v - title: %s, hash: %s", duration, torrent.Title, hash)
					}
					mu.Unlock()
				}(i, torrents[i])
			} else {
				y.logger.Debugf("[YGG] torrent does not match S%dE%d - title: %s", season, episode, torrents[i].Title)
			}
		}
		
		if matchCount == 0 {
			y.logger.Warnf("[YGG] NO torrents found matching S%dE%d out of %d torrents", season, episode, len(torrents))
		} else {
			y.logger.Infof("[YGG] found %d torrents matching S%dE%d, fetching hashes...", matchCount, season, episode)
		}
		
		wg.Wait()
		y.logger.Infof("[YGG] completed hash fetching for S%dE%d", season, episode)
	} else {
		// For movies or complete series, set source but don't fetch hashes
		for i := range torrents {
			torrents[i].Source = "YGG"
		}
	}
	
	// Process torrents and cache the result using generic method
	result := y.processTorrents(torrents, category, season, episode)
	y.CacheSearch("YGG", query, category, season, episode, result)
	
	return result, nil
}

func (y *YGG) GetTorrentHash(torrentID string) (string, error) {
	// Check cache using generic method
	if hash, found := y.GetCachedHash("YGG", torrentID); found {
		return hash, nil
	}
	
	y.rateLimiter.Wait()
	
	apiURL := fmt.Sprintf("https://yggapi.eu/torrent/%s", torrentID)
	y.logger.Infof("[YGG] starting hash request for torrent ID %s", torrentID)
	y.logger.Debugf("[YGG] API call to get torrent hash - URL: %s", apiURL)
	
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		y.logger.Errorf("[YGG] HTTP request failed for torrent %s: %v", torrentID, err)
		return "", fmt.Errorf("failed to get torrent hash: %w", err)
	}
	defer resp.Body.Close()
	
	y.logger.Infof("[YGG] received response with status %s for torrent %s", resp.Status, torrentID)
	
	var result struct {
		Hash string `json:"hash"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		y.logger.Errorf("[YGG] failed to decode JSON response for torrent %s: %v", torrentID, err)
		return "", fmt.Errorf("failed to decode hash response: %w", err)
	}
	
	y.logger.Infof("[YGG] successfully retrieved hash %s for torrent %s", result.Hash, torrentID)
	
	// Cache the hash result using generic method
	y.CacheHash("YGG", torrentID, result.Hash)
	
	return result.Hash, nil
}

func (y *YGG) processTorrents(torrents []models.YggTorrent, category string, season, episode int) *models.TorrentResults {
	genericTorrents := WrapYggTorrents(torrents)
	return y.BaseTorrentService.ProcessTorrents(genericTorrents, category, season, episode, "YGG", 0)
}

