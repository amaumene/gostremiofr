package services

import (
	"fmt"
	"strings"

	"github.com/amaumene/gostremiofr/internal/models"
)

// Package-level regex removed - using torrentsearch package functions

// filterByYear removed - movies now search with year appended instead of filtering

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
	
	if mediaType == "series" && season > 0 && episode > 0 {
		if torrent.GetSeason() == season && torrent.GetEpisode() == episode {
			return "episode", true
		}
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
	switch classification {
	case "movie":
		results.MovieTorrents = append(results.MovieTorrents, torrentInfo)
	case "complete_series":
		results.CompleteSeriesTorrents = append(results.CompleteSeriesTorrents, torrentInfo)
	case "episode":
		results.EpisodeTorrents = append(results.EpisodeTorrents, torrentInfo)
	case "season":
		results.CompleteSeasonTorrents = append(results.CompleteSeasonTorrents, torrentInfo)
	}
}

func (b *BaseTorrentService) processSingleTorrent(torrent GenericTorrent, mediaType string, season, episode int, serviceName string, year int) (models.TorrentInfo, string, bool) {
	torrentInfo := b.createTorrentInfo(torrent)
	classification, shouldAdd := b.classifyTorrent(torrent, mediaType, season, episode, serviceName)
	
	if !shouldAdd {
		return models.TorrentInfo{}, "", false
	}
	
	return torrentInfo, classification, true
}

// Legacy query building functions removed - now using torrentsearch package functions

func classifyAsMovie(title, mediaType string) (string, bool) {
	if mediaType == "movie" {
		return "movie", true
	}
	return "", false
}

func classifyAsCompleteSeries(title string) (string, bool) {
	if strings.Contains(strings.ToUpper(title), "COMPLETE") {
		return "complete_series", true
	}
	return "", false
}

func classifyBySeason(title string, season int, base *BaseTorrentService) (string, bool) {
	if season > 0 && base.MatchesSeason(title, season) {
		return "season", true
	}
	return "", false
}

func classifyByEpisode(title string, season, episode int, base *BaseTorrentService) (string, bool) {
	if season > 0 && episode > 0 && base.MatchesEpisode(title, season, episode) {
		return "episode", true
	}
	return "", false
}

func classifySeasonEpisode(title string, season int, base *BaseTorrentService) (string, bool) {
	if season > 0 && base.ContainsSeasonEpisode(title) {
		titleUpper := strings.ToUpper(title)
		seasonPattern := fmt.Sprintf("S%02d", season)
		if strings.Contains(titleUpper, seasonPattern) {
			return "episode", true
		}
	}
	return "", false
}