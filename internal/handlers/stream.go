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

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/config"
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
	
	mediaType, title, frenchTitle, err := h.services.TMDB.GetIMDBInfo(imdbID)
	if err != nil {
		h.services.Logger.Errorf("[StreamHandler] failed to get TMDB info for %s: %v", imdbID, err)
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	h.services.Logger.Infof("[StreamHandler] processing %s request - %s (%s)", mediaType, title, imdbID)
	
	var streams []models.Stream
	
	if mediaType == "movie" {
		streams = h.searchMovieStreams(title, frenchTitle, apiKey)
	} else if mediaType == "series" {
		streams = h.searchSeriesStreams(title, frenchTitle, season, episode, apiKey, imdbID)
	}
	
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

func (h *Handler) searchMovieStreams(title, frenchTitle, apiKey string) []models.Stream {
	var wg sync.WaitGroup
	streamsChan := make(chan []models.Stream, 2)
	
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		streams := h.searchTorrentsWithIMDB(title, "movie", 0, 0, apiKey, "")
		streamsChan <- streams
	}()
	
	go func() {
		defer wg.Done()
		if frenchTitle != title {
			streams := h.searchTorrentsWithIMDB(frenchTitle, "movie", 0, 0, apiKey, "")
			streamsChan <- streams
		} else {
			streamsChan <- []models.Stream{}
		}
	}()
	
	go func() {
		wg.Wait()
		close(streamsChan)
	}()
	
	var allStreams []models.Stream
	for streams := range streamsChan {
		allStreams = append(allStreams, streams...)
	}
	
	return deduplicateStreams(allStreams)
}

func (h *Handler) searchSeriesStreams(title, frenchTitle string, season, episode int, apiKey, imdbID string) []models.Stream {
	var wg sync.WaitGroup
	streamsChan := make(chan []models.Stream, 2)
	
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		streams := h.searchTorrentsWithIMDB(title, "series", season, episode, apiKey, imdbID)
		streamsChan <- streams
	}()
	
	go func() {
		defer wg.Done()
		if frenchTitle != title {
			streams := h.searchTorrentsWithIMDB(frenchTitle, "series", season, episode, apiKey, imdbID)
			streamsChan <- streams
		} else {
			streamsChan <- []models.Stream{}
		}
	}()
	
	go func() {
		wg.Wait()
		close(streamsChan)
	}()
	
	var allStreams []models.Stream
	for streams := range streamsChan {
		allStreams = append(allStreams, streams...)
	}
	
	return deduplicateStreams(allStreams)
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
	
	return h.processResults(&combinedResults, apiKey)
}

func (h *Handler) processResults(results *models.CombinedTorrentResults, apiKey string) []models.Stream {
	var magnets []models.MagnetInfo
	hashToTorrent := make(map[string]models.TorrentInfo)
	
	processTorrents := func(torrents []models.TorrentInfo) {
		for _, torrent := range torrents {
			if torrent.Hash == "" && torrent.Source == "YGG" {
				hash, err := h.services.YGG.GetTorrentHash(torrent.ID)
				if err == nil {
					torrent.Hash = hash
				}
			}
			
			if torrent.Hash != "" {
				magnets = append(magnets, models.MagnetInfo{
					Hash:   torrent.Hash,
					Title:  torrent.Title,
					Source: torrent.Source,
				})
				hashToTorrent[torrent.Hash] = torrent
			}
		}
	}
	
	processTorrents(results.MovieTorrents)
	processTorrents(results.CompleteSeriesTorrents)
	processTorrents(results.CompleteSeasonTorrents)
	processTorrents(results.EpisodeTorrents)
	
	if len(magnets) == 0 {
		return []models.Stream{}
	}
	
	h.services.Logger.Debugf("[StreamHandler] checking %d magnets with AllDebrid", len(magnets))
	processedMagnets, err := h.services.AllDebrid.CheckMagnets(magnets, apiKey)
	if err != nil {
		h.services.Logger.Errorf("[StreamHandler] failed to check magnets with AllDebrid: %v", err)
		return []models.Stream{}
	}
	h.services.Logger.Debugf("[StreamHandler] AllDebrid processing completed - processed %d magnets", len(processedMagnets))
	
	var streams []models.Stream
	readyCount := 0
	for _, magnet := range processedMagnets {
		if magnet.Ready && len(magnet.Links) > 0 {
			readyCount++
			torrent := hashToTorrent[magnet.Hash]
			
			// Extract download URL from link object
			if linkObj, ok := magnet.Links[0].(map[string]interface{}); ok {
				// The AllDebrid web interface link is in the "link" field - need to unlock it
				if webLink, exists := linkObj["link"].(string); exists {
					// Unlock the link to get the direct download URL
					directURL, err := h.services.AllDebrid.UnlockLink(webLink, apiKey)
					if err != nil {
						h.services.Logger.Errorf("[StreamHandler] failed to unlock link %s: %v", webLink, err)
						continue
					}
					
					stream := models.Stream{
						Name:  fmt.Sprintf("[%s] %s", torrent.Source, torrent.Title),
						Title: fmt.Sprintf("%.2f GB - %s", magnet.Size/(1024*1024*1024), magnet.Name),
						URL:   directURL,
					}
					streams = append(streams, stream)
				}
			}
		}
	}
	h.services.Logger.Debugf("[StreamHandler] cache check results - ready: %d, total: %d", readyCount, len(processedMagnets))
	
	maxFiles := 10 // default
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

