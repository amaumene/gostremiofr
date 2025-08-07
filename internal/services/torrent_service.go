package services

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/httputil"
	"github.com/amaumene/gostremiofr/pkg/ratelimiter"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/utils"
	"github.com/cehbz/torrentname"
)


type TorrentService interface {
	SetConfig(cfg *config.Config)
}

// Generic torrent interface for processing different torrent types
type GenericTorrent interface {
	GetID() string
	GetTitle() string
	GetHash() string
	GetSource() string
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
}

func NewBaseTorrentService(db database.Database, cache *cache.LRUCache, rateLimit int, burstLimit int) *BaseTorrentService {
	return &BaseTorrentService{
		db:          db,
		cache:       cache,
		rateLimiter: ratelimiter.NewTokenBucket(int64(rateLimit), int64(burstLimit)),
		httpClient:  httputil.NewHTTPClient(30 * time.Second),
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
			return result, true
		}
	}
	return nil, false
}

func (b *BaseTorrentService) CacheSearch(provider, query, category string, season, episode int, result *models.TorrentResults) {
	cacheKey := fmt.Sprintf("torrent_search_%s_%s_%s_%d_%d", provider, query, category, season, episode)
	b.cache.Set(cacheKey, result)
}

func (b *BaseTorrentService) GetCachedHash(provider, torrentID string) (string, bool) {
	cacheKey := fmt.Sprintf("torrent_hash_%s_%s", provider, torrentID)
	if cached, found := b.cache.Get(cacheKey); found {
		if hash, ok := cached.(string); ok && hash != "" {
			return hash, true
		}
	}
	return "", false
}

func (b *BaseTorrentService) CacheHash(provider, torrentID, hash string) {
	if hash != "" {
		cacheKey := fmt.Sprintf("torrent_hash_%s_%s", provider, torrentID)
		b.cache.Set(cacheKey, hash)
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
	return utils.BuildSearchQuery(query, mediaType, season, episode, specificEpisode)
}



// MatchesYear function removed - movies now search with year appended instead of filtering

func (b *BaseTorrentService) MatchesEpisode(title string, season, episode int) bool {
	if season == 0 || episode == 0 {
		return false
	}
	
	parsed := torrentname.Parse(title)
	return parsed != nil && parsed.Season == season && parsed.Episode == episode
}

func (b *BaseTorrentService) MatchesSeason(title string, season int) bool {
	if season == 0 {
		return false
	}
	
	parsed := torrentname.Parse(title)
	if parsed == nil {
		return false
	}
	
	// Season match with no specific episode or complete season
	return parsed.Season == season && (parsed.Episode == 0 || parsed.IsComplete)
}

func (b *BaseTorrentService) ContainsSeason(title string) bool {
	parsed := torrentname.Parse(title)
	return parsed != nil && parsed.Season > 0 && parsed.Episode == 0
}

func (b *BaseTorrentService) ContainsSeasonEpisode(title string) bool {
	parsed := torrentname.Parse(title)
	return parsed != nil && parsed.Season > 0 && parsed.Episode > 0
}

func (b *BaseTorrentService) SortTorrents(torrents []models.TorrentInfo) {
	sort.Slice(torrents, func(i, j int) bool {
		// Check if titles contain "remux" (case-insensitive)
		iIsRemux := b.isRemux(torrents[i].Title)
		jIsRemux := b.isRemux(torrents[j].Title)
		
		// Remux torrents go first
		if iIsRemux != jIsRemux {
			return iIsRemux
		}
		
		// Then sort by size (largest first)
		return torrents[i].Size > torrents[j].Size
	})
}

func (b *BaseTorrentService) isRemux(title string) bool {
	titleLower := strings.ToLower(title)
	return strings.Contains(titleLower, "remux")
}

type TorrentSorter struct {
	*BaseTorrentService
}

func NewTorrentSorter(config *config.Config) *TorrentSorter {
	base := &BaseTorrentService{
		config: config,
	}
	return &TorrentSorter{BaseTorrentService: base}
}

func (ts *TorrentSorter) SortResults(results *models.TorrentResults) {
	ts.SortTorrents(results.MovieTorrents)
	ts.SortTorrents(results.CompleteSeriesTorrents)
	ts.SortTorrents(results.CompleteSeasonTorrents)
	ts.SortTorrents(results.EpisodeTorrents)
}

func ClassifyTorrent(title string, mediaType string, season, episode int, base *BaseTorrentService) string {
	// Try movie classification
	if class, ok := classifyAsMovie(title, mediaType); ok {
		return class
	}

	// Try complete series classification
	if class, ok := classifyAsCompleteSeries(title); ok {
		return class
	}

	// Try season classification
	if class, ok := classifyBySeason(title, season, base); ok {
		return class
	}

	// Try specific episode classification
	if class, ok := classifyByEpisode(title, season, episode, base); ok {
		return class
	}

	// Try season episode classification (episode == 0)
	if episode == 0 {
		if class, ok := classifySeasonEpisode(title, season, base); ok {
			return class
		}
	}

	return ""
}

// Generic torrent processing function
func (b *BaseTorrentService) ProcessTorrents(torrents []GenericTorrent, mediaType string, season, episode int, serviceName string, year int) *models.TorrentResults {
	results := &models.TorrentResults{}

	for _, torrent := range torrents {
		torrentInfo, classification, shouldAdd := b.processSingleTorrent(torrent, mediaType, season, episode, serviceName, year)
		if shouldAdd {
			b.addTorrentToResults(torrentInfo, classification, results, serviceName)
		}
	}

	// Sort results using existing functionality
	sorter := NewTorrentSorter(b.config)
	sorter.SortResults(results)

	return results
}

type YggTorrentWrapper struct {
	models.YggTorrent
}

func (y YggTorrentWrapper) GetID() string     { return fmt.Sprintf("%d", y.ID) }
func (y YggTorrentWrapper) GetTitle() string  { return y.Title }
func (y YggTorrentWrapper) GetHash() string   { return y.Hash }
func (y YggTorrentWrapper) GetSource() string { return y.Source }
func (y YggTorrentWrapper) GetType() string   { return "" }
func (y YggTorrentWrapper) GetSeason() int    { return 0 }
func (y YggTorrentWrapper) GetEpisode() int   { return 0 }
func (y YggTorrentWrapper) GetSize() int64    { return y.Size }

func WrapYggTorrents(torrents []models.YggTorrent) []GenericTorrent {
	generic := make([]GenericTorrent, len(torrents))
	for i, torrent := range torrents {
		generic[i] = YggTorrentWrapper{torrent}
	}
	return generic
}
