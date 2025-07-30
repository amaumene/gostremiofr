package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
		t.logger.Errorf("[TMDB] failed to set API key: invalid format (key: %s)", t.validator.MaskAPIKey(sanitizedKey))
		return
	}
	t.apiKey = sanitizedKey
}

func (t *TMDB) GetIMDBInfo(imdbID string) (string, string, string, int, string, error) {
	cacheKey := fmt.Sprintf("tmdb:%s", imdbID)

	if data, found := t.cache.Get(cacheKey); found {
		tmdbData := data.(*models.TMDBData)
		return tmdbData.Type, tmdbData.Title, tmdbData.Title, tmdbData.Year, tmdbData.OriginalLanguage, nil
	}

	if t.db != nil {
		if cached, err := t.db.GetCachedTMDB(imdbID); err == nil && cached != nil {
			tmdbData := &models.TMDBData{
				Type:             cached.Type,
				Title:            cached.Title,
				Year:             cached.Year,
				OriginalLanguage: cached.OriginalLanguage,
			}
			t.cache.Set(cacheKey, tmdbData)
			return cached.Type, cached.Title, cached.Title, cached.Year, cached.OriginalLanguage, nil
		}
	}

	// Validate API key before making request
	if t.apiKey == "" {
		return "", "", "", 0, "", fmt.Errorf("TMDB API key not configured")
	}

	if !t.validator.IsValidTMDBKey(t.apiKey) {
		t.logger.Errorf("[TMDB] failed to make API request: invalid API key format (key: %s)", t.validator.MaskAPIKey(t.apiKey))
		return "", "", "", 0, "", fmt.Errorf("invalid TMDB API key format")
	}

	t.rateLimiter.Wait()

	// Use request headers instead of URL parameters when possible
	// Unfortunately TMDB API requires the key as a query parameter
	url := fmt.Sprintf("https://api.themoviedb.org/3/find/%s?api_key=%s&external_source=imdb_id",
		imdbID, t.apiKey)

	t.logger.Debugf("[TMDB] fetching info for %s", imdbID)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return "", "", "", 0, "", fmt.Errorf("failed to fetch TMDB data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, "", fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var tmdbResp models.TMDBFindResponse
	if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
		return "", "", "", 0, "", fmt.Errorf("failed to decode TMDB response: %w", err)
	}

	var mediaType, title, originalLanguage string
	var year int

	if len(tmdbResp.MovieResults) > 0 {
		mediaType = "movie"
		title = tmdbResp.MovieResults[0].OriginalTitle
		originalLanguage = tmdbResp.MovieResults[0].OriginalLanguage
		// Extract year from release date (format: "2024-01-15")
		if tmdbResp.MovieResults[0].ReleaseDate != "" && len(tmdbResp.MovieResults[0].ReleaseDate) >= 4 {
			if parsedYear, err := strconv.Atoi(tmdbResp.MovieResults[0].ReleaseDate[:4]); err == nil {
				year = parsedYear
			}
		}
	} else if len(tmdbResp.TVResults) > 0 {
		mediaType = "series"
		title = tmdbResp.TVResults[0].OriginalName
		originalLanguage = tmdbResp.TVResults[0].OriginalLanguage
		// Extract year from first air date (format: "2024-01-15")
		if tmdbResp.TVResults[0].FirstAirDate != "" && len(tmdbResp.TVResults[0].FirstAirDate) >= 4 {
			if parsedYear, err := strconv.Atoi(tmdbResp.TVResults[0].FirstAirDate[:4]); err == nil {
				year = parsedYear
			}
		}
	} else {
		return "", "", "", 0, "", fmt.Errorf("no results found for IMDB ID: %s", imdbID)
	}

	tmdbData := &models.TMDBData{
		Type:             mediaType,
		Title:            title,
		Year:             year,
		OriginalLanguage: originalLanguage,
	}
	t.cache.Set(cacheKey, tmdbData)

	if t.db != nil {
		dbCache := &database.TMDBCache{
			IMDBId:           imdbID,
			Type:             mediaType,
			Title:            title,
			Year:             year,
			OriginalLanguage: originalLanguage,
		}
		if err := t.db.StoreTMDBCache(dbCache); err != nil {
			t.logger.Errorf("[TMDB] failed to store cache: %v", err)
		}
	}

	return mediaType, title, title, year, originalLanguage, nil
}

// GetTMDBInfo fetches info for a TMDB ID directly
func (t *TMDB) GetTMDBInfo(tmdbID string) (string, string, string, int, string, error) {
	// Extract numeric ID from tmdb:12345 format
	parts := strings.Split(tmdbID, ":")
	if len(parts) != 2 {
		return "", "", "", 0, "", fmt.Errorf("invalid TMDB ID format: %s", tmdbID)
	}

	id := parts[1]
	cacheKey := fmt.Sprintf("tmdb:direct:%s", tmdbID)

	if data, found := t.cache.Get(cacheKey); found {
		tmdbData := data.(*models.TMDBData)
		return tmdbData.Type, tmdbData.Title, tmdbData.Title, tmdbData.Year, tmdbData.OriginalLanguage, nil
	}

	if t.db != nil {
		if cached, err := t.db.GetCachedTMDB(tmdbID); err == nil && cached != nil {
			tmdbData := &models.TMDBData{
				Type:             cached.Type,
				Title:            cached.Title,
				Year:             cached.Year,
				OriginalLanguage: cached.OriginalLanguage,
			}
			t.cache.Set(cacheKey, tmdbData)
			return cached.Type, cached.Title, cached.Title, cached.Year, cached.OriginalLanguage, nil
		}
	}

	if err := t.validateAPIKey(); err != nil {
		return "", "", "", 0, "", err
	}

	t.rateLimiter.Wait()

	// Try both movie and TV endpoints since we don't know the type
	var mediaType, title, originalLanguage string
	var year int

	// Try movie first
	movieURL := fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s", id, t.apiKey)
	t.logger.Debugf("[TMDB] trying movie endpoint for TMDB ID %s", tmdbID)

	resp, err := t.httpClient.Get(movieURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var movie models.TMDBMovieDetails
		if err := json.NewDecoder(resp.Body).Decode(&movie); err == nil {
			mediaType = "movie"
			title = movie.OriginalTitle
			originalLanguage = movie.OriginalLanguage
			if movie.ReleaseDate != "" && len(movie.ReleaseDate) >= 4 {
				if parsedYear, err := strconv.Atoi(movie.ReleaseDate[:4]); err == nil {
					year = parsedYear
				}
			}
		}
	} else {
		if resp != nil {
			resp.Body.Close()
		}
		
		// Try TV endpoint
		tvURL := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s", id, t.apiKey)
		t.logger.Debugf("[TMDB] trying TV endpoint for TMDB ID %s", tmdbID)

		resp, err = t.httpClient.Get(tvURL)
		if err != nil {
			return "", "", "", 0, "", fmt.Errorf("failed to fetch TMDB data for %s: %w", tmdbID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", "", "", 0, "", fmt.Errorf("TMDB API error for %s: status %d", tmdbID, resp.StatusCode)
		}

		var tv models.TMDBTVDetails
		if err := json.NewDecoder(resp.Body).Decode(&tv); err != nil {
			return "", "", "", 0, "", fmt.Errorf("failed to decode TMDB TV response for %s: %w", tmdbID, err)
		}

		mediaType = "series"
		title = tv.OriginalName
		originalLanguage = tv.OriginalLanguage
		if tv.FirstAirDate != "" && len(tv.FirstAirDate) >= 4 {
			if parsedYear, err := strconv.Atoi(tv.FirstAirDate[:4]); err == nil {
				year = parsedYear
			}
		}
	}

	if mediaType == "" {
		return "", "", "", 0, "", fmt.Errorf("no results found for TMDB ID: %s", tmdbID)
	}

	tmdbData := &models.TMDBData{
		Type:             mediaType,
		Title:            title,
		Year:             year,
		OriginalLanguage: originalLanguage,
	}
	t.cache.Set(cacheKey, tmdbData)

	if t.db != nil {
		dbCache := &database.TMDBCache{
			IMDBId:           tmdbID, // Store TMDB ID in the same field
			Type:             mediaType,
			Title:            title,
			Year:             year,
			OriginalLanguage: originalLanguage,
		}
		if err := t.db.StoreTMDBCache(dbCache); err != nil {
			t.logger.Errorf("[TMDB] failed to store cache for %s: %v", tmdbID, err)
		}
	}

	return mediaType, title, title, year, originalLanguage, nil
}

// GetTMDBInfoWithType fetches info for a TMDB ID with a specific media type
func (t *TMDB) GetTMDBInfoWithType(tmdbID, mediaType string) (string, string, string, int, string, error) {
	// Extract numeric ID from tmdb:12345 format
	parts := strings.Split(tmdbID, ":")
	if len(parts) != 2 {
		return "", "", "", 0, "", fmt.Errorf("invalid TMDB ID format: %s", tmdbID)
	}

	id := parts[1]
	cacheKey := fmt.Sprintf("tmdb:direct:%s:%s", tmdbID, mediaType)

	if data, found := t.cache.Get(cacheKey); found {
		tmdbData := data.(*models.TMDBData)
		return tmdbData.Type, tmdbData.Title, tmdbData.Title, tmdbData.Year, tmdbData.OriginalLanguage, nil
	}

	if t.db != nil {
		if cached, err := t.db.GetCachedTMDB(tmdbID); err == nil && cached != nil {
			tmdbData := &models.TMDBData{
				Type:             cached.Type,
				Title:            cached.Title,
				Year:             cached.Year,
				OriginalLanguage: cached.OriginalLanguage,
			}
			t.cache.Set(cacheKey, tmdbData)
			return cached.Type, cached.Title, cached.Title, cached.Year, cached.OriginalLanguage, nil
		}
	}

	if err := t.validateAPIKey(); err != nil {
		return "", "", "", 0, "", err
	}

	t.rateLimiter.Wait()

	var title, originalLanguage string
	var year int
	var apiURL string

	// Use the specific endpoint based on media type
	if mediaType == "movie" {
		apiURL = fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s", id, t.apiKey)
		t.logger.Debugf("[TMDB] fetching movie info for TMDB ID %s", tmdbID)
	} else if mediaType == "tv" {
		apiURL = fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s", id, t.apiKey)
		t.logger.Debugf("[TMDB] fetching TV info for TMDB ID %s", tmdbID)
	} else {
		return "", "", "", 0, "", fmt.Errorf("unsupported media type: %s", mediaType)
	}

	resp, err := t.httpClient.Get(apiURL)
	if err != nil {
		return "", "", "", 0, "", fmt.Errorf("failed to fetch TMDB data for %s: %w", tmdbID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, "", fmt.Errorf("TMDB API error for %s: status %d", tmdbID, resp.StatusCode)
	}

	if mediaType == "movie" {
		var movie models.TMDBMovieDetails
		if err := json.NewDecoder(resp.Body).Decode(&movie); err != nil {
			return "", "", "", 0, "", fmt.Errorf("failed to decode TMDB movie response for %s: %w", tmdbID, err)
		}

		title = movie.OriginalTitle
		originalLanguage = movie.OriginalLanguage
		if movie.ReleaseDate != "" && len(movie.ReleaseDate) >= 4 {
			if parsedYear, err := strconv.Atoi(movie.ReleaseDate[:4]); err == nil {
				year = parsedYear
			}
		}
		mediaType = "movie"
	} else {
		var tv models.TMDBTVDetails
		if err := json.NewDecoder(resp.Body).Decode(&tv); err != nil {
			return "", "", "", 0, "", fmt.Errorf("failed to decode TMDB TV response for %s: %w", tmdbID, err)
		}

		title = tv.OriginalName
		originalLanguage = tv.OriginalLanguage
		if tv.FirstAirDate != "" && len(tv.FirstAirDate) >= 4 {
			if parsedYear, err := strconv.Atoi(tv.FirstAirDate[:4]); err == nil {
				year = parsedYear
			}
		}
		mediaType = "series"
	}

	tmdbData := &models.TMDBData{
		Type:             mediaType,
		Title:            title,
		Year:             year,
		OriginalLanguage: originalLanguage,
	}
	t.cache.Set(cacheKey, tmdbData)

	if t.db != nil {
		dbCache := &database.TMDBCache{
			IMDBId:           tmdbID, // Store TMDB ID in the same field
			Type:             mediaType,
			Title:            title,
			Year:             year,
			OriginalLanguage: originalLanguage,
		}
		if err := t.db.StoreTMDBCache(dbCache); err != nil {
			t.logger.Errorf("[TMDB] failed to store cache for %s: %v", tmdbID, err)
		}
	}

	return mediaType, title, title, year, originalLanguage, nil
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

	t.logger.Debugf("[TMDB] fetching popular movies page %d", page)

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

	t.logger.Debugf("[TMDB] fetching popular series page %d", page)

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

	t.logger.Debugf("[TMDB] fetching trending %s for %s", mediaType, timeWindow)

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

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	encodedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?api_key=%s&query=%s&page=%d&include_adult=false",
		t.apiKey, encodedQuery, page)

	t.logger.Debugf("[TMDB] searching for '%s' page %d", query, page)

	resp, err := t.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	// Parse search results and convert to metas
	var searchResp struct {
		Results []json.RawMessage `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	var metas []models.Meta
	for _, result := range searchResp.Results {
		var mediaType struct {
			MediaType string `json:"media_type"`
		}
		if err := json.Unmarshal(result, &mediaType); err != nil {
			continue
		}

		switch mediaType.MediaType {
		case "movie":
			var movie models.TMDBMovie
			if err := json.Unmarshal(result, &movie); err == nil {
				meta := t.convertMovieToMeta(movie)
				meta.Type = "movie"
				metas = append(metas, meta)
			}
		case "tv":
			var tv models.TMDBTV
			if err := json.Unmarshal(result, &tv); err == nil {
				meta := t.convertTVToMeta(tv)
				meta.Type = "series"
				metas = append(metas, meta)
			}
		}
	}

	t.cache.Set(cacheKey, metas)
	return metas, nil
}

// GetMetadata fetches detailed metadata for a specific item
func (t *TMDB) GetMetadata(mediaType, tmdbID string) (*models.Meta, error) {
	cacheKey := fmt.Sprintf("tmdb:meta:%s:%s", mediaType, tmdbID)

	if data, found := t.cache.Get(cacheKey); found {
		meta := data.(*models.Meta)
		return meta, nil
	}

	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	var meta models.Meta

	if mediaType == "movie" {
		url := fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s&append_to_response=credits",
			tmdbID, t.apiKey)

		resp, err := t.httpClient.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch movie details: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
		}

		var details models.TMDBMovieDetails
		if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
			return nil, fmt.Errorf("failed to decode movie details: %w", err)
		}

		meta = t.convertMovieDetailsToMeta(details)
	} else {
		url := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s&append_to_response=credits,external_ids",
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

		meta = t.convertTVDetailsToMeta(details)
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
		t.logger.Errorf("[TMDB] invalid API key format (key: %s)", t.validator.MaskAPIKey(t.apiKey))
		return fmt.Errorf("invalid TMDB API key format")
	}

	return nil
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

	runtime := ""
	if len(details.EpisodeRunTime) > 0 {
		runtime = fmt.Sprintf("%d min", details.EpisodeRunTime[0])
	}

	// Generate videos (episodes) for the series
	var videos []models.Video
	
	// Filter out special seasons and create a list of seasons to fetch
	var seasonsToFetch []models.TMDBSeason
	for _, season := range details.Seasons {
		if season.SeasonNumber > 0 {
			seasonsToFetch = append(seasonsToFetch, season)
		}
	}

	// For series with many seasons, only fetch recent seasons by default
	if len(seasonsToFetch) > 20 {
		t.logger.Infof("[TMDB] Series %d has %d seasons, fetching only recent seasons", details.ID, len(seasonsToFetch))
		// Sort seasons by number and take the last 10 seasons
		if len(seasonsToFetch) > 10 {
			seasonsToFetch = seasonsToFetch[len(seasonsToFetch)-10:]
		}
	}

	// For series with many seasons, fetch them concurrently in batches
	const batchSize = 5 // Process 5 seasons at a time to avoid overwhelming the API
	type seasonResult struct {
		seasonNumber int
		videos       []models.Video
	}
	resultsChan := make(chan seasonResult, len(seasonsToFetch))
	var wg sync.WaitGroup

	for i := 0; i < len(seasonsToFetch); i += batchSize {
		end := i + batchSize
		if end > len(seasonsToFetch) {
			end = len(seasonsToFetch)
		}

		// Process batch of seasons concurrently
		for j := i; j < end; j++ {
			wg.Add(1)
			go func(season models.TMDBSeason) {
				defer wg.Done()
				
				episodes, err := t.getSeasonEpisodes(details.ID, season.SeasonNumber)
				if err != nil {
					t.logger.Warnf("[TMDB] failed to fetch episodes for season %d of series %d: %v", season.SeasonNumber, details.ID, err)
					return
				}

				var seasonVideos []models.Video
				for _, episode := range episodes {
					video := models.Video{
						ID:        fmt.Sprintf("%s:%d:%d", details.ExternalIds.IMDBId, episode.SeasonNumber, episode.EpisodeNumber),
						Title:     episode.Name,
						Season:    episode.SeasonNumber,
						Episode:   episode.EpisodeNumber,
						Released:  episode.AirDate,
						Overview:  episode.Overview,
						Thumbnail: t.buildImageURL(episode.StillPath, "w300"),
					}
					seasonVideos = append(seasonVideos, video)
				}
				resultsChan <- seasonResult{
					seasonNumber: season.SeasonNumber,
					videos:       seasonVideos,
				}
			}(seasonsToFetch[j])
		}

		// Wait for current batch to complete before starting next batch
		wg.Wait()
	}

	close(resultsChan)

	// Collect all videos from the channel and sort by season number
	seasonMap := make(map[int][]models.Video)
	for result := range resultsChan {
		seasonMap[result.seasonNumber] = result.videos
	}
	
	// Add videos in season order
	for _, season := range seasonsToFetch {
		if seasonVideos, ok := seasonMap[season.SeasonNumber]; ok {
			videos = append(videos, seasonVideos...)
		}
	}

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

	t.logger.Debugf("[TMDB] fetching episodes for series %d season %d", seriesID, seasonNumber)

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
