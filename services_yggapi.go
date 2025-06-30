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

type YggTorrent struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Hash   string `json:"hash,omitempty"`
	Source string `json:"source,omitempty"`
}

type TorrentResults struct {
	CompleteSeriesTorrents []YggTorrent `json:"completeSeriesTorrents"`
	CompleteSeasonTorrents []YggTorrent `json:"completeSeasonTorrents"`
	EpisodeTorrents        []YggTorrent `json:"episodeTorrents"`
	MovieTorrents          []YggTorrent `json:"movieTorrents"`
}

type Priority struct {
	Resolution int
	Language   int
	Codec      int
}

func GetTorrentHashFromYgg(torrentId int) (string, error) {
	apiURL := fmt.Sprintf("https://yggapi.eu/torrent/%d", torrentId)
	
	resp, err := http.Get(apiURL)
	if err != nil {
		Logger.Error(fmt.Sprintf("❌ Hash Retrieval Error for %d: %v", torrentId, err))
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Hash string `json:"hash"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		Logger.Error(fmt.Sprintf("❌ Failed to decode hash response for %d: %v", torrentId, err))
		return "", err
	}

	return result.Hash, nil
}

func ProcessTorrents(torrents []YggTorrent, mediaType, season, episode string, config *Config) TorrentResults {
	var results TorrentResults

	if mediaType == "movie" {
		Logger.Debug("🔍 Searching for movies")
		for _, torrent := range torrents {
			if matchesFilters(torrent, config) {
				torrent.Source = "YGG"
				results.MovieTorrents = append(results.MovieTorrents, torrent)
			}
		}
		Logger.Debug(fmt.Sprintf("🎬 %d movie torrents found.", len(results.MovieTorrents)))
	}

	if mediaType == "series" {
		Logger.Debug("🔍 Searching for complete series with the word \"COMPLETE\"")
		for _, torrent := range torrents {
			if matchesFilters(torrent, config) && strings.Contains(strings.ToUpper(torrent.Title), "COMPLETE") {
				torrent.Source = "YGG"
				results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, torrent)
			}
		}
		Logger.Debug(fmt.Sprintf("🎬 %d complete series torrents found.", len(results.CompleteSeriesTorrents)))
	}

	if mediaType == "series" && season != "" {
		seasonFormatted := PadString(season, 2)
		Logger.Debug(fmt.Sprintf("🔍 Searching for complete season: S%s", seasonFormatted))
		
		seasonPattern := regexp.MustCompile(fmt.Sprintf(`(?i)s%se\d{2}`, seasonFormatted))
		seasonDotPattern := regexp.MustCompile(fmt.Sprintf(`(?i)s%s\.e\d{2}`, seasonFormatted))
		
		for _, torrent := range torrents {
			if matchesFilters(torrent, config) &&
				strings.Contains(strings.ToLower(torrent.Title), fmt.Sprintf("s%s", seasonFormatted)) &&
				!seasonPattern.MatchString(strings.ToLower(torrent.Title)) &&
				!seasonDotPattern.MatchString(strings.ToLower(torrent.Title)) {
				torrent.Source = "YGG"
				results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrent)
			}
		}
		Logger.Debug(fmt.Sprintf("🎬 %d complete season torrents found.", len(results.CompleteSeasonTorrents)))
	}

	if mediaType == "series" && season != "" && episode != "" {
		seasonFormatted := PadString(season, 2)
		episodeFormatted := PadString(episode, 2)
		Logger.Debug(fmt.Sprintf("🔍 Searching for specific episode: S%sE%s", seasonFormatted, episodeFormatted))
		
		for _, torrent := range torrents {
			if matchesFilters(torrent, config) {
				titleLower := strings.ToLower(torrent.Title)
				pattern1 := fmt.Sprintf("s%se%s", seasonFormatted, episodeFormatted)
				pattern2 := fmt.Sprintf("s%s.e%s", seasonFormatted, episodeFormatted)
				
				if strings.Contains(titleLower, pattern1) || strings.Contains(titleLower, pattern2) {
					torrent.Source = "YGG"
					results.EpisodeTorrents = append(results.EpisodeTorrents, torrent)
				}
			}
		}
		Logger.Debug(fmt.Sprintf("🎬 %d episode torrents found.", len(results.EpisodeTorrents)))
	}

	return results
}

func SearchYgg(title, mediaType, season, episode string, config *Config, titleFR string) (TorrentResults, error) {
	Logger.Debug("🔍 Searching for torrents on YggTorrent")
	
	torrents, err := performYggSearch(title, mediaType, config)
	if err != nil {
		return TorrentResults{}, err
	}

	if len(torrents) == 0 && titleFR != "" && title != titleFR {
		Logger.Warn(fmt.Sprintf("📢 No results found with \"%s\", trying with \"%s\"", title, titleFR))
		torrents, err = performYggSearch(titleFR, mediaType, config)
		if err != nil {
			return TorrentResults{}, err
		}
	}

	if len(torrents) == 0 {
		Logger.Error(fmt.Sprintf("❌ No torrents found for %s", title))
		return TorrentResults{}, nil
	}

	return ProcessTorrents(torrents, mediaType, season, episode, config), nil
}

func performYggSearch(searchTitle, mediaType string, config *Config) ([]YggTorrent, error) {
	var categoryIds []int
	if mediaType == "movie" {
		categoryIds = []int{2178, 2181, 2183}
	} else {
		categoryIds = []int{2179, 2181, 2182, 2184}
	}

	params := url.Values{}
	params.Add("q", searchTitle)
	params.Add("page", "1")
	params.Add("per_page", "100")
	params.Add("order_by", "uploaded_at")
	
	for _, id := range categoryIds {
		params.Add("category_id", fmt.Sprintf("%d", id))
	}

	requestURL := "https://yggapi.eu/torrents?" + params.Encode()
	Logger.Debug("🔍 Performing YGG search with URL: " + requestURL)

	resp, err := http.Get(requestURL)
	if err != nil {
		Logger.Error("❌ Ygg Search Error: ", err)
		return nil, err
	}
	defer resp.Body.Close()

	var torrents []YggTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		Logger.Error("❌ Failed to decode YGG response: ", err)
		return nil, err
	}

	Logger.Info(fmt.Sprintf("✅ Found %d torrents on YggTorrent for \"%s\".", len(torrents), searchTitle))

	// Sort torrents by priority
	sort.Slice(torrents, func(i, j int) bool {
		priorityA := prioritizeTorrent(torrents[i], config)
		priorityB := prioritizeTorrent(torrents[j], config)

		if priorityA.Resolution != priorityB.Resolution {
			return priorityA.Resolution < priorityB.Resolution
		}
		if priorityA.Language != priorityB.Language {
			return priorityA.Language < priorityB.Language
		}
		return priorityA.Codec < priorityB.Codec
	})

	return torrents, nil
}

func matchesFilters(torrent YggTorrent, config *Config) bool {
	titleLower := strings.ToLower(torrent.Title)
	
	// Check resolution
	resMatch := false
	for _, res := range config.ResToShow {
		if strings.Contains(titleLower, strings.ToLower(res)) {
			resMatch = true
			break
		}
	}
	
	// Check language
	langMatch := false
	for _, lang := range config.LangToShow {
		if strings.Contains(titleLower, strings.ToLower(lang)) {
			langMatch = true
			break
		}
	}
	
	// Check codec
	codecMatch := false
	for _, codec := range config.CodecsToShow {
		if strings.Contains(titleLower, strings.ToLower(codec)) {
			codecMatch = true
			break
		}
	}
	
	return resMatch && langMatch && codecMatch
}

func prioritizeTorrent(torrent YggTorrent, config *Config) Priority {
	titleLower := strings.ToLower(torrent.Title)
	
	resolutionPriority := len(config.ResToShow) // Default to end if not found
	for i, res := range config.ResToShow {
		if strings.Contains(titleLower, strings.ToLower(res)) {
			resolutionPriority = i
			break
		}
	}
	
	languagePriority := len(config.LangToShow)
	for i, lang := range config.LangToShow {
		if strings.Contains(titleLower, strings.ToLower(lang)) {
			languagePriority = i
			break
		}
	}
	
	codecPriority := len(config.CodecsToShow)
	for i, codec := range config.CodecsToShow {
		if strings.Contains(titleLower, strings.ToLower(codec)) {
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