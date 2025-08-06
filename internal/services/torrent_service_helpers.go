package services

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/logger"
)

// Compile regex once at package level for performance
var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9\s]+`)

func (b *BaseTorrentService) filterByYear(torrent GenericTorrent, mediaType string, year int, serviceName string) bool {
	if mediaType == "movie" && !b.MatchesYear(torrent.GetTitle(), year) {
		logger := logger.New()
		logger.Debugf("[%s] torrent filtered by year - title: %s (expected: %d)", serviceName, torrent.GetTitle(), year)
		return false
	}
	return true
}

func (b *BaseTorrentService) createTorrentInfo(torrent GenericTorrent) models.TorrentInfo {
	return models.TorrentInfo{
		ID:     torrent.GetID(),
		Title:  torrent.GetTitle(),
		Hash:   torrent.GetHash(),
		Source: torrent.GetSource(),
		Size:   torrent.GetSize(),
	}
}

func (b *BaseTorrentService) classifyByType(torrent GenericTorrent, mediaType string) (string, bool) {
	if torrent.GetType() == "" {
		return "", false
	}
	
	if mediaType == "movie" && torrent.GetType() == "movie" {
		return "movie", true
	}
	
	if mediaType == "series" && torrent.GetType() == "tvshow" {
		return "", false // Continue to title-based classification
	}
	
	return "", false
}

func (b *BaseTorrentService) classifyBySeasonEpisode(torrent GenericTorrent, mediaType string, season, episode int, serviceName string) (string, bool) {
	if torrent.GetSeason() == 0 || torrent.GetEpisode() == 0 {
		return "", false
	}
	
	logger := logger.New()
	
	if mediaType == "series" && season > 0 && episode > 0 {
		if torrent.GetSeason() == season && torrent.GetEpisode() == episode {
			logger.Infof("[%s] episode match found - s%02de%02d: %s", serviceName, season, episode, torrent.GetTitle())
			return "episode", true
		}
		logger.Infof("[%s] episode mismatch - found s%02de%02d, requested s%02de%02d: %s", 
			serviceName, torrent.GetSeason(), torrent.GetEpisode(), season, episode, torrent.GetTitle())
		return "", false
	}
	
	return "episode", true
}

func (b *BaseTorrentService) classifyTorrent(torrent GenericTorrent, mediaType string, season, episode int, serviceName string) (string, bool) {
	// Try type-based classification first
	if classification, shouldAdd := b.classifyByType(torrent, mediaType); shouldAdd {
		return classification, true
	}
	
	// Try season/episode-based classification
	if classification, shouldAdd := b.classifyBySeasonEpisode(torrent, mediaType, season, episode, serviceName); shouldAdd {
		return classification, true
	}
	
	// Fall back to title-based classification
	classification := ClassifyTorrent(torrent.GetTitle(), mediaType, season, episode, b)
	return classification, classification != ""
}

func (b *BaseTorrentService) addTorrentToResults(torrentInfo models.TorrentInfo, classification string, results *models.TorrentResults, serviceName string) {
	logger := logger.New()
	
	switch classification {
	case "movie":
		logger.Debugf("[%s] adding movie torrent - title: %s", serviceName, torrentInfo.Title)
		results.MovieTorrents = append(results.MovieTorrents, torrentInfo)
	case "complete_series":
		results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, torrentInfo)
	case "episode":
		logger.Debugf("[%s] adding episode torrent - title: %s", serviceName, torrentInfo.Title)
		results.EpisodeTorrents = append(results.EpisodeTorrents, torrentInfo)
	case "season":
		results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrentInfo)
	}
}

func (b *BaseTorrentService) processSingleTorrent(torrent GenericTorrent, mediaType string, season, episode int, serviceName string, year int) (models.TorrentInfo, string, bool) {
	// First filter: year matching for movies
	if !b.filterByYear(torrent, mediaType, year, serviceName) {
		return models.TorrentInfo{}, "", false
	}
	
	torrentInfo := b.createTorrentInfo(torrent)
	classification, shouldAdd := b.classifyTorrent(torrent, mediaType, season, episode, serviceName)
	
	if !shouldAdd {
		return models.TorrentInfo{}, "", false
	}
	
	logger := logger.New()
	logger.Debugf("[%s] torrent classification result - title: '%s', type: %s", serviceName, torrent.GetTitle(), classification)
	
	// Third filter: resolution (after classification)
	if !b.MatchesResolutionFilter(torrent.GetTitle()) {
		logger.Debugf("[%s] torrent filtered by resolution after classification - title: %s", serviceName, torrent.GetTitle())
		return models.TorrentInfo{}, "", false
	}
	
	return torrentInfo, classification, true
}

func extractTitleAndYear(query string) (string, string) {
	parts := strings.Fields(query)
	if len(parts) <= 1 {
		return query, ""
	}
	
	lastPart := parts[len(parts)-1]
	if yearMatch, _ := regexp.MatchString(`^\d{4}$`, lastPart); yearMatch {
		title := strings.Join(parts[:len(parts)-1], " ")
		return title, lastPart
	}
	
	return query, ""
}

func formatQueryString(title string) string {
	// Keep only alphanumeric characters and spaces
	title = nonAlphanumericRegex.ReplaceAllString(title, "")
	
	// Replace spaces with + for URL query
	title = strings.ReplaceAll(title, " ", "+")
	
	// Trim any leading/trailing + that might result from trimmed spaces
	title = strings.Trim(title, "+")
	
	return title
}

func buildMovieQuery(title, year string) string {
	if year != "" {
		return fmt.Sprintf("%s+%s", title, year)
	}
	return title
}

func buildSeriesQuery(title string, season, episode int, specificEpisode bool) string {
	if specificEpisode && season > 0 && episode > 0 {
		return fmt.Sprintf("%s+s%02de%02d", title, season, episode)
	}
	
	if season > 0 {
		return fmt.Sprintf("%s+s%02d", title, season)
	}
	
	return title
}

func classifyAsMovie(title, mediaType string) (string, bool) {
	if mediaType == "movie" {
		logger := logger.New()
		logger.Debugf("torrent classification - '%s' classified as movie (media type)", title)
		return "movie", true
	}
	return "", false
}

func classifyAsCompleteSeries(title string) (string, bool) {
	if strings.Contains(strings.ToUpper(title), "COMPLETE") {
		logger := logger.New()
		logger.Debugf("torrent classification - '%s' classified as complete_series (contains COMPLETE)", title)
		return "complete_series", true
	}
	return "", false
}

func classifyBySeason(title string, season int, base *BaseTorrentService) (string, bool) {
	if season > 0 && base.MatchesSeason(title, season) {
		logger := logger.New()
		logger.Debugf("torrent classification - '%s' classified as season (matches season %d)", title, season)
		return "season", true
	}
	return "", false
}

func classifyByEpisode(title string, season, episode int, base *BaseTorrentService) (string, bool) {
	if season > 0 && episode > 0 && base.MatchesEpisode(title, season, episode) {
		logger := logger.New()
		logger.Debugf("torrent classification - '%s' classified as episode (matches s%02de%02d)", title, season, episode)
		return "episode", true
	}
	return "", false
}

func classifySeasonEpisode(title string, season int, base *BaseTorrentService) (string, bool) {
	if season > 0 && base.ContainsSeasonEpisode(title) {
		titleUpper := strings.ToUpper(title)
		seasonPattern := fmt.Sprintf("S%02d", season)
		if strings.Contains(titleUpper, seasonPattern) {
			logger := logger.New()
			logger.Debugf("torrent classification - '%s' classified as episode (part of season %d)", title, season)
			return "episode", true
		}
	}
	return "", false
}