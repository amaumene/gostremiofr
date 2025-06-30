package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

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
	Logger.Info("--------------------")

	// Retrieve configuration
	config, err := GetConfig(c)
	if err != nil {
		Logger.Error("❌ Invalid configuration in request: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mediaType := c.Param("type")
	id := c.Param("id")
	
	// Remove .json suffix if present
	if strings.HasSuffix(id, ".json") {
		id = strings.TrimSuffix(id, ".json")
	}
	
	Logger.Info(fmt.Sprintf("📥 Stream request received for ID: %s", id))

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
	Logger.Info(fmt.Sprintf("🔍 Retrieving TMDB info for IMDB ID: %s", imdbId))
	tmdbData, err := GetTMDBData(imdbId, config)
	if err != nil || tmdbData == nil {
		Logger.Warn(fmt.Sprintf("❌ Unable to retrieve TMDB info for %s", imdbId))
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Call searchYgg and searchSharewood in parallel
	var yggResults TorrentResults
	var sharewoodResults SharewoodResults
	var wg sync.WaitGroup
	var yggErr, sharewoodErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		yggResults, yggErr = SearchYgg(
			tmdbData.Title,
			tmdbData.Type,
			season,
			episode,
			config,
			tmdbData.FrenchTitle,
		)
	}()

	go func() {
		defer wg.Done()
		sharewoodResults, sharewoodErr = SearchSharewood(
			tmdbData.Title,
			tmdbData.Type,
			season,
			episode,
			config,
		)
	}()

	wg.Wait()

	if yggErr != nil {
		Logger.Error("❌ YGG search error: ", yggErr)
	}
	if sharewoodErr != nil {
		Logger.Error("❌ Sharewood search error: ", sharewoodErr)
	}

	// Combine results from both sources
	combinedResults := CombinedTorrentResults{
		CompleteSeriesTorrents: make([]interface{}, 0),
		CompleteSeasonTorrents: make([]interface{}, 0),
		EpisodeTorrents:        make([]interface{}, 0),
		MovieTorrents:          make([]interface{}, 0),
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
		Logger.Warn("❌ No torrents found for the requested content.")
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Combine torrents based on type (series or movie)
	var allTorrents []TorrentInfo
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

	// Retrieve hashes for the torrents
	var magnets []MagnetInfo
	for _, torrent := range allTorrents {
		if torrent.Hash != "" {
			magnets = append(magnets, MagnetInfo{
				Hash:   torrent.Hash,
				Title:  torrent.Title,
				Source: torrent.Source,
			})
		} else if torrent.ID != 0 {
			hash, err := GetTorrentHashFromYgg(torrent.ID)
			if err == nil && hash != "" {
				magnets = append(magnets, MagnetInfo{
					Hash:   hash,
					Title:  torrent.Title,
					Source: torrent.Source,
				})
			} else {
				Logger.Warn(fmt.Sprintf("❌ Skipping torrent: %s (no hash found)", torrent.Title))
			}
		}
	}

	Logger.Info(fmt.Sprintf("✅ Processed %d torrents (limited to %d).", len(magnets), maxTorrentsToProcess))

	// Check if any magnets are available
	if len(magnets) == 0 {
		Logger.Warn("❌ No magnets available for upload.")
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Upload magnets to AllDebrid
	Logger.Info(fmt.Sprintf("🔄 Uploading %d magnets to AllDebrid", len(magnets)))
	uploadedStatuses, err := UploadMagnets(magnets, config)
	if err != nil {
		Logger.Error("❌ Failed to upload magnets: ", err)
		c.JSON(http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	// Filter ready torrents
	var readyTorrents []ProcessedMagnet
	for _, status := range uploadedStatuses {
		if status.Ready == "✅ Ready" {
			readyTorrents = append(readyTorrents, status)
		}
	}

	Logger.Info(fmt.Sprintf("✅ %d ready torrents found.", len(readyTorrents)))

	// Unlock files from ready torrents
	var streams []Stream
	for _, torrent := range readyTorrents {
		if len(streams) >= config.FilesToShow {
			Logger.Info(fmt.Sprintf("🎯 Reached the maximum number of streams (%d). Stopping.", config.FilesToShow))
			break
		}

		videoFiles, err := GetFilesFromMagnetID(torrent.ID, torrent.Source, config)
		if err != nil {
			Logger.Error("❌ Failed to get files from magnet: ", err)
			continue
		}

		// Filter relevant video files
		var filteredFiles []VideoFile
		for _, file := range videoFiles {
			fileName := strings.ToLower(file.Name)

			if mediaType == "series" {
				if matchesEpisode(fileName, season, episode) {
					filteredFiles = append(filteredFiles, file)
				}
			} else if mediaType == "movie" {
				Logger.Info(fmt.Sprintf("✅ File included (movie): %s", file.Name))
				filteredFiles = append(filteredFiles, file)
			}
		}

		// Unlock filtered files
		for _, file := range filteredFiles {
			if len(streams) >= config.FilesToShow {
				Logger.Info(fmt.Sprintf("🎯 Reached the maximum number of streams (%d). Stopping.", config.FilesToShow))
				break
			}

			unlockedLink, err := UnlockFileLink(file.Link, config)
			if err != nil || unlockedLink == "" {
				Logger.Error("❌ Failed to unlock file: ", err)
				continue
			}

			parsed := ParseFileName(file.Name)
			streamName := fmt.Sprintf("❤️ %s + AD | 🖥️ %s | 🎞️ %s", torrent.Source, parsed.Resolution, parsed.Codec)
			
			var streamTitle string
			if season != "" && episode != "" {
				streamTitle = fmt.Sprintf("%s - S%sE%s\n%s\n🎬 %s | 💾 %s",
					tmdbData.Title,
					PadString(season, 2),
					PadString(episode, 2),
					file.Name,
					parsed.Source,
					FormatSize(file.Size))
			} else {
				streamTitle = fmt.Sprintf("%s\n%s\n🎬 %s | 💾 %s",
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
			Logger.Info(fmt.Sprintf("✅ Unlocked video: %s", file.Name))
		}

		// Log a warning if no files were unlocked
		if len(filteredFiles) == 0 {
			Logger.Warn(fmt.Sprintf("⚠️ No files matched the requested season/episode for torrent %s", torrent.Hash))
		}
	}

	Logger.Info(fmt.Sprintf("🎉 %d stream(s) obtained", len(streams)))
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
	Logger.Debug(fmt.Sprintf("🔍 Checking episode pattern \"%s\" and \"%s\" against \"%s\": %t",
		pattern1, pattern2, title, matches))
	
	return matches
}