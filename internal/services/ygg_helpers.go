package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/amaumene/gostremiofr/internal/models"
)

func (t *frenchTitleTranslator) searchTMDBForTitle(originalTitle string) (*models.Meta, error) {
	searchResults, err := t.tmdbService.SearchMulti(originalTitle, 1)
	if err != nil || len(searchResults) == 0 {
		return nil, fmt.Errorf("no results found")
	}
	return &searchResults[0], nil
}

func (t *frenchTitleTranslator) getFrenchTitle(originalTitle string, result *models.Meta) string {
	tmdbID := t.extractTMDBID(result.ID)
	if tmdbID == "" {
		return originalTitle
	}

	frenchMeta, err := t.fetchFrenchMetadata(result.Type, tmdbID)
	if err != nil {
		t.logger.Debugf("could not get French metadata for '%s', using original", originalTitle)
		return originalTitle
	}

	if frenchMeta.Name != originalTitle && frenchMeta.Name != "" {
		t.logger.Debugf("using French title '%s' instead of '%s'", frenchMeta.Name, originalTitle)
		return frenchMeta.Name
	}

	return originalTitle
}


func (s *YGG) executeSearch(searchURL string) ([]models.YggTorrent, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var torrents []models.YggTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return torrents, nil
}

func (s *YGG) processRawResponse(body []byte) ([]models.YggTorrent, error) {
	var torrents []models.YggTorrent
	if err := json.Unmarshal(body, &torrents); err != nil {
		s.logger.Errorf("failed to decode JSON, trying without cleaning: %v", err)
		if err := json.Unmarshal(body, &torrents); err != nil {
			return nil, fmt.Errorf("error decoding response: %w", err)
		}
	}
	return torrents, nil
}

func (s *YGG) extractHashFromPage(pageURL string) (string, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	// Extract hash from the response
	hashStart := strings.Index(string(body), "btih:")
	if hashStart == -1 {
		return "", fmt.Errorf("hash not found in torrent page")
	}

	hashStart += 5
	hashEnd := hashStart + 40
	if hashEnd > len(body) {
		return "", fmt.Errorf("incomplete hash in torrent page")
	}

	return string(body[hashStart:hashEnd]), nil
}