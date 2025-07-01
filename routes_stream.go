package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type StreamResponse struct {
	Streams []Stream `json:"streams"`
}

type Stream struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type CombinedTorrentResults struct {
	CompleteSeriesTorrents []interface{} `json:"completeSeriesTorrents"`
	CompleteSeasonTorrents []interface{} `json:"completeSeasonTorrents"`
	EpisodeTorrents        []interface{} `json:"episodeTorrents"`
	MovieTorrents          []interface{} `json:"movieTorrents"`
}

func setupStreamRoutes(r *gin.Engine) {
	r.GET("/:variables/stream/:type/:id", handleStream)
}

func handleStream(c *gin.Context) {
	// Log the start of a new stream request
	Logger.Info("processing stream request")

	// Retrieve configuration
	config, err := GetConfig(c)
	if err != nil {
		Logger.Errorf("invalid configuration in request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mediaType := c.Param("type")
	id := c.Param("id")
	
	// Remove .json suffix if present
	if strings.HasSuffix(id, ".json") {
		id = strings.TrimSuffix(id, ".json")
	}
	
	Logger.Infof("stream request received for ID: %s", id)

	// Parse the ID to extract IMDB ID, season, and episode
	parts := strings.Split(id, ":")
	imdbId := parts[0]
	var season, episode string
	if len(parts) > 1 {
		season = parts[1]
	}
	if len(parts) > 2 {
		episode = parts[2]
	}

	// Retrieve TMDB data based on IMDB ID
	Logger.Infof("retrieving tmdb info for imdb id: %s", imdbId)
	tmdbData, err := GetTMDBData(imdbId, config)
	if err != nil || tmdbData == nil {
		Logger.Warnf("unable to retrieve tmdb info for %s", imdbId)
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Call searchYgg and searchSharewood in parallel with timeout
	var yggResults TorrentResults
	var sharewoodResults SharewoodResults
	var wg sync.WaitGroup
	var yggErr, sharewoodErr error

	searchCtx, searchCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer searchCancel()

	wg.Add(2)
	go func() {
		defer wg.Done()
		done := make(chan struct{})
		go func() {
			defer close(done)
			yggResults, yggErr = SearchYgg(
				tmdbData.Title,
				tmdbData.Type,
				season,
				episode,
				config,
				tmdbData.FrenchTitle,
			)
		}()
		
		select {
		case <-done:
		case <-searchCtx.Done():
			yggErr = searchCtx.Err()
		}
	}()

	go func() {
		defer wg.Done()
		done := make(chan struct{})
		go func() {
			defer close(done)
			sharewoodResults, sharewoodErr = SearchSharewood(
				tmdbData.Title,
				tmdbData.Type,
				season,
				episode,
				config,
			)
		}()
		
		select {
		case <-done:
		case <-searchCtx.Done():
			sharewoodErr = searchCtx.Err()
		}
	}()

	wg.Wait()

	if yggErr != nil {
		Logger.Errorf("ygg search error: %v", yggErr)
	}
	if sharewoodErr != nil {
		Logger.Errorf("sharewood search error: %v", sharewoodErr)
	}

	// Pre-allocate slices with estimated capacity
	estimatedCapacity := config.FilesToShow * 2
	combinedResults := CombinedTorrentResults{
		CompleteSeriesTorrents: make([]interface{}, 0, estimatedCapacity),
		CompleteSeasonTorrents: make([]interface{}, 0, estimatedCapacity),
		EpisodeTorrents:        make([]interface{}, 0, estimatedCapacity),
		MovieTorrents:          make([]interface{}, 0, estimatedCapacity),
	}

	// Add YGG results
	for _, torrent := range yggResults.CompleteSeriesTorrents {
		combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, torrent)
	}
	for _, torrent := range yggResults.CompleteSeasonTorrents {
		combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, torrent)
	}
	for _, torrent := range yggResults.EpisodeTorrents {
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, torrent)
	}
	for _, torrent := range yggResults.MovieTorrents {
		combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, torrent)
	}

	// Add Sharewood results
	for _, torrent := range sharewoodResults.CompleteSeriesTorrents {
		combinedResults.CompleteSeriesTorrents = append(combinedResults.CompleteSeriesTorrents, torrent)
	}
	for _, torrent := range sharewoodResults.CompleteSeasonTorrents {
		combinedResults.CompleteSeasonTorrents = append(combinedResults.CompleteSeasonTorrents, torrent)
	}
	for _, torrent := range sharewoodResults.EpisodeTorrents {
		combinedResults.EpisodeTorrents = append(combinedResults.EpisodeTorrents, torrent)
	}
	for _, torrent := range sharewoodResults.MovieTorrents {
		combinedResults.MovieTorrents = append(combinedResults.MovieTorrents, torrent)
	}

	// Check if any results were found
	totalResults := len(combinedResults.CompleteSeriesTorrents) +
		len(combinedResults.CompleteSeasonTorrents) +
		len(combinedResults.EpisodeTorrents) +
		len(combinedResults.MovieTorrents)

	if totalResults == 0 {
		Logger.Warn("no torrents found for the requested content")
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Combine torrents based on type (series or movie)
	allTorrents := make([]TorrentInfo, 0, maxTorrentsToProcess)
	if mediaType == "series" {
		// Add complete series torrents
		for _, t := range combinedResults.CompleteSeriesTorrents {
			if torrent := convertToTorrentInfo(t); torrent != nil {
				allTorrents = append(allTorrents, *torrent)
			}
		}

		// Add complete season torrents
		for _, t := range combinedResults.CompleteSeasonTorrents {
			if torrent := convertToTorrentInfo(t); torrent != nil {
				allTorrents = append(allTorrents, *torrent)
			}
		}

		// Filter episode torrents to ensure they match the requested season and episode
		for _, t := range combinedResults.EpisodeTorrents {
			if torrent := convertToTorrentInfo(t); torrent != nil {
				if matchesEpisode(torrent.Title, season, episode) {
					allTorrents = append(allTorrents, *torrent)
				}
			}
		}
	} else if mediaType == "movie" {
		for _, t := range combinedResults.MovieTorrents {
			if torrent := convertToTorrentInfo(t); torrent != nil {
				allTorrents = append(allTorrents, *torrent)
			}
		}
	}

	// Limit the number of torrents to process
	maxTorrentsToProcess := config.FilesToShow * 2
	if len(allTorrents) > maxTorrentsToProcess {
		allTorrents = allTorrents[:maxTorrentsToProcess]
	}

	// Retrieve hashes for the torrents using worker pool
	magnets := make([]MagnetInfo, 0, len(allTorrents))
	magnetCh := make(chan MagnetInfo, len(allTorrents))
	
	// Worker pool for hash retrieval
	const maxWorkers = 5
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	for _, torrent := range allTorrents {
		if torrent.Hash != "" {
			magnets = append(magnets, MagnetInfo{
				Hash:   torrent.Hash,
				Title:  torrent.Title,
				Source: torrent.Source,
			})
		} else if torrent.ID != 0 {
			wg.Add(1)
			go func(t TorrentInfo) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				
				select {
				case <-ctx.Done():
					return
				default:
					hash, err := GetTorrentHashFromYgg(t.ID)
					if err == nil && hash != "" {
						select {
						case magnetCh <- MagnetInfo{
							Hash:   hash,
							Title:  t.Title,
							Source: t.Source,
						}:
						case <-ctx.Done():
							return
						}
					} else {
						Logger.Warnf("skipping torrent: %s (no hash found)", t.Title)
					}
				}
			}(torrent)
		}
	}
	
	// Close channel when all workers are done
	go func() {
		wg.Wait()
		close(magnetCh)
	}()
	
	// Collect results
	for magnet := range magnetCh {
		magnets = append(magnets, magnet)
	}

	Logger.Infof("processed %d torrents (limited to %d)", len(magnets), maxTorrentsToProcess)

	// Check if any magnets are available
	if len(magnets) == 0 {
		Logger.Warn("no magnets available for upload")
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Upload magnets to AllDebrid
	Logger.Infof("uploading %d magnets to alldebrid", len(magnets))
	uploadedStatuses, err := UploadMagnets(magnets, config)
	if err != nil {
		Logger.Errorf("failed to upload magnets: %v", err)
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Filter ready torrents
	readyTorrents := make([]ProcessedMagnet, 0, len(uploadedStatuses))
	for _, status := range uploadedStatuses {
		if status.Ready == "ready" {
			readyTorrents = append(readyTorrents, status)
		}
	}

	Logger.Infof("%d ready torrents found", len(readyTorrents))

	// Unlock files from ready torrents
	streams := make([]Stream, 0, config.FilesToShow)
	for _, torrent := range readyTorrents {
		if len(streams) >= config.FilesToShow {
			Logger.Infof("reached maximum number of streams (%d), stopping", config.FilesToShow)
			break
		}

		videoFiles, err := GetFilesFromMagnetID(torrent.ID, torrent.Source, config)
		if err != nil {
			Logger.Errorf("failed to get files from magnet: %v", err)
			continue
		}

		// Filter relevant video files
		filteredFiles := make([]VideoFile, 0, len(videoFiles))
		for _, file := range videoFiles {
			fileName := strings.ToLower(file.Name)

			if mediaType == "series" {
				if matchesEpisode(fileName, season, episode) {
					filteredFiles = append(filteredFiles, file)
				}
			} else if mediaType == "movie" {
				Logger.Infof("file included (movie): %s", file.Name)
				filteredFiles = append(filteredFiles, file)
			}
		}

		// Unlock filtered files
		for _, file := range filteredFiles {
			if len(streams) >= config.FilesToShow {
				Logger.Infof("reached maximum number of streams (%d), stopping", config.FilesToShow)
				break
			}

			unlockedLink, err := UnlockFileLink(file.Link, config)
			if err != nil || unlockedLink == "" {
				Logger.Errorf("failed to unlock file: %v", err)
				continue
			}

			parsed := ParseFileName(file.Name)
			streamName := fmt.Sprintf("❤️ %s + AD | 🖥️ %s | 🎞️ %s", torrent.Source, parsed.Resolution, parsed.Codec)
			
			var streamTitle string
			if season != "" && episode != "" {
				streamTitle = fmt.Sprintf("%s - S%sE%s\n%s\n%s | %s",
					tmdbData.Title,
					PadString(season, 2),
					PadString(episode, 2),
					file.Name,
					parsed.Source,
					FormatSize(file.Size))
			} else {
				streamTitle = fmt.Sprintf("%s\n%s\n%s | %s",
					tmdbData.Title,
					file.Name,
					parsed.Source,
					FormatSize(file.Size))
			}

			streams = append(streams, Stream{
				Name:  streamName,
				Title: streamTitle,
				URL:   unlockedLink,
			})
			Logger.Infof("unlocked video: %s", file.Name)
		}

		// Log a warning if no files were unlocked
		if len(filteredFiles) == 0 {
			Logger.Warnf("no files matched the requested season/episode for torrent %s", torrent.Hash)
		}
	}

	Logger.Infof("generated %d streams", len(streams))
	c.JSON(http.StatusOK, StreamResponse{Streams: streams})
}

type TorrentInfo struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Hash   string `json:"hash"`
	Source string `json:"source"`
}

func convertToTorrentInfo(t interface{}) *TorrentInfo {
	switch torrent := t.(type) {
	case YggTorrent:
		return &TorrentInfo{
			ID:     torrent.ID,
			Title:  torrent.Title,
			Hash:   torrent.Hash,
			Source: torrent.Source,
		}
	case SharewoodTorrent:
		return &TorrentInfo{
			ID:     torrent.ID,
			Title:  torrent.Name,
			Hash:   torrent.InfoHash,
			Source: torrent.Source,
		}
	default:
		return nil
	}
}

func matchesEpisode(title, season, episode string) bool {
	if season == "" || episode == "" {
		return false
	}

	titleLower := strings.ToLower(title)
	seasonFormatted := PadString(season, 2)
	episodeFormatted := PadString(episode, 2)
	
	pattern1 := fmt.Sprintf("s%se%s", seasonFormatted, episodeFormatted)
	pattern2 := fmt.Sprintf("s%s.e%s", seasonFormatted, episodeFormatted)
	
	matches := strings.Contains(titleLower, pattern1) || strings.Contains(titleLower, pattern2)
	Logger.Debugf("checking episode pattern '%s' and '%s' against '%s': %t",
		pattern1, pattern2, title, matches)
	
	return matches
}