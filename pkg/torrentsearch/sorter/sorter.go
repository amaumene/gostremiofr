package sorter

import (
	"fmt"
	"sort"

	"github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
	"github.com/cehbz/torrentname"
)

type TorrentSorter struct{}

func NewTorrentSorter() *TorrentSorter {
	return &TorrentSorter{}
}

func (ts *TorrentSorter) ParseAndScore(torrents []models.TorrentInfo) []models.TorrentInfo {
	for i := range torrents {
		parsed := torrentname.Parse(torrents[i].Title)
		if parsed == nil {
			torrents[i].ConfidenceScore = 0
			torrents[i].ParsedInfo = nil
			continue
		}
		
		torrents[i].ParsedInfo = parsed
		// Use the built-in confidence score from torrentname
		torrents[i].ConfidenceScore = float64(parsed.Confidence)
		
		if parsed.Season > 0 {
			torrents[i].Season = parsed.Season
		}
		if parsed.Episode > 0 {
			torrents[i].Episode = parsed.Episode
		}
	}
	
	return torrents
}

func (ts *TorrentSorter) SortByConfidence(torrents []models.TorrentInfo) []models.TorrentInfo {
	torrents = ts.ParseAndScore(torrents)
	
	sort.Slice(torrents, func(i, j int) bool {
		return torrents[i].ConfidenceScore > torrents[j].ConfidenceScore
	})
	
	return torrents
}

func (ts *TorrentSorter) SortResults(results *models.SearchResults) {
	results.MovieTorrents = ts.SortByConfidence(results.MovieTorrents)
	results.CompleteSeriesTorrents = ts.SortByConfidence(results.CompleteSeriesTorrents)
	results.CompleteSeasonTorrents = ts.SortByConfidence(results.CompleteSeasonTorrents)
	results.EpisodeTorrents = ts.SortByConfidence(results.EpisodeTorrents)
}

func (ts *TorrentSorter) FilterByMinConfidence(torrents []models.TorrentInfo, minConfidence float64) []models.TorrentInfo {
	var filtered []models.TorrentInfo
	for _, torrent := range torrents {
		if torrent.ConfidenceScore >= minConfidence {
			filtered = append(filtered, torrent)
		}
	}
	return filtered
}

func (ts *TorrentSorter) MatchesEpisode(torrent models.TorrentInfo, season, episode int) bool {
	if torrent.ParsedInfo == nil {
		return false
	}
	
	return torrent.ParsedInfo.Season == season && torrent.ParsedInfo.Episode == episode
}

func (ts *TorrentSorter) MatchesSeason(torrent models.TorrentInfo, season int) bool {
	if torrent.ParsedInfo == nil {
		return false
	}
	
	// Season torrent has season but no specific episode
	return torrent.ParsedInfo.Season == season && torrent.ParsedInfo.Episode == 0
}

func (ts *TorrentSorter) IsCompleteSeries(torrent models.TorrentInfo) bool {
	if torrent.ParsedInfo == nil {
		return false
	}
	
	// Complete series typically has IsComplete flag or no season/episode specified
	return torrent.ParsedInfo.IsComplete || (torrent.ParsedInfo.Season == 0 && torrent.ParsedInfo.Episode == 0)
}

// GetSortedWithDebugInfo returns sorted torrents with debug information
func (ts *TorrentSorter) GetSortedWithDebugInfo(torrents []models.TorrentInfo) ([]models.TorrentInfo, []string) {
	sorted := ts.SortByConfidence(torrents)
	var debugInfo []string
	
	for i, t := range sorted {
		var details string
		if t.ParsedInfo != nil {
			details = fmt.Sprintf(" [%s, %d, %s, %s, %s]", 
				t.ParsedInfo.Title,
				t.ParsedInfo.Year,
				t.ParsedInfo.Resolution,
				t.ParsedInfo.Source,
				t.ParsedInfo.Codec)
		}
		debugInfo = append(debugInfo, fmt.Sprintf("%d. %.0f%% - %s%s", 
			i+1, 
			t.ConfidenceScore, 
			t.Title,
			details))
	}
	
	return sorted, debugInfo
}