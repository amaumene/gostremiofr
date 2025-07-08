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

const (
	// AllDebrid API configuration
	allDebridAPIBase      = "https://api.alldebrid.com/v4"
	allDebridMagnetStatus = "/magnet/status"
	allDebridAgent        = "stremio"

	// HTTP timeouts
	allDebridAPITimeout = 60 * time.Second

	// Status codes
	allDebridStatusReady   = 4
	allDebridStatusSuccess = "success"

	// Video file extensions
	videoExtensions = ".mp4,.mkv,.avi,.mov,.wmv,.flv,.webm,.m4v,.mpg,.mpeg"
)

type AllDebrid struct {
	apiKey      string
	rateLimiter *ratelimiter.TokenBucket
	client      *alldebrid.Client
	logger      logger.Logger
	validator   *security.APIKeyValidator
	fileParsers *fileParsers
}

// fileParsers handles parsing of file metadata
type fileParsers struct {
	episodePatterns    []*regexp.Regexp
	resolutionPatterns []*regexp.Regexp
	videoExtSet        map[string]bool
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
		fileParsers: newFileParsers(),
	}
}

// newFileParsers creates and compiles all regex patterns for file parsing
func newFileParsers() *fileParsers {
	// Episode patterns
	episodePatterns := []string{
		`(?i)s(\d{1,2})e(\d{1,2})`,                     // S01E01
		`(?i)s(\d{1,2})\.e(\d{1,2})`,                   // S01.E01
		`(?i)(\d{1,2})x(\d{1,2})`,                      // 1x01
		`(?i)season\s*(\d{1,2})\s*episode\s*(\d{1,2})`, // Season 1 Episode 1
	}

	// Resolution patterns
	resolutionPatterns := []string{
		`(?i)(2160p|4k)`,
		`(?i)(1080p)`,
		`(?i)(720p)`,
		`(?i)(480p)`,
		`(?i)(360p)`,
	}

	// Compile episode patterns
	compiledEpisode := make([]*regexp.Regexp, len(episodePatterns))
	for i, pattern := range episodePatterns {
		compiledEpisode[i] = regexp.MustCompile(pattern)
	}

	// Compile resolution patterns
	compiledResolution := make([]*regexp.Regexp, len(resolutionPatterns))
	for i, pattern := range resolutionPatterns {
		compiledResolution[i] = regexp.MustCompile(pattern)
	}

	// Video extensions set
	videoExtSet := make(map[string]bool)
	for _, ext := range strings.Split(videoExtensions, ",") {
		videoExtSet[ext] = true
	}

	return &fileParsers{
		episodePatterns:    compiledEpisode,
		resolutionPatterns: compiledResolution,
		videoExtSet:        videoExtSet,
	}
}

func (a *AllDebrid) CheckMagnets(magnets []models.MagnetInfo, apiKey string) ([]models.ProcessedMagnet, error) {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	// Build hash list for the API call
	hashes, hashToMagnet := a.buildHashMapping(magnets)

	a.rateLimiter.Wait()

	// Make API call to check magnet status
	response, err := a.checkMagnetStatus(apiKey, hashes)
	if err != nil {
		return nil, err
	}

	// Process response into structured data
	processed := a.processMagnetResponse(response, hashToMagnet)

	a.logger.Debugf("[AllDebrid] API call returned %d results for %d requested magnets", len(processed), len(magnets))

	return processed, nil
}

func (a *AllDebrid) UploadMagnet(hash, title, apiKey string) error {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return err
	}

	a.rateLimiter.Wait()

	magnetURL := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hash, url.QueryEscape(title))

	// Use our local client
	resp, err := a.client.UploadMagnet(apiKey, []string{magnetURL})
	if err != nil {
		return fmt.Errorf("failed to upload magnet: %w", err)
	}

	// Check response status and errors
	return a.checkAPIResponse(resp.Status, resp.Error, resp.Data.Magnets)
}

func (a *AllDebrid) GetEpisodeFiles(magnetID string, seasonTorrent models.TorrentInfo, apiKey string) ([]models.EpisodeFile, error) {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	a.rateLimiter.Wait()

	// Get magnet files
	resp, err := a.client.GetMagnetFiles(apiKey, magnetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get episode files: %w", err)
	}

	// Check response status
	if resp.Status != allDebridStatusSuccess {
		if resp.Error != nil {
			return nil, fmt.Errorf("AllDebrid API error: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("AllDebrid API error: %s", resp.Status)
	}

	// Process files into episode files
	episodeFiles := a.processEpisodeFiles(resp.Data.Magnets, seasonTorrent)

	a.logger.Debugf("[AllDebrid] extracted %d episode files from season torrent %s", len(episodeFiles), seasonTorrent.Title)
	return episodeFiles, nil
}

func (a *AllDebrid) GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error) {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	a.rateLimiter.Wait()

	// Get magnet files
	resp, err := a.client.GetMagnetFiles(apiKey, magnetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get video files: %w", err)
	}

	// Check response status
	if resp.Status != allDebridStatusSuccess {
		if resp.Error != nil {
			return nil, fmt.Errorf("AllDebrid API error: %s - %s", resp.Error.Code, resp.Error.Message)
		}
		return nil, fmt.Errorf("AllDebrid API error: %s", resp.Status)
	}

	// Process files into video files
	videoFiles := a.processVideoFiles(resp.Data.Magnets)

	return videoFiles, nil
}

func (a *AllDebrid) UnlockLink(link, apiKey string) (string, error) {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return "", err
	}

	a.rateLimiter.Wait()

	// Use our local client
	resp, err := a.client.UnlockLink(apiKey, link)
	if err != nil {
		return "", fmt.Errorf("failed to unlock link: %w", err)
	}

	// Check response status
	if err := a.checkAPIResponse(resp.Status, resp.Error, nil); err != nil {
		return "", err
	}

	if resp.Data.Link == "" {
		return "", fmt.Errorf("no direct link returned from unlock API")
	}

	return resp.Data.Link, nil
}

// Helper methods for API operations

// validateAndPrepareAPIKey validates and prepares the API key for use
func (a *AllDebrid) validateAndPrepareAPIKey(apiKey string) (string, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}

	// Sanitize and validate the API key
	apiKey = a.validator.SanitizeAPIKey(apiKey)
	if !a.validator.IsValidAllDebridKey(apiKey) {
		a.logger.Errorf("[AllDebrid] invalid API key format (key: %s)", a.validator.MaskAPIKey(apiKey))
		return "", fmt.Errorf("invalid AllDebrid API key format")
	}

	return apiKey, nil
}

// buildHashMapping creates a hash list and mapping for magnet checking
func (a *AllDebrid) buildHashMapping(magnets []models.MagnetInfo) ([]string, map[string]models.MagnetInfo) {
	var hashes []string
	hashToMagnet := make(map[string]models.MagnetInfo)

	for _, magnet := range magnets {
		hashes = append(hashes, magnet.Hash)
		hashToMagnet[magnet.Hash] = magnet
	}

	return hashes, hashToMagnet
}

// checkMagnetStatus makes the API call to check magnet status
func (a *AllDebrid) checkMagnetStatus(apiKey string, hashes []string) (*magnetStatusResponse, error) {
	// Use POST to avoid exposing API key in URL
	requestURL := allDebridAPIBase + allDebridMagnetStatus

	// Prepare form data
	formData := url.Values{}
	formData.Set("agent", allDebridAgent)
	formData.Set("apikey", apiKey)
	for _, hash := range hashes {
		formData.Add("magnets[]", hash)
	}

	a.logger.Infof("[AllDebrid] checking %d specific magnets (API key: %s)", len(hashes), a.validator.MaskAPIKey(apiKey))
	a.logger.Infof("[AllDebrid] making POST request to %s", requestURL)

	// Use a standard HTTP client with longer timeout
	httpClient := &http.Client{Timeout: allDebridAPITimeout}

	a.logger.Infof("[AllDebrid] sending POST request...")
	resp, err := httpClient.PostForm(requestURL, formData)
	if err != nil {
		a.logger.Errorf("[AllDebrid] POST request failed: %v", err)
		return nil, fmt.Errorf("failed to check magnets: %w", err)
	}
	a.logger.Infof("[AllDebrid] received HTTP response with status: %s", resp.Status)
	defer resp.Body.Close()

	// Parse the response
	var response magnetStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != allDebridStatusSuccess {
		if response.Error.Message != "" {
			return nil, fmt.Errorf("AllDebrid API error: %s - %s", response.Error.Code, response.Error.Message)
		}
		return nil, fmt.Errorf("AllDebrid API error: %s", response.Status)
	}

	return &response, nil
}

// processMagnetResponse processes the API response into structured data
func (a *AllDebrid) processMagnetResponse(response *magnetStatusResponse, hashToMagnet map[string]models.MagnetInfo) []models.ProcessedMagnet {
	var processed []models.ProcessedMagnet
	for _, magnet := range response.Data.Magnets {
		if original, ok := hashToMagnet[magnet.Hash]; ok {
			// StatusCode 4 means "Ready" (files are available)
			ready := magnet.StatusCode == allDebridStatusReady && len(magnet.Links) > 0
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
	return processed
}

// checkAPIResponse checks the status and error of an API response
func (a *AllDebrid) checkAPIResponse(status string, apiError interface{}, magnets interface{}) error {
	if status != allDebridStatusSuccess {
		if apiError != nil {
			// Handle different error types
			switch err := apiError.(type) {
			case *struct{ Code, Message string }:
				return fmt.Errorf("AllDebrid API error: %s - %s", err.Code, err.Message)
			default:
				return fmt.Errorf("AllDebrid API error: %s", status)
			}
		}
		return fmt.Errorf("AllDebrid API error: %s", status)
	}

	// Check for upload-specific errors
	if magnets != nil {
		// This assumes the magnets slice has upload error information
		// Implementation would depend on the specific data structure
	}

	return nil
}

// processEpisodeFiles processes magnet links into episode files
func (a *AllDebrid) processEpisodeFiles(magnets []struct {
	ID    int64  `json:"id"`
	Hash  string `json:"hash"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Ready bool   `json:"ready"`
	Links []struct {
		Link     string `json:"link"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"links"`
}, seasonTorrent models.TorrentInfo) []models.EpisodeFile {
	var episodeFiles []models.EpisodeFile

	for _, magnet := range magnets {
		a.logger.Debugf("[AllDebrid] processing magnet with %d links", len(magnet.Links))
		for _, link := range magnet.Links {
			a.logger.Debugf("[AllDebrid] checking file: %s (size: %d)", link.Filename, link.Size)
			if a.fileParsers.isVideoFile(link.Filename) {
				// Parse episode info from filename
				season, episode := a.fileParsers.parseEpisodeFromFilename(link.Filename)
				a.logger.Debugf("[AllDebrid] parsed episode info from '%s': S%02dE%02d", link.Filename, season, episode)

				if season > 0 && episode > 0 {
					resolution := a.fileParsers.parseResolutionFromFilename(link.Filename)
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

	return episodeFiles
}

// processVideoFiles processes magnet links into video files
func (a *AllDebrid) processVideoFiles(magnets []struct {
	ID    int64  `json:"id"`
	Hash  string `json:"hash"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Ready bool   `json:"ready"`
	Links []struct {
		Link     string `json:"link"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"links"`
}) []models.VideoFile {
	var videoFiles []models.VideoFile

	for _, magnet := range magnets {
		for _, link := range magnet.Links {
			if a.fileParsers.isVideoFile(link.Filename) {
				videoFiles = append(videoFiles, models.VideoFile{
					Name:   link.Filename,
					Size:   float64(link.Size),
					Link:   link.Link,
					Source: "AllDebrid",
				})
			}
		}
	}

	return videoFiles
}

// Response structures for better type safety
type magnetStatusResponse struct {
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

// File parsing methods

// parseEpisodeFromFilename extracts season and episode numbers from filename using compiled patterns
func (fp *fileParsers) parseEpisodeFromFilename(filename string) (season int, episode int) {
	for _, pattern := range fp.episodePatterns {
		matches := pattern.FindStringSubmatch(filename)
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

// Legacy function for backward compatibility
func parseEpisodeFromFilename(filename string) (season int, episode int) {
	// Create a temporary parser for legacy calls
	tempParser := newFileParsers()
	return tempParser.parseEpisodeFromFilename(filename)
}

// parseResolutionFromFilename extracts resolution from filename using compiled patterns
func (fp *fileParsers) parseResolutionFromFilename(filename string) string {
	for _, pattern := range fp.resolutionPatterns {
		if match := pattern.FindString(filename); match != "" {
			return strings.ToLower(match)
		}
	}
	return "unknown"
}

// Legacy function for backward compatibility
func parseResolutionFromFilename(filename string) string {
	tempParser := newFileParsers()
	return tempParser.parseResolutionFromFilename(filename)
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

// isVideoFile checks if a filename has a video extension using precompiled set
func (fp *fileParsers) isVideoFile(filename string) bool {
	filename = strings.ToLower(filename)
	for ext := range fp.videoExtSet {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

// Legacy function for backward compatibility
func isVideoFile(filename string) bool {
	tempParser := newFileParsers()
	return tempParser.isVideoFile(filename)
}
