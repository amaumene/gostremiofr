package handlers

import (
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
		streams = h.searchSeriesStreams(title, season, episode, apiKey, imdbID)
	}
	
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

func (h *Handler) searchMovieStreams(title string, year int, apiKey string, userConfig *config.Config) []models.Stream {
	// Search with original title only - YGG service will handle French title conversion internally
	results := h.searchTorrentsOnly(title, "movie", 0, 0, "", year)
	
	h.services.Logger.Infof("[StreamHandler] search results - movies: %d, starting AllDebrid processing", len(results.MovieTorrents))
	
	// Process results with AllDebrid
	return h.processResults(results, apiKey, userConfig, year)
}

func (h *Handler) searchSeriesStreams(title string, season, episode int, apiKey, imdbID string) []models.Stream {
	// Search with original title only - YGG service will handle French title conversion internally
	return h.searchTorrentsWithIMDB(title, "series", season, episode, apiKey, imdbID)
}

func (h *Handler) searchTorrents(query string, mediaType string, season, episode int, apiKey string) []models.Stream {
	return h.searchTorrentsWithIMDB(query, mediaType, season, episode, apiKey, "")
}

func (h *Handler) searchTorrentsWithIMDB(query string, mediaType string, season, episode int, apiKey string, imdbID string) []models.Stream {
	h.services.Logger.Debugf("[StreamHandler] torrent search initiated - query: %s, mediaType: %s, imdbID: %s", query, mediaType, imdbID)
	
	var combinedResults models.CombinedTorrentResults
	var wg sync.WaitGroup
	
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
		}
	}()
	
	
	// Add EZTV search for series with IMDB ID
	if mediaType == "series" && imdbID != "" {
		go func() {
			defer wg.Done()
			h.services.Logger.Debugf("[StreamHandler] EZTV search started for IMDB ID: %s", imdbID)
			results, err := h.services.EZTV.SearchTorrentsByIMDB(imdbID, season, episode)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] failed to search EZTV: %v", err)
				return
			}
			
			if results != nil {
				h.services.Logger.Debugf("[StreamHandler] EZTV results - complete series: %d, seasons: %d, episodes: %d", 
					len(results.CompleteSeriesTorrents), 
					len(results.CompleteSeasonTorrents), 
					len(results.EpisodeTorrents))
				for _, t := range results.CompleteSeriesTorrents {
					combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, t)
				}
				for _, t := range results.CompleteSeasonTorrents {
					combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, t)
				}
				for _, t := range results.EpisodeTorrents {
					combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
				}
			} else {
				h.services.Logger.Debugf("[StreamHandler] EZTV returned no results for IMDB ID: %s", imdbID)
			}
		}()
	} else {
		h.services.Logger.Debugf("[StreamHandler] skipping EZTV search - mediaType: %s, imdbID: %s", mediaType, imdbID)
	}
	
	wg.Wait()
	
	h.services.Logger.Infof("[StreamHandler] search completed - movies: %d, complete series: %d, seasons: %d, episodes: %d", 
		len(combinedResults.MovieTorrents), 
		len(combinedResults.CompleteSeriesTorrents), 
		len(combinedResults.CompleteSeasonTorrents), 
		len(combinedResults.EpisodeTorrents))
	
	return h.processResults(&combinedResults, apiKey, nil, 0)
}

// searchTorrentsOnly searches for torrents without processing through AllDebrid
func (h *Handler) searchTorrentsOnly(query, mediaType string, season, episode int, imdbID string, year int) *models.CombinedTorrentResults {
	var wg sync.WaitGroup
	combinedResults := models.CombinedTorrentResults{}
	
	// Add YGG search
	wg.Add(1)
	go func() {
		defer wg.Done()
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
		}
	}()
	
	// Add EZTV search for series with IMDB ID
	if mediaType == "series" && imdbID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.services.Logger.Debugf("[StreamHandler] EZTV search started for IMDB ID: %s", imdbID)
			results, err := h.services.EZTV.SearchTorrentsByIMDB(imdbID, season, episode)
			if err != nil {
				h.services.Logger.Errorf("[StreamHandler] failed to search EZTV: %v", err)
				return
			}
			
			if results != nil {
				h.services.Logger.Debugf("[StreamHandler] EZTV results - complete series: %d, seasons: %d, episodes: %d", 
					len(results.CompleteSeriesTorrents), 
					len(results.CompleteSeasonTorrents), 
					len(results.EpisodeTorrents))
				for _, t := range results.CompleteSeriesTorrents {
					combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, t)
				}
				for _, t := range results.CompleteSeasonTorrents {
					combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, t)
				}
				for _, t := range results.EpisodeTorrents {
					combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, t)
				}
			} else {
				h.services.Logger.Debugf("[StreamHandler] EZTV returned no results for IMDB ID: %s", imdbID)
			}
		}()
	} else {
		h.services.Logger.Debugf("[StreamHandler] skipping EZTV search - mediaType: %s, imdbID: %s", mediaType, imdbID)
	}
	
	wg.Wait()
	
	h.services.Logger.Infof("[StreamHandler] search completed - movies: %d, complete series: %d, seasons: %d, episodes: %d", 
		len(combinedResults.MovieTorrents), 
		len(combinedResults.CompleteSeriesTorrents), 
		len(combinedResults.CompleteSeasonTorrents), 
		len(combinedResults.EpisodeTorrents))
	
	return &combinedResults
}

func (h *Handler) processResults(results *models.CombinedTorrentResults, apiKey string, userConfig *config.Config, year int) []models.Stream {
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
				magnets = append(magnets, models.MagnetInfo{
					Hash:   torrent.Hash,
					Title:  torrent.Title,
					Source: torrent.Source,
				})
				hashToTorrent[torrent.Hash] = torrent
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
	
	if len(magnets) == 0 {
		return []models.Stream{}
	}
	
	// Limit the number of magnets to prevent AllDebrid API timeouts
	maxMagnets := 30
	if len(magnets) > maxMagnets {
		h.services.Logger.Infof("[StreamHandler] limiting magnets from %d to %d to prevent timeouts", len(magnets), maxMagnets)
		magnets = magnets[:maxMagnets]
	}
	
	h.services.Logger.Infof("[StreamHandler] checking %d magnets with AllDebrid", len(magnets))
	
	// Try checking magnets with AllDebrid, with retry logic for timing issues
	var processedMagnets []models.ProcessedMagnet
	var err error
	
	for attempt := 1; attempt <= 2; attempt++ {
		h.services.Logger.Infof("[StreamHandler] AllDebrid CheckMagnets attempt %d/2", attempt)
		processedMagnets, err = h.services.AllDebrid.CheckMagnets(magnets, apiKey)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] AllDebrid CheckMagnets attempt %d failed: %v", attempt, err)
			if attempt == 2 {
				return []models.Stream{}
			}
			continue
		}
		
		h.services.Logger.Infof("[StreamHandler] AllDebrid attempt %d completed - processed %d magnets", attempt, len(processedMagnets))
		
		// Count how many are ready
		readyCount := 0
		for _, magnet := range processedMagnets {
			if magnet.Ready && len(magnet.Links) > 0 {
				readyCount++
			}
		}
		
		// If we got some ready results, or this is the final attempt, use what we have
		if readyCount > 0 || attempt == 2 {
			h.services.Logger.Infof("[StreamHandler] using results from attempt %d - %d ready magnets", attempt, readyCount)
			break
		}
		
		// If no magnets ready on first attempt, wait 2 seconds and retry
		h.services.Logger.Infof("[StreamHandler] no ready magnets on attempt %d, waiting 2s before retry", attempt)
		time.Sleep(2 * time.Second)
	}
	
	var streams []models.Stream
	finalReadyCount := 0
	for i, magnet := range processedMagnets {
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
	h.services.Logger.Debugf("[StreamHandler] cache check results - ready: %d, total: %d", finalReadyCount, len(processedMagnets))
	
	maxFiles := constants.DefaultFilesToShow
	if h.config != nil {
		maxFiles = h.config.FilesToShow
	}
	
	if len(streams) < maxFiles {
		for _, magnet := range magnets {
			if len(streams) >= maxFiles {
				break
			}
			
			if !isMagnetReady(magnet.Hash, processedMagnets) {
				err := h.services.AllDebrid.UploadMagnet(magnet.Hash, magnet.Title, apiKey)
				if err == nil {
					h.services.Logger.Infof("[StreamHandler] magnet uploaded to AllDebrid - title: %s", magnet.Title)
				}
			}
		}
	}
	
	// Sort streams by size (biggest first)
	sort.Slice(streams, func(i, j int) bool {
		return getSizeFromTitle(streams[i].Title) > getSizeFromTitle(streams[j].Title)
	})
	
	h.services.Logger.Debugf("[StreamHandler] stream processing completed - returning %d streams", len(streams))
	return streams
}

func getSizeFromTitle(title string) float64 {
	// Extract size from title format like "1.50 GB - filename"
	parts := strings.Split(title, " GB - ")
	if len(parts) < 2 {
		return 0
	}
	
	sizeStr := parts[0]
	size, err := strconv.ParseFloat(sizeStr, 64)
	if err != nil {
		return 0
	}
	
	return size
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

