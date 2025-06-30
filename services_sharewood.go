package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type SharewoodTorrent struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	InfoHash    string `json:"info_hash"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
	Language    string `json:"language"`
	DownloadURL string `json:"download_url"`
	CreatedAt   string `json:"created_at"`
	Source      string `json:"source,omitempty"`
}

type SharewoodResults struct {
	CompleteSeriesTorrents []SharewoodTorrent `json:"completeSeriesTorrents"`
	CompleteSeasonTorrents []SharewoodTorrent `json:"completeSeasonTorrents"`
	EpisodeTorrents        []SharewoodTorrent `json:"episodeTorrents"`
	MovieTorrents          []SharewoodTorrent `json:"movieTorrents"`
}

var subcategoryMap = map[string][]int{
	"movie":  {9, 11},
	"series": {10, 12},
}

func SearchSharewood(title, mediaType, season, episode string, config *Config) (SharewoodResults, error) {
	subcategories, exists := subcategoryMap[mediaType]
	if !exists {
		Logger.Error(fmt.Sprintf("❌ Invalid type \"%s\" for Sharewood search.", mediaType))
		return SharewoodResults{}, nil
	}

	var subcategoryParams []string
	for _, id := range subcategories {
		subcategoryParams = append(subcategoryParams, fmt.Sprintf("subcategory_id=%d", id))
	}

	seasonFormatted := ""
	if season != "" {
		seasonFormatted = fmt.Sprintf(" S%s", PadString(season, 2))
	}

	params := url.Values{}
	params.Add("name", title+seasonFormatted)
	params.Add("category", "1")
	for _, param := range subcategoryParams {
		parts := strings.Split(param, "=")
		params.Add(parts[0], parts[1])
	}

	requestURL := fmt.Sprintf("https://www.sharewood.tv/api/%s/search?%s", 
		config.SharewoodPasskey, params.Encode())

	Logger.Debug("🔍 Performing Sharewood search with URL: " + requestURL)

	resp, err := http.Get(requestURL)
	if err != nil {
		Logger.Error("❌ Sharewood Search Error: ", err)
		return SharewoodResults{}, err
	}
	defer resp.Body.Close()

	var torrents []SharewoodTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		Logger.Error("❌ Failed to decode Sharewood response: ", err)
		return SharewoodResults{}, err
	}

	Logger.Info(fmt.Sprintf("✅ Found %d torrents on Sharewood for \"%s\".", len(torrents), title))

	return processSharewoodTorrents(torrents, mediaType, season, episode, config), nil
}

func processSharewoodTorrents(torrents []SharewoodTorrent, mediaType, season, episode string, config *Config) SharewoodResults {
	var results SharewoodResults

	// Sort torrents by priority
	sort.Slice(torrents, func(i, j int) bool {
		priorityA := prioritizeSharewoodTorrent(torrents[i], config)
		priorityB := prioritizeSharewoodTorrent(torrents[j], config)

		if priorityA.Resolution != priorityB.Resolution {
			return priorityA.Resolution < priorityB.Resolution
		}
		if priorityA.Language != priorityB.Language {
			return priorityA.Language < priorityB.Language
		}
		return priorityA.Codec < priorityB.Codec
	})

	if mediaType == "movie" {
		Logger.Debug("🔍 Filtering movies")
		for _, torrent := range torrents {
			if matchesSharewoodFilters(torrent, config) {
				torrent.Source = "SW"
				results.MovieTorrents = append(results.MovieTorrents, torrent)
			}
		}
		Logger.Debug(fmt.Sprintf("🎬 %d movie torrents found.", len(results.MovieTorrents)))
	}

	if mediaType == "series" {
		if season != "" {
			seasonFormatted := PadString(season, 2)
			Logger.Debug(fmt.Sprintf("🔍 Filtering complete seasons: S%s", seasonFormatted))
			
			seasonPattern := regexp.MustCompile(fmt.Sprintf(`(?i)s%se\d{2}`, seasonFormatted))
			seasonDotPattern := regexp.MustCompile(fmt.Sprintf(`(?i)s%s\.e\d{2}`, seasonFormatted))
			
			for _, torrent := range torrents {
				nameLower := strings.ToLower(torrent.Name)
				if strings.Contains(nameLower, fmt.Sprintf("s%s", seasonFormatted)) &&
					!seasonPattern.MatchString(nameLower) &&
					!seasonDotPattern.MatchString(nameLower) {
					torrent.Source = "SW"
					results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrent)
				}
			}
			Logger.Debug(fmt.Sprintf("🎬 %d complete season torrents found.", len(results.CompleteSeasonTorrents)))
		}

		if season != "" && episode != "" {
			seasonFormatted := PadString(season, 2)
			episodeFormatted := PadString(episode, 2)
			patterns := []string{
				fmt.Sprintf("s%se%s", seasonFormatted, episodeFormatted),
				fmt.Sprintf("s%s.e%s", seasonFormatted, episodeFormatted),
			}

			Logger.Debug(fmt.Sprintf("🔍 Filtering specific episodes: Patterns %s", strings.Join(patterns, ", ")))
			
			for _, torrent := range torrents {
				nameLower := strings.ToLower(torrent.Name)
				for _, pattern := range patterns {
					if strings.Contains(nameLower, pattern) {
						torrent.Source = "SW"
						results.EpisodeTorrents = append(results.EpisodeTorrents, torrent)
						break
					}
				}
			}
			Logger.Debug(fmt.Sprintf("🎬 %d episode torrents found.", len(results.EpisodeTorrents)))
		}
	}

	return results
}

func matchesSharewoodFilters(torrent SharewoodTorrent, config *Config) bool {
	nameLower := strings.ToLower(torrent.Name)
	languageLower := strings.ToLower(torrent.Language)
	
	// Check resolution
	resMatch := false
	for _, res := range config.ResToShow {
		if strings.Contains(nameLower, strings.ToLower(res)) {
			resMatch = true
			break
		}
	}
	
	// Check language
	langMatch := false
	for _, lang := range config.LangToShow {
		if strings.Contains(languageLower, strings.ToLower(lang)) {
			langMatch = true
			break
		}
	}
	
	// Check codec
	codecMatch := false
	for _, codec := range config.CodecsToShow {
		if strings.Contains(nameLower, strings.ToLower(codec)) {
			codecMatch = true
			break
		}
	}
	
	return resMatch && langMatch && codecMatch
}

func prioritizeSharewoodTorrent(torrent SharewoodTorrent, config *Config) Priority {
	nameLower := strings.ToLower(torrent.Name)
	languageLower := strings.ToLower(torrent.Language)
	
	resolutionPriority := len(config.ResToShow)
	for i, res := range config.ResToShow {
		if strings.Contains(nameLower, strings.ToLower(res)) {
			resolutionPriority = i
			break
		}
	}
	
	languagePriority := len(config.LangToShow)
	for i, lang := range config.LangToShow {
		if strings.Contains(languageLower, strings.ToLower(lang)) {
			languagePriority = i
			break
		}
	}
	
	codecPriority := len(config.CodecsToShow)
	for i, codec := range config.CodecsToShow {
		if strings.Contains(nameLower, strings.ToLower(codec)) {
			codecPriority = i
			break
		}
	}
	
	return Priority{
		Resolution: resolutionPriority,
		Language:   languagePriority,
		Codec:      codecPriority,
	}
}