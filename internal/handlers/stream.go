package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gostremiofr/gostremiofr/internal/models"
)

var (
	imdbIDRegex = regexp.MustCompile(`^tt\d+$`)
	episodeRegex = regexp.MustCompile(`^tt\d+:(\d+):(\d+)$`)
)

func (h *Handler) handleStream(c *gin.Context) {
	configuration := c.Param("configuration")
	id := c.Param("id")
	
	var userConfig map[string]string
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	
	apiKey := userConfig["API_KEY_ALLDEBRID"]
	if apiKey == "" {
		apiKey = h.config.APIKeyAllDebrid
	}
	
	if apiKey == "" {
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	imdbID, season, episode := parseStreamID(id)
	if imdbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}
	
	mediaType, title, frenchTitle, err := h.services.TMDB.GetIMDBInfo(imdbID)
	if err != nil {
		h.services.Logger.Errorf("failed to get TMDB info: %v", err)
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}
	
	var streams []models.Stream
	
	if mediaType == "movie" {
		streams = h.searchMovieStreams(title, frenchTitle, apiKey)
	} else if mediaType == "series" {
		streams = h.searchSeriesStreams(title, frenchTitle, season, episode, apiKey)
	}
	
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

func (h *Handler) searchMovieStreams(title, frenchTitle, apiKey string) []models.Stream {
	var wg sync.WaitGroup
	streamsChan := make(chan []models.Stream, 2)
	
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		streams := h.searchTorrents(title, "movie", 0, 0, apiKey)
		streamsChan <- streams
	}()
	
	go func() {
		defer wg.Done()
		if frenchTitle != title {
			streams := h.searchTorrents(frenchTitle, "movie", 0, 0, apiKey)
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

func (h *Handler) searchSeriesStreams(title, frenchTitle string, season, episode int, apiKey string) []models.Stream {
	var wg sync.WaitGroup
	streamsChan := make(chan []models.Stream, 2)
	
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		streams := h.searchTorrents(title, "series", season, episode, apiKey)
		streamsChan <- streams
	}()
	
	go func() {
		defer wg.Done()
		if frenchTitle != title {
			streams := h.searchTorrents(frenchTitle, "series", season, episode, apiKey)
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
	var combinedResults models.CombinedTorrentResults
	var wg sync.WaitGroup
	
	wg.Add(2)
	
	go func() {
		defer wg.Done()
		category := "movie"
		if mediaType == "series" {
			category = "series"
		}
		
		results, err := h.services.YGG.SearchTorrents(query, category)
		if err != nil {
			h.services.Logger.Errorf("YGG search error: %v", err)
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
	
	go func() {
		defer wg.Done()
		results, err := h.services.Sharewood.SearchTorrents(query, mediaType, season, episode)
		if err != nil {
			h.services.Logger.Errorf("Sharewood search error: %v", err)
			return
		}
		
		if results != nil {
			for _, t := range results.MovieTorrents {
				combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, models.TorrentInfo{
					ID:     t.ID,
					Title:  t.Name,
					Hash:   t.InfoHash,
					Source: t.Source,
				})
			}
			for _, t := range results.CompleteSeriesTorrents {
				combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, models.TorrentInfo{
					ID:     t.ID,
					Title:  t.Name,
					Hash:   t.InfoHash,
					Source: t.Source,
				})
			}
			for _, t := range results.CompleteSeasonTorrents {
				combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, models.TorrentInfo{
					ID:     t.ID,
					Title:  t.Name,
					Hash:   t.InfoHash,
					Source: t.Source,
				})
			}
			for _, t := range results.EpisodeTorrents {
				combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, models.TorrentInfo{
					ID:     t.ID,
					Title:  t.Name,
					Hash:   t.InfoHash,
					Source: t.Source,
				})
			}
		}
	}()
	
	wg.Wait()
	
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
	
	processedMagnets, err := h.services.AllDebrid.CheckMagnets(magnets, apiKey)
	if err != nil {
		h.services.Logger.Errorf("AllDebrid check error: %v", err)
		return []models.Stream{}
	}
	
	var streams []models.Stream
	for _, magnet := range processedMagnets {
		if magnet.Ready {
			torrent := hashToTorrent[magnet.Hash]
			stream := models.Stream{
				Name:  fmt.Sprintf("[%s] %s", torrent.Source, torrent.Title),
				Title: fmt.Sprintf("%.2f GB - %s", magnet.Size/(1024*1024*1024), magnet.Name),
				URL:   fmt.Sprintf("https://api.alldebrid.com/v4/magnet/instant?agent=stremio&apikey=%s&id=%s", apiKey, magnet.ID),
			}
			streams = append(streams, stream)
		}
	}
	
	if len(streams) < h.config.FilesToShow {
		for _, magnet := range magnets {
			if len(streams) >= h.config.FilesToShow {
				break
			}
			
			if !isMagnetReady(magnet.Hash, processedMagnets) {
				err := h.services.AllDebrid.UploadMagnet(magnet.Hash, magnet.Title, apiKey)
				if err == nil {
					h.services.Logger.Infof("uploaded magnet: %s", magnet.Title)
				}
			}
		}
	}
	
	return streams
}

func parseStreamID(id string) (string, int, int) {
	id = strings.TrimSuffix(id, ".json")
	
	if imdbIDRegex.MatchString(id) {
		return id, 0, 0
	}
	
	if matches := episodeRegex.FindStringSubmatch(id); len(matches) == 3 {
		season, _ := strconv.Atoi(matches[1])
		episode, _ := strconv.Atoi(matches[2])
		return matches[0][:9], season, episode
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