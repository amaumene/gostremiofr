package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/alldebrid"
	"github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/amaumene/gostremiofr/pkg/ratelimiter"
	"github.com/amaumene/gostremiofr/pkg/security"
)

type AllDebrid struct {
	apiKey      string
	rateLimiter *ratelimiter.TokenBucket
	client      *alldebrid.Client
	logger      logger.Logger
	validator   *security.APIKeyValidator
}

func NewAllDebrid(apiKey string) *AllDebrid {
	validator := security.NewAPIKeyValidator()
	
	// Sanitize the API key
	sanitizedKey := validator.SanitizeAPIKey(apiKey)
	
	return &AllDebrid{
		apiKey:      sanitizedKey,
		rateLimiter: ratelimiter.NewTokenBucket(constants.AllDebridRateLimit, constants.AllDebridRateBurst),
		client:      alldebrid.NewClient(),
		logger:      logger.New(),
		validator:   validator,
	}
}

func (a *AllDebrid) CheckMagnets(magnets []models.MagnetInfo, apiKey string) ([]models.ProcessedMagnet, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	// Sanitize and validate the API key
	apiKey = a.validator.SanitizeAPIKey(apiKey)
	if !a.validator.IsValidAllDebridKey(apiKey) {
		a.logger.Warnf("[AllDebrid] warning: invalid API key format provided (key: %s)", a.validator.MaskAPIKey(apiKey))
		return nil, fmt.Errorf("invalid AllDebrid API key format")
	}
	
	// Build hash list for the API call
	var hashes []string
	hashToMagnet := make(map[string]models.MagnetInfo)
	
	for _, magnet := range magnets {
		hashes = append(hashes, magnet.Hash)
		hashToMagnet[magnet.Hash] = magnet
	}
	
	a.rateLimiter.Wait()
	
	// Use POST to avoid exposing API key in URL
	requestURL := "https://api.alldebrid.com/v4/magnet/status"
	
	// Prepare form data
	formData := url.Values{}
	formData.Set("agent", "stremio")
	formData.Set("apikey", apiKey)
	for _, hash := range hashes {
		formData.Add("magnets[]", hash)
	}
	
	a.logger.Infof("[AllDebrid] checking %d specific magnets (API key: %s)", len(hashes), a.validator.MaskAPIKey(apiKey))
	a.logger.Infof("[AllDebrid] making POST request to %s", requestURL)
	
	// Use a standard HTTP client for this manual API call with longer timeout
	httpClient := &http.Client{Timeout: 60 * time.Second}
	
	a.logger.Infof("[AllDebrid] sending POST request...")
	resp, err := httpClient.PostForm(requestURL, formData)
	if err != nil {
		a.logger.Errorf("[AllDebrid] POST request failed: %v", err)
		return nil, fmt.Errorf("failed to check magnets: %w", err)
	}
	a.logger.Infof("[AllDebrid] received HTTP response with status: %s", resp.Status)
	defer resp.Body.Close()
	
	// Parse the response manually since we're using the direct API
	var response struct {
		Status string `json:"status"`
		Data   struct {
			Magnets []struct {
				Hash       string        `json:"hash"`
				Status     string        `json:"status"`
				StatusCode int           `json:"statusCode"`
				Filename   string        `json:"filename"`
				Size       float64       `json:"size"`
				ID         int64         `json:"id"`
				Links      []interface{} `json:"links"`
			} `json:"magnets"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if response.Status != "success" {
		if response.Error.Message != "" {
			return nil, fmt.Errorf("AllDebrid API error: %s - %s", response.Error.Code, response.Error.Message)
		}
		return nil, fmt.Errorf("AllDebrid API error: %s", response.Status)
	}
	
	var processed []models.ProcessedMagnet
	for _, magnet := range response.Data.Magnets {
		if original, ok := hashToMagnet[magnet.Hash]; ok {
			// StatusCode 4 means "Ready" (files are available)
			ready := magnet.StatusCode == 4 && len(magnet.Links) > 0
			name := magnet.Filename
			if name == "" {
				name = original.Title
			}
			
			a.logger.Debugf("[AllDebrid] magnet processing details - hash: %s, statusCode: %d, links: %d, ready: %v", 
				magnet.Hash, magnet.StatusCode, len(magnet.Links), ready)
			
			processed = append(processed, models.ProcessedMagnet{
				Hash:   magnet.Hash,
				Ready:  ready,
				Name:   name,
				Size:   magnet.Size,
				ID:     magnet.ID,
				Source: original.Source,
				Links:  magnet.Links,
			})
		}
	}
	
	a.logger.Debugf("[AllDebrid] API call returned %d results for %d requested magnets", len(processed), len(magnets))
	
	return processed, nil
}

func (a *AllDebrid) UploadMagnet(hash, title, apiKey string) error {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	// Sanitize and validate the API key
	apiKey = a.validator.SanitizeAPIKey(apiKey)
	if !a.validator.IsValidAllDebridKey(apiKey) {
		a.logger.Errorf("[AllDebrid] failed to validate API key for upload: invalid format (key: %s)", a.validator.MaskAPIKey(apiKey))
		return fmt.Errorf("invalid AllDebrid API key format")
	}
	
	a.rateLimiter.Wait()
	
	magnetURL := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hash, url.QueryEscape(title))
	
	// Use our local client
	resp, err := a.client.UploadMagnet(apiKey, []string{magnetURL})
	if err != nil {
		return fmt.Errorf("failed to upload magnet: %w", err)
	}
	
	if resp.Status != "success" {
		if resp.Error != nil {
			return fmt.Errorf("AllDebrid API error: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return fmt.Errorf("AllDebrid API error: %s", resp.Status)
	}
	
	// Check for upload errors
	if len(resp.Data.Magnets) > 0 {
		firstMagnet := resp.Data.Magnets[0]
		if firstMagnet.Error != nil {
			return fmt.Errorf("upload error: %s - %s", firstMagnet.Error.Code, firstMagnet.Error.Message)
		}
	}
	
	return nil
}

func (a *AllDebrid) GetEpisodeFiles(magnetID string, seasonTorrent models.TorrentInfo, apiKey string) ([]models.EpisodeFile, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	// Sanitize and validate the API key
	apiKey = a.validator.SanitizeAPIKey(apiKey)
	if !a.validator.IsValidAllDebridKey(apiKey) {
		a.logger.Errorf("[AllDebrid] failed to validate API key for episode files: invalid format (key: %s)", a.validator.MaskAPIKey(apiKey))
		return nil, fmt.Errorf("invalid AllDebrid API key format")
	}
	
	a.rateLimiter.Wait()
	
	// Use our local client
	resp, err := a.client.GetMagnetFiles(apiKey, magnetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get episode files: %w", err)
	}
	
	if resp.Status != "success" {
		if resp.Error != nil {
			return nil, fmt.Errorf("AllDebrid API error: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("AllDebrid API error: %s", resp.Status)
	}
	
	var episodeFiles []models.EpisodeFile
	for _, magnet := range resp.Data.Magnets {
		a.logger.Debugf("[AllDebrid] processing magnet with %d links", len(magnet.Links))
		for _, link := range magnet.Links {
			a.logger.Debugf("[AllDebrid] checking file: %s (size: %d)", link.Filename, link.Size)
			if isVideoFile(link.Filename) {
				// Parse episode info from filename
				season, episode := parseEpisodeFromFilename(link.Filename)
				a.logger.Debugf("[AllDebrid] parsed episode info from '%s': S%02dE%02d", link.Filename, season, episode)
				
				if season > 0 && episode > 0 {
					resolution := parseResolutionFromFilename(link.Filename)
					language := parseLanguageFromFilename(link.Filename)
					
					a.logger.Debugf("[AllDebrid] adding episode file: S%02dE%02d - %s (%s, %s)", 
						season, episode, link.Filename, resolution, language)
					
					episodeFiles = append(episodeFiles, models.EpisodeFile{
						Name:          link.Filename,
						Size:          float64(link.Size),
						Link:          link.Link,
						Source:        "AllDebrid",
						Season:        season,
						Episode:       episode,
						Resolution:    resolution,
						Language:      language,
						SeasonTorrent: seasonTorrent,
					})
				} else {
					a.logger.Debugf("[AllDebrid] skipping file with no episode info: %s", link.Filename)
				}
			} else {
				a.logger.Debugf("[AllDebrid] skipping non-video file: %s", link.Filename)
			}
		}
	}
	
	a.logger.Debugf("[AllDebrid] extracted %d episode files from season torrent %s", len(episodeFiles), seasonTorrent.Title)
	return episodeFiles, nil
}

func (a *AllDebrid) GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	// Sanitize and validate the API key
	apiKey = a.validator.SanitizeAPIKey(apiKey)
	if !a.validator.IsValidAllDebridKey(apiKey) {
		a.logger.Errorf("[AllDebrid] failed to validate API key for video files: invalid format (key: %s)", a.validator.MaskAPIKey(apiKey))
		return nil, fmt.Errorf("invalid AllDebrid API key format")
	}
	
	a.rateLimiter.Wait()
	
	// Use our local client
	resp, err := a.client.GetMagnetFiles(apiKey, magnetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get video files: %w", err)
	}
	
	if resp.Status != "success" {
		if resp.Error != nil {
			return nil, fmt.Errorf("AllDebrid API error: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("AllDebrid API error: %s", resp.Status)
	}
	
	var videoFiles []models.VideoFile
	for _, magnet := range resp.Data.Magnets {
		for _, link := range magnet.Links {
			if isVideoFile(link.Filename) {
				videoFiles = append(videoFiles, models.VideoFile{
					Name:   link.Filename,
					Size:   float64(link.Size),
					Link:   link.Link,
					Source: "AllDebrid",
				})
			}
		}
	}
	
	return videoFiles, nil
}

func (a *AllDebrid) UnlockLink(link, apiKey string) (string, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	// Sanitize and validate the API key
	apiKey = a.validator.SanitizeAPIKey(apiKey)
	if !a.validator.IsValidAllDebridKey(apiKey) {
		a.logger.Errorf("[AllDebrid] failed to validate API key for unlock: invalid format (key: %s)", a.validator.MaskAPIKey(apiKey))
		return "", fmt.Errorf("invalid AllDebrid API key format")
	}
	
	a.rateLimiter.Wait()
	
	// Use our local client
	resp, err := a.client.UnlockLink(apiKey, link)
	if err != nil {
		return "", fmt.Errorf("failed to unlock link: %w", err)
	}
	
	if resp.Status != "success" {
		if resp.Error != nil {
			return "", fmt.Errorf("AllDebrid unlock API error: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return "", fmt.Errorf("AllDebrid unlock API error: %s", resp.Status)
	}
	
	if resp.Data.Link == "" {
		return "", fmt.Errorf("no direct link returned from unlock API")
	}
	
	return resp.Data.Link, nil
}


func parseEpisodeFromFilename(filename string) (season int, episode int) {
	// Try different episode patterns
	patterns := []string{
		`(?i)s(\d{1,2})e(\d{1,2})`,      // S01E01
		`(?i)s(\d{1,2})\.e(\d{1,2})`,    // S01.E01
		`(?i)(\d{1,2})x(\d{1,2})`,       // 1x01
		`(?i)season\s*(\d{1,2})\s*episode\s*(\d{1,2})`, // Season 1 Episode 1
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(filename)
		if len(matches) >= 3 {
			if s, err := strconv.Atoi(matches[1]); err == nil {
				if e, err := strconv.Atoi(matches[2]); err == nil {
					return s, e
				}
			}
		}
	}
	
	return 0, 0
}

func parseResolutionFromFilename(filename string) string {
	patterns := []string{
		`(?i)(2160p|4k)`,
		`(?i)(1080p)`,
		`(?i)(720p)`,
		`(?i)(480p)`,
		`(?i)(360p)`,
	}
	
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindString(filename); match != "" {
			return strings.ToLower(match)
		}
	}
	
	return "unknown"
}

func parseLanguageFromFilename(filename string) string {
	filename = strings.ToLower(filename)
	
	if strings.Contains(filename, "multi") {
		return "multi"
	}
	if strings.Contains(filename, "french") || strings.Contains(filename, "vf") || strings.Contains(filename, "vff") {
		return "french"
	}
	if strings.Contains(filename, "vostfr") {
		return "vostfr"
	}
	if strings.Contains(filename, "english") || strings.Contains(filename, "vo") {
		return "english"
	}
	
	return "unknown"
}

func isVideoFile(filename string) bool {
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg"}
	filename = strings.ToLower(filename)
	
	for _, ext := range videoExtensions {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	
	return false
}