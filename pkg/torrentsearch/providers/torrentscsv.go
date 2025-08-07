package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
	"github.com/cehbz/torrentname"
)

const (
	torrentsCSVAPIBase        = "https://torrents-csv.com"
	torrentsCSVSearchEndpoint = "/service/search"
	defaultTimeout            = 30 * time.Second
)

var (
	packPatterns = []string{
		"collection", "trilogy", "quadrilogy", "pentalogy",
		"hexalogy", "saga", "complete", "duology", "anthology",
		"box set", "boxset", " pack", "movie series", "film series",
		"all parts", "all movies", "movies collection",
		"1-2", "1-3", "1-4", "1-5", "1-6", "1-7", "1-8", "1-9",
		"I-II", "I-III", "I-IV", "I-V", "I-VI",
	}
	yearRangePattern = regexp.MustCompile(`\d{4}\s*[-–—]\s*\d{4}`)
)

type TorrentsCSVProvider struct {
	httpClient *http.Client
	cache      interface{}
}

type torrentsCSVResponse struct {
	Torrents []torrentsCSVTorrent `json:"torrents"`
	Next     int64                `json:"next"`
}

type torrentsCSVTorrent struct {
	RowID       int64  `json:"rowid"`
	InfoHash    string `json:"infohash"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedUnix int64  `json:"created_unix"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
	Completed   int    `json:"completed"`
}

func NewTorrentsCSVProvider() *TorrentsCSVProvider {
	return &TorrentsCSVProvider{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func (p *TorrentsCSVProvider) SetCache(cache interface{}) {
	p.cache = cache
}

func (p *TorrentsCSVProvider) Search(options models.SearchOptions) (*models.SearchResults, error) {
	query := buildSearchQuery(options)
	
	torrents, err := p.fetchTorrents(query)
	if err != nil {
		return nil, err
	}

	// Filter out movie packs for movie searches
	if options.MediaType == "movie" {
		torrents = p.filterOutMoviePacks(torrents)
	}

	return p.processResults(torrents, options), nil
}

func (p *TorrentsCSVProvider) GetTorrentHash(torrentID string) (string, error) {
	// TorrentsCSV returns hashes directly in search results
	return torrentID, nil
}

func (p *TorrentsCSVProvider) fetchTorrents(query string) ([]torrentsCSVTorrent, error) {
	encodedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("%s%s?q=%s", torrentsCSVAPIBase, torrentsCSVSearchEndpoint, encodedQuery)

	resp, err := p.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response torrentsCSVResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Torrents, nil
}

func (p *TorrentsCSVProvider) processResults(torrents []torrentsCSVTorrent, options models.SearchOptions) *models.SearchResults {
	results := &models.SearchResults{
		MovieTorrents:          []models.TorrentInfo{},
		CompleteSeriesTorrents: []models.TorrentInfo{},
		CompleteSeasonTorrents: []models.TorrentInfo{},
		EpisodeTorrents:        []models.TorrentInfo{},
	}

	for _, torrent := range torrents {
		info := p.convertToTorrentInfo(torrent)
		classification := p.classifyTorrent(info, options)
		
		switch classification {
		case "movie":
			results.MovieTorrents = append(results.MovieTorrents, info)
		case "complete_series":
			results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, info)
		case "season":
			results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, info)
		case "episode":
			results.EpisodeTorrents = append(results.EpisodeTorrents, info)
		}
	}

	return results
}

func (p *TorrentsCSVProvider) convertToTorrentInfo(torrent torrentsCSVTorrent) models.TorrentInfo {
	return models.TorrentInfo{
		ID:       fmt.Sprintf("%d", torrent.RowID),
		Title:    torrent.Name,
		Hash:     torrent.InfoHash,
		Source:   ProviderTorrentsCSV,
		Size:     torrent.SizeBytes,
		Seeders:  torrent.Seeders,
		Leechers: torrent.Leechers,
	}
}

func (p *TorrentsCSVProvider) classifyTorrent(info models.TorrentInfo, options models.SearchOptions) string {
	parsed := torrentname.Parse(info.Title)
	if parsed == nil {
		if options.MediaType == "movie" {
			return "movie"
		}
		return ""
	}

	// Use torrentname for classification
	if options.MediaType == "movie" {
		return "movie"
	}

	if options.MediaType == "series" {
		if parsed.IsComplete {
			return "complete_series"
		}
		if options.Season > 0 && parsed.Season == options.Season {
			if options.Episode > 0 && parsed.Episode == options.Episode {
				return "episode"
			}
			if parsed.Episode == 0 {
				return "season"
			}
		}
		if parsed.Season > 0 && parsed.Episode > 0 {
			return "episode"
		}
		if parsed.Season > 0 {
			return "season"
		}
	}

	return ""
}

func (p *TorrentsCSVProvider) filterOutMoviePacks(torrents []torrentsCSVTorrent) []torrentsCSVTorrent {
	filtered := make([]torrentsCSVTorrent, 0, len(torrents))
	for _, torrent := range torrents {
		if !p.isMoviePack(torrent.Name) {
			filtered = append(filtered, torrent)
		}
	}
	return filtered
}

func (p *TorrentsCSVProvider) isMoviePack(name string) bool {
	nameLower := strings.ToLower(name)
	for _, pattern := range packPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}
	return yearRangePattern.MatchString(name)
}

func buildSearchQuery(options models.SearchOptions) string {
	query := options.Query
	
	// Build query based on media type
	if options.MediaType == "series" {
		if options.SpecificEpisode && options.Episode > 0 {
			query = fmt.Sprintf("%s s%02de%02d", query, options.Season, options.Episode)
		} else if options.Season > 0 {
			query = fmt.Sprintf("%s s%02d", query, options.Season)
		}
	}
	
	return query
}