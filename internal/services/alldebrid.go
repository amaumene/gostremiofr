package services

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/database"
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
	db          database.Database
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

// SetDB sets the database instance
func (a *AllDebrid) SetDB(db database.Database) {
	a.db = db
}

// newFileParsers creates and compiles all regex patterns for file parsing
func newFileParsers() *fileParsers {
	return &fileParsers{
		episodePatterns:    compilePatterns(getEpisodePatterns()),
		resolutionPatterns: compilePatterns(getResolutionPatterns()),
		videoExtSet:        createVideoExtensionSet(videoExtensions),
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

	a.logger.Debugf("API call returned %d results for %d requested magnets", len(processed), len(magnets))

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
	
	a.logger.Debugf("[AllDebrid] uploading magnet URL: %s", magnetURL)

	// Use our local client
	resp, err := a.client.UploadMagnet(apiKey, []string{magnetURL})
	if err != nil {
		return fmt.Errorf("failed to upload magnet: %w", err)
	}

	// Check response status and errors
	if err := a.checkAPIResponse(resp.Status, resp.Error, resp.Data.Magnets); err != nil {
		return err
	}

	// Store magnet information in database for cleanup
	if a.db != nil && len(resp.Data.Magnets) > 0 {
		magnet := resp.Data.Magnets[0]
		// Use AllDebrid's magnet ID as part of our ID to ensure uniqueness
		dbMagnet := &database.Magnet{
			ID:           fmt.Sprintf("ad_%s_%d_%d", hash, magnet.ID, time.Now().UnixNano()),
			Hash:         hash,
			Name:         title,
			AllDebridID:  fmt.Sprintf("%d", magnet.ID),
			AllDebridKey: apiKey,
		}
		if err := a.db.StoreMagnet(dbMagnet); err != nil {
			// Check if it's a duplicate constraint error
			if strings.Contains(err.Error(), "unique constraint") {
				a.logger.Debugf("magnet already exists in database: %s", hash)
			} else {
				a.logger.Warnf("failed to store magnet in database: %v", err)
			}
			// Don't fail the upload, just log the warning
		}
	}

	return nil
}

func (a *AllDebrid) GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error) {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	a.rateLimiter.Wait()

	// Get magnet files
	a.logger.Debugf("[AllDebrid] getting files for magnet ID: %s", magnetID)
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
	
	a.logger.Debugf("[AllDebrid] unlocking link: %s", link)

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

// DeleteMagnet deletes a magnet from AllDebrid
func (a *AllDebrid) DeleteMagnet(magnetID, apiKey string) error {
	// Validate API key
	apiKey, err := a.validateAndPrepareAPIKey(apiKey)
	if err != nil {
		return err
	}

	a.rateLimiter.Wait()
	
	a.logger.Debugf("[AllDebrid] deleting magnet ID: %s", magnetID)

	// Use our local client
	err = a.client.DeleteMagnet(apiKey, magnetID)
	if err != nil {
		return fmt.Errorf("failed to delete magnet: %w", err)
	}

	a.logger.Infof("[AllDebrid] successfully deleted non-cached magnet ID: %s", magnetID)
	return nil
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
		a.logger.Errorf("invalid API key format (key: %s)", a.validator.MaskAPIKey(apiKey))
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
	requestURL := allDebridAPIBase + allDebridMagnetStatus
	formData := a.buildMagnetFormData(apiKey, hashes)
	
	a.logger.Infof("checking %d specific magnets (API key: %s)", len(hashes), a.validator.MaskAPIKey(apiKey))
	a.logger.Infof("making POST request to %s", requestURL)
	a.logger.Debugf("[AllDebrid] API URL: %s (POST with %d hashes)", requestURL, len(hashes))

	resp, err := a.makeAPIRequest(requestURL, formData)
	if err != nil {
		return nil, fmt.Errorf("failed to check magnets: %w", err)
	}

	var response magnetStatusResponse
	if err := a.parseAPIResponse(resp, &response); err != nil {
		return nil, err
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
			// StatusCode 4 means "Ready" (files are available/cached)
			// StatusCode 0 = Not started, 1 = Downloading, 2 = Downloaded, 3 = Error, 4 = Ready
			ready := magnet.StatusCode == allDebridStatusReady && len(magnet.Links) > 0
			name := magnet.Filename
			if name == "" {
				name = original.Title
			}

			// Log cache status clearly
			var statusDesc string
			switch magnet.StatusCode {
			case 0:
				statusDesc = "NOT_STARTED"
			case 1:
				statusDesc = "DOWNLOADING (NOT CACHED)"
			case 2:
				statusDesc = "DOWNLOADED"
			case 3:
				statusDesc = "ERROR"
			case 4:
				statusDesc = "READY (CACHED)"
			default:
				statusDesc = fmt.Sprintf("UNKNOWN_%d", magnet.StatusCode)
			}

			a.logger.Infof("[AllDebrid] magnet status - %s: %s (hash: %s, links: %d)",
				name, statusDesc, magnet.Hash[:12], len(magnet.Links))

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

	return nil
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

// parseResolutionFromFilename extracts resolution from filename using compiled patterns
func (fp *fileParsers) parseResolutionFromFilename(filename string) string {
	for _, pattern := range fp.resolutionPatterns {
		if match := pattern.FindString(filename); match != "" {
			return strings.ToLower(match)
		}
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
