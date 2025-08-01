// Package handlers implements HTTP request handlers for the Stremio addon.
package handlers

import (
	"context"
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

	req, err := h.validateStreamRequest(c)
	if err != nil {
		c.JSON(http.StatusOK, models.StreamResponse{Streams: []models.Stream{}})
		return
	}

	streams := h.searchStreams(req.mediaType, req.title, req.year, req.season, req.episode, 
		req.apiKey, req.id, req.config, req.originalLanguage)
	c.JSON(http.StatusOK, models.StreamResponse{Streams: streams})
}

type streamRequest struct {
	id               string
	season           int
	episode          int
	apiKey           string
	mediaType        string
	title            string
	year             int
	originalLanguage string
	config           *config.Config
}

func (h *Handler) validateStreamRequest(c *gin.Context) (*streamRequest, error) {
	userConfig := decodeUserConfig(c.Param("configuration"))
	apiKey := h.extractAllDebridKey(userConfig)
	if apiKey == "" {
		h.services.Logger.Warnf("missing AllDebrid API key")
		return nil, errors.NewAPIKeyMissingError("AllDebrid")
	}

	h.configureTMDBService(userConfig)

	id, season, episode := extractMediaIdentifiers(c.Param("id"))
	if id == "" {
		h.services.Logger.Errorf("invalid stream ID: %s", c.Param("id"))
		return nil, errors.NewInvalidIDError(c.Param("id"))
	}

	userConfigStruct := config.CreateFromUserData(userConfig, h.config)
	h.configureTorrentServices(userConfigStruct)

	return h.buildStreamRequest(c, id, season, episode, apiKey, userConfigStruct)
}

func (h *Handler) buildStreamRequest(c *gin.Context, id string, season, episode int, apiKey string, userConfig *config.Config) (*streamRequest, error) {
	mediaType, title, year, originalLanguage, err := h.getMediaInfo(id, c.Param("type"))
	if err != nil {
		h.services.Logger.Debugf("TMDB lookup failed: %v", err)
		return nil, err
	}

	h.services.Logger.Infof("processing %s: %s", mediaType, title)

	return &streamRequest{
		id:               id,
		season:           season,
		episode:          episode,
		apiKey:           apiKey,
		mediaType:        mediaType,
		title:            title,
		year:             year,
		originalLanguage: originalLanguage,
		config:           userConfig,
	}, nil
}

func (h *Handler) monitorTimeout(ctx context.Context, id string) {
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			timeoutErr := errors.NewTimeoutError(fmt.Sprintf("request processing for ID: %s", id))
			h.services.Logger.Errorf("request timeout: %v", timeoutErr)
		}
	}()
}

func (h *Handler) extractAllDebridKey(userConfig map[string]interface{}) string {
	if val, ok := userConfig["API_KEY_ALLDEBRID"]; ok {
		if key, ok := val.(string); ok && key != "" {
			return key
		}
	}
	if h.config != nil {
		return h.config.APIKeyAllDebrid
	}
	return ""
}

func (h *Handler) extractTMDBKey(userConfig map[string]interface{}) string {
	if val, ok := userConfig["TMDB_API_KEY"]; ok {
		if key, ok := val.(string); ok && key != "" {
			return key
		}
	}
	if h.config != nil {
		return h.config.TMDBAPIKey
	}
	return ""
}

func (h *Handler) configureTMDBService(userConfig map[string]interface{}) {
	tmdbAPIKey := h.extractTMDBKey(userConfig)
	if tmdbAPIKey == "" {
		return
	}
	if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
		tmdb.SetAPIKey(tmdbAPIKey)
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

	h.services.Logger.Debugf("found %d movie torrents", len(results.MovieTorrents))

	return h.processResults(results, apiKey, userConfig, year, 0, 0)
}

// Two-phase search: season packs first, then specific episodes
func (h *Handler) searchSeriesStreams(title string, season, episode int, apiKey, id string, userConfig *config.Config, originalLanguage string) []models.Stream {
	h.services.Logger.Debugf("searching for season %d", season)

	params := SearchParams{
		Query:     title,
		MediaType: "series",
		Season:    season,
		Episode:   episode,
		ID:        id,
	}

	// Phase 1: Season pack search
	results := h.performLanguageBasedSearch(params, originalLanguage)
	streams := h.processResults(results, apiKey, userConfig, 0, season, episode)

	if len(streams) > 0 {
		return streams
	}

	// Phase 2: Episode-specific search if needed
	if season > 0 && episode > 0 {
		return h.searchSpecificEpisode(params, apiKey, userConfig, originalLanguage, title, season, episode)
	}

	return streams
}

func (h *Handler) searchSpecificEpisode(params SearchParams, apiKey string, userConfig *config.Config, originalLanguage, title string, season, episode int) []models.Stream {
	h.services.Logger.Debugf("trying episode-specific search: s%02de%02d", season, episode)

	params.EpisodeOnly = true
	episodeResults := h.performLanguageBasedSearch(params, originalLanguage)
	streams := h.processResults(episodeResults, apiKey, userConfig, 0, season, episode)

	if len(streams) == 0 {
		h.services.Logger.Debugf("falling back to title-only search")

		broadParams := SearchParams{
			Query:     title,
			MediaType: "series",
		}
		broadResults := h.performLanguageBasedSearch(broadParams, originalLanguage)
		if broadResults != nil && len(broadResults.EpisodeTorrents) > 0 {
			streams = h.processResults(broadResults, apiKey, userConfig, 0, season, episode)
		}
	}

	return streams
}

func (h *Handler) performLanguageBasedSearch(params SearchParams, originalLanguage string) *models.CombinedTorrentResults {
	// Route to YGG if original language is French, otherwise to Apibay
	if originalLanguage == "fr" {
		h.services.Logger.Debugf("French content: using YGG")
		return h.performYGGSearch(params)
	} else {
		h.services.Logger.Debugf("non-French content: using Apibay")
		return h.performApibaySearch(params)
	}
}

func (h *Handler) performYGGSearch(params SearchParams) *models.CombinedTorrentResults {
	combinedResults := models.CombinedTorrentResults{}

	results, err := h.executeYGGSearchQuery(params)
	if err != nil {
		h.services.Logger.Errorf("YGG search failed: %v", err)
		return &combinedResults
	}

	h.aggregateYGGResults(results, &combinedResults, params.EpisodeOnly)
	return &combinedResults
}

func (h *Handler) executeYGGSearchQuery(params SearchParams) (*models.TorrentResults, error) {
	if params.EpisodeOnly {
		h.services.Logger.Debugf("YGG episode search: %s s%02de%02d", params.Query, params.Season, params.Episode)
		return h.services.YGG.SearchTorrentsSpecificEpisode(params.Query, params.MediaType, params.Season, params.Episode)
	}

	category := params.MediaType
	if params.MediaType == "series" {
		category = "series"
	} else {
		category = "movie"
	}
	h.services.Logger.Debugf("YGG search: %s (%s)", params.Query, category)
	return h.services.YGG.SearchTorrents(params.Query, category, params.Season, params.Episode)
}

func (h *Handler) aggregateYGGResults(results *models.TorrentResults, combinedResults *models.CombinedTorrentResults, episodeOnly bool) {
	if results == nil {
		return
	}

	if episodeOnly {
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
	} else {
		var mu sync.Mutex
		aggregateResults(results, combinedResults, &mu)
	}
}

func (h *Handler) performApibaySearch(params SearchParams) *models.CombinedTorrentResults {
	combinedResults := models.CombinedTorrentResults{}
	query := h.buildApibayQuery(params)

	results, err := h.executeApibaySearchQuery(params, query)
	if err != nil {
		h.services.Logger.Errorf("Apibay search failed: %v", err)
		return &combinedResults
	}

	h.aggregateApibayResults(results, &combinedResults, params.EpisodeOnly)
	return &combinedResults
}

func (h *Handler) executeApibaySearchQuery(params SearchParams, query string) (*models.TorrentResults, error) {
	if params.EpisodeOnly {
		h.services.Logger.Debugf("Apibay episode search: %s s%02de%02d", query, params.Season, params.Episode)
		return h.services.Apibay.SearchTorrentsSpecificEpisode(query, params.MediaType, params.Season, params.Episode)
	}

	h.services.Logger.Debugf("Apibay search: %s (%s)", query, params.MediaType)
	return h.services.Apibay.SearchTorrents(query, params.MediaType, params.Season, params.Episode)
}

func (h *Handler) aggregateApibayResults(results *models.TorrentResults, combinedResults *models.CombinedTorrentResults, episodeOnly bool) {
	if results == nil {
		return
	}

	if episodeOnly {
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
	} else {
		var mu sync.Mutex
		aggregateResults(results, combinedResults, &mu)
	}
}

func (h *Handler) performParallelSearch(params SearchParams) *models.CombinedTorrentResults {
	var wg sync.WaitGroup
	var mu sync.Mutex
	combinedResults := models.CombinedTorrentResults{}

	wg.Add(constants.TorrentSearchGoroutines)
	go h.searchYGGAsync(params, &combinedResults, &mu, &wg)
	go h.searchApibayAsync(params, &combinedResults, &mu, &wg)

	h.waitForSearchCompletion(&wg, params.Query)
	h.logSearchResults(&combinedResults)
	return &combinedResults
}

func (h *Handler) waitForSearchCompletion(wg *sync.WaitGroup, query string) {
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		h.services.Logger.Debugf("all searches completed")
	case <-time.After(constants.SearchTimeout):
		h.services.Logger.Errorf("search timeout after %v: %s", constants.SearchTimeout, query)
	}
}

func (h *Handler) searchYGGAsync(params SearchParams, combinedResults *models.CombinedTorrentResults, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	defer h.recoverFromPanic("YGG")

	results, err := h.executeYGGSearch(params)
	if err != nil {
		h.services.Logger.Errorf("YGG search failed: %v", err)
		return
	}

	h.aggregateSearchResults(results, combinedResults, mu, params.EpisodeOnly)
}

func (h *Handler) searchApibayAsync(params SearchParams, combinedResults *models.CombinedTorrentResults, mu *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	defer h.recoverFromPanic("Apibay")

	query := h.buildApibayQuery(params)
	results, err := h.executeApibaySearch(params, query)
	if err != nil {
		h.services.Logger.Errorf("Apibay search failed: %v", err)
		return
	}

	h.aggregateSearchResults(results, combinedResults, mu, params.EpisodeOnly)
}

func (h *Handler) recoverFromPanic(source string) {
	if r := recover(); r != nil {
		h.services.Logger.Errorf("%s search panic: %v", source, r)
	}
}

func (h *Handler) executeYGGSearch(params SearchParams) (*models.TorrentResults, error) {
	if params.EpisodeOnly {
		h.services.Logger.Debugf("YGG episode search: %s s%02de%02d", params.Query, params.Season, params.Episode)
		return h.services.YGG.SearchTorrentsSpecificEpisode(params.Query, params.MediaType, params.Season, params.Episode)
	}

	category := params.MediaType
	h.services.Logger.Debugf("YGG search: %s (%s)", params.Query, category)
	return h.services.YGG.SearchTorrents(params.Query, category, params.Season, params.Episode)
}

func (h *Handler) buildApibayQuery(params SearchParams) string {
	if params.MediaType == "movie" && params.Year > 0 {
		return fmt.Sprintf("%s %d", params.Query, params.Year)
	}
	return params.Query
}

func (h *Handler) executeApibaySearch(params SearchParams, query string) (*models.TorrentResults, error) {
	if params.EpisodeOnly {
		h.services.Logger.Debugf("Apibay episode search: %s s%02de%02d", query, params.Season, params.Episode)
		return h.services.Apibay.SearchTorrentsSpecificEpisode(query, params.MediaType, params.Season, params.Episode)
	}

	h.services.Logger.Debugf("Apibay search: %s (%s)", query, params.MediaType)
	return h.services.Apibay.SearchTorrents(query, params.MediaType, params.Season, params.Episode)
}

func (h *Handler) aggregateSearchResults(results *models.TorrentResults, combinedResults *models.CombinedTorrentResults, mu *sync.Mutex, episodeOnly bool) {
	if results == nil {
		return
	}

	if episodeOnly {
		mu.Lock()
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, results.EpisodeTorrents...)
		mu.Unlock()
	} else {
		aggregateResults(results, combinedResults, mu)
	}
}

func (h *Handler) logSearchResults(results *models.CombinedTorrentResults) {
	h.services.Logger.Debugf("found %d torrents", 
		len(results.MovieTorrents) + len(results.CompleteSeriesTorrents) + 
		len(results.CompleteSeasonTorrents) + len(results.EpisodeTorrents))

}

func (h *Handler) processResults(results *models.CombinedTorrentResults, apiKey string, userConfig *config.Config, year int, targetSeason, targetEpisode int) []models.Stream {
	h.services.Logger.Debugf("processing %d results", h.countResults(results))

	if year > 0 && len(results.MovieTorrents) > 0 {
		results.MovieTorrents = h.filterMoviesByYear(results.MovieTorrents, year)
	}

	allTorrents := h.prioritizeTorrents(results, targetSeason, targetEpisode)
	h.services.Logger.Infof(" processing %d torrents in priority order", len(allTorrents))

	h.sortTorrents(allTorrents)
	return h.processSequentialTorrents(allTorrents, apiKey, userConfig, targetSeason, targetEpisode)
}

func (h *Handler) countResults(results *models.CombinedTorrentResults) int {
	return len(results.MovieTorrents) + len(results.EpisodeTorrents) + 
		len(results.CompleteSeasonTorrents) + len(results.CompleteSeriesTorrents)
}

func (h *Handler) filterMoviesByYear(movies []models.TorrentInfo, year int) []models.TorrentInfo {
	var filteredMovies []models.TorrentInfo
	for _, torrent := range movies {
		if h.matchesYear(torrent.Title, year) {
			filteredMovies = append(filteredMovies, torrent)
		} else {
			h.services.Logger.Debugf(" torrent filtered by year - title: %s (expected: %d)", torrent.Title, year)
		}
	}
	h.services.Logger.Infof(" year filtering: %d -> %d movie torrents", len(movies), len(filteredMovies))
	return filteredMovies
}

func (h *Handler) prioritizeTorrents(results *models.CombinedTorrentResults, targetSeason, targetEpisode int) []models.TorrentInfo {
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

	return allTorrents
}

func (h *Handler) sortTorrents(torrents []models.TorrentInfo) {
	sorter := h.services.TorrentSorter
	if sorter != nil {
		sorter.SortTorrents(torrents)
		h.services.Logger.Infof(" sorted torrents by priority (resolution, size)")
	}
}

// processSequentialTorrents processes torrents one by one until a working stream is found
func (h *Handler) processSequentialTorrents(torrents []models.TorrentInfo, apiKey string, userConfig *config.Config, targetSeason, targetEpisode int) []models.Stream {
	if len(torrents) == 0 {
		h.services.Logger.Infof(" no torrents to process")
		return []models.Stream{}
	}

	h.services.Logger.Infof(" processing %d torrents sequentially", len(torrents))

	for i, torrent := range torrents {
		stream := h.processSingleTorrent(torrent, i+1, len(torrents), apiKey, targetSeason, targetEpisode)
		if stream != nil {
			h.services.Logger.Infof(" successfully created stream from torrent: %s", torrent.Title)
			return []models.Stream{*stream}
		}
	}

	h.services.Logger.Infof(" no working torrents found")
	return []models.Stream{}
}

func (h *Handler) processSingleTorrent(torrent models.TorrentInfo, current, total int, apiKey string, targetSeason, targetEpisode int) *models.Stream {
	h.services.Logger.Infof(" trying torrent %d/%d: %s (source: %s)", current, total, torrent.Title, torrent.Source)

	hash, err := h.getTorrentHash(torrent)
	if err != nil {
		return nil
	}

	if hash == "" {
		h.services.Logger.Warnf(" skipping torrent without hash: %s", torrent.Title)
		return nil
	}

	if err := h.uploadTorrent(hash, torrent.Title, apiKey); err != nil {
		return nil
	}

	readyMagnet := h.waitForMagnetReady(hash, torrent, apiKey)
	if readyMagnet == nil {
		return nil
	}

	stream := h.processSingleReadyMagnet(readyMagnet, torrent, targetSeason, targetEpisode, apiKey)
	if stream == nil {
		h.services.Logger.Warnf(" failed to create stream from ready magnet: %s", torrent.Title)
	}
	return stream
}

func (h *Handler) getTorrentHash(torrent models.TorrentInfo) (string, error) {
	hash := torrent.Hash
	if hash != "" || torrent.Source != "YGG" {
		return hash, nil
	}

	h.services.Logger.Infof(" fetching hash for YGG torrent: %s", torrent.Title)
	fetchedHash, err := h.services.YGG.GetTorrentHash(torrent.ID)
	if err != nil {
		h.services.Logger.Errorf(" failed to fetch hash for torrent %s: %v", torrent.Title, err)
		return "", err
	}
	if fetchedHash == "" {
		h.services.Logger.Warnf(" torrent %s returned empty hash", torrent.Title)
		return "", fmt.Errorf("empty hash")
	}
	return fetchedHash, nil
}

func (h *Handler) uploadTorrent(hash, title, apiKey string) error {
	h.services.Logger.Infof(" uploading magnet: %s", title)
	err := h.services.AllDebrid.UploadMagnet(hash, title, apiKey)
	if err != nil {
		h.services.Logger.Errorf(" failed to upload magnet %s: %v", title, err)
	}
	return err
}

func (h *Handler) isMagnetReady(magnets []models.ProcessedMagnet) bool {
	return len(magnets) > 0 && magnets[0].Ready && len(magnets[0].Links) > 0
}

func (h *Handler) waitForMagnetReady(hash string, torrent models.TorrentInfo, apiKey string) *models.ProcessedMagnet {
	for attempt := 1; attempt <= constants.MaxMagnetCheckAttempts; attempt++ {
		h.services.Logger.Infof(" checking magnet status - attempt %d/2", attempt)

		magnetInfo := models.MagnetInfo{Hash: hash, Title: torrent.Title, Source: torrent.Source}
		processedMagnets, err := h.services.AllDebrid.CheckMagnets([]models.MagnetInfo{magnetInfo}, apiKey)
		if err != nil {
			h.services.Logger.Errorf(" CheckMagnets failed: %v", err)
			if attempt < constants.MaxMagnetCheckAttempts {
				time.Sleep(constants.MagnetCheckRetryDelay)
				continue
			}
			break
		}

		if h.isMagnetReady(processedMagnets) {
			h.services.Logger.Infof(" magnet is ready with %d links!", len(processedMagnets[0].Links))
			return &processedMagnets[0]
		}

		if attempt < constants.MaxMagnetCheckAttempts {
			h.services.Logger.Infof(" magnet not ready yet, waiting before retry")
			time.Sleep(constants.MagnetReadyRetryDelay)
		}
	}

	h.services.Logger.Infof(" magnet not ready after %d attempts, trying next", constants.MaxMagnetCheckAttempts)
	return nil
}

func (h *Handler) processSingleReadyMagnet(magnet *models.ProcessedMagnet, torrent models.TorrentInfo, targetSeason, targetEpisode int, apiKey string) *models.Stream {
	isSeasonPack := h.isSeasonPack(torrent.Title)

	if targetSeason > 0 && targetEpisode > 0 {
		return h.processEpisodeFromMagnet(magnet, torrent, targetSeason, targetEpisode, isSeasonPack, apiKey)
	}

	return h.processLargestFile(magnet, torrent, targetSeason, targetEpisode, isSeasonPack, apiKey)
}

func (h *Handler) processEpisodeFromMagnet(magnet *models.ProcessedMagnet, torrent models.TorrentInfo, targetSeason, targetEpisode int, isSeasonPack bool, apiKey string) *models.Stream {
	if isSeasonPack {
		h.services.Logger.Infof(" processing season pack for specific episode s%02de%02d", targetSeason, targetEpisode)
	} else {
		h.services.Logger.Infof(" processing episode torrent for s%02de%02d", targetSeason, targetEpisode)
	}

	if file, found := h.findEpisodeFile(magnet.Links, targetSeason, targetEpisode); found {
		h.services.Logger.Infof(" found target episode file")
		return h.createStreamFromFile(file, torrent, apiKey)
	}

	if isSeasonPack {
		h.services.Logger.Warnf(" target episode s%02de%02d not found in season pack, using largest file", targetSeason, targetEpisode)
		return h.processLargestFile(magnet, torrent, targetSeason, targetEpisode, isSeasonPack, apiKey)
	}

	h.services.Logger.Warnf(" target episode s%02de%02d not found in episode torrent", targetSeason, targetEpisode)
	return nil
}

func (h *Handler) processLargestFile(magnet *models.ProcessedMagnet, torrent models.TorrentInfo, targetSeason, targetEpisode int, isSeasonPack bool, apiKey string) *models.Stream {
	if targetSeason > 0 && targetEpisode == 0 && isSeasonPack {
		h.services.Logger.Infof(" processing complete season pack for season %d", targetSeason)
	} else if targetSeason == 0 && targetEpisode == 0 {
		h.services.Logger.Infof(" processing movie torrent, finding largest file")
	} else {
		h.services.Logger.Infof(" using largest file as fallback")
	}

	if file, found := findLargestFile(magnet.Links); found {
		return h.createStreamFromFile(file, torrent, apiKey)
	}

	h.services.Logger.Warnf(" no valid files found in magnet")
	return nil
}

func (h *Handler) isSeasonPack(title string) bool {
	titleLower := strings.ToLower(title)
	return strings.Contains(titleLower, "complete") ||
		strings.Contains(titleLower, "season") ||
		strings.Contains(titleLower, "saison")
}

func formatFileInfoString(linkObj map[string]interface{}) string {
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

func extractMediaIdentifiers(id string) (string, int, int) {
	id = strings.TrimSuffix(id, ".json")

	// Try IMDB episode format
	if mediaID, season, episode, ok := parseIMDBEpisodeFormat(id); ok {
		return mediaID, season, episode
	}

	// Try TMDB episode format
	if mediaID, season, episode, ok := parseTMDBEpisodeFormat(id); ok {
		return mediaID, season, episode
	}

	// Check if it's a movie format
	if isMovieFormat(id) {
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

func (h *Handler) extractSeasonEpisodeFromFilename(filename string) (int, int) {
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
	matchingFiles := h.findMatchingEpisodeFiles(links, targetSeason, targetEpisode)
	if len(matchingFiles) == 0 {
		return nil, false
	}

	return h.selectLargestMatchingFile(matchingFiles)
}

func (h *Handler) findMatchingEpisodeFiles(links []interface{}, targetSeason, targetEpisode int) []map[string]interface{} {
	var matchingFiles []map[string]interface{}

	for _, link := range links {
		if linkObj, ok := link.(map[string]interface{}); ok {
			if filename, ok := linkObj["filename"].(string); ok {
				season, episode := h.extractSeasonEpisodeFromFilename(filename)
				if season == targetSeason && episode == targetEpisode {
					matchingFiles = append(matchingFiles, linkObj)
				}
			}
		}
	}

	return matchingFiles
}

func (h *Handler) selectLargestMatchingFile(matchingFiles []map[string]interface{}) (map[string]interface{}, bool) {
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
		h.services.Logger.Errorf(" failed to unlock link: %v", err)
		return nil
	}

	streamTitle := fmt.Sprintf("%s\n%s", torrent.Title, formatFileInfoString(file))
	return &models.Stream{
		Name:  torrent.Source,
		Title: streamTitle,
		URL:   directURL,
	}
}
