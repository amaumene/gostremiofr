package translator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type FrenchTranslator struct {
	tmdbAPIKey string
	httpClient *http.Client
	cache      Cache
	mu         sync.RWMutex
}

type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
}

type TMDBSearchResult struct {
	Results []struct {
		ID           int    `json:"id"`
		Title        string `json:"title,omitempty"`
		Name         string `json:"name,omitempty"`
		OriginalName string `json:"original_name,omitempty"`
		ReleaseDate  string `json:"release_date,omitempty"`
		FirstAirDate string `json:"first_air_date,omitempty"`
	} `json:"results"`
}

type TMDBDetail struct {
	Title        string `json:"title,omitempty"`
	Name         string `json:"name,omitempty"`
	OriginalName string `json:"original_name,omitempty"`
}

func NewFrenchTranslator(apiKey string, cache Cache) *FrenchTranslator {
	return &FrenchTranslator{
		tmdbAPIKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: cache,
	}
}

func (ft *FrenchTranslator) TranslateTitle(originalTitle string, mediaType string) string {
	if ft.tmdbAPIKey == "" {
		return originalTitle
	}

	cacheKey := fmt.Sprintf("fr_title:%s:%s", mediaType, originalTitle)
	if ft.cache != nil {
		if cached, found := ft.cache.Get(cacheKey); found {
			if title, ok := cached.(string); ok {
				return title
			}
		}
	}

	frenchTitle := ft.fetchFrenchTitle(originalTitle, mediaType)
	
	if ft.cache != nil && frenchTitle != originalTitle {
		ft.cache.Set(cacheKey, frenchTitle)
	}

	return frenchTitle
}

func (ft *FrenchTranslator) fetchFrenchTitle(originalTitle string, mediaType string) string {
	searchResult, err := ft.searchTMDB(originalTitle, mediaType)
	if err != nil || len(searchResult.Results) == 0 {
		return originalTitle
	}

	tmdbID := searchResult.Results[0].ID
	
	frenchDetail, err := ft.fetchFrenchMetadata(tmdbID, mediaType)
	if err != nil {
		return originalTitle
	}

	return ft.extractTitle(frenchDetail, originalTitle)
}

func (ft *FrenchTranslator) searchTMDB(title string, mediaType string) (*TMDBSearchResult, error) {
	endpoint := "movie"
	if mediaType == "series" || mediaType == "tv" {
		endpoint = "tv"
	}

	url := fmt.Sprintf("https://api.themoviedb.org/3/search/%s?api_key=%s&query=%s",
		endpoint, ft.tmdbAPIKey, title)

	resp, err := ft.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result TMDBSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (ft *FrenchTranslator) fetchFrenchMetadata(tmdbID int, mediaType string) (*TMDBDetail, error) {
	endpoint := "movie"
	if mediaType == "series" || mediaType == "tv" {
		endpoint = "tv"
	}

	url := fmt.Sprintf("https://api.themoviedb.org/3/%s/%d?api_key=%s&language=fr-FR",
		endpoint, tmdbID, ft.tmdbAPIKey)

	resp, err := ft.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var detail TMDBDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

func (ft *FrenchTranslator) extractTitle(detail *TMDBDetail, originalTitle string) string {
	var frenchTitle string
	
	if detail.Title != "" {
		frenchTitle = detail.Title
	} else if detail.Name != "" {
		frenchTitle = detail.Name
	}

	if frenchTitle == "" || strings.EqualFold(frenchTitle, originalTitle) {
		return originalTitle
	}

	return frenchTitle
}

func (ft *FrenchTranslator) TranslateTitles(titles []string, mediaType string) map[string]string {
	results := make(map[string]string)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, title := range titles {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			translated := ft.TranslateTitle(t, mediaType)
			mu.Lock()
			results[t] = translated
			mu.Unlock()
		}(title)
	}

	wg.Wait()
	return results
}