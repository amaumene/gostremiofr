package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/httputil"
	"github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/amaumene/gostremiofr/pkg/ratelimiter"
	"github.com/amaumene/gostremiofr/pkg/security"
)

type TMDB struct {
	apiKey      string
	cache       *cache.LRUCache
	db          database.Database
	rateLimiter *ratelimiter.TokenBucket
	httpClient  *http.Client
	logger      logger.Logger
	validator   *security.APIKeyValidator
}

func NewTMDB(apiKey string, cache *cache.LRUCache) *TMDB {
	validator := security.NewAPIKeyValidator()

	// Sanitize the API key if provided
	sanitizedKey := ""
	if apiKey != "" {
		sanitizedKey = validator.SanitizeAPIKey(apiKey)
	}

	return &TMDB{
		apiKey:      sanitizedKey,
		cache:       cache,
		rateLimiter: ratelimiter.NewTokenBucket(constants.TMDBRateLimit, constants.TMDBRateBurst),
		httpClient:  httputil.NewHTTPClient(10 * time.Second),
		logger:      logger.New(),
		validator:   validator,
	}
}

func (t *TMDB) SetDB(db database.Database) {
	t.db = db
}

func (t *TMDB) SetAPIKey(apiKey string) {
	// Sanitize and validate the API key
	sanitizedKey := t.validator.SanitizeAPIKey(apiKey)
	if apiKey != "" && !t.validator.IsValidTMDBKey(sanitizedKey) {
		t.logger.Errorf("failed to set API key: invalid format (key: %s)", t.validator.MaskAPIKey(sanitizedKey))
		return
	}
	t.apiKey = sanitizedKey
}

func (t *TMDB) GetIMDBInfo(imdbID string) (string, string, string, int, string, error) {
	cacheKey := fmt.Sprintf("tmdb:%s", imdbID)

	if result := t.checkMemoryCache(cacheKey); result != nil {
		return result.Type, result.Title, result.Title, result.Year, result.OriginalLanguage, nil
	}

	if result := t.checkDatabaseCache(imdbID, cacheKey); result != nil {
		return result.Type, result.Title, result.Title, result.Year, result.OriginalLanguage, nil
	}

	tmdbResp, err := t.fetchIMDBData(imdbID)
	if err != nil {
		return "", "", "", 0, "", err
	}

	tmdbData, err := t.processIMDBResponse(tmdbResp, imdbID)
	if err != nil {
		return "", "", "", 0, "", err
	}

	t.cache.Set(cacheKey, tmdbData)
	t.storeTMDBCache(imdbID, tmdbData)

	return tmdbData.Type, tmdbData.Title, tmdbData.Title, tmdbData.Year, tmdbData.OriginalLanguage, nil
}

// GetTMDBInfo fetches info for a TMDB ID directly
func (t *TMDB) GetTMDBInfo(tmdbID string) (string, string, string, int, string, error) {
	id, err := t.extractTMDBNumericID(tmdbID)
	if err != nil {
		return "", "", "", 0, "", err
	}

	cacheKey := fmt.Sprintf("tmdb:direct:%s", tmdbID)
	
	if result := t.checkMemoryCache(cacheKey); result != nil {
		return result.Type, result.Title, result.Title, result.Year, result.OriginalLanguage, nil
	}

	if result := t.checkDatabaseCache(tmdbID, cacheKey); result != nil {
		return result.Type, result.Title, result.Title, result.Year, result.OriginalLanguage, nil
	}

	if err := t.validateAPIKey(); err != nil {
		return "", "", "", 0, "", err
	}

	t.rateLimiter.Wait()

	mediaType, title, originalLanguage, year, err := t.tryFetchTMDBData(id, tmdbID)
	if err != nil {
		return "", "", "", 0, "", err
	}

	t.cacheTMDBResult(cacheKey, tmdbID, mediaType, title, year, originalLanguage)
	return mediaType, title, title, year, originalLanguage, nil
}

func (t *TMDB) extractTMDBNumericID(tmdbID string) (string, error) {
	parts := strings.Split(tmdbID, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid TMDB ID format: %s", tmdbID)
	}
	return parts[1], nil
}

func (t *TMDB) tryFetchTMDBData(id, tmdbID string) (string, string, string, int, error) {
	// Try movie first
	if mediaType, title, originalLanguage, year, err := t.tryFetchMovie(id, tmdbID); err == nil {
		return mediaType, title, originalLanguage, year, nil
	}

	// Try TV if movie fails
	return t.tryFetchTV(id, tmdbID)
}

func (t *TMDB) tryFetchMovie(id, tmdbID string) (string, string, string, int, error) {
	movieURL := fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s", id, t.apiKey)
	t.logger.Debugf("trying movie endpoint for TMDB ID %s", tmdbID)

	resp, err := t.httpClient.Get(movieURL)
	if err != nil {
		return "", "", "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, fmt.Errorf("not a movie")
	}

	var movie models.TMDBMovieDetails
	if err := json.NewDecoder(resp.Body).Decode(&movie); err != nil {
		return "", "", "", 0, err
	}

	year := 0
	if movie.ReleaseDate != "" && len(movie.ReleaseDate) >= 4 {
		if parsedYear, err := strconv.Atoi(movie.ReleaseDate[:4]); err == nil {
			year = parsedYear
		}
	}
	return "movie", movie.OriginalTitle, movie.OriginalLanguage, year, nil
}

func (t *TMDB) tryFetchTV(id, tmdbID string) (string, string, string, int, error) {
	tvURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s", id, t.apiKey)
	t.logger.Debugf("trying TV endpoint for TMDB ID %s", tmdbID)

	resp, err := t.httpClient.Get(tvURL)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("failed to fetch TMDB data for %s: %w", tmdbID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, fmt.Errorf("TMDB API error for %s: status %d", tmdbID, resp.StatusCode)
	}

	var tv models.TMDBTVDetails
	if err := json.NewDecoder(resp.Body).Decode(&tv); err != nil {
		return "", "", "", 0, fmt.Errorf("failed to decode TMDB TV response for %s: %w", tmdbID, err)
	}

	year := 0
	if tv.FirstAirDate != "" && len(tv.FirstAirDate) >= 4 {
		if parsedYear, err := strconv.Atoi(tv.FirstAirDate[:4]); err == nil {
			year = parsedYear
		}
	}
	return "series", tv.OriginalName, tv.OriginalLanguage, year, nil
}

func (t *TMDB) cacheTMDBResult(cacheKey, tmdbID, mediaType, title string, year int, originalLanguage string) {
	tmdbData := &models.TMDBData{
		Type:             mediaType,
		Title:            title,
		Year:             year,
		OriginalLanguage: originalLanguage,
	}
	t.cache.Set(cacheKey, tmdbData)

	if t.db != nil {
		dbCache := &database.TMDBCache{
			IMDBId:           tmdbID,
			Type:             mediaType,
			Title:            title,
			Year:             year,
			OriginalLanguage: originalLanguage,
		}
		if err := t.db.StoreTMDBCache(dbCache); err != nil {
			t.logger.Errorf("failed to store cache for %s: %v", tmdbID, err)
		}
	}
}

// GetTMDBInfoWithType fetches info for a TMDB ID with a specific media type
func (t *TMDB) GetTMDBInfoWithType(tmdbID, mediaType string) (string, string, string, int, string, error) {
	id, err := t.extractTMDBID(tmdbID)
	if err != nil {
		return "", "", "", 0, "", err
	}

	cacheKey := fmt.Sprintf("tmdb:direct:%s:%s", tmdbID, mediaType)

	if result := t.checkMemoryCache(cacheKey); result != nil {
		return result.Type, result.Title, result.Title, result.Year, result.OriginalLanguage, nil
	}

	if result := t.checkDatabaseCache(tmdbID, cacheKey); result != nil {
		return result.Type, result.Title, result.Title, result.Year, result.OriginalLanguage, nil
	}

	if err := t.validateAPIKey(); err != nil {
		return "", "", "", 0, "", err
	}

	t.rateLimiter.Wait()

	tmdbData, err := t.fetchTMDBDetails(id, tmdbID, mediaType)
	if err != nil {
		return "", "", "", 0, "", err
	}

	t.cache.Set(cacheKey, tmdbData)
	t.storeTMDBCache(tmdbID, tmdbData)

	return tmdbData.Type, tmdbData.Title, tmdbData.Title, tmdbData.Year, tmdbData.OriginalLanguage, nil
}

// GetPopularMovies fetches popular movies from TMDB
func (t *TMDB) GetPopularMovies(page int, genreID string) ([]models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb:popular:movies:%d:%s", page, genreID)

	if data, found := t.cache.Get(cacheKey); found {
		return data.([]models.Meta), nil
	}

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	url := fmt.Sprintf("https://api.themoviedb.org/3/movie/popular?api_key=%s&page=%d&region=FR",
		t.apiKey, page)

	if genreID != "" {
		url += "&with_genres=" + genreID
	}

	t.logger.Debugf("fetching popular movies page %d", page)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch popular movies: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var tmdbResp models.TMDBMovieResponse
	if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB response: %w", err)
	}

	metas := t.convertMoviesToMetas(tmdbResp.Results)
	t.cache.Set(cacheKey, metas)

	return metas, nil
}

// GetPopularSeries fetches popular TV series from TMDB
func (t *TMDB) GetPopularSeries(page int, genreID string) ([]models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb:popular:series:%d:%s", page, genreID)

	if data, found := t.cache.Get(cacheKey); found {
		return data.([]models.Meta), nil
	}

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	url := fmt.Sprintf("https://api.themoviedb.org/3/tv/popular?api_key=%s&page=%d",
		t.apiKey, page)

	if genreID != "" {
		url += "&with_genres=" + genreID
	}

	t.logger.Debugf("fetching popular series page %d", page)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch popular series: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var tmdbResp models.TMDBTVResponse
	if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB response: %w", err)
	}

	metas := t.convertTVToMetas(tmdbResp.Results)
	t.cache.Set(cacheKey, metas)

	return metas, nil
}

// GetTrending fetches trending content from TMDB
func (t *TMDB) GetTrending(mediaType string, timeWindow string, page int) ([]models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb:trending:%s:%s:%d", mediaType, timeWindow, page)

	if data, found := t.cache.Get(cacheKey); found {
		return data.([]models.Meta), nil
	}

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	url := fmt.Sprintf("https://api.themoviedb.org/3/trending/%s/%s?api_key=%s&page=%d",
		mediaType, timeWindow, t.apiKey, page)

	t.logger.Debugf("fetching trending %s for %s", mediaType, timeWindow)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trending: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var metas []models.Meta

	if mediaType == "movie" {
		var tmdbResp models.TMDBMovieResponse
		if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
			return nil, fmt.Errorf("failed to decode TMDB response: %w", err)
		}
		metas = t.convertMoviesToMetas(tmdbResp.Results)
	} else {
		var tmdbResp models.TMDBTVResponse
		if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
			return nil, fmt.Errorf("failed to decode TMDB response: %w", err)
		}
		metas = t.convertTVToMetas(tmdbResp.Results)
	}

	t.cache.Set(cacheKey, metas)
	return metas, nil
}

// SearchMulti searches for movies and TV shows
func (t *TMDB) SearchMulti(query string, page int) ([]models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb:search:%s:%d", query, page)

	if data, found := t.cache.Get(cacheKey); found {
		return data.([]models.Meta), nil
	}

	results, err := t.fetchSearchResults(query, page)
	if err != nil {
		return nil, err
	}

	metas := t.processSearchResults(results)
	
	t.cache.Set(cacheKey, metas)
	return metas, nil
}

func (t *TMDB) processSearchResults(results []json.RawMessage) []models.Meta {
	var metas []models.Meta
	for _, result := range results {
		if meta := t.processSearchResult(result); meta != nil {
			metas = append(metas, *meta)
		}
	}
	return metas
}

// GetMetadata fetches detailed metadata for a specific item
func (t *TMDB) GetMetadata(mediaType, tmdbID string) (*models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb:meta:%s:%s", mediaType, tmdbID)

	if data, found := t.cache.Get(cacheKey); found {
		meta := data.(*models.Meta)
		return meta, nil
	}

	details, err := t.fetchMediaDetails(mediaType, tmdbID)
	if err != nil {
		return nil, err
	}

	var meta models.Meta
	if mediaType == "movie" {
		movieDetails := details.(*models.TMDBMovieDetails)
		meta = t.convertMovieDetailsToMeta(*movieDetails)
	} else {
		tvDetails := details.(*models.TMDBTVDetails)
		meta = t.convertTVDetailsToMeta(*tvDetails)
	}

	t.cache.Set(cacheKey, &meta)
	return &meta, nil
}

// Helper methods

func (t *TMDB) validateAPIKey() error {
	if t.apiKey == "" {
		return fmt.Errorf("TMDB API key not configured")
	}

	if !t.validator.IsValidTMDBKey(t.apiKey) {
		t.logger.Errorf("invalid API key format (key: %s)", t.validator.MaskAPIKey(t.apiKey))
		return fmt.Errorf("invalid TMDB API key format")
	}

	return nil
}

func (t *TMDB) buildTMDBURL(endpoint string, params ...interface{}) string {
	baseURL := "https://api.themoviedb.org/3"
	path := fmt.Sprintf(endpoint, params...)
	return fmt.Sprintf("%s%s?api_key=%s", baseURL, path, t.apiKey)
}

func (t *TMDB) convertMoviesToMetas(movies []models.TMDBMovie) []models.Meta {
	metas := make([]models.Meta, 0, len(movies))
	for _, movie := range movies {
		metas = append(metas, t.convertMovieToMeta(movie))
	}
	return metas
}

func (t *TMDB) convertMovieToMeta(movie models.TMDBMovie) models.Meta {
	// We need to fetch IMDB ID separately or in details
	return models.Meta{
		ID:          fmt.Sprintf("tmdb:%d", movie.ID),
		Type:        "movie",
		Name:        movie.Title,
		Poster:      t.buildImageURL(movie.PosterPath, "w342"),
		Background:  t.buildImageURL(movie.BackdropPath, "w1280"),
		Description: movie.Overview,
		ReleaseInfo: movie.ReleaseDate,
		IMDBRating:  movie.VoteAverage,
		Genres:      t.mapGenreIDs(movie.GenreIDs, "movie"),
	}
}

func (t *TMDB) convertTVToMetas(shows []models.TMDBTV) []models.Meta {
	metas := make([]models.Meta, 0, len(shows))
	for _, show := range shows {
		metas = append(metas, t.convertTVToMeta(show))
	}
	return metas
}

// fetchTVDetails fetches detailed TV show information from TMDB
func (t *TMDB) fetchTVDetails(tmdbID int) (*models.TMDBTVDetails, error) {
	cacheKey := fmt.Sprintf("tmdb:tv_details:%d", tmdbID)
	if data, found := t.cache.Get(cacheKey); found {
		details := data.(*models.TMDBTVDetails)
		return details, nil
	}

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	url := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d?api_key=%s&append_to_response=credits,external_ids",
		tmdbID, t.apiKey)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TV details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var details models.TMDBTVDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode TV details: %w", err)
	}

	t.cache.Set(cacheKey, &details)
	return &details, nil
}

func (t *TMDB) convertTVToMeta(show models.TMDBTV) models.Meta {
	// For search results, return basic info without fetching all episodes
	// This prevents the timeout issue when searching for TV shows
	return models.Meta{
		ID:          fmt.Sprintf("tmdb:%d", show.ID),
		Type:        "series",
		Name:        show.Name,
		Poster:      t.buildImageURL(show.PosterPath, "w342"),
		Background:  t.buildImageURL(show.BackdropPath, "w1280"),
		Description: show.Overview,
		ReleaseInfo: show.FirstAirDate,
		IMDBRating:  show.VoteAverage,
		Genres:      t.mapGenreIDs(show.GenreIDs, "tv"),
		Videos:      []models.Video{}, // Episodes will be fetched when the specific series is requested
	}
}

func (t *TMDB) convertMovieDetailsToMeta(details models.TMDBMovieDetails) models.Meta {
	var genres []string
	for _, g := range details.Genres {
		genres = append(genres, g.Name)
	}

	var cast []string
	for i, actor := range details.Credits.Cast {
		if i >= 5 { // Limit to top 5 actors
			break
		}
		cast = append(cast, actor.Name)
	}

	var directors []string
	for _, crew := range details.Credits.Crew {
		if crew.Job == "Director" {
			directors = append(directors, crew.Name)
		}
	}

	runtime := ""
	if details.Runtime > 0 {
		runtime = fmt.Sprintf("%d min", details.Runtime)
	}

	return models.Meta{
		ID:          details.IMDBId,
		Type:        "movie",
		Name:        details.Title,
		Poster:      t.buildImageURL(details.PosterPath, "w342"),
		Background:  t.buildImageURL(details.BackdropPath, "w1280"),
		Description: details.Overview,
		ReleaseInfo: details.ReleaseDate,
		IMDBRating:  details.VoteAverage,
		Runtime:     runtime,
		Genres:      genres,
		Cast:        cast,
		Director:    directors,
	}
}

func (t *TMDB) convertTVDetailsToMeta(details models.TMDBTVDetails) models.Meta {
	genres := t.extractGenres(details.Genres)
	cast := t.extractTopCast(details.Credits.Cast, 5)
	runtime := t.formatRuntime(details.EpisodeRunTime)
	videos := t.fetchAllSeasonVideos(details)

	return models.Meta{
		ID:          details.ExternalIds.IMDBId,
		Type:        "series",
		Name:        details.Name,
		Poster:      t.buildImageURL(details.PosterPath, "w342"),
		Background:  t.buildImageURL(details.BackdropPath, "w1280"),
		Description: details.Overview,
		ReleaseInfo: details.FirstAirDate,
		IMDBRating:  details.VoteAverage,
		Runtime:     runtime,
		Genres:      genres,
		Cast:        cast,
		Language:    details.OriginalLanguage,
		Videos:      videos,
	}
}

func (t *TMDB) extractGenres(genres []models.TMDBGenre) []string {
	var result []string
	for _, g := range genres {
		result = append(result, g.Name)
	}
	return result
}

func (t *TMDB) extractTopCast(cast []models.CastMember, limit int) []string {
	var result []string
	for i, actor := range cast {
		if i >= limit {
			break
		}
		result = append(result, actor.Name)
	}
	return result
}

func (t *TMDB) formatRuntime(episodeRunTime []int) string {
	if len(episodeRunTime) > 0 {
		return fmt.Sprintf("%d min", episodeRunTime[0])
	}
	return ""
}

func (t *TMDB) fetchAllSeasonVideos(details models.TMDBTVDetails) []models.Video {
	seasonsToFetch := t.filterRegularSeasons(details.Seasons)
	seasonsToFetch = t.limitSeasonsForLargeSeries(seasonsToFetch, details.ID)
	
	seasonVideos := t.fetchSeasonsInBatches(details.ID, details.ExternalIds.IMDBId, seasonsToFetch)
	return t.combineSeasonVideos(seasonsToFetch, seasonVideos)
}

func (t *TMDB) filterRegularSeasons(seasons []models.TMDBSeason) []models.TMDBSeason {
	var result []models.TMDBSeason
	for _, season := range seasons {
		if season.SeasonNumber > 0 {
			result = append(result, season)
		}
	}
	return result
}

func (t *TMDB) limitSeasonsForLargeSeries(seasons []models.TMDBSeason, seriesID int) []models.TMDBSeason {
	if len(seasons) > 20 {
		t.logger.Infof("Series %d has %d seasons, fetching only recent seasons", seriesID, len(seasons))
		if len(seasons) > 10 {
			return seasons[len(seasons)-10:]
		}
	}
	return seasons
}

type seasonResult struct {
	seasonNumber int
	videos       []models.Video
}

func (t *TMDB) fetchSeasonsInBatches(seriesID int, imdbID string, seasons []models.TMDBSeason) map[int][]models.Video {
	const batchSize = 5
	resultsChan := make(chan seasonResult, len(seasons))
	var wg sync.WaitGroup

	for i := 0; i < len(seasons); i += batchSize {
		end := i + batchSize
		if end > len(seasons) {
			end = len(seasons)
		}
		t.processBatchOfSeasons(seriesID, imdbID, seasons[i:end], resultsChan, &wg)
		wg.Wait()
	}

	close(resultsChan)
	return t.collectSeasonResults(resultsChan)
}

func (t *TMDB) processBatchOfSeasons(seriesID int, imdbID string, batch []models.TMDBSeason, resultsChan chan<- seasonResult, wg *sync.WaitGroup) {
	for _, season := range batch {
		wg.Add(1)
		go t.fetchSeasonVideos(seriesID, imdbID, season, resultsChan, wg)
	}
}

func (t *TMDB) fetchSeasonVideos(seriesID int, imdbID string, season models.TMDBSeason, resultsChan chan<- seasonResult, wg *sync.WaitGroup) {
	defer wg.Done()

	episodes, err := t.getSeasonEpisodes(seriesID, season.SeasonNumber)
	if err != nil {
		t.logger.Warnf("failed to fetch episodes for season %d of series %d: %v", season.SeasonNumber, seriesID, err)
		return
	}

	videos := t.convertEpisodesToVideos(episodes, imdbID)
	resultsChan <- seasonResult{
		seasonNumber: season.SeasonNumber,
		videos:       videos,
	}
}

func (t *TMDB) convertEpisodesToVideos(episodes []models.TMDBEpisode, imdbID string) []models.Video {
	var videos []models.Video
	for _, episode := range episodes {
		video := models.Video{
			ID:        fmt.Sprintf("%s:%d:%d", imdbID, episode.SeasonNumber, episode.EpisodeNumber),
			Title:     episode.Name,
			Season:    episode.SeasonNumber,
			Episode:   episode.EpisodeNumber,
			Released:  episode.AirDate,
			Overview:  episode.Overview,
			Thumbnail: t.buildImageURL(episode.StillPath, "w300"),
		}
		videos = append(videos, video)
	}
	return videos
}

func (t *TMDB) collectSeasonResults(resultsChan <-chan seasonResult) map[int][]models.Video {
	seasonMap := make(map[int][]models.Video)
	for result := range resultsChan {
		seasonMap[result.seasonNumber] = result.videos
	}
	return seasonMap
}

func (t *TMDB) combineSeasonVideos(seasonsToFetch []models.TMDBSeason, seasonMap map[int][]models.Video) []models.Video {
	var videos []models.Video
	for _, season := range seasonsToFetch {
		if seasonVideos, ok := seasonMap[season.SeasonNumber]; ok {
			videos = append(videos, seasonVideos...)
		}
	}
	return videos
}

// getSeasonEpisodes fetches episodes for a specific season
func (t *TMDB) getSeasonEpisodes(seriesID, seasonNumber int) ([]models.TMDBEpisode, error) {
	cacheKey := fmt.Sprintf("tmdb:season:%d:%d", seriesID, seasonNumber)

	if data, found := t.cache.Get(cacheKey); found {
		return data.([]models.TMDBEpisode), nil
	}

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	url := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%d?api_key=%s",
		seriesID, seasonNumber, t.apiKey)

	t.logger.Debugf("fetching episodes for series %d season %d", seriesID, seasonNumber)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch season details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var seasonDetails models.TMDBSeasonDetails
	if err := json.NewDecoder(resp.Body).Decode(&seasonDetails); err != nil {
		return nil, fmt.Errorf("failed to decode season details: %w", err)
	}

	t.cache.Set(cacheKey, seasonDetails.Episodes)
	return seasonDetails.Episodes, nil
}

// prefetchSeasons fetches multiple seasons concurrently with batching
func (t *TMDB) prefetchSeasons(seriesID int, seasons []models.TMDBSeason) {
	const batchSize = 10 // Increase batch size for prefetching
	var wg sync.WaitGroup

	for i := 0; i < len(seasons); i += batchSize {
		end := i + batchSize
		if end > len(seasons) {
			end = len(seasons)
		}

		for j := i; j < end; j++ {
			if seasons[j].SeasonNumber == 0 {
				continue
			}

			wg.Add(1)
			go func(seasonNum int) {
				defer wg.Done()
				// Attempt to fetch but don't fail if individual season fails
				_, _ = t.getSeasonEpisodes(seriesID, seasonNum)
			}(seasons[j].SeasonNumber)
		}

		// Wait for batch to complete
		wg.Wait()
	}
}

func (t *TMDB) buildImageURL(path, size string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf("https://image.tmdb.org/t/p/%s%s", size, path)
}

func (t *TMDB) mapGenreIDs(ids []int, mediaType string) []string {
	// This is a simplified version - in production you'd cache genre mappings
	// For now, return empty array - can be enhanced later
	return []string{}
}

// GetSeasonVideos fetches episodes for a specific season and returns them as videos
func (t *TMDB) GetSeasonVideos(imdbID string, seriesID int, seasonNumber int) ([]models.Video, error) {
	episodes, err := t.getSeasonEpisodes(seriesID, seasonNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch season %d: %w", seasonNumber, err)
	}

	var videos []models.Video
	for _, episode := range episodes {
		video := models.Video{
			ID:        fmt.Sprintf("%s:%d:%d", imdbID, episode.SeasonNumber, episode.EpisodeNumber),
			Title:     episode.Name,
			Season:    episode.SeasonNumber,
			Episode:   episode.EpisodeNumber,
			Released:  episode.AirDate,
			Overview:  episode.Overview,
			Thumbnail: t.buildImageURL(episode.StillPath, "w300"),
		}
		videos = append(videos, video)
	}

	return videos, nil
}
