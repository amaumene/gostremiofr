// Package handlers implements HTTP request handlers for the Stremio addon.
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

	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/errors"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/internal/services"
	"github.com/gin-gonic/gin"
)

var (
	imdbIDRegex      = regexp.MustCompile(`^tt\d+$`)
	episodeRegex     = regexp.MustCompile(`^tt\d+:(\d+):(\d+)$`)
	tmdbIDRegex      = regexp.MustCompile(`^tmdb:\d+$`)
	tmdbEpisodeRegex = regexp.MustCompile(`^tmdb:(\d+):(\d+):(\d+)$`)
)

type SearchParams struct {
	Query       string
	MediaType   string
	Season      int
	Episode     int
	Year        int
	ID          string
	EpisodeOnly bool
}

type TorrentService interface {
	SearchTorrents(query, category string, season, episode int) (*models.TorrentResults, error)
	SearchTorrentsSpecificEpisode(query, mediaType string, season, episode int) (*models.TorrentResults, error)
}

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

	id, season, episode := parseStreamID(c.Param("id"))
	if id == "" {
		err := errors.NewInvalidIDError(c.Param("id"))
		h.services.Logger.Errorf("[StreamHandler] %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userConfigStruct := config.CreateFromUserData(userConfig, h.config)
	h.configureTorrentServices(userConfigStruct)

	// Get media type from URL path for TMDB IDs
	urlMediaType := c.Param("type")
	mediaType, title, year, originalLanguage, err := h.getMediaInfo(id, urlMediaType)
	if err != nil {
		tmdbErr := errors.NewTMDBError(fmt.Sprintf("failed to get info for %s", id), err)
		h.services.Logger.Errorf("[StreamHandler] %v", tmdbErr)
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}

	h.services.Logger.Infof("[StreamHandler] processing %s request - %s (%s)", mediaType, title, id)

	streams := h.searchStreams(mediaType, title, year, season, episode, apiKey, id, userConfigStruct, originalLanguage)
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

func (h *Handler) monitorTimeout(ctx context.Context, id string) {
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			timeoutErr := errors.NewTimeoutError(fmt.Sprintf("request processing for ID: %s", id))
			h.services.Logger.Errorf("[StreamHandler] %v", timeoutErr)
		}
	}()
}

func (h *Handler) parseUserConfiguration(configuration string) map[string]interface{} {
	var userConfig map[string]interface{}
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	return userConfig
}

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

func (h *Handler) configureTMDBService(userConfig map[string]interface{}) {
	tmdbAPIKey := h.extractAPIKey(userConfig, "TMDB_API_KEY")
	if tmdbAPIKey != "" && h.services.TMDB != nil {
		if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
			tmdb.SetAPIKey(tmdbAPIKey)
		}
	}
}

func (h *Handler) configureTorrentServices(userConfig *config.Config) {
	if h.services.YGG != nil {
		h.services.YGG.SetConfig(userConfig)
	}
	if h.services.Apibay != nil {
		h.services.Apibay.SetConfig(userConfig)
	}
	if h.services.TorrentSorter != nil {
		h.services.TorrentSorter = services.NewTorrentSorter(userConfig)
	}
}

func (h *Handler) getMediaInfo(id, urlMediaType string) (string, string, int, string, error) {
	if strings.HasPrefix(id, "tmdb:") {
		return h.getTMDBInfo(id, urlMediaType)
	} else {
		mediaType, title, _, year, originalLanguage, err := h.services.TMDB.GetIMDBInfo(id)
		return mediaType, title, year, originalLanguage, err
	}
}

func (h *Handler) getTMDBInfo(tmdbID, urlMediaType string) (string, string, int, string, error) {
	// Convert URL media type to TMDB format
	tmdbMediaType := urlMediaType
	if urlMediaType == "series" {
		tmdbMediaType = "tv"
	}
	
	mediaType, title, _, year, originalLanguage, err := h.services.TMDB.GetTMDBInfoWithType(tmdbID, tmdbMediaType)
	return mediaType, title, year, originalLanguage, err
}

func (h *Handler) searchStreams(mediaType, title string, year, season, episode int, apiKey, id string, userConfig *config.Config, originalLanguage string) []models.Stream {
	if mediaType == "movie" {
		return h.searchMovieStreams(title, year, apiKey, userConfig, originalLanguage)
	} else if mediaType == "series" {
		return h.searchSeriesStreams(title, season, episode, apiKey, id, userConfig, originalLanguage)
	}
	return []models.Stream{}
}

func (h *Handler) searchMovieStreams(title string, year int, apiKey string, userConfig *config.Config, originalLanguage string) []models.Stream {
	params := SearchParams{
		Query:     title,
		MediaType: "movie",
		Year:      year,
	}
	results := h.performLanguageBasedSearch(params, originalLanguage)

	h.services.Logger.Infof("[StreamHandler] search results - movies: %d, starting AllDebrid processing", len(results.MovieTorrents))

	return h.processResults(results, apiKey, userConfig, year, 0, 0)
}

// Two-phase search: season packs first, then specific episodes
func (h *Handler) searchSeriesStreams(title string, season, episode int, apiKey, id string, userConfig *config.Config, originalLanguage string) []models.Stream {
	h.services.Logger.Infof("[StreamHandler] starting first search prioritizing complete seasons for s%02d", season)

	params := SearchParams{
		Query:     title,
		MediaType: "series",
		Season:    season,
		Episode:   episode,
		ID:        id,
	}
	results := h.performLanguageBasedSearch(params, originalLanguage)
	streams := h.processResults(results, apiKey, userConfig, 0, season, episode)

	if len(streams) > 0 {
		return streams
	}

	if season > 0 && episode > 0 {
		h.services.Logger.Infof("[StreamHandler] no working streams from season search, trying episode-specific search for s%02de%02d", season, episode)

		params.EpisodeOnly = true
		episodeResults := h.performLanguageBasedSearch(params, originalLanguage)
		streams = h.processResults(episodeResults, apiKey, userConfig, 0, season, episode)

		if len(streams) == 0 {
			h.services.Logger.Infof("[StreamHandler] episode-specific search also failed, trying broader title search")

			broadParams := SearchParams{
				Query:     title,
				MediaType: "series",
			}
			broadResults := h.performLanguageBasedSearch(broadParams, originalLanguage)
			if broadResults != nil && len(broadResults.EpisodeTorrents) > 0 {
				streams = h.processResults(broadResults, apiKey, userConfig, 0, season, episode)
			}
		}
	}

	return streams
}

func (h *Handler) performLanguageBasedSearch(params SearchParams, originalLanguage string) *models.CombinedTorrentResults {
	// Route to YGG if original language is French, otherwise to Apibay
	if originalLanguage == "fr" {
		h.services.Logger.Infof("[StreamHandler] original language is French, routing to YGG only")
		return h.performYGGSearch(params)
	} else {
		h.services.Logger.Infof("[StreamHandler] original language is %s, routing to Apibay only", originalLanguage)
		return h.performApibaySearch(params)
	}
}

func (h *Handler) performYGGSearch(params SearchParams) *models.CombinedTorrentResults {
	combinedResults := models.CombinedTorrentResults{}
	
	var results *models.TorrentResults
	var err error

	if params.EpisodeOnly {
		h.services.Logger.Infof("[StreamHandler] YGG specific episode search - %s s%02de%02d", params.Query, params.Season, params.Episode)
		results, err = h.services.YGG.SearchTorrentsSpecificEpisode(params.Query, params.MediaType, params.Season, params.Episode)
	} else {
		category := params.MediaType
		if params.MediaType == "series" {
			category = "series"
		} else {
			category = "movie"
		}
		h.services.Logger.Infof("[StreamHandler] YGG search - %s (%s)", params.Query, category)
		results, err = h.services.YGG.SearchTorrents(params.Query, category, params.Season, params.Episode)
	}

	if err != nil {
		h.services.Logger.Errorf("[StreamHandler] YGG search failed: %v", err)
		return &combinedResults
	}

	if params.EpisodeOnly && results != nil {
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
	} else {
		var mu sync.Mutex
		aggregateResults(results, &combinedResults, &mu)
	}

	return &combinedResults
}

func (h *Handler) performApibaySearch(params SearchParams) *models.CombinedTorrentResults {
	combinedResults := models.CombinedTorrentResults{}
	
	query := params.Query
	if params.MediaType == "movie" && params.Year > 0 {
		query = fmt.Sprintf("%s %d", params.Query, params.Year)
	}

	var results *models.TorrentResults
	var err error

	if params.EpisodeOnly {
		h.services.Logger.Infof("[StreamHandler] Apibay specific episode search - %s s%02de%02d", query, params.Season, params.Episode)
		results, err = h.services.Apibay.SearchTorrentsSpecificEpisode(query, params.MediaType, params.Season, params.Episode)
	} else {
		h.services.Logger.Infof("[StreamHandler] Apibay search - %s (%s)", query, params.MediaType)
		results, err = h.services.Apibay.SearchTorrents(query, params.MediaType, params.Season, params.Episode)
	}

	if err != nil {
		h.services.Logger.Errorf("[StreamHandler] Apibay search failed: %v", err)
		return &combinedResults
	}

	if params.EpisodeOnly && results != nil {
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
	} else {
		var mu sync.Mutex
		aggregateResults(results, &combinedResults, &mu)
	}

	return &combinedResults
}

func (h *Handler) performParallelSearch(params SearchParams) *models.CombinedTorrentResults {
	var wg sync.WaitGroup
	var mu sync.Mutex
	combinedResults := models.CombinedTorrentResults{}

	searchTimeout := constants.SearchTimeout
	done := make(chan bool)

	wg.Add(constants.TorrentSearchGoroutines)

	// YGG search goroutine
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] YGG search panic recovered: %v", r)
			}
		}()

		var results *models.TorrentResults
		var err error

		if params.EpisodeOnly {
			h.services.Logger.Infof("[StreamHandler] YGG specific episode search - %s s%02de%02d", params.Query, params.Season, params.Episode)
			results, err = h.services.YGG.SearchTorrentsSpecificEpisode(params.Query, params.MediaType, params.Season, params.Episode)
		} else {
			category := params.MediaType
			if params.MediaType == "series" {
				category = "series"
			} else {
				category = "movie"
			}
			h.services.Logger.Infof("[StreamHandler] YGG search - %s (%s)", params.Query, category)
			results, err = h.services.YGG.SearchTorrents(params.Query, category, params.Season, params.Episode)
		}

		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] YGG search failed: %v", err)
			return
		}

		if params.EpisodeOnly && results != nil {
			// For episode-only searches, only add episode torrents
			mu.Lock()
			combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
			mu.Unlock()
		} else {
			aggregateResults(results, &combinedResults, &mu)
		}
	}()

	// Apibay search goroutine
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				h.services.Logger.Errorf("[StreamHandler] Apibay search panic recovered: %v", r)
			}
		}()

		query := params.Query
		if params.MediaType == "movie" && params.Year > 0 {
			query = fmt.Sprintf("%s %d", params.Query, params.Year)
		}

		var results *models.TorrentResults
		var err error

		if params.EpisodeOnly {
			h.services.Logger.Infof("[StreamHandler] Apibay specific episode search - %s s%02de%02d", query, params.Season, params.Episode)
			results, err = h.services.Apibay.SearchTorrentsSpecificEpisode(query, params.MediaType, params.Season, params.Episode)
		} else {
			h.services.Logger.Infof("[StreamHandler] Apibay search - %s (%s)", query, params.MediaType)
			results, err = h.services.Apibay.SearchTorrents(query, params.MediaType, params.Season, params.Episode)
		}

		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] Apibay search failed: %v", err)
			return
		}

		if params.EpisodeOnly && results != nil {
			// For episode-only searches, only add episode torrents
			mu.Lock()
			combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
			mu.Unlock()
		} else {
			aggregateResults(results, &combinedResults, &mu)
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
		h.services.Logger.Errorf("[StreamHandler] search timeout after %v for query: %s", searchTimeout, params.Query)
	}

	h.services.Logger.Infof("[StreamHandler] search completed - movies: %d, series: %d, seasons: %d, episodes: %d",
		len(combinedResults.MovieTorrents), len(combinedResults.CompleteSeriesTorrents),
		len(combinedResults.CompleteSeasonTorrents), len(combinedResults.EpisodeTorrents))

	// Log sample episode torrents for debugging
	for i, torrent := range combinedResults.EpisodeTorrents {
		if i < constants.MaxEpisodeTorrentsToLog {
			h.services.Logger.Infof("[StreamHandler] episode torrent %d: %s (%s)", i+1, torrent.Title, torrent.Source)
		}
	}

	return &combinedResults
}




func (h *Handler) processResults(results *models.CombinedTorrentResults, apiKey string, userConfig *config.Config, year int, targetSeason, targetEpisode int) []models.Stream {
	h.services.Logger.Infof("[StreamHandler] processResults started - movies: %d, episodes: %d, seasons: %d, complete series: %d",
		len(results.MovieTorrents), len(results.EpisodeTorrents), len(results.CompleteSeasonTorrents), len(results.CompleteSeriesTorrents))

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

	var allTorrents []models.TorrentInfo

	if targetSeason > 0 && targetEpisode > 0 {
		allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
		allTorrents = append(allTorrents, results.EpisodeTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
	} else if targetSeason > 0 && targetEpisode == 0 {
		allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
		allTorrents = append(allTorrents, results.EpisodeTorrents...)
	} else {
		allTorrents = append(allTorrents, results.MovieTorrents...)
		allTorrents = append(allTorrents, results.EpisodeTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
		allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
	}

	h.services.Logger.Infof("[StreamHandler] processing %d torrents in priority order", len(allTorrents))

	// Sort all torrents by priority before sequential processing
	sorter := h.services.TorrentSorter
	if sorter != nil {
		sorter.SortTorrents(allTorrents)
		h.services.Logger.Infof("[StreamHandler] sorted torrents by priority (resolution, size)")
	}

	return h.processSequentialTorrents(allTorrents, apiKey, userConfig, targetSeason, targetEpisode)
}

// processSequentialTorrents processes torrents one by one until a working stream is found
func (h *Handler) processSequentialTorrents(torrents []models.TorrentInfo, apiKey string, userConfig *config.Config, targetSeason, targetEpisode int) []models.Stream {
	if len(torrents) == 0 {
		h.services.Logger.Infof("[StreamHandler] no torrents to process")
		return []models.Stream{}
	}

	h.services.Logger.Infof("[StreamHandler] processing %d torrents sequentially", len(torrents))

	for i, torrent := range torrents {
		h.services.Logger.Infof("[StreamHandler] trying torrent %d/%d: %s (source: %s)", i+1, len(torrents), torrent.Title, torrent.Source)

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

		h.services.Logger.Infof("[StreamHandler] uploading magnet: %s", torrent.Title)
		err := h.services.AllDebrid.UploadMagnet(hash, torrent.Title, apiKey)
		if err != nil {
			h.services.Logger.Errorf("[StreamHandler] failed to upload magnet %s: %v", torrent.Title, err)
			continue
		}

		var isReady bool
		var readyMagnet *models.ProcessedMagnet

		for attempt := 1; attempt <= constants.MaxMagnetCheckAttempts; attempt++ {
			h.services.Logger.Infof("[StreamHandler] checking magnet status - attempt %d/2", attempt)

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

func (h *Handler) processSingleReadyMagnet(magnet *models.ProcessedMagnet, torrent models.TorrentInfo, targetSeason, targetEpisode int, apiKey string) *models.Stream {
	isSeasonPack := h.isSeasonPack(torrent.Title)

	if targetSeason > 0 && targetEpisode > 0 {
		if isSeasonPack {
			h.services.Logger.Infof("[StreamHandler] processing season pack for specific episode s%02de%02d", targetSeason, targetEpisode)
		} else {
			h.services.Logger.Infof("[StreamHandler] processing episode torrent for s%02de%02d", targetSeason, targetEpisode)
		}

		if file, found := h.findEpisodeFile(magnet.Links, targetSeason, targetEpisode); found {
			h.services.Logger.Infof("[StreamHandler] found target episode file")
			return h.createStreamFromFile(file, torrent, apiKey)
		}

		if isSeasonPack {
			h.services.Logger.Warnf("[StreamHandler] target episode s%02de%02d not found in season pack, using largest file", targetSeason, targetEpisode)
		} else {
			h.services.Logger.Warnf("[StreamHandler] target episode s%02de%02d not found in episode torrent", targetSeason, targetEpisode)
			return nil
		}
	}

	if targetSeason > 0 && targetEpisode == 0 && isSeasonPack {
		h.services.Logger.Infof("[StreamHandler] processing complete season pack for season %d", targetSeason)
	} else if targetSeason == 0 && targetEpisode == 0 {
		h.services.Logger.Infof("[StreamHandler] processing movie torrent, finding largest file")
	} else {
		h.services.Logger.Infof("[StreamHandler] using largest file as fallback")
	}

	if file, found := findLargestFile(magnet.Links); found {
		return h.createStreamFromFile(file, torrent, apiKey)
	}

	h.services.Logger.Warnf("[StreamHandler] no valid files found in magnet")
	return nil
}

func (h *Handler) isSeasonPack(title string) bool {
	titleLower := strings.ToLower(title)
	return strings.Contains(titleLower, "complete") ||
		strings.Contains(titleLower, "season") ||
		strings.Contains(titleLower, "saison")
}

func parseFileInfo(linkObj map[string]interface{}) string {
	info := ""

	if size, ok := linkObj["size"].(float64); ok {
		sizeGB := size / constants.BytesToGB
		info += fmt.Sprintf("💾 %.2f GB", sizeGB)
	}

	if filename, ok := linkObj["filename"].(string); ok {
		if info != "" {
			info += " • "
		}
		info += fmt.Sprintf("📄 %s", filename)
	}

	return info
}

func parseStreamID(id string) (string, int, int) {
	id = strings.TrimSuffix(id, ".json")

	// Handle IMDB episode format: tt123456:1:1
	if episodeRegex.MatchString(id) {
		matches := episodeRegex.FindStringSubmatch(id)
		if len(matches) == 3 {
			imdbID := strings.Split(id, ":")[0]
			season, _ := strconv.Atoi(matches[1])
			episode, _ := strconv.Atoi(matches[2])
			return imdbID, season, episode
		}
	}

	// Handle TMDB episode format: tmdb:123456:1:1
	if tmdbEpisodeRegex.MatchString(id) {
		matches := tmdbEpisodeRegex.FindStringSubmatch(id)
		if len(matches) == 4 {
			tmdbID := fmt.Sprintf("tmdb:%s", matches[1])
			season, _ := strconv.Atoi(matches[2])
			episode, _ := strconv.Atoi(matches[3])
			return tmdbID, season, episode
		}
	}

	// Handle IMDB movie format: tt123456
	if imdbIDRegex.MatchString(id) {
		return id, 0, 0
	}

	// Handle TMDB movie format: tmdb:123456
	if tmdbIDRegex.MatchString(id) {
		return id, 0, 0
	}

	return "", 0, 0
}

func (h *Handler) matchesYear(title string, year int) bool {
	if year == 0 {
		return true
	}

	yearStr := fmt.Sprintf("%d", year)
	return strings.Contains(title, yearStr)
}

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

func aggregateResults(results *models.TorrentResults, combined *models.CombinedTorrentResults, mu *sync.Mutex) {
	if results == nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	combined.MovieTorrents = append(combined.MovieTorrents, results.MovieTorrents...)
	combined.CompleteSeriesTorrents = append(combined.CompleteSeriesTorrents, results.CompleteSeriesTorrents...)
	combined.CompleteSeasonTorrents = append(combined.CompleteSeasonTorrents, results.CompleteSeasonTorrents...)
	combined.EpisodeTorrents = append(combined.EpisodeTorrents, results.EpisodeTorrents...)
}

func findLargestFile(links []interface{}) (map[string]interface{}, bool) {
	var largestFile map[string]interface{}
	var largestSize float64

	for _, link := range links {
		if linkObj, ok := link.(map[string]interface{}); ok {
			if size, ok := linkObj["size"].(float64); ok {
				if size > largestSize {
					largestSize = size
					largestFile = linkObj
				}
			}
		}
	}

	return largestFile, largestFile != nil
}

func (h *Handler) findEpisodeFile(links []interface{}, targetSeason, targetEpisode int) (map[string]interface{}, bool) {
	var matchingFiles []map[string]interface{}
	
	for _, link := range links {
		if linkObj, ok := link.(map[string]interface{}); ok {
			if filename, ok := linkObj["filename"].(string); ok {
				season, episode := h.parseEpisodeFromFilename(filename)
				if season == targetSeason && episode == targetEpisode {
					matchingFiles = append(matchingFiles, linkObj)
				}
			}
		}
	}
	
	if len(matchingFiles) == 0 {
		return nil, false
	}
	
	// Find the largest file among matches
	var largestFile map[string]interface{}
	var largestSize float64
	
	for _, file := range matchingFiles {
		if size, ok := file["size"].(float64); ok {
			if size > largestSize {
				largestSize = size
				largestFile = file
			}
		}
	}
	
	return largestFile, largestFile != nil
}

func (h *Handler) createStreamFromFile(file map[string]interface{}, torrent models.TorrentInfo, apiKey string) *models.Stream {
	linkStr, ok := file["link"].(string)
	if !ok {
		return nil
	}

	directURL, err := h.services.AllDebrid.UnlockLink(linkStr, apiKey)
	if err != nil {
		h.services.Logger.Errorf("[StreamHandler] failed to unlock link: %v", err)
		return nil
	}

	streamTitle := fmt.Sprintf("%s\n%s", torrent.Title, parseFileInfo(file))
	return &models.Stream{
		Name:  torrent.Source,
		Title: streamTitle,
		URL:   directURL,
	}
}
