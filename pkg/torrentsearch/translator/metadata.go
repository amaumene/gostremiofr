package translator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type MetadataFetcher struct {
	tmdbAPIKey string
	httpClient *http.Client
	cache      Cache
	mu         sync.RWMutex
}

type ContentMetadata struct {
	OriginalTitle    string
	OriginalLanguage string
	EnglishTitle     string
	FrenchTitle      string
	Year             int
}

type TMDBSearchResponse struct {
	Results []struct {
		ID               int    `json:"id"`
		Title            string `json:"title,omitempty"`           // For movies
		Name             string `json:"name,omitempty"`            // For TV shows
		OriginalTitle    string `json:"original_title,omitempty"`  // For movies
		OriginalName     string `json:"original_name,omitempty"`   // For TV shows
		OriginalLanguage string `json:"original_language"`
		ReleaseDate      string `json:"release_date,omitempty"`
		FirstAirDate     string `json:"first_air_date,omitempty"`
	} `json:"results"`
}

type TMDBMovieDetail struct {
	Title            string `json:"title"`
	OriginalTitle    string `json:"original_title"`
	OriginalLanguage string `json:"original_language"`
	ReleaseDate      string `json:"release_date"`
}

type TMDBTVDetail struct {
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	OriginalLanguage string `json:"original_language"`
	FirstAirDate     string `json:"first_air_date"`
}

type TMDBFindResponse struct {
	MovieResults []struct {
		ID               int    `json:"id"`
		OriginalLanguage string `json:"original_language"`
		ReleaseDate      string `json:"release_date"`
	} `json:"movie_results"`
	TVResults []struct {
		ID               int    `json:"id"`
		OriginalLanguage string `json:"original_language"`
		FirstAirDate     string `json:"first_air_date"`
	} `json:"tv_results"`
}

func NewMetadataFetcher(apiKey string, cache Cache) *MetadataFetcher {
	return &MetadataFetcher{
		tmdbAPIKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: cache,
	}
}

func (mf *MetadataFetcher) FetchMetadata(query string, mediaType string) (*ContentMetadata, error) {
	if mf.tmdbAPIKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	cacheKey := fmt.Sprintf("metadata:%s:%s", mediaType, query)
	if mf.cache != nil {
		if cached, found := mf.cache.Get(cacheKey); found {
			if metadata, ok := cached.(*ContentMetadata); ok {
				return metadata, nil
			}
		}
	}

	// Search for the content
	searchResult, err := mf.searchTMDB(query, mediaType)
	if err != nil || len(searchResult.Results) == 0 {
		return nil, fmt.Errorf("content not found on TMDB")
	}

	result := searchResult.Results[0]
	tmdbID := result.ID
	
	metadata := &ContentMetadata{
		OriginalLanguage: result.OriginalLanguage,
	}

	// Extract year
	if result.ReleaseDate != "" {
		fmt.Sscanf(result.ReleaseDate, "%d", &metadata.Year)
	} else if result.FirstAirDate != "" {
		fmt.Sscanf(result.FirstAirDate, "%d", &metadata.Year)
	}

	// Fetch titles in different languages
	var wg sync.WaitGroup
	wg.Add(2)

	// Fetch English title
	go func() {
		defer wg.Done()
		if englishTitle := mf.fetchTitleInLanguage(tmdbID, mediaType, "en-US"); englishTitle != "" {
			metadata.EnglishTitle = englishTitle
		}
	}()

	// Fetch French title
	go func() {
		defer wg.Done()
		if frenchTitle := mf.fetchTitleInLanguage(tmdbID, mediaType, "fr-FR"); frenchTitle != "" {
			metadata.FrenchTitle = frenchTitle
		}
	}()

	wg.Wait()

	// Set original title based on original language
	if result.OriginalTitle != "" {
		metadata.OriginalTitle = result.OriginalTitle
	} else if result.OriginalName != "" {
		metadata.OriginalTitle = result.OriginalName
	}

	// Fallback: if no English title was found, use the original
	if metadata.EnglishTitle == "" {
		if result.Title != "" {
			metadata.EnglishTitle = result.Title
		} else if result.Name != "" {
			metadata.EnglishTitle = result.Name
		}
	}

	if mf.cache != nil {
		mf.cache.Set(cacheKey, metadata)
	}

	return metadata, nil
}

// FetchMetadataByIMDBID fetches metadata using IMDB ID lookup
func (mf *MetadataFetcher) FetchMetadataByIMDBID(imdbID string, mediaType string) (*ContentMetadata, error) {
	if mf.tmdbAPIKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	cacheKey := fmt.Sprintf("metadata:imdb:%s:%s", mediaType, imdbID)
	if mf.cache != nil {
		if cached, found := mf.cache.Get(cacheKey); found {
			if metadata, ok := cached.(*ContentMetadata); ok {
				return metadata, nil
			}
		}
	}

	// Use find API to get content by IMDB ID
	findResult, err := mf.findByIMDBID(imdbID)
	if err != nil {
		return nil, err
	}

	var tmdbID int
	var originalLanguage string
	var year int

	// Extract data from find result
	if mediaType == "movie" && len(findResult.MovieResults) > 0 {
		movie := findResult.MovieResults[0]
		tmdbID = movie.ID
		originalLanguage = movie.OriginalLanguage
		if movie.ReleaseDate != "" {
			fmt.Sscanf(movie.ReleaseDate, "%d", &year)
		}
	} else if (mediaType == "series" || mediaType == "tv") && len(findResult.TVResults) > 0 {
		tv := findResult.TVResults[0]
		tmdbID = tv.ID
		originalLanguage = tv.OriginalLanguage
		if tv.FirstAirDate != "" {
			fmt.Sscanf(tv.FirstAirDate, "%d", &year)
		}
	} else {
		return nil, fmt.Errorf("content not found on TMDB for IMDB ID %s", imdbID)
	}

	metadata := &ContentMetadata{
		OriginalLanguage: originalLanguage,
		Year:             year,
	}

	// Fetch titles in different languages
	var wg sync.WaitGroup
	wg.Add(2)

	// Fetch English title
	go func() {
		defer wg.Done()
		if englishTitle := mf.fetchTitleInLanguage(tmdbID, mediaType, "en-US"); englishTitle != "" {
			metadata.EnglishTitle = englishTitle
		}
	}()

	// Fetch French title
	go func() {
		defer wg.Done()
		if frenchTitle := mf.fetchTitleInLanguage(tmdbID, mediaType, "fr-FR"); frenchTitle != "" {
			metadata.FrenchTitle = frenchTitle
		}
	}()

	wg.Wait()

	if mf.cache != nil {
		mf.cache.Set(cacheKey, metadata)
	}

	return metadata, nil
}

func (mf *MetadataFetcher) searchTMDB(query string, mediaType string) (*TMDBSearchResponse, error) {
	endpoint := "movie"
	if mediaType == "series" || mediaType == "tv" {
		endpoint = "tv"
	}

	url := fmt.Sprintf("https://api.themoviedb.org/3/search/%s?api_key=%s&query=%s",
		endpoint, mf.tmdbAPIKey, query)

	resp, err := mf.httpClient.Get(url)
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

	var result TMDBSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (mf *MetadataFetcher) findByIMDBID(imdbID string) (*TMDBFindResponse, error) {
	url := fmt.Sprintf("https://api.themoviedb.org/3/find/%s?api_key=%s&external_source=imdb_id",
		imdbID, mf.tmdbAPIKey)

	resp, err := mf.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB Find API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result TMDBFindResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (mf *MetadataFetcher) fetchTitleInLanguage(tmdbID int, mediaType string, language string) string {
	endpoint := "movie"
	if mediaType == "series" || mediaType == "tv" {
		endpoint = "tv"
	}

	url := fmt.Sprintf("https://api.themoviedb.org/3/%s/%d?api_key=%s&language=%s",
		endpoint, tmdbID, mf.tmdbAPIKey, language)

	resp, err := mf.httpClient.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	if mediaType == "movie" {
		var detail TMDBMovieDetail
		if err := json.Unmarshal(body, &detail); err != nil {
			return ""
		}
		return detail.Title
	} else {
		var detail TMDBTVDetail
		if err := json.Unmarshal(body, &detail); err != nil {
			return ""
		}
		return detail.Name
	}
}