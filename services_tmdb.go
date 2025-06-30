package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type TMDBFindResponse struct {
	MovieResults []TMDBMovie `json:"movie_results"`
	TVResults    []TMDBTV    `json:"tv_results"`
}

type TMDBMovie struct {
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
}

type TMDBTV struct {
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
}

type TMDBData struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	FrenchTitle string `json:"french_title"`
}

func GetTMDBData(imdbId string, config *Config) (*TMDBData, error) {
	// Check cache first
	cachedData, err := GetCachedTMDB(imdbId)
	if err == nil && cachedData != nil {
		Logger.Info("✅ TMDB cache hit for IMDB ID: " + imdbId)
		return &TMDBData{
			Type:        cachedData.Type,
			Title:       cachedData.Title,
			FrenchTitle: cachedData.FrenchTitle,
		}, nil
	}

	// Make API request
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/find/%s", imdbId)
	params := url.Values{}
	params.Add("api_key", config.TMDBAPIKey)
	params.Add("external_source", "imdb_id")

	resp, err := http.Get(apiURL + "?" + params.Encode())
	if err != nil {
		Logger.Error("❌ TMDB API request failed: ", err)
		return nil, err
	}
	defer resp.Body.Close()

	var findResponse TMDBFindResponse
	if err := json.NewDecoder(resp.Body).Decode(&findResponse); err != nil {
		Logger.Error("❌ Failed to decode TMDB response: ", err)
		return nil, err
	}

	// Check if the result is a movie
	if len(findResponse.MovieResults) > 0 {
		movie := findResponse.MovieResults[0]
		title := movie.Title
		frenchTitle := movie.OriginalTitle

		Logger.Info(fmt.Sprintf("✅ Movie found: %s (FR Title: %s)", title, frenchTitle))

		// Store in cache
		if err := StoreTMDB(imdbId, "movie", title, frenchTitle); err != nil {
			Logger.Warn("Failed to cache TMDB data: ", err)
		}

		return &TMDBData{
			Type:        "movie",
			Title:       title,
			FrenchTitle: frenchTitle,
		}, nil
	}

	// Check if the result is a TV series
	if len(findResponse.TVResults) > 0 {
		tv := findResponse.TVResults[0]
		title := tv.Name
		frenchTitle := tv.OriginalName

		Logger.Info(fmt.Sprintf("✅ Series found: %s (FR Title: %s)", title, frenchTitle))

		// Store in cache
		if err := StoreTMDB(imdbId, "series", title, frenchTitle); err != nil {
			Logger.Warn("Failed to cache TMDB data: ", err)
		}

		return &TMDBData{
			Type:        "series",
			Title:       title,
			FrenchTitle: frenchTitle,
		}, nil
	}

	// Return nil if no data is found
	Logger.Warn("⚠️ No TMDB data found for IMDB ID: " + imdbId)
	return nil, nil
}