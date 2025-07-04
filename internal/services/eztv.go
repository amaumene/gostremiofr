package services

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)

type EZTV struct {
	*BaseTorrentService
}

type EZTVTorrent struct {
	ID               int    `json:"id"`
	Hash             string `json:"hash"`
	Filename         string `json:"filename"`
	TorrentURL       string `json:"torrent_url"`
	MagnetURL        string `json:"magnet_url"`
	Title            string `json:"title"`
	Season           string `json:"season"`
	Episode          string `json:"episode"`
	SmallScreenshot  string `json:"small_screenshot"`
	LargeScreenshot  string `json:"large_screenshot"`
	Source           string `json:"-"` // Set at runtime, not from JSON
	Seeds            int    `json:"seeds"`
	Peers            int    `json:"peers"`
	DateReleasedUnix int64  `json:"date_released_unix"`
	SizeBytes        string `json:"size_bytes"`
}

type EZTVResponse struct {
	IMDBID         string        `json:"imdb_id"`
	TorrentsCount  int           `json:"torrents_count"`
	Limit          int           `json:"limit"`
	Page           int           `json:"page"`
	Torrents       []EZTVTorrent `json:"torrents"`
}

func NewEZTV(db database.Database, cache *cache.LRUCache) *EZTV {
	return &EZTV{
		BaseTorrentService: NewBaseTorrentService(db, cache, 5, 1),
	}
}

func (e *EZTV) SetConfig(cfg *config.Config) {
	e.BaseTorrentService.SetConfig(cfg)
}

func (e *EZTV) SearchTorrentsByIMDB(imdbID string, season, episode int) (*models.TorrentResults, error) {
	e.rateLimiter.Wait()
	
	// Clean IMDB ID (remove tt prefix if present)
	cleanIMDBID := imdbID
	if len(imdbID) > 2 && imdbID[:2] == "tt" {
		cleanIMDBID = imdbID[2:]
	}

	apiURL := fmt.Sprintf("https://eztvx.to/api/get-torrents?imdb_id=%s", cleanIMDBID)
	e.logger.Debugf("[EZTV] API call to search torrents - URL: %s", apiURL)
	
	resp, err := e.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search EZTV: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EZTV API error: status %d", resp.StatusCode)
	}

	var eztvResp EZTVResponse
	if err := json.NewDecoder(resp.Body).Decode(&eztvResp); err != nil {
		return nil, fmt.Errorf("failed to decode EZTV response: %w", err)
	}

	// Set source for all torrents
	for i := range eztvResp.Torrents {
		eztvResp.Torrents[i].Source = "EZTV"
	}
	
	e.logger.Debugf("[EZTV] API call completed - found %d torrents for IMDB ID %s", len(eztvResp.Torrents), imdbID)

	return e.processTorrents(eztvResp.Torrents, season, episode), nil
}

func (e *EZTV) processTorrents(torrents []EZTVTorrent, season, episode int) *models.TorrentResults {
	genericTorrents := WrapEZTVTorrents(torrents)
	return e.BaseTorrentService.ProcessTorrents(genericTorrents, "series", season, episode, "EZTV", 0)
}

