package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gostremiofr/gostremiofr/internal/models"
	"github.com/gostremiofr/gostremiofr/pkg/logger"
	"github.com/gostremiofr/gostremiofr/pkg/ratelimiter"
)

type AllDebrid struct {
	apiKey      string
	rateLimiter *ratelimiter.TokenBucket
	httpClient  *http.Client
	logger      logger.Logger
}

func NewAllDebrid(apiKey string) *AllDebrid {
	return &AllDebrid{
		apiKey:      apiKey,
		rateLimiter: ratelimiter.NewTokenBucket(15, 3),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.New(),
	}
}

func (a *AllDebrid) CheckMagnets(magnets []models.MagnetInfo, apiKey string) ([]models.ProcessedMagnet, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	if apiKey == "" {
		return nil, fmt.Errorf("AllDebrid API key not provided")
	}
	
	var hashes []string
	hashToMagnet := make(map[string]models.MagnetInfo)
	
	for _, magnet := range magnets {
		hashes = append(hashes, magnet.Hash)
		hashToMagnet[magnet.Hash] = magnet
	}
	
	a.rateLimiter.Wait()
	
	requestURL := fmt.Sprintf("https://api.alldebrid.com/v4/magnet/status?agent=stremio&apikey=%s&magnets[]=%s",
		apiKey, strings.Join(hashes, "&magnets[]="))
	
	resp, err := a.httpClient.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to check magnets: %w", err)
	}
	defer resp.Body.Close()
	
	var response struct {
		Status string `json:"status"`
		Data   struct {
			Magnets []models.AllDebridMagnet `json:"magnets"`
		} `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if response.Status != "success" {
		return nil, fmt.Errorf("AllDebrid API error: %s", response.Status)
	}
	
	var processed []models.ProcessedMagnet
	for _, magnet := range response.Data.Magnets {
		if original, ok := hashToMagnet[magnet.Hash]; ok {
			processed = append(processed, models.ProcessedMagnet{
				Hash:   magnet.Hash,
				Ready:  magnet.Ready,
				Name:   magnet.Name,
				Size:   magnet.Size,
				ID:     magnet.ID,
				Source: original.Source,
			})
		}
	}
	
	return processed, nil
}

func (a *AllDebrid) UploadMagnet(hash, title, apiKey string) error {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	if apiKey == "" {
		return fmt.Errorf("AllDebrid API key not provided")
	}
	
	a.rateLimiter.Wait()
	
	magnetURL := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hash, url.QueryEscape(title))
	requestURL := fmt.Sprintf("https://api.alldebrid.com/v4/magnet/upload?agent=stremio&apikey=%s&magnets[]=%s",
		apiKey, url.QueryEscape(magnetURL))
	
	resp, err := a.httpClient.Get(requestURL)
	if err != nil {
		return fmt.Errorf("failed to upload magnet: %w", err)
	}
	defer resp.Body.Close()
	
	var response struct {
		Status string `json:"status"`
		Data   struct {
			Magnets []struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			} `json:"magnets"`
		} `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	
	if response.Status != "success" {
		return fmt.Errorf("AllDebrid API error: %s", response.Status)
	}
	
	if len(response.Data.Magnets) > 0 && response.Data.Magnets[0].Error.Code != "" {
		return fmt.Errorf("upload error: %s - %s", 
			response.Data.Magnets[0].Error.Code,
			response.Data.Magnets[0].Error.Message)
	}
	
	return nil
}

func (a *AllDebrid) GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error) {
	if apiKey == "" {
		apiKey = a.apiKey
	}
	
	if apiKey == "" {
		return nil, fmt.Errorf("AllDebrid API key not provided")
	}
	
	a.rateLimiter.Wait()
	
	requestURL := fmt.Sprintf("https://api.alldebrid.com/v4/magnet/files?agent=stremio&apikey=%s&id=%s",
		apiKey, magnetID)
	
	resp, err := a.httpClient.Get(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get video files: %w", err)
	}
	defer resp.Body.Close()
	
	var response struct {
		Status string `json:"status"`
		Data   struct {
			Files []models.FileInfo `json:"files"`
		} `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if response.Status != "success" {
		return nil, fmt.Errorf("AllDebrid API error: %s", response.Status)
	}
	
	var videoFiles []models.VideoFile
	for _, file := range response.Data.Files {
		videoFiles = append(videoFiles, extractVideoFiles(file, "")...)
	}
	
	return videoFiles, nil
}

func extractVideoFiles(file models.FileInfo, parentPath string) []models.VideoFile {
	var videos []models.VideoFile
	
	fullPath := parentPath
	if fullPath != "" {
		fullPath += "/" + file.Name
	} else {
		fullPath = file.Name
	}
	
	if len(file.Files) > 0 {
		for _, subFile := range file.Files {
			videos = append(videos, extractVideoFiles(subFile, fullPath)...)
		}
	} else if isVideoFile(file.Name) {
		videos = append(videos, models.VideoFile{
			Name:   fullPath,
			Size:   file.Size,
			Link:   file.Link,
			Source: file.Source,
		})
	}
	
	return videos
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