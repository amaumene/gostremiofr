// Package torrentsearch provides torrent search functionality across multiple providers.
// It supports intelligent routing based on content language and metadata fetching from TMDB.
package torrentsearch

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/providers"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/sorter"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/translator"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/utils"
)

// TorrentProvider defines the interface for torrent search providers.
type TorrentProvider interface {
	Search(options models.SearchOptions) (*models.SearchResults, error)
	GetTorrentHash(torrentID string) (string, error)
	SetCache(cache interface{})
}

// Cache defines the interface for caching search results.
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
}

// TorrentSearch orchestrates search across multiple torrent providers.
type TorrentSearch struct {
	providers       map[string]TorrentProvider
	metadataFetcher *translator.MetadataFetcher
	sorter          *sorter.TorrentSorter
	cache           Cache
	tmdbAPIKey      string
	providerErrors  map[string]error
	providerURLs    map[string]string
}

// SearchMetadata contains metadata about the searched content.
type SearchMetadata struct {
	OriginalLanguage string
	EnglishTitle     string
	FrenchTitle      string
	Year             int
}

// New creates a new TorrentSearch instance with the given cache.
func New(cache Cache) *TorrentSearch {
	return &TorrentSearch{
		providers:      make(map[string]TorrentProvider),
		sorter:         sorter.NewTorrentSorter(),
		cache:          cache,
		providerErrors: make(map[string]error),
		providerURLs:   make(map[string]string),
	}
}

// RegisterProvider adds a new torrent provider to the search engine.
func (ts *TorrentSearch) RegisterProvider(name string, provider TorrentProvider) {
	if ts.cache != nil {
		provider.SetCache(ts.cache)
	}
	ts.providers[name] = provider
}

// SetTMDBAPIKey configures the TMDB API key for metadata fetching.
func (ts *TorrentSearch) SetTMDBAPIKey(apiKey string) {
	ts.tmdbAPIKey = apiKey
	ts.metadataFetcher = translator.NewMetadataFetcher(apiKey, ts.cache)
}

// SearchSmart performs intelligent routing based on content's original language.
func (ts *TorrentSearch) SearchSmart(query string, mediaType string, season, episode int, specificEpisode bool) (*models.CombinedSearchResults, *SearchMetadata, error) {
	if ts.metadataFetcher == nil {
		return nil, nil, fmt.Errorf("TMDB API key not configured")
	}

	if ts.isIMDBID(query) {
		return ts.searchSmartByIMDBID(query, mediaType, season, episode, specificEpisode)
	}

	metadata, err := ts.metadataFetcher.FetchMetadata(query, mediaType)
	if err != nil {
		results, fallbackErr := ts.searchWithoutMetadata(query, mediaType, season, episode, specificEpisode)
		return results, nil, fallbackErr
	}

	searchMeta := ts.buildSearchMetadata(metadata)
	combined := ts.searchWithMetadata(metadata, mediaType, season, episode, specificEpisode)

	return combined, searchMeta, nil
}

// searchSmartByIMDBID performs intelligent routing using direct IMDB ID lookup.
func (ts *TorrentSearch) searchSmartByIMDBID(imdbID string, mediaType string, season, episode int, specificEpisode bool) (*models.CombinedSearchResults, *SearchMetadata, error) {
	if ts.metadataFetcher == nil {
		return nil, nil, fmt.Errorf("TMDB API key not configured")
	}

	metadata, err := ts.metadataFetcher.FetchMetadataByIMDBID(imdbID, mediaType)
	if err != nil {
		results, fallbackErr := ts.searchWithoutMetadata(imdbID, mediaType, season, episode, specificEpisode)
		return results, nil, fallbackErr
	}

	searchMeta := ts.buildSearchMetadata(metadata)
	combined := ts.searchWithMetadata(metadata, mediaType, season, episode, specificEpisode)

	return combined, searchMeta, nil
}

// isIMDBID checks if the query is an IMDB ID.
func (ts *TorrentSearch) isIMDBID(query string) bool {
	return strings.HasPrefix(query, "tt") && regexp.MustCompile(`^tt\d+$`).MatchString(query)
}

// buildSearchMetadata creates SearchMetadata from translator metadata.
func (ts *TorrentSearch) buildSearchMetadata(metadata *translator.ContentMetadata) *SearchMetadata {
	return &SearchMetadata{
		OriginalLanguage: metadata.OriginalLanguage,
		EnglishTitle:     metadata.EnglishTitle,
		FrenchTitle:      metadata.FrenchTitle,
		Year:             metadata.Year,
	}
}

// searchWithMetadata performs search using metadata for intelligent routing.
func (ts *TorrentSearch) searchWithMetadata(metadata *translator.ContentMetadata, mediaType string, season, episode int, specificEpisode bool) *models.CombinedSearchResults {
	// Clear provider errors and URLs for new search
	ts.providerErrors = nil
	ts.providerURLs = make(map[string]string)
	
	combined := &models.CombinedSearchResults{
		Results:   make(map[string]*models.SearchResults),
		DebugInfo: make(map[string]string),
	}

	searchOptions := ts.buildSearchOptions(metadata, mediaType, season, episode, specificEpisode)

	if metadata.OriginalLanguage == "en" {
		ts.searchEnglishProviders(searchOptions, combined, metadata.EnglishTitle)
	} else {
		ts.searchNonEnglishProviders(searchOptions, combined, metadata)
	}

	return combined
}

// buildSearchOptions creates SearchOptions from metadata and parameters.
func (ts *TorrentSearch) buildSearchOptions(metadata *translator.ContentMetadata, mediaType string, season, episode int, specificEpisode bool) models.SearchOptions {
	return models.SearchOptions{
		MediaType:       mediaType,
		Season:          season,
		Episode:         episode,
		SpecificEpisode: specificEpisode,
		Year:            metadata.Year,
	}
}

// searchEnglishProviders searches all providers except YGG for English content.
func (ts *TorrentSearch) searchEnglishProviders(options models.SearchOptions, combined *models.CombinedSearchResults, title string) {
	options.Query = title
	
	// Search providers in parallel (excluding YGG)
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for name, provider := range ts.providers {
		if name == providers.ProviderYGG {
			continue
		}
		
		wg.Add(1)
		go func(n string, p TorrentProvider) {
			defer wg.Done()
			ts.searchProviderConcurrent(n, p, options, combined, &mu)
		}(name, provider)
	}
	
	wg.Wait()
}

// searchNonEnglishProviders searches YGG with French title and others with English title.
func (ts *TorrentSearch) searchNonEnglishProviders(options models.SearchOptions, combined *models.CombinedSearchResults, metadata *translator.ContentMetadata) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Search YGG with French title in parallel
	if metadata.FrenchTitle != "" {
		if yggProvider, exists := ts.providers[providers.ProviderYGG]; exists {
			wg.Add(1)
			go func() {
				defer wg.Done()
				frenchOptions := options
				frenchOptions.Query = metadata.FrenchTitle
				frenchOptions.Language = "fr"
				ts.searchProviderConcurrent(providers.ProviderYGG, yggProvider, frenchOptions, combined, &mu)
			}()
		}
	}
	
	// Search other providers with English title in parallel
	englishOptions := options
	englishOptions.Query = metadata.EnglishTitle
	englishOptions.Language = ""
	
	for name, provider := range ts.providers {
		if name == providers.ProviderYGG {
			continue
		}
		
		wg.Add(1)
		go func(n string, p TorrentProvider) {
			defer wg.Done()
			ts.searchProviderConcurrent(n, p, englishOptions, combined, &mu)
		}(name, provider)
	}
	
	wg.Wait()
}

// searchProvider executes search for a single provider and adds results (sequential version).
func (ts *TorrentSearch) searchProvider(name string, provider TorrentProvider, options models.SearchOptions, combined *models.CombinedSearchResults) {
	// Build and store the API URL for debugging
	apiURL := ts.buildProviderURL(name, options)
	if ts.providerURLs == nil {
		ts.providerURLs = make(map[string]string)
	}
	ts.providerURLs[name] = apiURL
	combined.DebugInfo[name] = apiURL
	
	results, err := provider.Search(options)
	if err != nil {
		// Store the error for debugging
		if ts.providerErrors == nil {
			ts.providerErrors = make(map[string]error)
		}
		ts.providerErrors[name] = err
		
		// Return empty results so the provider appears in the output
		combined.Results[name] = &models.SearchResults{
			MovieTorrents:          []models.TorrentInfo{},
			CompleteSeriesTorrents: []models.TorrentInfo{},
			CompleteSeasonTorrents: []models.TorrentInfo{},
			EpisodeTorrents:        []models.TorrentInfo{},
		}
		return
	}

	ts.sorter.SortResults(results)
	combined.Results[name] = results
}

func (ts *TorrentSearch) searchWithoutMetadata(query string, mediaType string, season, episode int, specificEpisode bool) (*models.CombinedSearchResults, error) {
	combined := &models.CombinedSearchResults{
		Results:   make(map[string]*models.SearchResults),
		DebugInfo: make(map[string]string),
	}

	searchOptions := models.SearchOptions{
		Query:           query,
		MediaType:       mediaType,
		Season:          season,
		Episode:         episode,
		SpecificEpisode: specificEpisode,
	}

	// Search all providers in parallel
	ts.searchAllProvidersParallel(searchOptions, combined)

	if len(combined.Results) == 0 {
		return nil, fmt.Errorf("no results found from any provider")
	}

	return combined, nil
}

// Search searches a specific provider by name.
func (ts *TorrentSearch) Search(providerName string, options models.SearchOptions) (*models.SearchResults, error) {
	provider, exists := ts.providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}

	results, err := provider.Search(options)
	if err != nil {
		return nil, err
	}

	ts.sorter.SortResults(results)
	return results, nil
}

// FilterByMinConfidence filters search results by minimum confidence score.
func (ts *TorrentSearch) FilterByMinConfidence(results *models.SearchResults, minConfidence float64) *models.SearchResults {
	return &models.SearchResults{
		MovieTorrents:          ts.sorter.FilterByMinConfidence(results.MovieTorrents, minConfidence),
		CompleteSeriesTorrents: ts.sorter.FilterByMinConfidence(results.CompleteSeriesTorrents, minConfidence),
		CompleteSeasonTorrents: ts.sorter.FilterByMinConfidence(results.CompleteSeasonTorrents, minConfidence),
		EpisodeTorrents:        ts.sorter.FilterByMinConfidence(results.EpisodeTorrents, minConfidence),
	}
}

// GetDebugInfo returns debug information for torrents.
func (ts *TorrentSearch) GetDebugInfo(torrents []models.TorrentInfo) []string {
	_, debugInfo := ts.sorter.GetSortedWithDebugInfo(torrents)
	return debugInfo
}








// GetProviderHash fetches the torrent hash from a specific provider.
func (ts *TorrentSearch) GetProviderHash(providerName string, torrentID string) (string, error) {
	provider, exists := ts.providers[providerName]
	if !exists {
		return "", fmt.Errorf("provider %s not found", providerName)
	}
	
	return provider.GetTorrentHash(torrentID)
}

// GetProviderErrors returns any errors that occurred during the last search.
func (ts *TorrentSearch) GetProviderErrors() map[string]error {
	return ts.providerErrors
}

// GetProviderURLs returns the API URLs used in the last search.
func (ts *TorrentSearch) GetProviderURLs() map[string]string {
	return ts.providerURLs
}

// searchProviderConcurrent is a thread-safe version of searchProvider for parallel execution.
func (ts *TorrentSearch) searchProviderConcurrent(name string, provider TorrentProvider, options models.SearchOptions, combined *models.CombinedSearchResults, mu *sync.Mutex) {
	// Build and store the API URL for debugging
	apiURL := ts.buildProviderURL(name, options)
	
	mu.Lock()
	if ts.providerURLs == nil {
		ts.providerURLs = make(map[string]string)
	}
	ts.providerURLs[name] = apiURL
	combined.DebugInfo[name] = apiURL
	mu.Unlock()
	
	results, err := provider.Search(options)
	
	mu.Lock()
	defer mu.Unlock()
	
	if err != nil {
		// Store the error for debugging
		if ts.providerErrors == nil {
			ts.providerErrors = make(map[string]error)
		}
		ts.providerErrors[name] = err
		
		// Return empty results so the provider appears in the output
		combined.Results[name] = &models.SearchResults{
			MovieTorrents:          []models.TorrentInfo{},
			CompleteSeriesTorrents: []models.TorrentInfo{},
			CompleteSeasonTorrents: []models.TorrentInfo{},
			EpisodeTorrents:        []models.TorrentInfo{},
		}
		return
	}
	
	ts.sorter.SortResults(results)
	combined.Results[name] = results
}

// searchAllProvidersParallel searches all providers in parallel.
func (ts *TorrentSearch) searchAllProvidersParallel(options models.SearchOptions, combined *models.CombinedSearchResults) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for name, provider := range ts.providers {
		wg.Add(1)
		go func(n string, p TorrentProvider) {
			defer wg.Done()
			ts.searchProviderConcurrent(n, p, options, combined, &mu)
		}(name, provider)
	}
	
	wg.Wait()
}

// buildProviderURL builds the API URL for a provider (for debugging).
func (ts *TorrentSearch) buildProviderURL(name string, options models.SearchOptions) string {
	query := utils.BuildSearchQuery(options.Query, options.MediaType, options.Season, options.Episode, options.SpecificEpisode)
	
	// Query already has + for spaces from formatQueryString, no need to escape it
	switch name {
	case providers.ProviderYGG:
		categories := ""
		if options.MediaType == "movie" {
			categories = "&category_id=2178&category_id=2181&category_id=2183"
		} else if options.MediaType == "series" {
			categories = "&category_id=2179&category_id=2181&category_id=2182&category_id=2184"
		}
		return fmt.Sprintf("https://yggapi.eu/torrents?q=%s&page=1&per_page=100%s", query, categories)
		
	case providers.ProviderApiBay:
		return fmt.Sprintf("https://apibay.org/q.php?q=%s&cat=video", query)
		
	case providers.ProviderTorrentsCSV:
		return fmt.Sprintf("https://torrents-csv.com/service/search?q=%s&size=100", query)
		
	default:
		return fmt.Sprintf("unknown provider: %s", name)
	}
}