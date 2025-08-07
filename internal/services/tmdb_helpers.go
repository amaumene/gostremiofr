package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)

func (t *TMDB) extractTMDBID(tmdbID string) (string, error) {
	parts := strings.Split(tmdbID, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid TMDB ID format: %s", tmdbID)
	}
	return parts[1], nil
}

func (t *TMDB) checkMemoryCache(cacheKey string) *models.TMDBData {
	if data, found := t.cache.Get(cacheKey); found {
		return data.(*models.TMDBData)
	}
	return nil
}

func (t *TMDB) checkDatabaseCache(tmdbID, cacheKey string) *models.TMDBData {
	if t.db == nil {
		return nil
	}
	
	cached, err := t.db.GetCachedTMDB(tmdbID)
	if err != nil || cached == nil {
		return nil
	}
	
	tmdbData := &models.TMDBData{
		Type:             cached.Type,
		Title:            cached.Title,
		Year:             cached.Year,
		OriginalLanguage: cached.OriginalLanguage,
	}
	t.cache.Set(cacheKey, tmdbData)
	return tmdbData
}

func (t *TMDB) fetchTMDBDetails(id, tmdbID, mediaType string) (*models.TMDBData, error) {
	apiURL := t.buildTMDBDetailURL(id, mediaType)
	if apiURL == "" {
		return nil, fmt.Errorf("unsupported media type: %s", mediaType)
	}

	t.logger.Debugf("fetching %s info for TMDB ID %s", mediaType, tmdbID)
	t.logger.Debugf("[TMDB] API URL: %s", apiURL)

	resp, err := t.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TMDB data for %s: %w", tmdbID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB API error for %s: status %d", tmdbID, resp.StatusCode)
	}

	if mediaType == "movie" {
		return t.parseMovieDetails(resp.Body, tmdbID)
	}
	return t.parseTVDetails(resp.Body, tmdbID)
}

func (t *TMDB) buildTMDBDetailURL(id, mediaType string) string {
	if mediaType == "movie" {
		return fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s", id, t.apiKey)
	}
	if mediaType == "tv" {
		return fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s", id, t.apiKey)
	}
	return ""
}

func (t *TMDB) parseMovieDetails(body io.Reader, tmdbID string) (*models.TMDBData, error) {
	var movie models.TMDBMovieDetails
	if err := json.NewDecoder(body).Decode(&movie); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB movie response for %s: %w", tmdbID, err)
	}

	year := t.extractYearFromDate(movie.ReleaseDate)
	return &models.TMDBData{
		Type:             "movie",
		Title:            movie.OriginalTitle,
		Year:             year,
		OriginalLanguage: movie.OriginalLanguage,
	}, nil
}

func (t *TMDB) parseTVDetails(body io.Reader, tmdbID string) (*models.TMDBData, error) {
	var tv models.TMDBTVDetails
	if err := json.NewDecoder(body).Decode(&tv); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB TV response for %s: %w", tmdbID, err)
	}

	year := t.extractYearFromDate(tv.FirstAirDate)
	return &models.TMDBData{
		Type:             "series",
		Title:            tv.OriginalName,
		Year:             year,
		OriginalLanguage: tv.OriginalLanguage,
	}, nil
}

func (t *TMDB) extractYearFromDate(date string) int {
	if date != "" && len(date) >= 4 {
		if year, err := strconv.Atoi(date[:4]); err == nil {
			return year
		}
	}
	return 0
}

func (t *TMDB) storeTMDBCache(tmdbID string, tmdbData *models.TMDBData) {
	if t.db == nil {
		return
	}
	
	dbCache := &database.TMDBCache{
		IMDBId:           tmdbID,
		Type:             tmdbData.Type,
		Title:            tmdbData.Title,
		Year:             tmdbData.Year,
		OriginalLanguage: tmdbData.OriginalLanguage,
	}
	if err := t.db.StoreTMDBCache(dbCache); err != nil {
		t.logger.Errorf("failed to store cache for %s: %v", tmdbID, err)
	}
}

func (t *TMDB) fetchIMDBData(imdbID string) (*models.TMDBFindResponse, error) {
	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	url := fmt.Sprintf("https://api.themoviedb.org/3/find/%s?api_key=%s&external_source=imdb_id",
		imdbID, t.apiKey)

	t.logger.Debugf("fetching info for %s", imdbID)
	t.logger.Debugf("[TMDB] API URL: %s", url)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TMDB data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var tmdbResp models.TMDBFindResponse
	if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB response: %w", err)
	}

	return &tmdbResp, nil
}

func (t *TMDB) extractMovieData(movie models.TMDBMovie) *models.TMDBData {
	year := t.extractYearFromDate(movie.ReleaseDate)
	return &models.TMDBData{
		Type:             "movie",
		Title:            movie.OriginalTitle,
		Year:             year,
		OriginalLanguage: movie.OriginalLanguage,
	}
}

func (t *TMDB) extractTVData(tv models.TMDBTV) *models.TMDBData {
	year := t.extractYearFromDate(tv.FirstAirDate)
	return &models.TMDBData{
		Type:             "series",
		Title:            tv.OriginalName,
		Year:             year,
		OriginalLanguage: tv.OriginalLanguage,
	}
}

func (t *TMDB) processIMDBResponse(tmdbResp *models.TMDBFindResponse, imdbID string) (*models.TMDBData, error) {
	if len(tmdbResp.MovieResults) > 0 {
		return t.extractMovieData(tmdbResp.MovieResults[0]), nil
	}
	
	if len(tmdbResp.TVResults) > 0 {
		return t.extractTVData(tmdbResp.TVResults[0]), nil
	}
	
	return nil, fmt.Errorf("no results found for IMDB ID: %s", imdbID)
}

func (t *TMDB) fetchSearchResults(query string, page int) ([]json.RawMessage, error) {
	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	encodedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/search/multi?api_key=%s&query=%s&page=%d&include_adult=false",
		t.apiKey, encodedQuery, page)

	t.logger.Debugf("searching for '%s' page %d", query, page)
	t.logger.Debugf("[TMDB] API URL: %s", apiURL)

	resp, err := t.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var searchResp struct {
		Results []json.RawMessage `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	return searchResp.Results, nil
}

func (t *TMDB) processSearchResult(result json.RawMessage) *models.Meta {
	var mediaType struct {
		MediaType string `json:"media_type"`
	}
	
	if err := json.Unmarshal(result, &mediaType); err != nil {
		return nil
	}

	switch mediaType.MediaType {
	case "movie":
		var movie models.TMDBMovie
		if err := json.Unmarshal(result, &movie); err == nil {
			meta := t.convertMovieToMeta(movie)
			meta.Type = "movie"
			return &meta
		}
	case "tv":
		var tv models.TMDBTV
		if err := json.Unmarshal(result, &tv); err == nil {
			meta := t.convertTVToMeta(tv)
			meta.Type = "series"
			return &meta
		}
	}
	
	return nil
}

func (t *TMDB) fetchMediaDetails(mediaType, tmdbID string) (interface{}, error) {
	if err := t.validateAPIKey(); err != nil {
		return nil, err
	}

	t.rateLimiter.Wait()

	if mediaType == "movie" {
		return t.fetchMovieDetails(tmdbID)
	}
	return t.fetchTVDetailsWithAppend(tmdbID)
}

func (t *TMDB) fetchMovieDetails(tmdbID string) (*models.TMDBMovieDetails, error) {
	url := fmt.Sprintf("https://api.themoviedb.org/3/movie/%s?api_key=%s&append_to_response=credits",
		tmdbID, t.apiKey)
	
	t.logger.Debugf("[TMDB] API URL: %s", url)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch movie details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var details models.TMDBMovieDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode movie details: %w", err)
	}

	return &details, nil
}

func (t *TMDB) fetchTVDetailsWithAppend(tmdbID string) (*models.TMDBTVDetails, error) {
	url := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s&append_to_response=credits,external_ids",
		tmdbID, t.apiKey)
	
	t.logger.Debugf("[TMDB] API URL: %s", url)

	resp, err := t.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch TV details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}

	var details models.TMDBTVDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode TV details: %w", err)
	}

	return &details, nil
}