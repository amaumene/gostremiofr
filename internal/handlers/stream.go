package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/internal/services"
)

var (
	imdbIDRegex = regexp.MustCompile(`^tt\d+$`)
	episodeRegex = regexp.MustCompile(`^tt\d+:(\d+):(\d+)$`)
)

func (h *Handler) handleStream(c *gin.Context) {
	// Add 30-second timeout for the entire request
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	
	// Monitor timeout
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			h.services.Logger.Errorf("[StreamHandler] request timeout for ID: %s", c.Param("id"))
		}
	}()
	
	configuration := c.Param("configuration")
	id := c.Param("id")
	
	var userConfig map[string]interface{}
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	
	// Extract AllDebrid API key
	apiKey := ""
	if val, ok := userConfig["API_KEY_ALLDEBRID"]; ok {
		if str, ok := val.(string); ok {
			apiKey = str
		}
	}
	if apiKey == "" && h.config != nil {
		apiKey = h.config.APIKeyAllDebrid
	}
	
	if apiKey == "" {
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	// Extract TMDB API key and update service if available
	tmdbAPIKey := ""
	if val, ok := userConfig["TMDB_API_KEY"]; ok {
		if str, ok := val.(string); ok {
			tmdbAPIKey = str
		}
	}
	if tmdbAPIKey == "" && h.config != nil {
		tmdbAPIKey = h.config.TMDBAPIKey
	}
	
	// Update TMDB service with the API key if available
	if tmdbAPIKey != "" && h.services.TMDB != nil {
		if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
			tmdb.SetAPIKey(tmdbAPIKey)
		}
	}
	
	imdbID, season, episode := parseStreamID(id)
	if imdbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}
	
	// Create config from user configuration and defaults
	userConfigStruct := config.CreateFromUserData(userConfig, h.config)
	
	// Set config for services that need it
	if h.services.EZTV != nil {
		h.services.EZTV.SetConfig(userConfigStruct)
	}
	if h.services.YGG != nil {
		h.services.YGG.SetConfig(userConfigStruct)
	}
	
	mediaType, title, _, year, err := h.services.TMDB.GetIMDBInfo(imdbID)
	if err != nil {
		h.services.Logger.Errorf("[StreamHandler] failed to get TMDB info for %s: %v", imdbID, err)
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	h.services.Logger.Infof("[StreamHandler] processing %s request - %s (%s)", mediaType, title, imdbID)
	
	var streams []models.Stream
	
	if mediaType == "movie" {
		streams = h.searchMovieStreams(title, year, apiKey, userConfigStruct)
	} else if mediaType == "series" {
		streams = h.searchSeriesStreams(title, season, episode, apiKey, imdbID, userConfigStruct)
	}
	
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

func (h *Handler) searchMovieStreams(title string, year int, apiKey string, userConfig *config.Config) []models.Stream {
	// Search with original title only - YGG service will handle French title conversion internally
	results := h.searchTorrentsOnly(title, "movie", 0, 0, "", year)
	
	h.services.Logger.Infof("[StreamHandler] search results - movies: %d, starting AllDebrid processing", len(results.MovieTorrents))
	
	// Process results with AllDebrid
	return h.processResults(results, apiKey, userConfig, year, 0, 0)
}

func (h *Handler) searchSeriesStreams(title string, season, episode int, apiKey, imdbID string, userConfig *config.Config) []models.Stream {
	// Search with original title only - YGG service will handle French title conversion internally
	return h.searchTorrentsWithIMDB(title, "series", season, episode, apiKey, imdbID, userConfig)
}

func (h *Handler) searchTorrents(query string, mediaType string, season, episode int, apiKey string) []models.Stream {
	return h.searchTorrentsWithIMDB(query, mediaType, season, episode, apiKey, "", nil)
}

func (h *Handler) searchTorrentsWithIMDB(query string, mediaType string, season, episode int, apiKey string, imdbID string, userConfig *config.Config) []models.Stream {
	h.services.Logger.Debugf("[StreamHandler] torrent search initiated - query: %s, mediaType: %s, imdbID: %s", query, mediaType, imdbID)
	
	var combinedResults models.CombinedTorrentResults
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Create timeout channel for search operations
	searchTimeout := 15 * time.Second
	done := make(chan bool)
	
	goroutineCount := 1
	if mediaType == "series" && imdbID != "" {
		goroutineCount = 2 // Add EZTV for series
		h.services.Logger.Debugf("[StreamHandler] EZTV search enabled for series - goroutineCount: %d", goroutineCount)
	} else {
		h.services.Logger.Debugf("[StreamHandler] EZTV search disabled - mediaType: %s, imdbID: %s", mediaType, imdbID)
	}
	
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
	
	
	// Add EZTV search for series with IMDB ID
	if mediaType == "series" && imdbID != "" {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					h.services.Logger.Errorf("[StreamHandler] EZTV search panic recovered: %v", r)
				}
			}()
			
			h.services.Logger.Infof("[StreamHandler] EZTV search started for IMDB ID: %s, S%dE%d", imdbID, season, episode)
			results, err := h.services.EZTV.SearchTorrentsByIMDB(imdbID, season, episode)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] failed to search EZTV: %v", err)
				return
			}
			
			if results != nil {
				h.services.Logger.Infof("[StreamHandler] EZTV results - complete series: %d, seasons: %d, episodes: %d", 
					len(results.CompleteSeriesTorrents), 
					len(results.CompleteSeasonTorrents), 
					len(results.EpisodeTorrents))
				mu.Lock()
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
				h.services.Logger.Debugf("[StreamHandler] EZTV returned no results for IMDB ID: %s", imdbID)
			}
		}()
	} else {
		h.services.Logger.Debugf("[StreamHandler] skipping EZTV search - mediaType: %s, imdbID: %s", mediaType, imdbID)
	}
	
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
		if i < 5 { // Log first 5
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
	searchTimeout := 15 * time.Second
	done := make(chan bool)
	
	// Add YGG search
	wg.Add(1)
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
	
	// Add EZTV search for series with IMDB ID
	if mediaType == "series" && imdbID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					h.services.Logger.Errorf("[StreamHandler] EZTV search panic recovered: %v", r)
				}
			}()
			
			h.services.Logger.Infof("[StreamHandler] EZTV search started for IMDB ID: %s, S%dE%d", imdbID, season, episode)
			results, err := h.services.EZTV.SearchTorrentsByIMDB(imdbID, season, episode)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] failed to search EZTV: %v", err)
				return
			}
			
			if results != nil {
				h.services.Logger.Infof("[StreamHandler] EZTV results - complete series: %d, seasons: %d, episodes: %d", 
					len(results.CompleteSeriesTorrents), 
					len(results.CompleteSeasonTorrents), 
					len(results.EpisodeTorrents))
				mu.Lock()
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
				h.services.Logger.Debugf("[StreamHandler] EZTV returned no results for IMDB ID: %s", imdbID)
			}
		}()
	} else {
		h.services.Logger.Debugf("[StreamHandler] skipping EZTV search - mediaType: %s, imdbID: %s", mediaType, imdbID)
	}
	
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

func (h *Handler) processResults(results *models.CombinedTorrentResults, apiKey string, userConfig *config.Config, year int, targetSeason, targetEpisode int) []models.Stream {
	h.services.Logger.Infof("[StreamHandler] processResults started with %d movie torrents", len(results.MovieTorrents))
	
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
	
	var magnets []models.MagnetInfo
	hashToTorrent := make(map[string]models.TorrentInfo)
	
	processTorrents := func(torrents []models.TorrentInfo) {
		h.services.Logger.Infof("[StreamHandler] processing %d torrents for hash extraction", len(torrents))
		
		// Process torrents that already have hashes first
		for _, torrent := range torrents {
			if torrent.Hash != "" {
				h.services.Logger.Infof("[StreamHandler] adding torrent with hash - title: %s, source: %s, hash: %s", torrent.Title, torrent.Source, torrent.Hash)
				magnets = append(magnets, models.MagnetInfo{
					Hash:   torrent.Hash,
					Title:  torrent.Title,
					Source: torrent.Source,
				})
				hashToTorrent[torrent.Hash] = torrent
			} else {
				h.services.Logger.Debugf("[StreamHandler] skipping torrent without hash - title: %s, source: %s", torrent.Title, torrent.Source)
			}
		}
		
		// Collect YGG torrents that need hash retrieval
		var yggTorrents []models.TorrentInfo
		for _, torrent := range torrents {
			if torrent.Hash == "" && torrent.Source == "YGG" {
				yggTorrents = append(yggTorrents, torrent)
			}
		}
		
		if len(yggTorrents) > 0 {
			// Get user's FilesToShow setting to determine how many torrents to process
			maxTorrents := constants.DefaultFilesToShow
			if userConfig != nil {
				maxTorrents = userConfig.FilesToShow
			}
			
			// Limit to available torrents and a reasonable maximum for hash retrieval
			if len(yggTorrents) < maxTorrents {
				maxTorrents = len(yggTorrents)
			}
			if maxTorrents > 10 { // Cap at 10 for performance
				maxTorrents = 10
			}
			
			requestedFiles := "default"
			if userConfig != nil {
				requestedFiles = fmt.Sprintf("%d", userConfig.FilesToShow)
			}
			h.services.Logger.Infof("[StreamHandler] processing %d YGG torrents for hashes (user requested %s files)", maxTorrents, requestedFiles)
			
			// Process torrents sequentially for reliability
			// Note: torrents are already filtered and sorted by the torrent service
			successCount := 0
			for i := 0; i < maxTorrents; i++ {
				torrent := yggTorrents[i]
				h.services.Logger.Debugf("[StreamHandler] getting hash for torrent %d/%d: %s", i+1, maxTorrents, torrent.Title)
				
				hash, err := h.services.YGG.GetTorrentHash(torrent.ID)
				if err != nil {
					h.services.Logger.Warnf("[StreamHandler] failed to get hash for torrent %s: %v", torrent.Title, err)
				} else if hash != "" {
					successCount++
					h.services.Logger.Debugf("[StreamHandler] got hash %s for torrent %s", hash, torrent.Title)
					torrent.Hash = hash
					magnets = append(magnets, models.MagnetInfo{
						Hash:   hash,
						Title:  torrent.Title,
						Source: torrent.Source,
					})
					hashToTorrent[hash] = torrent
				} else {
					h.services.Logger.Warnf("[StreamHandler] torrent %s returned empty hash", torrent.Title)
				}
			}
			
			h.services.Logger.Infof("[StreamHandler] hash extraction completed - %d successful out of %d attempted", successCount, maxTorrents)
		}
		
		h.services.Logger.Infof("[StreamHandler] total magnets collected: %d", len(magnets))
	}
	
	processTorrents(results.MovieTorrents)
	processTorrents(results.CompleteSeriesTorrents)
	processTorrents(results.CompleteSeasonTorrents)
	processTorrents(results.EpisodeTorrents)
	
	// Process complete season torrents to extract specific episodes
	var episodeStreams []models.Stream
	if targetSeason > 0 && targetEpisode > 0 && len(results.CompleteSeasonTorrents) > 0 {
		h.services.Logger.Infof("[StreamHandler] processing %d complete season torrents for S%02dE%02d", 
			len(results.CompleteSeasonTorrents), targetSeason, targetEpisode)
		for _, torrent := range results.CompleteSeasonTorrents {
			h.services.Logger.Debugf("[StreamHandler] season torrent: %s (hash: %s)", torrent.Title, torrent.Hash)
		}
		episodeStreams = h.extractEpisodesFromSeasons(results.CompleteSeasonTorrents, apiKey, userConfig, targetSeason, targetEpisode)
		h.services.Logger.Infof("[StreamHandler] extracted %d episode streams from complete seasons", len(episodeStreams))
	} else {
		h.services.Logger.Infof("[StreamHandler] no season torrents to process - targetSeason: %d, targetEpisode: %d, seasonTorrents: %d", 
			targetSeason, targetEpisode, len(results.CompleteSeasonTorrents))
	}
	
	if len(magnets) == 0 {
		return episodeStreams // Return episode streams if no regular magnets found
	}
	
	// Limit the number of magnets to prevent AllDebrid API timeouts
	maxMagnets := 30
	if len(magnets) > maxMagnets {
		h.services.Logger.Infof("[StreamHandler] limiting magnets from %d to %d to prevent timeouts", len(magnets), maxMagnets)
		magnets = magnets[:maxMagnets]
	}
	
	// Group magnets by their provider
	magnetsByProvider := make(map[string][]models.MagnetInfo)
	for _, magnet := range magnets {
		provider := strings.ToLower(magnet.Source)
		magnetsByProvider[provider] = append(magnetsByProvider[provider], magnet)
	}
	
	h.services.Logger.Infof("[StreamHandler] checking %d magnets with debrid services (grouped by provider)", len(magnets))
	
	// Process magnets grouped by provider
	var allProcessedMagnets []models.ProcessedMagnet
	
	for provider, providerMagnets := range magnetsByProvider {
		// Get the debrid service for this provider
		debridService := "alldebrid" // default
		if userConfig != nil {
			debridService = userConfig.GetDebridForProvider(provider)
		}
		
		h.services.Logger.Infof("[StreamHandler] processing %d magnets from %s with %s", len(providerMagnets), provider, debridService)
		
		// Currently only AllDebrid is supported
		if debridService != "alldebrid" {
			h.services.Logger.Warnf("[StreamHandler] unsupported debrid service %s for provider %s, skipping", debridService, provider)
			continue
		}
		
		// First, upload all magnets to AllDebrid
		h.services.Logger.Infof("[StreamHandler] uploading %d magnets to AllDebrid", len(providerMagnets))
		uploadCount := 0
		for _, magnet := range providerMagnets {
			err := h.services.AllDebrid.UploadMagnet(magnet.Hash, magnet.Title, apiKey)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] failed to upload magnet %s: %v", magnet.Title, err)
			} else {
				uploadCount++
				h.services.Logger.Infof("[StreamHandler] magnet uploaded to AllDebrid - title: %s", magnet.Title)
			}
		}
		h.services.Logger.Infof("[StreamHandler] uploaded %d/%d magnets successfully", uploadCount, len(providerMagnets))
		
		// Now check magnets with AllDebrid, with retry logic for timing issues
		var processedMagnets []models.ProcessedMagnet
		var err error
		
		for attempt := 1; attempt <= 8; attempt++ {
			h.services.Logger.Infof("[StreamHandler] %s CheckMagnets attempt %d/8 for provider %s", debridService, attempt, provider)
			processedMagnets, err = h.services.AllDebrid.CheckMagnets(providerMagnets, apiKey)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] %s CheckMagnets attempt %d failed for provider %s: %v", debridService, attempt, provider, err)
				if attempt == 8 {
					continue // Skip this provider's magnets
				}
				// Wait a bit before retrying
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			
			h.services.Logger.Infof("[StreamHandler] %s attempt %d completed for provider %s - processed %d magnets", debridService, attempt, provider, len(processedMagnets))
			
			// Count how many are ready
			readyCount := 0
			for _, magnet := range processedMagnets {
				if magnet.Ready && len(magnet.Links) > 0 {
					readyCount++
				}
			}
			
			// If we got some ready results, or this is the final attempt, use what we have
			if readyCount > 0 || attempt == 8 {
				h.services.Logger.Infof("[StreamHandler] using results from attempt %d - %d ready magnets for provider %s", attempt, readyCount, provider)
				break
			}
			
			// If no magnets ready, wait before retry (longer wait for later attempts)
			waitTime := time.Duration(attempt) * 2 * time.Second
			h.services.Logger.Infof("[StreamHandler] no ready magnets on attempt %d for provider %s, waiting %v before retry", attempt, provider, waitTime)
			time.Sleep(waitTime)
		}
		
		// Add processed magnets to the combined results
		allProcessedMagnets = append(allProcessedMagnets, processedMagnets...)
	}
	
	var streams []models.Stream
	finalReadyCount := 0
	for i, magnet := range allProcessedMagnets {
		h.services.Logger.Debugf("[StreamHandler] magnet %d: ready=%v, links_count=%d, hash=%s", i, magnet.Ready, len(magnet.Links), magnet.Hash)
		
		if magnet.Ready && len(magnet.Links) > 0 {
			finalReadyCount++
			torrent := hashToTorrent[magnet.Hash]
			
			h.services.Logger.Debugf("[StreamHandler] processing ready magnet - links: %+v", magnet.Links)
			
			// Find the largest video file from the links
			videoLink := h.findLargestVideoFile(magnet.Links)
			if videoLink == nil {
				h.services.Logger.Warnf("[StreamHandler] no video file found in magnet links")
				continue
			}
			
			h.services.Logger.Debugf("[StreamHandler] selected video link: %+v", videoLink)
			
			// Extract download URL from the video link object
			if webLink, exists := videoLink["link"].(string); exists {
				h.services.Logger.Debugf("[StreamHandler] attempting to unlock video link: %s", webLink)
				// Unlock the link to get the direct download URL
				directURL, err := h.services.AllDebrid.UnlockLink(webLink, apiKey)
				if err != nil {
					h.services.Logger.Errorf("[StreamHandler] failed to unlock link %s: %v", webLink, err)
					continue
				}
				
				h.services.Logger.Debugf("[StreamHandler] successfully unlocked video link: %s", directURL)
				
				// Get video file size and name
				videoSize := float64(0)
				videoName := magnet.Name
				if size, ok := videoLink["size"].(float64); ok {
					videoSize = size
				}
				if filename, ok := videoLink["filename"].(string); ok {
					videoName = filename
				}
				
				stream := models.Stream{
					Name:  fmt.Sprintf("[%s] %s", torrent.Source, torrent.Title),
					Title: fmt.Sprintf("%.2f GB - %s", videoSize/(1024*1024*1024), videoName),
					URL:   directURL,
				}
				streams = append(streams, stream)
			} else {
				h.services.Logger.Warnf("[StreamHandler] video link object missing 'link' field")
			}
		} else {
			h.services.Logger.Debugf("[StreamHandler] skipping magnet - ready=%v, links=%d", magnet.Ready, len(magnet.Links))
		}
	}
	h.services.Logger.Debugf("[StreamHandler] cache check results - ready: %d, total: %d", finalReadyCount, len(allProcessedMagnets))
	
	// Note: Magnets are already uploaded before checking, so no need to upload again
	
	// Combine episode streams from complete seasons with regular streams
	// Prioritize episodes from complete seasons by placing them first
	allStreams := append(episodeStreams, streams...)
	
	// Sort all streams using comprehensive logic: resolution priority, language priority, size, and season priority
	h.sortAllStreams(allStreams, len(episodeStreams), userConfig)
	
	h.services.Logger.Debugf("[StreamHandler] stream processing completed - returning %d streams (%d from complete seasons, %d regular)", 
		len(allStreams), len(episodeStreams), len(streams))
	return allStreams
}

func parseStreamID(id string) (string, int, int) {
	id = strings.TrimSuffix(id, ".json")
	
	if imdbIDRegex.MatchString(id) {
		return id, 0, 0
	}
	
	if matches := episodeRegex.FindStringSubmatch(id); len(matches) == 3 {
		season, _ := strconv.Atoi(matches[1])
		episode, _ := strconv.Atoi(matches[2])
		// Extract IMDB ID from the full match (e.g., "tt19514434:1:1" -> "tt19514434")
		colonIndex := strings.Index(matches[0], ":")
		if colonIndex > 0 {
			return matches[0][:colonIndex], season, episode
		}
		return matches[0], season, episode
	}
	
	return "", 0, 0
}

// extractEpisodesFromSeasons processes complete season torrents to extract specific episodes
func (h *Handler) extractEpisodesFromSeasons(seasonTorrents []models.TorrentInfo, apiKey string, userConfig *config.Config, targetSeason, targetEpisode int) []models.Stream {
	var episodeStreams []models.Stream
	
	h.services.Logger.Infof("[StreamHandler] extractEpisodesFromSeasons started - torrents: %d, target: S%02dE%02d", 
		len(seasonTorrents), targetSeason, targetEpisode)
	
	// First, get hashes for YGG season torrents that don't have them
	for i, torrent := range seasonTorrents {
		if torrent.Hash == "" && torrent.Source == "YGG" {
			h.services.Logger.Infof("[StreamHandler] fetching hash for YGG season torrent: %s", torrent.Title)
			hash, err := h.services.YGG.GetTorrentHash(torrent.ID)
			if err != nil {
				h.services.Logger.Warnf("[StreamHandler] failed to get hash for season torrent %s: %v", torrent.Title, err)
			} else if hash != "" {
				seasonTorrents[i].Hash = hash
				h.services.Logger.Infof("[StreamHandler] got hash %s for season torrent %s", hash, torrent.Title)
			}
		}
	}
	
	// Group season torrents by provider to use appropriate debrid service
	magnetsByProvider := make(map[string][]models.TorrentInfo)
	for _, torrent := range seasonTorrents {
		if torrent.Hash != "" {
			provider := strings.ToLower(torrent.Source)
			magnetsByProvider[provider] = append(magnetsByProvider[provider], torrent)
			h.services.Logger.Debugf("[StreamHandler] grouped season torrent - provider: %s, title: %s, hash: %s", 
				provider, torrent.Title, torrent.Hash)
		} else {
			h.services.Logger.Warnf("[StreamHandler] season torrent has no hash after retrieval - title: %s", torrent.Title)
		}
	}
	
	for provider, providerTorrents := range magnetsByProvider {
		// Get the debrid service for this provider
		debridService := "alldebrid" // default
		if userConfig != nil {
			debridService = userConfig.GetDebridForProvider(provider)
		}
		
		// Currently only AllDebrid is supported
		if debridService != "alldebrid" {
			h.services.Logger.Warnf("[StreamHandler] unsupported debrid service %s for provider %s, skipping season processing", debridService, provider)
			continue
		}
		
		// Convert torrents to magnet info for checking availability
		var magnets []models.MagnetInfo
		for _, torrent := range providerTorrents {
			magnets = append(magnets, models.MagnetInfo{
				Hash:   torrent.Hash,
				Title:  torrent.Title,
				Source: torrent.Source,
			})
		}
		
		// Check which season torrents are available
		h.services.Logger.Infof("[StreamHandler] checking %d season magnets with AllDebrid", len(magnets))
		processedMagnets, err := h.services.AllDebrid.CheckMagnets(magnets, apiKey)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to check season magnets: %v", err)
			continue
		}
		
		h.services.Logger.Infof("[StreamHandler] AllDebrid returned %d processed magnets", len(processedMagnets))
		
		// Process each season torrent - upload if not ready
		for idx, magnet := range processedMagnets {
			h.services.Logger.Debugf("[StreamHandler] processed magnet %d - ready: %v, links: %d, hash: %s", 
				idx, magnet.Ready, len(magnet.Links), magnet.Hash)
			
			// If not ready, try to upload to AllDebrid
			if !magnet.Ready {
				h.services.Logger.Infof("[StreamHandler] uploading season torrent to AllDebrid: %s", magnet.Name)
				err := h.services.AllDebrid.UploadMagnet(magnet.Hash, magnet.Name, apiKey)
				if err != nil {
					h.services.Logger.Errorf("[StreamHandler] failed to upload season torrent: %v", err)
					continue
				}
				
				// Wait for torrent to be processed
				h.services.Logger.Infof("[StreamHandler] waiting for torrent to be processed...")
				time.Sleep(3 * time.Second)
				
				// Check again
				singleMagnet := []models.MagnetInfo{{Hash: magnet.Hash, Title: magnet.Name, Source: magnet.Source}}
				updatedMagnets, err := h.services.AllDebrid.CheckMagnets(singleMagnet, apiKey)
				if err != nil {
					h.services.Logger.Errorf("[StreamHandler] failed to recheck uploaded season torrent: %v", err)
					continue
				}
				
				if len(updatedMagnets) > 0 {
					magnet = updatedMagnets[0]
					h.services.Logger.Infof("[StreamHandler] updated status - ready: %v, links: %d", magnet.Ready, len(magnet.Links))
				}
			}
			
			if magnet.Ready && len(magnet.Links) > 0 {
				// Find the original torrent info
				var originalTorrent models.TorrentInfo
				for _, torrent := range providerTorrents {
					if torrent.Hash == magnet.Hash {
						originalTorrent = torrent
						break
					}
				}
				
				// Extract episode files from this season torrent
				h.services.Logger.Infof("[StreamHandler] extracting episode files from season torrent: %s (ID: %d)", 
					originalTorrent.Title, magnet.ID)
				
				episodeFiles, err := h.services.AllDebrid.GetEpisodeFiles(fmt.Sprintf("%d", magnet.ID), originalTorrent, apiKey)
				if err != nil {
					h.services.Logger.Errorf("[StreamHandler] failed to get episode files from season torrent %s: %v", originalTorrent.Title, err)
					continue
				}
				
				h.services.Logger.Infof("[StreamHandler] extracted %d episode files from season torrent", len(episodeFiles))
				
				// Filter episodes to find the target episode
				for _, episode := range episodeFiles {
					h.services.Logger.Debugf("[StreamHandler] checking episode: S%02dE%02d - %s (target: S%02dE%02d)", 
						episode.Season, episode.Episode, episode.Name, targetSeason, targetEpisode)
					
					if episode.Season == targetSeason && episode.Episode == targetEpisode {
						h.services.Logger.Infof("[StreamHandler] found matching episode: S%02dE%02d - %s", 
							episode.Season, episode.Episode, episode.Name)
						
						// Apply same filtering logic as regular torrents
						if h.isEpisodeAllowed(episode, userConfig) {
							h.services.Logger.Infof("[StreamHandler] episode passed filtering: %s", episode.Name)
							// Unlock the episode link
							directURL, err := h.services.AllDebrid.UnlockLink(episode.Link, apiKey)
							if err != nil {
								h.services.Logger.Errorf("[StreamHandler] failed to unlock episode link: %v", err)
								continue
							}
							
							stream := models.Stream{
								Name:  fmt.Sprintf("[%s] %s", episode.Source, originalTorrent.Title),
								Title: fmt.Sprintf("%.2f GB - %s (S%02dE%02d from season)", episode.Size/(1024*1024*1024), episode.Name, episode.Season, episode.Episode),
								URL:   directURL,
							}
							episodeStreams = append(episodeStreams, stream)
						}
					}
				}
			}
		}
	}
	
	// Sort episode streams by quality (resolution, language, size)
	h.sortEpisodeStreams(episodeStreams, userConfig)
	
	return episodeStreams
}

// isEpisodeAllowed checks if an episode meets the filtering criteria
func (h *Handler) isEpisodeAllowed(episode models.EpisodeFile, userConfig *config.Config) bool {
	if userConfig == nil {
		return true
	}
	
	h.services.Logger.Debugf("[StreamHandler] filtering episode: %s (resolution: %s, language: %s)", episode.Name, episode.Resolution, episode.Language)
	
	// Check resolution filter - only filter if we have a valid resolution
	if episode.Resolution != "" && episode.Resolution != "unknown" {
		if !userConfig.IsResolutionAllowed(episode.Resolution) {
			h.services.Logger.Debugf("[StreamHandler] episode filtered by resolution: %s not in %v", episode.Resolution, userConfig.ResToShow)
			return false
		}
	}
	
	// Check language filter using the same logic as regular torrents
	languageAllowed := len(userConfig.LangToShow) == 0 // Allow all if no filter set
	if !languageAllowed {
		for _, lang := range userConfig.LangToShow {
			if h.containsLanguage(episode.Name, lang) || (episode.Language != "" && strings.EqualFold(episode.Language, lang)) {
				languageAllowed = true
				break
			}
		}
		if !languageAllowed {
			h.services.Logger.Debugf("[StreamHandler] episode filtered by language: %s not matching %v", episode.Language, userConfig.LangToShow)
			return false
		}
	}
	
	h.services.Logger.Debugf("[StreamHandler] episode passed all filters: %s", episode.Name)
	return true
}

// containsLanguage checks if a title contains a specific language
func (h *Handler) containsLanguage(title, language string) bool {
	titleLower := strings.ToLower(title)
	langLower := strings.ToLower(language)

	switch langLower {
	case "multi", "multi_fr":
		return strings.Contains(titleLower, "multi")
	case "french", "vf", "vff":
		return strings.Contains(titleLower, "french") ||
			strings.Contains(titleLower, "vff") ||
			strings.Contains(titleLower, "vf") ||
			strings.Contains(titleLower, "truefrench")
	case "vo":
		return strings.Contains(titleLower, "vo") ||
			strings.Contains(titleLower, "vostfr") ||
			strings.Contains(titleLower, "english") ||
			(!strings.Contains(titleLower, "vf") && !strings.Contains(titleLower, "french") && !strings.Contains(titleLower, "multi"))
	case "english":
		return strings.Contains(titleLower, "english") ||
			strings.Contains(titleLower, "vostfr")
	case "vostfr":
		return strings.Contains(titleLower, "vostfr")
	default:
		return strings.Contains(titleLower, langLower)
	}
}

// sortEpisodeStreams sorts episode streams by resolution priority, language priority, and size
func (h *Handler) sortEpisodeStreams(episodes []models.Stream, userConfig *config.Config) {
	if userConfig == nil {
		return
	}
	
	h.services.Logger.Debugf("[StreamHandler] sorting %d episode streams", len(episodes))
	
	sort.Slice(episodes, func(i, j int) bool {
		titleI := episodes[i].Name + " " + episodes[i].Title
		titleJ := episodes[j].Name + " " + episodes[j].Title
		
		// 1. Resolution priority (higher priority first)
		resI := h.extractResolutionFromTitle(episodes[i].Title)
		resJ := h.extractResolutionFromTitle(episodes[j].Title)
		
		priResI := userConfig.GetResolutionPriority(resI)
		priResJ := userConfig.GetResolutionPriority(resJ)
		
		h.services.Logger.Debugf("[StreamHandler] comparing resolutions: %s (priority %d) vs %s (priority %d)", resI, priResI, resJ, priResJ)
		
		if priResI != priResJ {
			return priResI > priResJ
		}
		
		// 2. Language priority (higher priority first)
		priLangI := userConfig.GetLanguagePriority(titleI)
		priLangJ := userConfig.GetLanguagePriority(titleJ)
		
		h.services.Logger.Debugf("[StreamHandler] comparing languages: %s (priority %d) vs %s (priority %d)", titleI, priLangI, titleJ, priLangJ)
		
		if priLangI != priLangJ {
			return priLangI > priLangJ
		}
		
		// 3. Size (larger files first)
		sizeI := h.getSizeFromTitle(episodes[i].Title)
		sizeJ := h.getSizeFromTitle(episodes[j].Title)
		
		h.services.Logger.Debugf("[StreamHandler] comparing sizes: %.2f GB vs %.2f GB", sizeI, sizeJ)
		
		return sizeI > sizeJ
	})
	
	h.services.Logger.Debugf("[StreamHandler] episode sorting completed")
}

// extractResolutionFromTitle extracts resolution from episode title or stream name
func (h *Handler) extractResolutionFromTitle(title string) string {
	titleLower := strings.ToLower(title)
	
	// Check for 4K/2160p
	if strings.Contains(titleLower, "2160p") || strings.Contains(titleLower, "4k") {
		return "2160p"
	}
	// Check for 1080p
	if strings.Contains(titleLower, "1080p") {
		return "1080p"
	}
	// Check for 720p
	if strings.Contains(titleLower, "720p") {
		return "720p"
	}
	// Check for 480p
	if strings.Contains(titleLower, "480p") {
		return "480p"
	}
	
	return "unknown"
}

// getSizeFromTitle extracts file size from episode title (format: "X.XX GB")
func (h *Handler) getSizeFromTitle(title string) float64 {
	// Look for pattern like "2.34 GB" at the beginning of the title
	re := regexp.MustCompile(`(\d+\.?\d*)\s*GB`)
	matches := re.FindStringSubmatch(title)
	if len(matches) >= 2 {
		if size, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return size
		}
	}
	return 0.0
}

// sortAllStreams sorts all streams (both episode streams from seasons and regular streams)
// with comprehensive logic: season priority, resolution priority, language priority, and size
func (h *Handler) sortAllStreams(allStreams []models.Stream, episodeCount int, userConfig *config.Config) {
	if userConfig == nil {
		return
	}
	
	h.services.Logger.Debugf("[StreamHandler] sorting %d total streams (%d from seasons)", len(allStreams), episodeCount)
	
	sort.Slice(allStreams, func(i, j int) bool {
		// 1. Prioritize episodes from complete seasons (they come first in the slice)
		iFromSeason := i < episodeCount
		jFromSeason := j < episodeCount
		
		if iFromSeason && !jFromSeason {
			return true // Episodes from seasons come first
		}
		if !iFromSeason && jFromSeason {
			return false // Episodes from seasons come first
		}
		
		// 2. Within the same group, use comprehensive sorting
		titleI := allStreams[i].Name + " " + allStreams[i].Title
		titleJ := allStreams[j].Name + " " + allStreams[j].Title
		
		// 2a. Resolution priority (higher priority first)
		resI := h.extractResolutionFromTitle(allStreams[i].Title)
		resJ := h.extractResolutionFromTitle(allStreams[j].Title)
		
		priResI := userConfig.GetResolutionPriority(resI)
		priResJ := userConfig.GetResolutionPriority(resJ)
		
		if priResI != priResJ {
			return priResI > priResJ
		}
		
		// 2b. Language priority (higher priority first)
		priLangI := userConfig.GetLanguagePriority(titleI)
		priLangJ := userConfig.GetLanguagePriority(titleJ)
		
		if priLangI != priLangJ {
			return priLangI > priLangJ
		}
		
		// 2c. Size (larger files first)
		sizeI := h.getSizeFromTitle(allStreams[i].Title)
		sizeJ := h.getSizeFromTitle(allStreams[j].Title)
		
		return sizeI > sizeJ
	})
	
	h.services.Logger.Debugf("[StreamHandler] all streams sorting completed")
}

func deduplicateStreams(streams []models.Stream) []models.Stream {
	seen := make(map[string]bool)
	var unique []models.Stream
	
	for _, stream := range streams {
		key := stream.URL
		if !seen[key] {
			seen[key] = true
			unique = append(unique, stream)
		}
	}
	
	return unique
}

func isMagnetReady(hash string, processedMagnets []models.ProcessedMagnet) bool {
	for _, m := range processedMagnets {
		if m.Hash == hash && m.Ready {
			return true
		}
	}
	return false
}

// findLargestVideoFile finds the largest video file from AllDebrid magnet links
func (h *Handler) findLargestVideoFile(links []interface{}) map[string]interface{} {
	var largestVideoFile map[string]interface{}
	var largestSize float64 = 0
	
	for _, link := range links {
		if linkObj, ok := link.(map[string]interface{}); ok {
			// Check if this is a video file
			if filename, exists := linkObj["filename"].(string); exists {
				if isVideoFile(filename) {
					// Get file size
					var size float64 = 0
					if sizeVal, exists := linkObj["size"].(float64); exists {
						size = sizeVal
					}
					
					h.services.Logger.Debugf("[StreamHandler] found video file: %s (%.2f GB)", filename, size/(1024*1024*1024))
					
					// Keep track of the largest video file
					if size > largestSize {
						largestSize = size
						largestVideoFile = linkObj
					}
				} else {
					h.services.Logger.Debugf("[StreamHandler] skipping non-video file: %s", filename)
				}
			}
		}
	}
	
	if largestVideoFile != nil {
		if filename, exists := largestVideoFile["filename"].(string); exists {
			h.services.Logger.Infof("[StreamHandler] selected largest video file: %s (%.2f GB)", filename, largestSize/(1024*1024*1024))
		}
	}
	
	return largestVideoFile
}

// isVideoFile checks if a filename has a video extension
func isVideoFile(filename string) bool {
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".ts", ".m2ts"}
	filename = strings.ToLower(filename)
	
	for _, ext := range videoExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	
	return false
}

// matchesYear checks if a torrent title contains the expected year
func (h *Handler) matchesYear(title string, expectedYear int) bool {
	if expectedYear == 0 {
		return true // If no year provided, don't filter
	}
	
	// Look for 4-digit year patterns in the title
	yearPattern := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	matches := yearPattern.FindAllString(title, -1)
	
	for _, match := range matches {
		if year, err := strconv.Atoi(match); err == nil {
			// Allow some flexibility: exact year or within 1 year
			if year == expectedYear || year == expectedYear-1 || year == expectedYear+1 {
				return true
			}
		}
	}
	
	// If no year found in title, allow it (some torrents don't include year)
	return len(matches) == 0
}

