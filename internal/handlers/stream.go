// Package handlers implements HTTP request handlers for the Stremio addon.
// The stream handler is the core component that processes stream requests,
// searches torrents from multiple providers, and generates playable streams through AllDebrid.
package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/errors"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/internal/services"
)

var (
	// imdbIDRegex matches IMDB ID format (e.g., "tt1234567")
	imdbIDRegex = regexp.MustCompile(`^tt\d+$`)
	// episodeRegex matches episode ID format (e.g., "tt1234567:1:5" for season 1, episode 5)
	episodeRegex = regexp.MustCompile(`^tt\d+:(\d+):(\d+)$`)
)

// handleStream processes stream requests for movies and TV series.
// It handles configuration parsing, API key extraction, TMDB metadata fetching,
// torrent searching, and stream generation through AllDebrid.
//
// URL format: /stream/{configuration}/{id}
// - configuration: base64-encoded JSON containing user preferences and API keys
// - id: IMDB ID (movies) or IMDB:season:episode (TV series)
func (h *Handler) handleStream(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), constants.RequestTimeout)
	defer cancel()
	
	h.monitorTimeout(ctx, c.Param("id"))
	
	userConfig := h.parseUserConfiguration(c.Param("configuration"))
	apiKey := h.extractAPIKey(userConfig, "API_KEY_ALLDEBRID")
	
	if apiKey == "" {
		err := errors.NewAPIKeyMissingError("AllDebrid")
		h.services.Logger.Warnf("[StreamHandler] %v", err)
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	h.configureTMDBService(userConfig)
	
	imdbID, season, episode := parseStreamID(c.Param("id"))
	if imdbID == "" {
		err := errors.NewInvalidIDError(c.Param("id"))
		h.services.Logger.Errorf("[StreamHandler] %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	userConfigStruct := config.CreateFromUserData(userConfig, h.config)
	h.configureTorrentServices(userConfigStruct)
	
	mediaType, title, year, err := h.getMediaInfo(imdbID)
	if err != nil {
		tmdbErr := errors.NewTMDBError(fmt.Sprintf("failed to get info for %s", imdbID), err)
		h.services.Logger.Errorf("[StreamHandler] %v", tmdbErr)
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	h.services.Logger.Infof("[StreamHandler] processing %s request - %s (%s)", mediaType, title, imdbID)
	
	streams := h.searchStreams(mediaType, title, year, season, episode, apiKey, imdbID, userConfigStruct)
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

// monitorTimeout monitors request timeout and logs timeout events.
// This helps with debugging timeout issues in production.
func (h *Handler) monitorTimeout(ctx context.Context, id string) {
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			timeoutErr := errors.NewTimeoutError(fmt.Sprintf("request processing for ID: %s", id))
			h.services.Logger.Errorf("[StreamHandler] %v", timeoutErr)
		}
	}()
}

// parseUserConfiguration decodes base64-encoded user configuration.
// The configuration contains user preferences, API keys, and other settings.
func (h *Handler) parseUserConfiguration(configuration string) map[string]interface{} {
	var userConfig map[string]interface{}
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	return userConfig
}

// extractAPIKey extracts API keys from user configuration with fallback to server config.
// Supports both user-provided keys and server-wide defaults.
func (h *Handler) extractAPIKey(userConfig map[string]interface{}, keyName string) string {
	if val, ok := userConfig[keyName]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	
	switch keyName {
	case "API_KEY_ALLDEBRID":
		if h.config != nil {
			return h.config.APIKeyAllDebrid
		}
	case "TMDB_API_KEY":
		if h.config != nil {
			return h.config.TMDBAPIKey
		}
	}
	
	return ""
}

// configureTMDBService updates the TMDB service with user-provided API key.
// This allows users to use their own TMDB API keys for higher rate limits.
func (h *Handler) configureTMDBService(userConfig map[string]interface{}) {
	tmdbAPIKey := h.extractAPIKey(userConfig, "TMDB_API_KEY")
	if tmdbAPIKey != "" && h.services.TMDB != nil {
		if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
			tmdb.SetAPIKey(tmdbAPIKey)
		}
	}
}

// configureTorrentServices applies user configuration to torrent search services.
// This includes language preferences, quality settings, and provider preferences.
func (h *Handler) configureTorrentServices(userConfig *config.Config) {
	if h.services.YGG != nil {
		h.services.YGG.SetConfig(userConfig)
	}
	if h.services.Apibay != nil {
		h.services.Apibay.SetConfig(userConfig)
	}
}

// getMediaInfo fetches media metadata from TMDB using IMDB ID.
// Returns media type (movie/series), title, year, and any errors.
func (h *Handler) getMediaInfo(imdbID string) (string, string, int, error) {
	mediaType, title, _, year, err := h.services.TMDB.GetIMDBInfo(imdbID)
	return mediaType, title, year, err
}

// searchStreams orchestrates the search process based on media type.
// Routes to appropriate search methods for movies vs TV series.
func (h *Handler) searchStreams(mediaType, title string, year, season, episode int, apiKey, imdbID string, userConfig *config.Config) []models.Stream {
	if mediaType == "movie" {
		return h.searchMovieStreams(title, year, apiKey, userConfig)
	} else if mediaType == "series" {
		return h.searchSeriesStreams(title, season, episode, apiKey, imdbID, userConfig)
	}
	return []models.Stream{}
}

// searchMovieStreams searches for movie torrents and processes them through AllDebrid.
// Uses the title and year to find appropriate torrents from configured providers.
func (h *Handler) searchMovieStreams(title string, year int, apiKey string, userConfig *config.Config) []models.Stream {
	// Search with original title only - YGG service will handle French title conversion internally
	results := h.searchTorrentsOnly(title, "movie", 0, 0, "", year)
	
	h.services.Logger.Infof("[StreamHandler] search results - movies: %d, starting AllDebrid processing", len(results.MovieTorrents))
	
	// Process results with AllDebrid
	return h.processResults(results, apiKey, userConfig, year, 0, 0)
}

// searchSeriesStreams implements a two-phase search strategy for TV series:
// 1. Prioritize complete season packs (better quality, contains target episode)
// 2. Fallback to specific episode searches if season packs don't work
// This approach maximizes success rate while preferring higher quality sources.
func (h *Handler) searchSeriesStreams(title string, season, episode int, apiKey, imdbID string, userConfig *config.Config) []models.Stream {
	// First search: Prioritize complete seasons (season packs)
	h.services.Logger.Infof("[StreamHandler] starting first search prioritizing complete seasons for s%02d", season)
	streams := h.searchTorrentsWithIMDB(title, "series", season, episode, apiKey, imdbID, userConfig)
	
	// If we found working streams, return them
	if len(streams) > 0 {
		return streams
	}
	
	// Second search: If no working streams found and we're looking for a specific episode,
	// fallback to searching for specific episodes
	if season > 0 && episode > 0 {
		h.services.Logger.Infof("[StreamHandler] no working streams from season search, trying episode-specific search for s%02de%02d", season, episode)
		streams = h.searchTorrentsWithIMDBSpecificEpisode(title, "series", season, episode, apiKey, imdbID, userConfig)
		
		// If specific episode search also fails, try a broader search with just the title
		if len(streams) == 0 {
			h.services.Logger.Infof("[StreamHandler] episode-specific search also failed, trying broader title search")
			broadResults := h.searchTorrentsOnly(title, "series", 0, 0, "", 0)
			if broadResults != nil && len(broadResults.EpisodeTorrents) > 0 {
				// Filter for matching episodes during processing
				streams = h.processResults(broadResults, apiKey, userConfig, 0, season, episode)
			}
		}
	}
	
	return streams
}

func (h *Handler) searchTorrents(query string, mediaType string, season, episode int, apiKey string) []models.Stream {
	return h.searchTorrentsWithIMDB(query, mediaType, season, episode, apiKey, "", nil)
}

// searchTorrentsWithIMDBSpecificEpisode searches for a specific episode only (fallback when season search fails)
func (h *Handler) searchTorrentsWithIMDBSpecificEpisode(query string, mediaType string, season, episode int, apiKey string, imdbID string, userConfig *config.Config) []models.Stream {
	h.services.Logger.Debugf("[StreamHandler] specific episode search initiated - query: %s, s%02de%02d", query, season, episode)
	
	// Search specifically for the episode
	results := h.searchTorrentsOnlySpecificEpisode(query, mediaType, season, episode, imdbID, 0)
	
	h.services.Logger.Infof("[StreamHandler] specific episode search results - episodes: %d", len(results.EpisodeTorrents))
	
	// Process only episode torrents (no season packs in this search)
	return h.processResults(results, apiKey, userConfig, 0, season, episode)
}

func (h *Handler) searchTorrentsWithIMDB(query string, mediaType string, season, episode int, apiKey string, imdbID string, userConfig *config.Config) []models.Stream {
	h.services.Logger.Debugf("[StreamHandler] torrent search initiated - query: %s, mediaType: %s, imdbID: %s", query, mediaType, imdbID)
	
	var combinedResults models.CombinedTorrentResults
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Create timeout channel for search operations
	searchTimeout := constants.SearchTimeout
	done := make(chan bool)
	
	goroutineCount := constants.TorrentSearchGoroutines
	
	wg.Add(goroutineCount)
	
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] YGG search panic recovered: %v", r)
			}
		}()
		
		category := "movie"
		if mediaType == "series" {
			category = "series"
		}
		
		h.services.Logger.Infof("[StreamHandler] YGG search started - query: %s, category: %s, season: %d, episode: %d", query, category, season, episode)
		results, err := h.services.YGG.SearchTorrents(query, category, season, episode)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to search YGG: %v", err)
			return
		}
		
		if results != nil {
			mu.Lock()
			for _, t := range results.MovieTorrents {
				combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, t)
			}
			for _, t := range results.CompleteSeriesTorrents {
				combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, t)
			}
			for _, t := range results.CompleteSeasonTorrents {
				combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, t)
			}
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
			}
			mu.Unlock()
		}
	}()
	
	// Add Apibay search
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] Apibay search panic recovered: %v", r)
			}
		}()
		
		// Use query as-is for Apibay (no year info available in this function)
		apibayQuery := query
		
		h.services.Logger.Infof("[StreamHandler] Apibay search started - query: %s, mediaType: %s, season: %d, episode: %d", apibayQuery, mediaType, season, episode)
		results, err := h.services.Apibay.SearchTorrents(apibayQuery, mediaType, season, episode)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to search Apibay: %v", err)
			return
		}
		
		if results != nil {
			mu.Lock()
			for _, t := range results.MovieTorrents {
				combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, t)
			}
			for _, t := range results.CompleteSeriesTorrents {
				combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, t)
			}
			for _, t := range results.CompleteSeasonTorrents {
				combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, t)
			}
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
			}
			mu.Unlock()
		} else {
			h.services.Logger.Debugf("[StreamHandler] Apibay returned no results")
		}
	}()
	
	// Wait for searches with timeout
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		h.services.Logger.Debugf("[StreamHandler] all searches completed successfully")
	case <-time.After(searchTimeout):
		h.services.Logger.Errorf("[StreamHandler] search timeout after %v for query: %s", searchTimeout, query)
	}
	
	h.services.Logger.Infof("[StreamHandler] search completed - movies: %d, complete series: %d, seasons: %d, episodes: %d", 
		len(combinedResults.MovieTorrents), 
		len(combinedResults.CompleteSeriesTorrents), 
		len(combinedResults.CompleteSeasonTorrents), 
		len(combinedResults.EpisodeTorrents))
	
	// Log episode torrents for debugging
	for i, torrent := range combinedResults.EpisodeTorrents {
		if i < constants.MaxEpisodeTorrentsToLog {
			h.services.Logger.Infof("[StreamHandler] episode torrent %d: %s (source: %s, hash: %s)", i+1, torrent.Title, torrent.Source, torrent.Hash)
		}
	}
	
	return h.processResults(&combinedResults, apiKey, userConfig, 0, season, episode)
}

// searchTorrentsOnly searches for torrents without processing through AllDebrid
func (h *Handler) searchTorrentsOnly(query, mediaType string, season, episode int, imdbID string, year int) *models.CombinedTorrentResults {
	var wg sync.WaitGroup
	var mu sync.Mutex
	combinedResults := models.CombinedTorrentResults{}
	
	// Create timeout channel for search operations
	searchTimeout := constants.SearchTimeout
	done := make(chan bool)
	
	// Add YGG search
	wg.Add(constants.TorrentSearchGoroutines)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] YGG search panic recovered: %v", r)
			}
		}()
		
		category := "movie"
		if mediaType == "series" {
			category = "series"
		}
		
		h.services.Logger.Infof("[StreamHandler] YGG search started - query: %s, category: %s, season: %d, episode: %d", query, category, season, episode)
		results, err := h.services.YGG.SearchTorrents(query, category, season, episode)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to search YGG: %v", err)
			return
		}
		
		if results != nil {
			mu.Lock()
			for _, t := range results.MovieTorrents {
				combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, t)
			}
			for _, t := range results.CompleteSeriesTorrents {
				combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, t)
			}
			for _, t := range results.CompleteSeasonTorrents {
				combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, t)
			}
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
			}
			mu.Unlock()
		}
	}()
	
	// Add Apibay search
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] Apibay search panic recovered: %v", r)
			}
		}()
		
		// Build query with year for movies
		apibayQuery := query
		if mediaType == "movie" && year > 0 {
			apibayQuery = fmt.Sprintf("%s %d", query, year)
		}
		
		h.services.Logger.Infof("[StreamHandler] Apibay search started - query: %s, mediaType: %s, season: %d, episode: %d", apibayQuery, mediaType, season, episode)
		results, err := h.services.Apibay.SearchTorrents(apibayQuery, mediaType, season, episode)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to search Apibay: %v", err)
			return
		}
		
		if results != nil {
			h.services.Logger.Infof("[StreamHandler] Apibay results - movies: %d, complete series: %d, seasons: %d, episodes: %d", 
				len(results.MovieTorrents), 
				len(results.CompleteSeriesTorrents), 
				len(results.CompleteSeasonTorrents), 
				len(results.EpisodeTorrents))
			
			mu.Lock()
			for _, t := range results.MovieTorrents {
				combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, t)
			}
			for _, t := range results.CompleteSeriesTorrents {
				combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, t)
			}
			for _, t := range results.CompleteSeasonTorrents {
				combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, t)
			}
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
			}
			mu.Unlock()
		} else {
			h.services.Logger.Debugf("[StreamHandler] Apibay returned no results")
		}
	}()
	
	
	// Wait for searches with timeout
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		h.services.Logger.Debugf("[StreamHandler] all searches completed successfully")
	case <-time.After(searchTimeout):
		h.services.Logger.Errorf("[StreamHandler] search timeout after %v for query: %s", searchTimeout, query)
	}
	
	h.services.Logger.Infof("[StreamHandler] search completed - movies: %d, complete series: %d, seasons: %d, episodes: %d", 
		len(combinedResults.MovieTorrents), 
		len(combinedResults.CompleteSeriesTorrents), 
		len(combinedResults.CompleteSeasonTorrents), 
		len(combinedResults.EpisodeTorrents))
	
	return &combinedResults
}

// searchTorrentsOnlySpecificEpisode searches for a specific episode using episode-specific queries
func (h *Handler) searchTorrentsOnlySpecificEpisode(query, mediaType string, season, episode int, imdbID string, year int) *models.CombinedTorrentResults {
	var wg sync.WaitGroup
	var mu sync.Mutex
	combinedResults := models.CombinedTorrentResults{}
	
	// Create timeout channel for search operations
	searchTimeout := constants.SearchTimeout
	done := make(chan bool)
	
	// Add YGG search with specific episode query
	wg.Add(constants.TorrentSearchGoroutines)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] YGG specific episode search panic recovered: %v", r)
			}
		}()
		
		h.services.Logger.Infof("[StreamHandler] YGG specific episode search started - query: %s, s%02de%02d", query, season, episode)
		
		// Use YGG's SearchTorrentsSpecificEpisode if available, otherwise use regular search
		results, err := h.services.YGG.SearchTorrentsSpecificEpisode(query, mediaType, season, episode)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to search YGG for specific episode: %v", err)
			return
		}
		
		if results != nil {
			mu.Lock()
			// Only add episode torrents from this search
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
			}
			mu.Unlock()
		}
	}()
	
	// Add Apibay search with specific episode query
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] Apibay specific episode search panic recovered: %v", r)
			}
		}()
		
		// Use Apibay's specific episode search method
		h.services.Logger.Infof("[StreamHandler] Apibay specific episode search started - query: %s, s%02de%02d", query, season, episode)
		results, err := h.services.Apibay.SearchTorrentsSpecificEpisode(query, mediaType, season, episode)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to search Apibay for specific episode: %v", err)
			return
		}
		
		if results != nil {
			mu.Lock()
			// Only add episode torrents from this search
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
			}
			mu.Unlock()
		}
	}()
	
	
	// Wait for searches with timeout
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		h.services.Logger.Debugf("[StreamHandler] all specific episode searches completed successfully")
	case <-time.After(searchTimeout):
		h.services.Logger.Errorf("[StreamHandler] specific episode search timeout after %v", searchTimeout)
	}
	
	h.services.Logger.Infof("[StreamHandler] specific episode search completed - episodes found: %d", len(combinedResults.EpisodeTorrents))
	
	return &combinedResults
}

// processResults applies filtering and prioritization to torrent search results.
// Implements intelligent prioritization based on request type:
// - For specific episodes: season packs > episodes > complete series
// - For season requests: season packs > complete series > episodes
// - For movies: applies year filtering when specified
// Returns the first working stream found (sequential processing).
func (h *Handler) processResults(results *models.CombinedTorrentResults, apiKey string, userConfig *config.Config, year int, targetSeason, targetEpisode int) []models.Stream {
	h.services.Logger.Infof("[StreamHandler] processResults started - movies: %d, episodes: %d, seasons: %d, complete series: %d", 
		len(results.MovieTorrents), len(results.EpisodeTorrents), len(results.CompleteSeasonTorrents), len(results.CompleteSeriesTorrents))
	
	// Apply year filtering to movie torrents if year is provided
	if year > 0 && len(results.MovieTorrents) > 0 {
		var filteredMovies []models.TorrentInfo
		for _, torrent := range results.MovieTorrents {
			if h.matchesYear(torrent.Title, year) {
				filteredMovies = append(filteredMovies, torrent)
			} else {
				h.services.Logger.Debugf("[StreamHandler] torrent filtered by year - title: %s (expected: %d)", torrent.Title, year)
			}
		}
		h.services.Logger.Infof("[StreamHandler] year filtering: %d -> %d movie torrents", len(results.MovieTorrents), len(filteredMovies))
		results.MovieTorrents = filteredMovies
	}
	
	// Collect all torrents in priority order
	var allTorrents []models.TorrentInfo
	
	// Add torrents in priority order based on request type
	if targetSeason > 0 && targetEpisode > 0 {
		// For specific episodes, prioritize: complete seasons -> episodes -> complete series
		// Complete seasons are better quality and contain the episode we want
		allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
		allTorrents = append(allTorrents, results.EpisodeTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
	} else if targetSeason > 0 && targetEpisode == 0 {
		// For complete season requests, prioritize: season packs -> complete series -> episodes
		allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
		allTorrents = append(allTorrents, results.EpisodeTorrents...)
	} else {
		// For movies or general searches
		allTorrents = append(allTorrents, results.MovieTorrents...)
		allTorrents = append(allTorrents, results.EpisodeTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
	}
	
	h.services.Logger.Infof("[StreamHandler] processing %d torrents in priority order", len(allTorrents))
	
	// Process torrents one by one until we find a working stream
	return h.processSequentialTorrents(allTorrents, apiKey, userConfig, targetSeason, targetEpisode)
}

// processSequentialTorrents processes torrents one by one until a working stream is found
// processSequentialTorrents processes torrents one by one until a working stream is found.
// This approach stops at the first success to minimize processing time and API calls.
// For each torrent: fetches hash (if needed) -> uploads to AllDebrid -> checks readiness -> creates stream.
func (h *Handler) processSequentialTorrents(torrents []models.TorrentInfo, apiKey string, userConfig *config.Config, targetSeason, targetEpisode int) []models.Stream {
	if len(torrents) == 0 {
		h.services.Logger.Infof("[StreamHandler] no torrents to process")
		return []models.Stream{}
	}
	
	h.services.Logger.Infof("[StreamHandler] processing %d torrents sequentially", len(torrents))
	
	for i, torrent := range torrents {
		h.services.Logger.Infof("[StreamHandler] trying torrent %d/%d: %s (source: %s)", i+1, len(torrents), torrent.Title, torrent.Source)
		
		// Get hash if needed
		hash := torrent.Hash
		if hash == "" && torrent.Source == "YGG" {
			h.services.Logger.Infof("[StreamHandler] fetching hash for YGG torrent: %s", torrent.Title)
			fetchedHash, err := h.services.YGG.GetTorrentHash(torrent.ID)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] failed to fetch hash for torrent %s: %v", torrent.Title, err)
				continue
			}
			if fetchedHash == "" {
				h.services.Logger.Warnf("[StreamHandler] torrent %s returned empty hash", torrent.Title)
				continue
			}
			hash = fetchedHash
		}
		
		if hash == "" {
			h.services.Logger.Warnf("[StreamHandler] skipping torrent without hash: %s", torrent.Title)
			continue
		}
		
		// Upload the magnet
		h.services.Logger.Infof("[StreamHandler] uploading magnet: %s", torrent.Title)
		err := h.services.AllDebrid.UploadMagnet(hash, torrent.Title, apiKey)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to upload magnet %s: %v", torrent.Title, err)
			continue
		}
		
		// Check if magnet is ready (try twice)
		var isReady bool
		var readyMagnet *models.ProcessedMagnet
		
		for attempt := 1; attempt <= constants.MaxMagnetCheckAttempts; attempt++ {
			h.services.Logger.Infof("[StreamHandler] checking magnet status - attempt %d/2", attempt)
			
			// Check just this one magnet
			magnetInfo := models.MagnetInfo{Hash: hash, Title: torrent.Title, Source: torrent.Source}
			processedMagnets, err := h.services.AllDebrid.CheckMagnets([]models.MagnetInfo{magnetInfo}, apiKey)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] CheckMagnets failed: %v", err)
				if attempt < constants.MaxMagnetCheckAttempts {
					time.Sleep(constants.MagnetCheckRetryDelay)
					continue
				}
				break
			}
			
			if len(processedMagnets) > 0 && processedMagnets[0].Ready && len(processedMagnets[0].Links) > 0 {
				h.services.Logger.Infof("[StreamHandler] magnet is ready with %d links!", len(processedMagnets[0].Links))
				isReady = true
				readyMagnet = &processedMagnets[0]
				break
			}
			
			if attempt < constants.MaxMagnetCheckAttempts {
				h.services.Logger.Infof("[StreamHandler] magnet not ready yet, waiting before retry")
				time.Sleep(constants.MagnetReadyRetryDelay)
			}
		}
		
		if !isReady || readyMagnet == nil {
			h.services.Logger.Infof("[StreamHandler] magnet not ready after %d attempts, trying next", constants.MaxMagnetCheckAttempts)
			continue
		}
		
		// Process the ready magnet
		stream := h.processSingleReadyMagnet(readyMagnet, torrent, targetSeason, targetEpisode, apiKey)
		if stream != nil {
			h.services.Logger.Infof("[StreamHandler] successfully created stream from torrent: %s", torrent.Title)
			return []models.Stream{*stream}
		}
		
		h.services.Logger.Warnf("[StreamHandler] failed to create stream from ready magnet: %s", torrent.Title)
	}
	
	h.services.Logger.Infof("[StreamHandler] no working torrents found")
	return []models.Stream{}
}

// processSingleReadyMagnet processes a single ready magnet and returns a stream if successful
// processSingleReadyMagnet creates a stream from a ready AllDebrid magnet.
// Handles different scenarios:
// - Season pack + specific episode: finds target episode file within the pack
// - Season pack + full season: returns largest file (complete season archive)
// - Regular torrent: returns largest file (typically the main video)
func (h *Handler) processSingleReadyMagnet(magnet *models.ProcessedMagnet, torrent models.TorrentInfo, targetSeason, targetEpisode int, apiKey string) *models.Stream {
	// Check if this is a season pack
	isSeasonPack := h.isSeasonPack(torrent.Title)
	
	// Case 1: Season pack and we need a specific episode
	if targetSeason > 0 && targetEpisode > 0 && isSeasonPack {
		h.services.Logger.Infof("[StreamHandler] processing season pack for specific episode s%02de%02d", targetSeason, targetEpisode)
		
		// Find the specific episode file
		for _, link := range magnet.Links {
			if linkObj, ok := link.(map[string]interface{}); ok {
				if filename, ok := linkObj["filename"].(string); ok {
					season, episode := h.parseEpisodeFromFilename(filename)
					if season == targetSeason && episode == targetEpisode {
						h.services.Logger.Infof("[StreamHandler] found target episode: %s", filename)
						
						if linkStr, ok := linkObj["link"].(string); ok {
							directURL, err := h.services.AllDebrid.UnlockLink(linkStr, apiKey)
							if err != nil {
								h.services.Logger.Errorf("[StreamHandler] failed to unlock episode link: %v", err)
								continue
							}
							
							streamTitle := fmt.Sprintf("%s\n%s", torrent.Title, parseFileInfo(linkObj))
							return &models.Stream{
								Name:  torrent.Source,
								Title: streamTitle,
								URL:   directURL,
							}
						}
					}
				}
			}
		}
		
		h.services.Logger.Warnf("[StreamHandler] target episode s%02de%02d not found in season pack", targetSeason, targetEpisode)
		return nil
	}
	
	// Case 2: Complete season requested (targetEpisode == 0) and this is a season pack
	if targetSeason > 0 && targetEpisode == 0 && isSeasonPack {
		h.services.Logger.Infof("[StreamHandler] processing complete season pack for season %d", targetSeason)
		
		// For complete season, return the largest file (usually the entire season archive)
		var largestFile map[string]interface{}
		var largestSize float64
		
		for _, link := range magnet.Links {
			if linkObj, ok := link.(map[string]interface{}); ok {
				if size, ok := linkObj["size"].(float64); ok {
					if size > largestSize {
						largestSize = size
						largestFile = linkObj
					}
				}
			}
		}
		
		if largestFile != nil {
			if linkStr, ok := largestFile["link"].(string); ok {
				directURL, err := h.services.AllDebrid.UnlockLink(linkStr, apiKey)
				if err != nil {
					h.services.Logger.Errorf("[StreamHandler] failed to unlock season pack link: %v", err)
					return nil
				}
				
				streamTitle := fmt.Sprintf("%s\n%s", torrent.Title, parseFileInfo(largestFile))
				return &models.Stream{
					Name:  torrent.Source,
					Title: streamTitle,
					URL:   directURL,
				}
			}
		}
		
		h.services.Logger.Warnf("[StreamHandler] no valid files found in season pack")
		return nil
	}
	
	// Regular torrent - find the largest file
	h.services.Logger.Infof("[StreamHandler] processing regular torrent, finding largest file")
	
	var largestFile map[string]interface{}
	var largestSize float64
	
	for _, link := range magnet.Links {
		if linkObj, ok := link.(map[string]interface{}); ok {
			if size, ok := linkObj["size"].(float64); ok {
				if size > largestSize {
					largestSize = size
					largestFile = linkObj
				}
			}
		}
	}
	
	if largestFile == nil {
		h.services.Logger.Warnf("[StreamHandler] no files found in magnet")
		return nil
	}
	
	if linkStr, ok := largestFile["link"].(string); ok {
		directURL, err := h.services.AllDebrid.UnlockLink(linkStr, apiKey)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to unlock link: %v", err)
			return nil
		}
		
		streamTitle := fmt.Sprintf("%s\n%s", torrent.Title, parseFileInfo(largestFile))
		return &models.Stream{
			Name:  torrent.Source,
			Title: streamTitle,
			URL:   directURL,
		}
	}
	
	h.services.Logger.Warnf("[StreamHandler] no valid link found in largest file")
	return nil
}

// isSeasonPack determines if a torrent title indicates a complete season pack
// isSeasonPack determines if a torrent title indicates a complete season pack.
// Season packs contain multiple episodes and typically offer better quality.
func (h *Handler) isSeasonPack(title string) bool {
	titleLower := strings.ToLower(title)
	return strings.Contains(titleLower, "complete") || 
		   strings.Contains(titleLower, "season") ||
		   strings.Contains(titleLower, "saison")
}

// parseFileInfo extracts and formats file information for display in stream titles.
// Includes file size in GB and filename for user information.
func parseFileInfo(linkObj map[string]interface{}) string {
	info := ""
	
	// Add size if available
	if size, ok := linkObj["size"].(float64); ok {
		sizeGB := size / constants.BytesToGB
		info += fmt.Sprintf("💾 %.2f GB", sizeGB)
	}
	
	// Add filename if available
	if filename, ok := linkObj["filename"].(string); ok {
		if info != "" {
			info += " • "
		}
		info += fmt.Sprintf("📄 %s", filename)
	}
	
	return info
}

// parseStreamID extracts IMDB ID and episode information from stream request ID.
// Supports both movie format ("tt1234567") and episode format ("tt1234567:1:5").
// Returns imdbID, season, episode (season and episode are 0 for movies).
func parseStreamID(id string) (string, int, int) {
	id = strings.TrimSuffix(id, ".json")
	
	if episodeRegex.MatchString(id) {
		matches := episodeRegex.FindStringSubmatch(id)
		if len(matches) == 3 {
			imdbID := strings.Split(id, ":")[0]
			season, _ := strconv.Atoi(matches[1])
			episode, _ := strconv.Atoi(matches[2])
			return imdbID, season, episode
		}
	}
	
	if imdbIDRegex.MatchString(id) {
		return id, 0, 0
	}
	
	return "", 0, 0
}

// matchesYear checks if a torrent title contains the expected year.
// Used for filtering movie torrents to ensure correct year matches.
func (h *Handler) matchesYear(title string, year int) bool {
	if year == 0 {
		return true
	}
	
	yearStr := fmt.Sprintf("%d", year)
	return strings.Contains(title, yearStr)
}

// parseEpisodeFromFilename extracts season and episode numbers from filenames.
// Supports multiple common naming patterns (S01E05, 1x05, Season 1 Episode 5, etc.).
// Used when processing season packs to find specific episodes.
func (h *Handler) parseEpisodeFromFilename(filename string) (int, int) {
	patterns := []string{
		`[sS](\d{1,2})[eE](\d{1,2})`,
		`[sS](\d{1,2})\.?[eE](\d{1,2})`,
		`(\d{1,2})x(\d{1,2})`,
		`[sS]eason\s*(\d{1,2})\s*[eE]pisode\s*(\d{1,2})`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(filename); len(matches) >= 3 {
			season, err1 := strconv.Atoi(matches[1])
			episode, err2 := strconv.Atoi(matches[2])
			if err1 == nil && err2 == nil {
				return season, episode
			}
		}
	}
	
	return 0, 0
}