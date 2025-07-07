package services

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/amaumene/gostremiofr/pkg/ratelimiter"
)

var (
	parseRegexOnce        sync.Once
	resolutionRegex       *regexp.Regexp
	sourceRegex           *regexp.Regexp
	seasonEpisodePatterns []*regexp.Regexp
	seasonPatterns        []*regexp.Regexp
)

func initParseRegexPatterns() {
	parseRegexOnce.Do(func() {
		resolutionRegex = regexp.MustCompile(`(?i)(4k|\d{3,4}p)`)
		sourceRegex = regexp.MustCompile(`(?i)(BluRay|WEB[-]?DL|WEB|HDRip|DVDRip|BRRip)`)

		seasonEpisodePatterns = []*regexp.Regexp{
			regexp.MustCompile(`(?i)s\d{2}e\d{2}`),
			regexp.MustCompile(`(?i)s\d{2}\.e\d{2}`),
			regexp.MustCompile(`(?i)\d{1,2}x\d{2}`),
			regexp.MustCompile(`(?i)season\s*\d+\s*episode\s*\d+`),
		}

		seasonPatterns = []*regexp.Regexp{
			regexp.MustCompile(`(?i)(saison|season)\s*\d+`),
			regexp.MustCompile(`(?i)s\d{2}(?:[^e]|$)`), // season without episode
		}
	})
}

type TorrentService interface {
	SetConfig(cfg *config.Config)
}

// Generic torrent interface for processing different torrent types
type GenericTorrent interface {
	GetID() string
	GetTitle() string
	GetHash() string
	GetSource() string
	GetLanguage() string
	GetType() string // For services that have type info
	GetSeason() int  // For services that have season info
	GetEpisode() int // For services that have episode info
	GetSize() int64  // Size in bytes
}

type BaseTorrentService struct {
	config      *config.Config
	db          database.Database
	cache       *cache.LRUCache
	rateLimiter *ratelimiter.TokenBucket
	httpClient  *http.Client
	logger      logger.Logger
}

func NewBaseTorrentService(db database.Database, cache *cache.LRUCache, rateLimit int, burstLimit int) *BaseTorrentService {
	initParseRegexPatterns()
	return &BaseTorrentService{
		db:          db,
		cache:       cache,
		rateLimiter: ratelimiter.NewTokenBucket(int64(rateLimit), int64(burstLimit)),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.New(),
	}
}

func (b *BaseTorrentService) SetConfig(cfg *config.Config) {
	b.config = cfg
}

// Generic caching methods for any torrent provider
func (b *BaseTorrentService) GetCachedSearch(provider, query, category string, season, episode int) (*models.TorrentResults, bool) {
	cacheKey := fmt.Sprintf("torrent_search_%s_%s_%s_%d_%d", provider, query, category, season, episode)
	if cached, found := b.cache.Get(cacheKey); found {
		if result, ok := cached.(*models.TorrentResults); ok {
			b.logger.Infof("[%s] cache hit for query: %s (%d movies, %d series, %d seasons, %d episodes)", 
				provider, query, len(result.MovieTorrents), len(result.CompleteSeriesTorrents), 
				len(result.CompleteSeasonTorrents), len(result.EpisodeTorrents))
			return result, true
		}
	}
	b.logger.Infof("[%s] cache miss, will fetch from API for query: %s", provider, query)
	return nil, false
}

func (b *BaseTorrentService) CacheSearch(provider, query, category string, season, episode int, result *models.TorrentResults) {
	cacheKey := fmt.Sprintf("torrent_search_%s_%s_%s_%d_%d", provider, query, category, season, episode)
	b.cache.Set(cacheKey, result)
	b.logger.Infof("[%s] cached result for query: %s (%d movies, %d series, %d seasons, %d episodes)", 
		provider, query, len(result.MovieTorrents), len(result.CompleteSeriesTorrents), 
		len(result.CompleteSeasonTorrents), len(result.EpisodeTorrents))
}

func (b *BaseTorrentService) GetCachedHash(provider, torrentID string) (string, bool) {
	cacheKey := fmt.Sprintf("torrent_hash_%s_%s", provider, torrentID)
	if cached, found := b.cache.Get(cacheKey); found {
		if hash, ok := cached.(string); ok && hash != "" {
			b.logger.Debugf("[%s] cache hit for torrent hash %s: %s", provider, torrentID, hash)
			return hash, true
		}
	}
	b.logger.Debugf("[%s] cache miss for hash, will fetch from API for torrent ID %s", provider, torrentID)
	return "", false
}

func (b *BaseTorrentService) CacheHash(provider, torrentID, hash string) {
	if hash != "" {
		cacheKey := fmt.Sprintf("torrent_hash_%s_%s", provider, torrentID)
		b.cache.Set(cacheKey, hash)
		b.logger.Debugf("[%s] cached hash for torrent %s: %s", provider, torrentID, hash)
	}
}

// BuildSearchQuery builds a standardized search query for torrent sites
// Format:
// - Movies: "title+year"
// - Series: "name+sXX" (matches both episodes and complete seasons)
func (b *BaseTorrentService) BuildSearchQuery(query string, mediaType string, season, episode int) string {
	return b.BuildSearchQueryWithMode(query, mediaType, season, episode, false)
}

// BuildSearchQueryWithMode builds a search query with option for specific episode
// Format:
// - Movies: "title+year"
// - Series (season mode): "name+sXX" (matches both episodes and complete seasons)
// - Series (episode mode): "name+sXXeXX" (matches specific episode)
func (b *BaseTorrentService) BuildSearchQueryWithMode(query string, mediaType string, season, episode int, specificEpisode bool) string {
	// Extract year from query if it's at the end (format: "title year")
	var title string
	var year string
	parts := strings.Fields(query)
	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		if yearMatch, _ := regexp.MatchString(`^\d{4}$`, lastPart); yearMatch {
			year = lastPart
			title = strings.Join(parts[:len(parts)-1], " ")
		} else {
			title = query
		}
	} else {
		title = query
	}
	
	// Replace spaces with +
	title = strings.ReplaceAll(title, " ", "+")
	
	// Build query based on type
	if mediaType == "movie" {
		// For movies: title+year (no prefix)
		if year != "" {
			return fmt.Sprintf("%s+%s", title, year)
		}
		return title
	} else if mediaType == "series" {
		// For series with specific episode mode
		if specificEpisode && season > 0 && episode > 0 {
			// Search for specific episode: name+s04e01
			return fmt.Sprintf("%s+s%02de%02d", title, season, episode)
		}
		// For series, search by season to get both episodes and complete packs
		if season > 0 {
			// Search for season: name+s04 (will match both s04e01 and complete seasons)
			// Using lowercase 's' as it's the standard format
			return fmt.Sprintf("%s+s%02d", title, season)
		}
		// Just series name if no season specified
		return title
	}
	
	return title
}

func (b *BaseTorrentService) ParseFileName(fileName string) models.ParsedFileName {
	var result models.ParsedFileName

	if match := resolutionRegex.FindString(fileName); match != "" {
		result.Resolution = match
	} else {
		result.Resolution = "?"
	}

	result.Codec = "?"

	if match := sourceRegex.FindString(fileName); match != "" {
		result.Source = match
	} else {
		result.Source = "?"
	}

	return result
}

func (b *BaseTorrentService) GetTorrentPriority(title string) models.Priority {
	priority := models.Priority{}
	titleLower := strings.ToLower(title)
	logger := logger.New()

	// Resolution priority based on user-specified order
	if b.config != nil {
		parsed := b.ParseFileName(title)
		priority.Resolution = b.config.GetResolutionPriority(parsed.Resolution)
	} else {
		// Fallback to default priority if no config
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
	}

	// Language priority based on user-specified order
	if b.config != nil {
		priority.Language = b.config.GetLanguagePriority(title)
	} else {
		// Fallback to default priority if no config
		if strings.Contains(titleLower, "multi") {
			priority.Language = 3
		} else if strings.Contains(titleLower, "french") || strings.Contains(titleLower, "vff") || strings.Contains(titleLower, "truefrench") {
			priority.Language = 2
		} else {
			priority.Language = 1
		}
	}


	logger.Debugf("[TorrentService] torrent priority details - title: '%s', resolution: %d, language: %d",
		title, priority.Resolution, priority.Language)
	return priority
}

func (b *BaseTorrentService) MatchesLanguageFilter(title string, language string) bool {
	if b.config == nil {
		return true
	}

	logger := logger.New()

	// Check language filter
	langAllowed := len(b.config.LangToShow) == 0
	if !langAllowed {
		// If a language is explicitly provided, use it directly
		if language != "" {
			for _, lang := range b.config.LangToShow {
				if strings.EqualFold(language, lang) {
					langAllowed = true
					break
				}
			}
		} else {
			// Only parse from title if no language is provided (like YGG)
			for _, lang := range b.config.LangToShow {
				if b.ContainsLanguage(title, lang) {
					langAllowed = true
					break
				}
			}
		}
		
		if !langAllowed {
			logger.Debugf("[TorrentService] language filter applied - title: %s, provided language: %s", title, language)
		}
	}

	return langAllowed
}

func (b *BaseTorrentService) MatchesResolutionFilter(title string) bool {
	if b.config == nil {
		return true
	}

	parsed := b.ParseFileName(title)
	logger := logger.New()

	// Check resolution filter
	if parsed.Resolution != "?" && !b.config.IsResolutionAllowed(parsed.Resolution) {
		logger.Debugf("[TorrentService] resolution filter applied - resolution: '%s', title: %s", parsed.Resolution, title)
		return false
	}

	return true
}

func (b *BaseTorrentService) ContainsLanguage(title, language string) bool {
	titleLower := strings.ToLower(title)
	langLower := strings.ToLower(language)

	switch langLower {
	case "multi", "multi_fr":
		return strings.Contains(titleLower, "multi")
	case "french", "vf", "vff":
		return strings.Contains(titleLower, "french") ||
			strings.Contains(titleLower, "vff") ||
			strings.Contains(titleLower, "vf") ||
			strings.Contains(titleLower, "truefrench")
	case "vo":
		return strings.Contains(titleLower, "vo") ||
			strings.Contains(titleLower, "vostfr") ||
			strings.Contains(titleLower, "english") ||
			(!strings.Contains(titleLower, "vf") && !strings.Contains(titleLower, "french") && !strings.Contains(titleLower, "multi"))
	case "english":
		return strings.Contains(titleLower, "english") ||
			strings.Contains(titleLower, "vostfr")
	case "vostfr":
		return strings.Contains(titleLower, "vostfr")
	default:
		return strings.Contains(titleLower, langLower)
	}
}

func (b *BaseTorrentService) MatchesYear(title string, expectedYear int) bool {
	if expectedYear == 0 {
		return true // If no year provided, don't filter
	}
	
	// Look for 4-digit year patterns in the title
	yearPattern := regexp.MustCompile(`\b(19|20)\d{2}\b`)
	matches := yearPattern.FindAllString(title, -1)
	
	for _, match := range matches {
		if year, err := strconv.Atoi(match); err == nil {
			// Allow some flexibility: exact year or within 1 year
			if year == expectedYear || year == expectedYear-1 || year == expectedYear+1 {
				return true
			}
		}
	}
	
	// If no year found in title, allow it (some torrents don't include year)
	return len(matches) == 0
}

func (b *BaseTorrentService) MatchesEpisode(title string, season, episode int) bool {
	if season == 0 || episode == 0 {
		return false
	}

	patterns := []string{
		// Standard formats: S01E01, S1E1, s01e01, etc.
		fmt.Sprintf(`(?i)s%02de%02d`, season, episode),
		fmt.Sprintf(`(?i)s%de%d`, season, episode),
		// With dots: S01.E01, S1.E1
		fmt.Sprintf(`(?i)s%02d\.e%02d`, season, episode),
		fmt.Sprintf(`(?i)s%d\.e%d`, season, episode),
		// With x: 1x01, 01x01
		fmt.Sprintf(`(?i)%dx%02d`, season, episode),
		fmt.Sprintf(`(?i)%02dx%02d`, season, episode),
		// Written out: Season 1 Episode 1
		fmt.Sprintf(`(?i)season\s*%d\s*episode\s*%d`, season, episode),
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, title); matched {
			return true
		}
	}

	return false
}

func (b *BaseTorrentService) MatchesSeason(title string, season int) bool {
	if season == 0 {
		return false
	}

	logger := logger.New()
	logger.Debugf("[TorrentService] checking if '%s' matches season %d", title, season)

	patterns := []string{
		fmt.Sprintf(`(?i)s%02d(?:[^e]|$)`, season),                    // S04 but not S04E01
		fmt.Sprintf(`(?i)s%d(?:[^e]|$)`, season),                     // S4 but not S4E1
		fmt.Sprintf(`(?i)season\s*%d(?:[^e]|\s*[^e]|$)`, season),     // Season 4
		fmt.Sprintf(`(?i)saison\s*%d`, season),                       // Saison 4 (French)
		fmt.Sprintf(`(?i)complete.*s%02d`, season),                   // Complete S04
		fmt.Sprintf(`(?i)complete.*season\s*%d`, season),             // Complete Season 4
		fmt.Sprintf(`(?i)s%02d.*complete`, season),                   // S04 Complete
		fmt.Sprintf(`(?i)season\s*%d.*complete`, season),             // Season 4 Complete
		fmt.Sprintf(`(?i)s%02d.*pack`, season),                       // S04 Pack
		fmt.Sprintf(`(?i)season\s*%d.*pack`, season),                 // Season 4 Pack
		fmt.Sprintf(`(?i)pack.*s%02d`, season),                       // Pack S04
		fmt.Sprintf(`(?i)pack.*season\s*%d`, season),                 // Pack Season 4
		fmt.Sprintf(`(?i)integrale.*s%02d`, season),                  // Integrale S04 (French)
		fmt.Sprintf(`(?i)integrale.*saison\s*%d`, season),            // Integrale Saison 4 (French)
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, title); matched {
			logger.Debugf("[TorrentService] '%s' matches season %d with pattern: %s", title, season, pattern)
			return true
		}
	}

	logger.Debugf("[TorrentService] '%s' does not match season %d", title, season)
	return false
}

func (b *BaseTorrentService) ContainsSeason(title string) bool {
	for _, pattern := range seasonPatterns {
		if pattern.MatchString(title) {
			return true
		}
	}
	return false
}

func (b *BaseTorrentService) ContainsSeasonEpisode(title string) bool {
	for _, pattern := range seasonEpisodePatterns {
		if pattern.MatchString(title) {
			return true
		}
	}
	return false
}

func (b *BaseTorrentService) SortTorrents(torrents []models.TorrentInfo) {
	sort.Slice(torrents, func(i, j int) bool {
		priorityI := b.GetTorrentPriority(torrents[i].Title)
		priorityJ := b.GetTorrentPriority(torrents[j].Title)

		// 1. First sort by resolution (higher is better)
		if priorityI.Resolution != priorityJ.Resolution {
			return priorityI.Resolution > priorityJ.Resolution
		}
		
		// 2. Then by language priority (higher is better) - only for YGG
		if torrents[i].Source == "YGG" && torrents[j].Source == "YGG" {
			if priorityI.Language != priorityJ.Language {
				return priorityI.Language > priorityJ.Language
			}
		}

		// 3. Finally by size (larger is better)
		return torrents[i].Size > torrents[j].Size
	})
}

type TorrentSorter struct {
	*BaseTorrentService
}

func NewTorrentSorter(config *config.Config) *TorrentSorter {
	initParseRegexPatterns()
	base := &BaseTorrentService{
		config: config,
	}
	return &TorrentSorter{BaseTorrentService: base}
}

func (ts *TorrentSorter) SortResults(results *models.TorrentResults) {
	logger := logger.New()
	logger.Debugf("[TorrentService] sorting torrents - movies: %d, complete series: %d, seasons: %d, episodes: %d",
		len(results.MovieTorrents), len(results.CompleteSeriesTorrents),
		len(results.CompleteSeasonTorrents), len(results.EpisodeTorrents))

	ts.SortTorrents(results.MovieTorrents)
	ts.SortTorrents(results.CompleteSeriesTorrents)
	ts.SortTorrents(results.CompleteSeasonTorrents)
	ts.SortTorrents(results.EpisodeTorrents)

	// Log top 3 movies after sorting for debugging
	if len(results.MovieTorrents) > 0 {
		logger.Infof("[TorrentService] top movie torrents after sorting:")
		for i := 0; i < 3 && i < len(results.MovieTorrents); i++ {
			t := results.MovieTorrents[i]
			parsed := ts.ParseFileName(t.Title)
			priority := ts.GetTorrentPriority(t.Title)
			logger.Infof("  %d. %s - Resolution: %s (priority:%d), Size: %.2f GB, Source: %s", 
				i+1, t.Title, parsed.Resolution, priority.Resolution, float64(t.Size)/(1024*1024*1024), t.Source)
		}
	}

	logger.Debugf("[TorrentService] torrent sorting completed")
}


func ClassifyTorrent(title string, mediaType string, season, episode int, base *BaseTorrentService) string {
	titleUpper := strings.ToUpper(title)
	logger := logger.New()

	if mediaType == "movie" {
		logger.Debugf("[TorrentService] torrent classification - '%s' classified as movie (media type)", title)
		return "movie"
	}

	if strings.Contains(titleUpper, "COMPLETE") {
		logger.Debugf("[TorrentService] torrent classification - '%s' classified as complete_series (contains COMPLETE)", title)
		return "complete_series"
	}

	// Check for complete seasons first - matches the requested season
	if season > 0 && base.MatchesSeason(title, season) {
		logger.Debugf("[TorrentService] torrent classification - '%s' classified as season (matches season %d)", title, season)
		return "season"
	}

	// If looking for a specific episode
	if season > 0 && episode > 0 {
		if base.MatchesEpisode(title, season, episode) {
			logger.Debugf("[TorrentService] torrent classification - '%s' classified as episode (matches s%02de%02d)", title, season, episode)
			return "episode"
		}
	}

	// If looking for complete season (episode == 0)
	if season > 0 && episode == 0 {
		// Any episode from this season can be included
		if base.ContainsSeasonEpisode(title) {
			// Check if it's from the right season
			titleUpper := strings.ToUpper(title)
			seasonPattern := fmt.Sprintf("S%02d", season)
			if strings.Contains(titleUpper, seasonPattern) {
				logger.Debugf("[TorrentService] torrent classification - '%s' classified as episode (part of season %d)", title, season)
				return "episode"
			}
		}
	}

	return ""
}

// Generic torrent processing function
func (b *BaseTorrentService) ProcessTorrents(torrents []GenericTorrent, mediaType string, season, episode int, serviceName string, year int) *models.TorrentResults {
	results := &models.TorrentResults{}
	logger := logger.New()

	logger.Debugf("[%s] torrent processing started - total torrents: %d", serviceName, len(torrents))

	for _, torrent := range torrents {
		// First filter: language only
		if !b.MatchesLanguageFilter(torrent.GetTitle(), torrent.GetLanguage()) {
			logger.Infof("[%s] torrent filtered by language - title: %s, language: %s", serviceName, torrent.GetTitle(), torrent.GetLanguage())
			continue
		}

		// Second filter: year matching for movies
		if mediaType == "movie" && !b.MatchesYear(torrent.GetTitle(), year) {
			logger.Debugf("[%s] torrent filtered by year - title: %s (expected: %d)", serviceName, torrent.GetTitle(), year)
			continue
		}

		torrentInfo := models.TorrentInfo{
			ID:     torrent.GetID(),
			Title:  torrent.GetTitle(),
			Hash:   torrent.GetHash(),
			Source: torrent.GetSource(),
			Size:   torrent.GetSize(),
		}

		var classification string
		var shouldAdd bool

		// For services that provide type info directly
		if torrent.GetType() != "" {
			if mediaType == "movie" && torrent.GetType() == "movie" {
				classification = "movie"
				shouldAdd = true
			} else if mediaType == "series" && torrent.GetType() == "tvshow" {
				// Continue to title-based classification below
			} else {
				// Type doesn't match what we're looking for
				continue
			}
		}

		// For services that provide season/episode info directly
		if !shouldAdd && torrent.GetSeason() > 0 && torrent.GetEpisode() > 0 {
			// Check if this episode matches what we're looking for
			if mediaType == "series" && season > 0 && episode > 0 {
				if torrent.GetSeason() == season && torrent.GetEpisode() == episode {
					classification = "episode"
					shouldAdd = true
					logger.Infof("[%s] episode match found - s%02de%02d: %s", serviceName, season, episode, torrent.GetTitle())
				} else {
					logger.Infof("[%s] episode mismatch - found s%02de%02d, requested s%02de%02d: %s", serviceName, torrent.GetSeason(), torrent.GetEpisode(), season, episode, torrent.GetTitle())
				}
			} else {
				// If no specific episode requested, accept any episode
				classification = "episode" 
				shouldAdd = true
			}
		}

		// Use title-based classification if not already determined
		if !shouldAdd {
			classification = ClassifyTorrent(torrent.GetTitle(), mediaType, season, episode, b)
			shouldAdd = classification != ""
		}

		if !shouldAdd {
			continue
		}

		logger.Debugf("[%s] torrent classification result - title: '%s', type: %s", serviceName, torrent.GetTitle(), classification)

		// Third filter: resolution (after classification)
		if !b.MatchesResolutionFilter(torrent.GetTitle()) {
			logger.Debugf("[%s] torrent filtered by resolution after classification - title: %s", serviceName, torrent.GetTitle())
			continue
		}

		switch classification {
		case "movie":
			logger.Debugf("[%s] adding movie torrent - title: %s", serviceName, torrent.GetTitle())
			results.MovieTorrents = append(results.MovieTorrents, torrentInfo)
		case "complete_series":
			results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, torrentInfo)
		case "episode":
			logger.Debugf("[%s] adding episode torrent - title: %s", serviceName, torrent.GetTitle())
			results.EpisodeTorrents = append(results.EpisodeTorrents, torrentInfo)
		case "season":
			results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrentInfo)
		}
	}

	// Sort results
	sorter := NewTorrentSorter(b.config)
	sorter.SortResults(results)

	logger.Debugf("[%s] torrent processing completed - movies: %d, complete series: %d, seasons: %d, episodes: %d",
		serviceName, len(results.MovieTorrents), len(results.CompleteSeriesTorrents),
		len(results.CompleteSeasonTorrents), len(results.EpisodeTorrents))

	return results
}

// Wrapper types that implement GenericTorrent interface

type YggTorrentWrapper struct {
	models.YggTorrent
}

func (y YggTorrentWrapper) GetID() string       { return fmt.Sprintf("%d", y.ID) }
func (y YggTorrentWrapper) GetTitle() string    { return y.Title }
func (y YggTorrentWrapper) GetHash() string     { return y.Hash }
func (y YggTorrentWrapper) GetSource() string   { return y.Source }
func (y YggTorrentWrapper) GetLanguage() string { return "" } // YGG doesn't have explicit language
func (y YggTorrentWrapper) GetType() string     { return "" } // YGG doesn't have explicit type
func (y YggTorrentWrapper) GetSeason() int      { return 0 }  // YGG doesn't have explicit season
func (y YggTorrentWrapper) GetEpisode() int     { return 0 }  // YGG doesn't have explicit episode
func (y YggTorrentWrapper) GetSize() int64      { return y.Size }


// Helper functions to convert slices to GenericTorrent slices

func WrapYggTorrents(torrents []models.YggTorrent) []GenericTorrent {
	generic := make([]GenericTorrent, len(torrents))
	for i, torrent := range torrents {
		generic[i] = YggTorrentWrapper{torrent}
	}
	return generic
}


