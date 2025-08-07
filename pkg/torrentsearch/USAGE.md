# Using TorrentSearch Package in Other Projects

> **Note**: This package now uses confidence-based sorting powered by the [torrentname](https://github.com/cehbz/torrentname) parser. Torrents are automatically sorted by how well their names can be parsed, with higher confidence scores indicating better quality naming and metadata.

## Installation

In your Go project, import the package:

```bash
go get github.com/amaumene/gostremiofr/pkg/torrentsearch
```

## Basic Usage Example

Create a new file `main.go` in your project:

```go
package main

import (
    "fmt"
    "log"
    "os"
    
    "github.com/amaumene/gostremiofr/pkg/torrentsearch"
    "github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
    "github.com/amaumene/gostremiofr/pkg/torrentsearch/providers"
)

// Simple in-memory cache implementation
type MemoryCache struct {
    data map[string]interface{}
}

func NewMemoryCache() *MemoryCache {
    return &MemoryCache{
        data: make(map[string]interface{}),
    }
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
    val, exists := c.data[key]
    return val, exists
}

func (c *MemoryCache) Set(key string, value interface{}) {
    c.data[key] = value
}

func main() {
    // Create cache
    cache := NewMemoryCache()
    
    // Initialize search engine
    searchEngine := torrentsearch.New(cache)
    
    // Enable French translation (optional, requires TMDB API key)
    tmdbKey := os.Getenv("TMDB_API_KEY")
    if tmdbKey != "" {
        searchEngine.SetFrenchTranslator(tmdbKey)
    }
    
    // Configure resolution priority (optional)
    searchEngine.SetResolutionOrder([]string{"1080p", "720p", "4k", "480p"})
    
    // Register YGG provider
    ygg := providers.NewYGGProvider()
    searchEngine.RegisterProvider("ygg", ygg)
    
    // Example 1: Search for a French movie
    movieSearch := models.SearchOptions{
        Query:     "Inception",
        MediaType: "movie",
        Language:  "fr", // Will translate to French automatically
        ResolutionFilter: []string{"1080p", "720p"},
    }
    
    movieResults, err := searchEngine.Search("ygg", movieSearch)
    if err != nil {
        log.Printf("Error searching movies: %v", err)
    } else {
        fmt.Printf("Found %d movie torrents\n", len(movieResults.MovieTorrents))
        for i, torrent := range movieResults.MovieTorrents[:min(5, len(movieResults.MovieTorrents))] {
            fmt.Printf("%d. %s (%.2f GB, %d seeders)\n", 
                i+1, torrent.Title, 
                float64(torrent.Size)/(1024*1024*1024),
                torrent.Seeders)
        }
    }
    
    // Example 2: Search for a specific TV series episode
    seriesSearch := models.SearchOptions{
        Query:           "The Witcher",
        MediaType:       "series",
        Season:          1,
        Episode:         1,
        SpecificEpisode: true,
        Language:        "fr",
    }
    
    seriesResults, err := searchEngine.Search("ygg", seriesSearch)
    if err != nil {
        log.Printf("Error searching series: %v", err)
    } else {
        fmt.Printf("\nFound %d episode torrents for S01E01\n", len(seriesResults.EpisodeTorrents))
        for i, torrent := range seriesResults.EpisodeTorrents[:min(3, len(seriesResults.EpisodeTorrents))] {
            fmt.Printf("%d. %s (%d seeders)\n", i+1, torrent.Title, torrent.Seeders)
        }
    }
    
    // Example 3: Use auto-detection for French content
    autoSearch := models.SearchOptions{
        Query:     "Lupin", // French series
        MediaType: "series",
        Season:    1,
    }
    
    // SearchWithFrench automatically detects if content should use French search
    autoResults, err := searchEngine.SearchWithFrench(autoSearch)
    if err != nil {
        log.Printf("Error in auto search: %v", err)
    } else {
        fmt.Printf("\nFound %d season torrents for Season 1\n", len(autoResults.CompleteSeasonTorrents))
    }
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

## Advanced Features

### Custom Filtering

```go
// Get raw results
results, _ := searchEngine.Search("ygg", searchOptions)

// Apply custom filters using the sorter
sorter := sorter.NewTorrentSorter(nil)

// Filter by minimum seeders
activeOnly := sorter.FilterByMinSeeders(results.MovieTorrents, 10)

// Filter by file size (min 1GB, max 10GB)
sizeFiltered := sorter.FilterBySize(activeOnly, 1024*1024*1024, 10*1024*1024*1024)
```

### Batch Translation

```go
// Translate multiple titles at once
translator := translator.NewFrenchTranslator(tmdbAPIKey, cache)

titles := []string{"The Matrix", "Fight Club", "Pulp Fiction"}
translations := translator.TranslateTitles(titles, "movie")

for original, french := range translations {
    fmt.Printf("%s -> %s\n", original, french)
}
```

### Custom Provider Implementation

```go
type CustomProvider struct {
    cache interface{}
}

func (p *CustomProvider) Search(options models.SearchOptions) (*models.SearchResults, error) {
    // Implement your custom search logic
    results := &models.SearchResults{
        MovieTorrents: []models.TorrentInfo{
            {
                ID:      "custom-1",
                Title:   "Custom Result",
                Hash:    "abc123",
                Source:  "CustomProvider",
                Size:    1024 * 1024 * 1024, // 1GB
                Seeders: 100,
            },
        },
    }
    return results, nil
}

func (p *CustomProvider) GetTorrentHash(torrentID string) (string, error) {
    // Return the hash for a specific torrent
    return "hash-" + torrentID, nil
}

func (p *CustomProvider) SetCache(cache interface{}) {
    p.cache = cache
}

// Register the custom provider
customProvider := &CustomProvider{}
searchEngine.RegisterProvider("custom", customProvider)
```

## Environment Variables

- `TMDB_API_KEY`: Required for French title translation feature

## Features Summary

1. **Automatic French Translation**: Translates English titles to French for better search results on French torrent sites
2. **Smart Sorting**: Results are automatically sorted by resolution priority, source quality, and seeders
3. **Flexible Filtering**: Filter by resolution, file size, seeders, etc.
4. **Caching**: Reduces API calls by caching search results and translations
5. **Episode/Season Detection**: Intelligent pattern matching for TV content
6. **Extensible**: Easy to add new torrent providers

## Common Use Cases

### Movies in French
```go
options := models.SearchOptions{
    Query:     "Avatar",
    MediaType: "movie",
    Language:  "fr",
}
```

### Complete TV Season
```go
options := models.SearchOptions{
    Query:     "Breaking Bad",
    MediaType: "series",
    Season:    5,
    Language:  "fr",
}
```

### Specific Episode
```go
options := models.SearchOptions{
    Query:           "Game of Thrones",
    MediaType:       "series",
    Season:          8,
    Episode:         3,
    SpecificEpisode: true,
}
```

### High Quality Only
```go
options := models.SearchOptions{
    Query:            "Dune",
    MediaType:        "movie",
    ResolutionFilter: []string{"2160p", "4k", "1080p"},
}
```