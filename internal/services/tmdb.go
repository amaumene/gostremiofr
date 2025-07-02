package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gostremiofr/gostremiofr/internal/cache"
	"github.com/gostremiofr/gostremiofr/internal/database"
	"github.com/gostremiofr/gostremiofr/internal/models"
	"github.com/gostremiofr/gostremiofr/pkg/logger"
	"github.com/gostremiofr/gostremiofr/pkg/ratelimiter"
)

type TMDB struct {
	apiKey      string
	cache       *cache.LRUCache
	db          *database.DB
	rateLimiter *ratelimiter.TokenBucket
	httpClient  *http.Client
	logger      logger.Logger
}

func NewTMDB(apiKey string, cache *cache.LRUCache) *TMDB {
	return &TMDB{
		apiKey:      apiKey,
		cache:       cache,
		rateLimiter: ratelimiter.NewTokenBucket(20, 5),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger.New(),
	}
}

func (t *TMDB) SetDB(db *database.DB) {
	t.db = db
}

func (t *TMDB) GetIMDBInfo(imdbID string) (string, string, string, error) {
	cacheKey := fmt.Sprintf("tmdb:%s", imdbID)
	
	if data, found := t.cache.Get(cacheKey); found {
		tmdbData := data.(*models.TMDBData)
		return tmdbData.Type, tmdbData.Title, tmdbData.FrenchTitle, nil
	}
	
	if t.db != nil {
		if cached, err := t.db.GetCachedTMDB(imdbID); err == nil && cached != nil {
			tmdbData := &models.TMDBData{
				Type:        cached.Type,
				Title:       cached.Title,
				FrenchTitle: cached.FrenchTitle,
			}
			t.cache.Set(cacheKey, tmdbData)
			return cached.Type, cached.Title, cached.FrenchTitle, nil
		}
	}
	
	t.rateLimiter.Wait()
	
	url := fmt.Sprintf("https://api.themoviedb.org/3/find/%s?api_key=%s&external_source=imdb_id&language=fr-FR",
		imdbID, t.apiKey)
	
	resp, err := t.httpClient.Get(url)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch TMDB data: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("TMDB API error: status %d", resp.StatusCode)
	}
	
	var tmdbResp models.TMDBFindResponse
	if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
		return "", "", "", fmt.Errorf("failed to decode TMDB response: %w", err)
	}
	
	var mediaType, title, frenchTitle string
	
	if len(tmdbResp.MovieResults) > 0 {
		mediaType = "movie"
		title = tmdbResp.MovieResults[0].OriginalTitle
		frenchTitle = tmdbResp.MovieResults[0].Title
	} else if len(tmdbResp.TVResults) > 0 {
		mediaType = "series"
		title = tmdbResp.TVResults[0].OriginalName
		frenchTitle = tmdbResp.TVResults[0].Name
	} else {
		return "", "", "", fmt.Errorf("no results found for IMDB ID: %s", imdbID)
	}
	
	tmdbData := &models.TMDBData{
		Type:        mediaType,
		Title:       title,
		FrenchTitle: frenchTitle,
	}
	t.cache.Set(cacheKey, tmdbData)
	
	if t.db != nil {
		dbCache := &database.TMDBCache{
			IMDBId:      imdbID,
			Type:        mediaType,
			Title:       title,
			FrenchTitle: frenchTitle,
		}
		if err := t.db.StoreTMDBCache(dbCache); err != nil {
			t.logger.Errorf("failed to store TMDB cache: %v", err)
		}
	}
	
	return mediaType, title, frenchTitle, nil
}