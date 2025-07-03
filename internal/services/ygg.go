package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
)


type YGG struct {
	*BaseTorrentService
}

func NewYGG(db *database.DB, cache *cache.LRUCache) *YGG {
	return &YGG{
		BaseTorrentService: NewBaseTorrentService(db, cache, 10, 2),
	}
}

func (y *YGG) SetConfig(cfg *config.Config) {
	y.BaseTorrentService.SetConfig(cfg)
}

func (y *YGG) SearchTorrents(query string, category string, season, episode int) (*models.TorrentResults, error) {
	y.rateLimiter.Wait()
	
	encodedQuery := url.QueryEscape(query)
	
	// Set category IDs based on content type
	var categoryParams string
	if category == "movie" {
		categoryParams = "&category_id=2178&category_id=2181&category_id=2183"
	} else if category == "series" {
		categoryParams = "&category_id=2179&category_id=2181&category_id=2182&category_id=2184"
	}
	
	apiURL := fmt.Sprintf("https://yggapi.eu/torrents?q=%s&page=1&per_page=100%s", encodedQuery, categoryParams)
	y.logger.Debugf("[YGG] searching torrents: %s", apiURL)
	
	resp, err := y.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search YGG: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YGG API returned status %d", resp.StatusCode)
	}
	
	var torrents []models.YggTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("failed to decode YGG response: %w", err)
	}
	
	y.logger.Infof("[YGG] found %d torrents for query: %s", len(torrents), query)
	
	// For series episodes, fetch hash only for torrents matching the specific episode
	if category == "series" && season > 0 && episode > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		
		for i := range torrents {
			torrents[i].Source = "YGG"
			
			// Check if this torrent matches the specific episode
			if y.BaseTorrentService.MatchesEpisode(torrents[i].Title, season, episode) {
				wg.Add(1)
				go func(index int, torrent models.YggTorrent) {
					defer wg.Done()
					
					y.logger.Debugf("[YGG] fetching hash for episode match - title: %s", torrent.Title)
					hash, err := y.GetTorrentHash(fmt.Sprintf("%d", torrent.ID))
					
					mu.Lock()
					if err != nil {
						y.logger.Debugf("[YGG] failed to fetch hash for torrent %d: %v", torrent.ID, err)
					} else {
						torrents[index].Hash = hash
						y.logger.Debugf("[YGG] hash fetched successfully - title: %s, hash: %s", torrent.Title, hash)
					}
					mu.Unlock()
				}(i, torrents[i])
			}
		}
		wg.Wait()
	} else {
		// For movies or complete series, set source but don't fetch hashes
		for i := range torrents {
			torrents[i].Source = "YGG"
		}
	}
	
	return y.processTorrents(torrents, category, season, episode), nil
}

func (y *YGG) GetTorrentHash(torrentID string) (string, error) {
	y.rateLimiter.Wait()
	
	apiURL := fmt.Sprintf("https://yggapi.eu/torrent/%s", torrentID)
	y.logger.Debugf("[YGG] API call to get torrent hash - URL: %s", apiURL)
	
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

func (y *YGG) processTorrents(torrents []models.YggTorrent, category string, season, episode int) *models.TorrentResults {
	genericTorrents := WrapYggTorrents(torrents)
	return y.BaseTorrentService.ProcessTorrents(genericTorrents, category, season, episode, "YGG")
}

