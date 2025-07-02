package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gostremiofr/gostremiofr/internal/cache"
	"github.com/gostremiofr/gostremiofr/internal/config"
	"github.com/gostremiofr/gostremiofr/internal/database"
	"github.com/gostremiofr/gostremiofr/internal/models"
	"github.com/gostremiofr/gostremiofr/pkg/logger"
	"github.com/gostremiofr/gostremiofr/pkg/ratelimiter"
)

type Sharewood struct {
	passkey     string
	db          *database.DB
	cache       *cache.LRUCache
	config      *config.Config
	rateLimiter *ratelimiter.TokenBucket
	httpClient  *http.Client
	logger      logger.Logger
}

func NewSharewood(passkey string, db *database.DB, cache *cache.LRUCache) *Sharewood {
	return &Sharewood{
		passkey:     passkey,
		db:          db,
		cache:       cache,
		rateLimiter: ratelimiter.NewTokenBucket(5, 1),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.New(),
	}
}

func (s *Sharewood) SetConfig(cfg *config.Config) {
	s.config = cfg
}

func (s *Sharewood) SearchTorrents(query string, mediaType string, season, episode int) (*models.SharewoodResults, error) {
	if s.passkey == "" {
		return &models.SharewoodResults{}, nil
	}
	
	s.rateLimiter.Wait()
	
	encodedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://www.sharewood.tv/api/torrents?passkey=%s&search=%s&limit=100", 
		s.passkey, encodedQuery)
	
	resp, err := s.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search Sharewood: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Sharewood API error: status %d", resp.StatusCode)
	}
	
	var apiResponse struct {
		Torrents []models.SharewoodTorrent `json:"torrents"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode Sharewood response: %w", err)
	}
	
	for i := range apiResponse.Torrents {
		apiResponse.Torrents[i].Source = "Sharewood"
	}
	
	return s.processTorrents(apiResponse.Torrents, mediaType, season, episode), nil
}

func (s *Sharewood) processTorrents(torrents []models.SharewoodTorrent, mediaType string, season, episode int) *models.SharewoodResults {
	results := &models.SharewoodResults{}
	
	for _, torrent := range torrents {
		if !s.matchesFilters(torrent) {
			continue
		}
		
		if mediaType == "movie" && torrent.Type == "movie" {
			results.MovieTorrents = append(results.MovieTorrents, torrent)
		} else if mediaType == "series" && torrent.Type == "tvshow" {
			if strings.Contains(strings.ToUpper(torrent.Name), "COMPLETE") {
				results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, torrent)
			} else if s.matchesEpisode(torrent.Name, season, episode) {
				results.EpisodeTorrents = append(results.EpisodeTorrents, torrent)
			} else if s.matchesSeason(torrent.Name, season) {
				results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrent)
			}
		}
	}
	
	s.sortResults(results)
	
	return results
}

func (s *Sharewood) matchesFilters(torrent models.SharewoodTorrent) bool {
	if s.config == nil {
		return true
	}
	
	parsed := s.parseFileName(torrent.Name)
	
	if parsed.Resolution != "?" && !s.config.IsResolutionAllowed(parsed.Resolution) {
		return false
	}
	
	if parsed.Codec != "?" && !s.config.IsCodecAllowed(parsed.Codec) {
		return false
	}
	
	if torrent.Language != "" && !s.config.IsLanguageAllowed(torrent.Language) {
		return false
	}
	
	return true
}

func (s *Sharewood) matchesEpisode(title string, season, episode int) bool {
	if season == 0 || episode == 0 {
		return false
	}
	
	patterns := []string{
		fmt.Sprintf(`(?i)s%02de%02d`, season, episode),
		fmt.Sprintf(`(?i)s%02d\.e%02d`, season, episode),
		fmt.Sprintf(`(?i)%dx%02d`, season, episode),
		fmt.Sprintf(`(?i)season\s*%d\s*episode\s*%d`, season, episode),
	}
	
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, title); matched {
			return true
		}
	}
	
	return false
}

func (s *Sharewood) matchesSeason(title string, season int) bool {
	if season == 0 {
		return false
	}
	
	patterns := []string{
		fmt.Sprintf(`(?i)s%02d`, season),
		fmt.Sprintf(`(?i)season\s*%d`, season),
		fmt.Sprintf(`(?i)saison\s*%d`, season),
	}
	
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, title); matched {
			episodePattern := regexp.MustCompile(`(?i)e\d{2}`)
			if !episodePattern.MatchString(title) {
				return true
			}
		}
	}
	
	return false
}

func (s *Sharewood) sortResults(results *models.SharewoodResults) {
	sortFunc := func(torrents []models.SharewoodTorrent) {
		sort.Slice(torrents, func(i, j int) bool {
			if torrents[i].Seeders != torrents[j].Seeders {
				return torrents[i].Seeders > torrents[j].Seeders
			}
			
			priorityI := s.getTorrentPriority(torrents[i])
			priorityJ := s.getTorrentPriority(torrents[j])
			
			if priorityI.Resolution != priorityJ.Resolution {
				return priorityI.Resolution > priorityJ.Resolution
			}
			if priorityI.Language != priorityJ.Language {
				return priorityI.Language > priorityJ.Language
			}
			if priorityI.Codec != priorityJ.Codec {
				return priorityI.Codec > priorityJ.Codec
			}
			
			if torrents[i].Size != torrents[j].Size {
				return torrents[i].Size > torrents[j].Size
			}
			
			return torrents[i].CreatedAt.After(torrents[j].CreatedAt)
		})
	}
	
	sortFunc(results.MovieTorrents)
	sortFunc(results.CompleteSeriesTorrents)
	sortFunc(results.CompleteSeasonTorrents)
	sortFunc(results.EpisodeTorrents)
}

func (s *Sharewood) getTorrentPriority(torrent models.SharewoodTorrent) models.Priority {
	priority := models.Priority{}
	
	parsed := s.parseFileName(torrent.Name)
	titleLower := strings.ToLower(torrent.Name)
	
	switch {
	case strings.Contains(parsed.Resolution, "2160") || strings.Contains(titleLower, "4k"):
		priority.Resolution = 4
	case strings.Contains(parsed.Resolution, "1080"):
		priority.Resolution = 3
	case strings.Contains(parsed.Resolution, "720"):
		priority.Resolution = 2
	default:
		priority.Resolution = 1
	}
	
	langLower := strings.ToLower(torrent.Language)
	if langLower == "multi" || strings.Contains(titleLower, "multi") {
		priority.Language = 3
	} else if langLower == "french" || strings.Contains(titleLower, "french") {
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

func (s *Sharewood) parseFileName(fileName string) models.ParsedFileName {
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