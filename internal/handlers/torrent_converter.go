package handlers

import (
	"fmt"
	
	"github.com/amaumene/gostremiofr/internal/models"
	tsmodels "github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
)

// convertTorrentSearchResults converts torrentsearch results to internal format
func (h *Handler) convertTorrentSearchResults(results *tsmodels.CombinedSearchResults) *models.CombinedTorrentResults {
	combined := &models.CombinedTorrentResults{}
	
	// Convert results from all providers
	for provider, providerResults := range results.Results {
		h.services.Logger.Debugf("[%s] converting %d results", provider,
			len(providerResults.MovieTorrents)+len(providerResults.CompleteSeriesTorrents)+
			len(providerResults.CompleteSeasonTorrents)+len(providerResults.EpisodeTorrents))
		
		// Combine all results into the main categories
		combined.MovieTorrents = append(combined.MovieTorrents, h.convertTorrentInfoList(providerResults.MovieTorrents, provider)...)
		combined.CompleteSeriesTorrents = append(combined.CompleteSeriesTorrents, h.convertTorrentInfoList(providerResults.CompleteSeriesTorrents, provider)...)
		combined.CompleteSeasonTorrents = append(combined.CompleteSeasonTorrents, h.convertTorrentInfoList(providerResults.CompleteSeasonTorrents, provider)...)
		combined.EpisodeTorrents = append(combined.EpisodeTorrents, h.convertTorrentInfoList(providerResults.EpisodeTorrents, provider)...)
	}
	
	return combined
}

func (h *Handler) convertTorrentInfoList(torrents []tsmodels.TorrentInfo, provider string) []models.TorrentInfo {
	var result []models.TorrentInfo
	for i, t := range torrents {
		// Log confidence score in debug mode
		if t.ConfidenceScore > 0 {
			var details string
			if t.ParsedInfo != nil {
				details = fmt.Sprintf(" [%s, %d, %s, %s, %s]", 
					t.ParsedInfo.Title,
					t.ParsedInfo.Year,
					t.ParsedInfo.Resolution,
					t.ParsedInfo.Source,
					t.ParsedInfo.Codec)
			}
			h.services.Logger.Debugf("[%s] torrent %d: %.0f%% confidence - %s%s", 
				provider, i+1, t.ConfidenceScore, t.Title, details)
		} else {
			h.services.Logger.Debugf("[%s] torrent %d: no confidence score - %s", provider, i+1, t.Title)
		}
		
		result = append(result, models.TorrentInfo{
			ID:              t.ID,
			Title:           t.Title,
			Hash:            t.Hash,
			Source:          provider,
			Size:            t.Size,
			ConfidenceScore: t.ConfidenceScore,
		})
	}
	return result
}