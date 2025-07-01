package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type MagnetInfo struct {
	Hash   string `json:"hash"`
	Title  string `json:"title"`
	Source string `json:"source"`
}

type AllDebridMagnet struct {
	Hash  string `json:"hash"`
	Ready bool   `json:"ready"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	ID    int    `json:"id"`
}

type AllDebridUploadResponse struct {
	Status string `json:"status"`
	Data   struct {
		Magnets []AllDebridMagnet `json:"magnets"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type ProcessedMagnet struct {
	Hash   string `json:"hash"`
	Ready  string `json:"ready"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	ID     int    `json:"id"`
	Source string `json:"source"`
}

type FileInfo struct {
	Name   string `json:"n"`
	Size   int64  `json:"s"`
	Link   string `json:"l"`
	Files  []FileInfo `json:"e,omitempty"`
	Source string `json:"source,omitempty"`
}

type AllDebridFilesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Magnets []struct {
			Files []FileInfo `json:"files"`
		} `json:"magnets"`
	} `json:"data"`
}

type AllDebridUnlockResponse struct {
	Status string `json:"status"`
	Data   struct {
		Link string `json:"link"`
	} `json:"data"`
}

type VideoFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Link   string `json:"link"`
	Source string `json:"source"`
}

var cleanupTimeout *time.Timer

func ScheduleCleanup(config *Config, delayMs time.Duration) {
	if cleanupTimeout != nil {
		cleanupTimeout.Stop()
	}
	cleanupTimeout = time.AfterFunc(delayMs, func() {
		CleanupOldMagnets(config, 500, 10)
	})
}

func UploadMagnets(magnets []MagnetInfo, config *Config) ([]ProcessedMagnet, error) {
	if !allDebridRateLimiter.TakeToken() {
		Logger.Warn("rate limited for AllDebrid upload")
		return nil, fmt.Errorf("rate limited")
	}
	
	apiURL := fmt.Sprintf("https://api.alldebrid.com/v4/magnet/upload?apikey=%s", config.APIKeyAllDebrid)

	// Create form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for _, magnet := range magnets {
		if err := writer.WriteField("magnets[]", magnet.Hash); err != nil {
			return nil, err
		}
	}
	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.APIKeyAllDebrid)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Make request
	resp, err := HTTPClient.Do(req)
	if err != nil {
		Logger.Errorf("upload error: %v", err)
		ScheduleCleanup(config, time.Minute)
		return nil, err
	}
	defer resp.Body.Close()

	var response AllDebridUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		Logger.Errorf("failed to decode upload response: %v", err)
		ScheduleCleanup(config, time.Minute)
		return nil, err
	}

	if response.Status == "success" {
		Logger.Infof("successfully uploaded %d magnets", len(response.Data.Magnets))
		ScheduleCleanup(config, time.Minute)
		
		// Store magnets in database
		for _, magnet := range response.Data.Magnets {
			if err := StoreMagnet(fmt.Sprintf("%d", magnet.ID), magnet.Hash, magnet.Name); err != nil {
				Logger.Warnf("failed to store magnet in database: %v", err)
			}
		}

		// Convert to processed magnets
		var processed []ProcessedMagnet
		for _, magnet := range response.Data.Magnets {
			ready := "not ready"
			if magnet.Ready {
				ready = "ready"
			}
			
			// Find source from original magnets
			source := "Unknown"
			for _, orig := range magnets {
				if orig.Hash == magnet.Hash {
					source = orig.Source
					break
				}
			}

			processed = append(processed, ProcessedMagnet{
				Hash:   magnet.Hash,
				Ready:  ready,
				Name:   magnet.Name,
				Size:   magnet.Size,
				ID:     magnet.ID,
				Source: source,
			})
		}

		return processed, nil
	} else {
		// Log error details with full response
		Logger.Debugf("full alldebrid response: %+v", response)
		if response.Error != nil && response.Error.Code != "" && response.Error.Message != "" {
			Logger.Errorf("error uploading magnets: status=%s, code=%s, message=%s",
				response.Status, response.Error.Code, response.Error.Message)
		} else {
			Logger.Warnf("error uploading magnets: status=%s (unknown error)", response.Status)
		}
		ScheduleCleanup(config, time.Minute)
		return []ProcessedMagnet{}, nil
	}
}

func GetFilesFromMagnetID(magnetID int, source string, config *Config) ([]VideoFile, error) {
	apiURL := fmt.Sprintf("https://api.alldebrid.com/v4/magnet/files?apikey=%s", config.APIKeyAllDebrid)

	// Create form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("id[]", fmt.Sprintf("%d", magnetID))
	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.APIKeyAllDebrid)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Make request
	resp, err := HTTPClient.Do(req)
	if err != nil {
		Logger.Errorf("file retrieval error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var response AllDebridFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		Logger.Errorf("failed to decode files response: %v", err)
		return nil, err
	}

	if response.Status != "success" {
		Logger.Warn("failed to retrieve files from alldebrid")
		return []VideoFile{}, nil
	}

	files := response.Data.Magnets[0].Files
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv"}

	// Extract video files recursively
	videoFiles := extractVideoFiles(files, videoExtensions, source)

	Logger.Infof("%d video files found for magnet ID: %d", len(videoFiles), magnetID)
	return videoFiles, nil
}

func extractVideoFiles(files []FileInfo, videoExtensions []string, source string) []VideoFile {
	var videos []VideoFile

	for _, file := range files {
		if len(file.Files) > 0 {
			// Recursively process sub-files
			videos = append(videos, extractVideoFiles(file.Files, videoExtensions, source)...)
		} else if file.Name != "" && file.Link != "" {
			// Check if file is a video
			fileName := strings.ToLower(file.Name)
			for _, ext := range videoExtensions {
				if strings.HasSuffix(fileName, ext) {
					videos = append(videos, VideoFile{
						Name:   file.Name,
						Size:   file.Size,
						Link:   file.Link,
						Source: source,
					})
					break
				}
			}
		}
	}

	return videos
}

func UnlockFileLink(fileLink string, config *Config) (string, error) {
	apiURL := "http://api.alldebrid.com/v4/link/unlock"

	// Create form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("link", fileLink)
	writer.Close()

	// Create request
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+config.APIKeyAllDebrid)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Make request
	resp, err := HTTPClient.Do(req)
	if err != nil {
		Logger.Errorf("unlock error: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	var response AllDebridUnlockResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		Logger.Errorf("failed to decode unlock response: %v", err)
		return "", err
	}

	if response.Status == "success" {
		return response.Data.Link, nil
	} else {
		Logger.Warn("failed to unlock link")
		return "", nil
	}
}

func CleanupOldMagnets(config *Config, maxCount, deleteCount int) error {
	magnets, err := GetAllMagnets()
	if err != nil {
		Logger.Errorf("error during magnet cleanup: %v", err)
		return err
	}

	Logger.Debugf("magnets in database: %d", len(magnets))
	
	if len(magnets) > maxCount {
		toDelete := magnets[:deleteCount]
		Logger.Infof("deleting %d oldest magnets (limit: %d) because total > %d",
			len(toDelete), deleteCount, maxCount)

		// Create form data
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for _, magnet := range toDelete {
			writer.WriteField("ids[]", magnet.ID)
		}
		writer.Close()

		apiURL := fmt.Sprintf("https://api.alldebrid.com/v4/magnet/delete?apikey=%s", config.APIKeyAllDebrid)
		
		// Create request
		req, err := http.NewRequest("POST", apiURL, body)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+config.APIKeyAllDebrid)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		// Make request
		resp, err := HTTPClient.Do(req)
		if err != nil {
			Logger.Errorf("error deleting magnets: %v", err)
			return err
		}
		defer resp.Body.Close()

		var response struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			Logger.Errorf("failed to decode delete response: %v", err)
			return err
		}

		if response.Status == "success" {
			var names []string
			for _, magnet := range toDelete {
				if magnet.Name != "" {
					names = append(names, magnet.Name)
				} else {
					names = append(names, magnet.ID)
				}
			}
			Logger.Infof("deleted magnets: %s", strings.Join(names, ", "))
			
			// Remove from database
			for _, magnet := range toDelete {
				if err := DeleteMagnet(magnet.ID); err != nil {
					Logger.Warnf("failed to delete magnet from database: %v", err)
				}
			}
		} else {
			Logger.Warn("failed to delete magnets from alldebrid")
		}
	}

	return nil
}