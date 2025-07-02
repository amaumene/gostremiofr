package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gostremiofr/gostremiofr/internal/cache"
	"github.com/gostremiofr/gostremiofr/internal/config"
	"github.com/gostremiofr/gostremiofr/internal/database"
	"github.com/gostremiofr/gostremiofr/internal/models"
	"github.com/gostremiofr/gostremiofr/pkg/logger"
	"github.com/gostremiofr/gostremiofr/pkg/ratelimiter"
)

var (
	seasonEpisodeRegex    *regexp.Regexp
	seasonDotEpisodeRegex *regexp.Regexp
	regexOnce            sync.Once
)

func initRegexPatterns() {
	regexOnce.Do(func() {
		seasonEpisodeRegex = regexp.MustCompile(`(?i)s\d{2}e\d{2}`)
		seasonDotEpisodeRegex = regexp.MustCompile(`(?i)s\d{2}\.e\d{2}`)
	})
}

type YGG struct {
	db          *database.DB
	cache       *cache.LRUCache
	config      *config.Config
	rateLimiter *ratelimiter.TokenBucket
	httpClient  *http.Client
	logger      logger.Logger
}

func NewYGG(db *database.DB, cache *cache.LRUCache) *YGG {
	initRegexPatterns()
	return &YGG{
		db:          db,
		cache:       cache,
		rateLimiter: ratelimiter.NewTokenBucket(10, 2),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.New(),
	}
}

func (y *YGG) SetConfig(cfg *config.Config) {
	y.config = cfg
}

func (y *YGG) SearchTorrents(query string, category string) (*models.TorrentResults, error) {
	y.rateLimiter.Wait()
	
	encodedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://yggapi.eu/search?q=%s&category=%s&limit=100", encodedQuery, category)
	
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search YGG: %w", err)
	}
	defer resp.Body.Close()
	
	var torrents []models.YggTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("failed to decode YGG response: %w", err)
	}
	
	for i := range torrents {
		torrents[i].Source = "YGG"
	}
	
	return y.processTorrents(torrents, category), nil
}

func (y *YGG) GetTorrentHash(torrentID string) (string, error) {
	y.rateLimiter.Wait()
	
	apiURL := fmt.Sprintf("https://yggapi.eu/torrent/%s", torrentID)
	
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to get torrent hash: %w", err)
	}
	defer resp.Body.Close()
	
	var result struct {
		Hash string `json:"hash"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode hash response: %w", err)
	}
	
	return result.Hash, nil
}

func (y *YGG) processTorrents(torrents []models.YggTorrent, category string) *models.TorrentResults {
	results := &models.TorrentResults{}
	
	for _, torrent := range torrents {
		if !y.matchesFilters(torrent) {
			continue
		}
		
		torrentInfo := models.TorrentInfo{
			ID:     fmt.Sprintf("%d", torrent.ID),
			Title:  torrent.Title,
			Hash:   torrent.Hash,
			Source: torrent.Source,
		}
		
		if category == "movie" {
			results.MovieTorrents = append(results.MovieTorrents, torrentInfo)
		} else if category == "series" {
			if strings.Contains(strings.ToUpper(torrent.Title), "COMPLETE") {
				results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, torrentInfo)
			} else if seasonEpisodeRegex.MatchString(torrent.Title) || seasonDotEpisodeRegex.MatchString(torrent.Title) {
				results.EpisodeTorrents = append(results.EpisodeTorrents, torrentInfo)
			} else if containsSeason(torrent.Title) {
				results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrentInfo)
			}
		}
	}
	
	y.sortResults(results)
	
	return results
}

func (y *YGG) matchesFilters(torrent models.YggTorrent) bool {
	if y.config == nil {
		return true
	}
	
	parsed := parseFileName(torrent.Title)
	
	if parsed.Resolution != "?" && !y.config.IsResolutionAllowed(parsed.Resolution) {
		return false
	}
	
	if parsed.Codec != "?" && !y.config.IsCodecAllowed(parsed.Codec) {
		return false
	}
	
	langAllowed := false
	for _, lang := range y.config.LangToShow {
		if containsLanguage(torrent.Title, lang) {
			langAllowed = true
			break
		}
	}
	
	return langAllowed || len(y.config.LangToShow) == 0
}

func (y *YGG) sortResults(results *models.TorrentResults) {
	sortFunc := func(torrents []models.TorrentInfo) {
		sort.Slice(torrents, func(i, j int) bool {
			priorityI := y.getTorrentPriority(torrents[i].Title)
			priorityJ := y.getTorrentPriority(torrents[j].Title)
			
			if priorityI.Resolution != priorityJ.Resolution {
				return priorityI.Resolution > priorityJ.Resolution
			}
			if priorityI.Language != priorityJ.Language {
				return priorityI.Language > priorityJ.Language
			}
			if priorityI.Codec != priorityJ.Codec {
				return priorityI.Codec > priorityJ.Codec
			}
			
			return false
		})
	}
	
	sortFunc(results.MovieTorrents)
	sortFunc(results.CompleteSeriesTorrents)
	sortFunc(results.CompleteSeasonTorrents)
	sortFunc(results.EpisodeTorrents)
}

func (y *YGG) getTorrentPriority(title string) models.Priority {
	priority := models.Priority{}
	titleLower := strings.ToLower(title)
	
	switch {
	case strings.Contains(titleLower, "2160p") || strings.Contains(titleLower, "4k"):
		priority.Resolution = 4
	case strings.Contains(titleLower, "1080p"):
		priority.Resolution = 3
	case strings.Contains(titleLower, "720p"):
		priority.Resolution = 2
	default:
		priority.Resolution = 1
	}
	
	if strings.Contains(titleLower, "multi") {
		priority.Language = 3
	} else if strings.Contains(titleLower, "french") || strings.Contains(titleLower, "vff") {
		priority.Language = 2
	} else {
		priority.Language = 1
	}
	
	switch {
	case strings.Contains(titleLower, "h265") || strings.Contains(titleLower, "x265") || strings.Contains(titleLower, "hevc"):
		priority.Codec = 3
	case strings.Contains(titleLower, "h264") || strings.Contains(titleLower, "x264"):
		priority.Codec = 2
	default:
		priority.Codec = 1
	}
	
	return priority
}

func parseFileName(fileName string) models.ParsedFileName {
	var result models.ParsedFileName
	
	resolutionRegex := regexp.MustCompile(`(?i)(4k|\d{3,4}p)`)
	if match := resolutionRegex.FindString(fileName); match != "" {
		result.Resolution = match
	} else {
		result.Resolution = "?"
	}
	
	codecRegex := regexp.MustCompile(`(?i)(h\.264|h\.265|x\.264|x\.265|h264|h265|x264|x265|AV1|HEVC)`)
	if match := codecRegex.FindString(fileName); match != "" {
		result.Codec = match
	} else {
		result.Codec = "?"
	}
	
	sourceRegex := regexp.MustCompile(`(?i)(BluRay|WEB[-]?DL|WEB|HDRip|DVDRip|BRRip)`)
	if match := sourceRegex.FindString(fileName); match != "" {
		result.Source = match
	} else {
		result.Source = "?"
	}
	
	return result
}

func containsSeason(title string) bool {
	seasonRegex := regexp.MustCompile(`(?i)(saison|season)\s*\d+`)
	return seasonRegex.MatchString(title)
}

func containsLanguage(title, language string) bool {
	titleLower := strings.ToLower(title)
	langLower := strings.ToLower(language)
	
	if langLower == "multi" {
		return strings.Contains(titleLower, "multi")
	} else if langLower == "french" {
		return strings.Contains(titleLower, "french") || 
			   strings.Contains(titleLower, "vff") || 
			   strings.Contains(titleLower, "truefrench")
	} else if langLower == "english" {
		return strings.Contains(titleLower, "english") || 
			   strings.Contains(titleLower, "vostfr")
	}
	
	return strings.Contains(titleLower, langLower)
}