package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/amaumene/gostremiofr/pkg/httputil"
)

func compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		compiled[i] = regexp.MustCompile(pattern)
	}
	return compiled
}

func createVideoExtensionSet(extensions string) map[string]bool {
	extSet := make(map[string]bool)
	for _, ext := range strings.Split(extensions, ",") {
		extSet[ext] = true
	}
	return extSet
}

func getEpisodePatterns() []string {
	return []string{
		`(?i)s(\d{1,2})e(\d{1,2})`,                     // S01E01
		`(?i)s(\d{1,2})\.e(\d{1,2})`,                   // S01.E01
		`(?i)(\d{1,2})x(\d{1,2})`,                      // 1x01
		`(?i)season\s*(\d{1,2})\s*episode\s*(\d{1,2})`, // Season 1 Episode 1
	}
}

func getResolutionPatterns() []string {
	return []string{
		`(?i)(2160p|4k)`,
		`(?i)(1080p)`,
		`(?i)(720p)`,
		`(?i)(480p)`,
		`(?i)(360p)`,
	}
}


func (a *AllDebrid) buildMagnetFormData(apiKey string, hashes []string) url.Values {
	formData := url.Values{}
	formData.Set("agent", allDebridAgent)
	formData.Set("apikey", apiKey)
	for _, hash := range hashes {
		formData.Add("magnets[]", hash)
	}
	return formData
}

func (a *AllDebrid) makeAPIRequest(requestURL string, formData url.Values) (*http.Response, error) {
	httpClient := httputil.NewHTTPClient(allDebridAPITimeout)
	
	a.logger.Infof("sending POST request...")
	resp, err := httpClient.PostForm(requestURL, formData)
	if err != nil {
		a.logger.Errorf("POST request failed: %v", err)
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	
	a.logger.Infof("received HTTP response with status: %s", resp.Status)
	return resp, nil
}

func (a *AllDebrid) parseAPIResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

