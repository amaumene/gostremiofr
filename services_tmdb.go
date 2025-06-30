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
		Logger.Debugf("tmdb cache hit for imdb id: %s", imdbId)
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
		Logger.Errorf("tmdb api request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var findResponse TMDBFindResponse
	if err := json.NewDecoder(resp.Body).Decode(&findResponse); err != nil {
		Logger.Errorf("failed to decode tmdb response: %v", err)
		return nil, err
	}

	// Check if the result is a movie
	if len(findResponse.MovieResults) > 0 {
		movie := findResponse.MovieResults[0]
		title := movie.Title
		frenchTitle := movie.OriginalTitle

		Logger.Infof("movie found: %s (french title: %s)", title, frenchTitle)

		// Store in cache
		if err := StoreTMDB(imdbId, "movie", title, frenchTitle); err != nil {
			Logger.Warnf("failed to cache tmdb data: %v", err)
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

		Logger.Infof("series found: %s (french title: %s)", title, frenchTitle)

		// Store in cache
		if err := StoreTMDB(imdbId, "series", title, frenchTitle); err != nil {
			Logger.Warnf("failed to cache tmdb data: %v", err)
		}

		return &TMDBData{
			Type:        "series",
			Title:       title,
			FrenchTitle: frenchTitle,
		}, nil
	}

	// Return nil if no data is found
	Logger.Warnf("no tmdb data found for imdb id: %s", imdbId)
	return nil, nil
}