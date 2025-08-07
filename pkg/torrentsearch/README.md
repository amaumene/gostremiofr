# TorrentSearch Package

A reusable Go package for intelligent torrent searching with language-based routing, automatic title translation, and confidence-based sorting using the torrentname parser.

## Features

- **Smart Language Routing**: Automatically routes searches based on content's original language:
  - English content → All providers except YGG (French-only site)
  - Non-English content → French title for YGG, English title for other providers
- **Confidence-Based Sorting**: Uses [torrentname](https://github.com/cehbz/torrentname) parser to analyze torrent names and sort by confidence score
- **Automatic Title Translation**: Fetches both English and French titles from TMDB for optimal searching
- **No Internal Logging**: Package returns data/errors only - logging is handled at application level
- **Smart Parsing**: Extracts title, year, resolution, codec, source, and more from torrent names
- **Provider Agnostic**: Extensible architecture supporting multiple torrent providers
- **Caching**: Built-in caching support to reduce API calls
- **Episode/Season Matching**: Intelligent pattern matching for TV series content using parsed metadata

## Installation

```go
import "github.com/amaumene/gostremiofr/pkg/torrentsearch"
```

## Quick Start

```go
package main

import (
    "github.com/amaumene/gostremiofr/pkg/torrentsearch"
    "github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
    "github.com/amaumene/gostremiofr/pkg/torrentsearch/providers"
)

func main() {
    // Create a simple cache
    cache := NewSimpleCache() // Implement Cache interface
    
    // Initialize search engine
    search := torrentsearch.New(cache)
    
    // Set TMDB API key (required for smart search)
    search.SetTMDBAPIKey("your-tmdb-api-key")
    
    // Register providers
    yggProvider := providers.NewYGGProvider()
    search.RegisterProvider("ygg", yggProvider)
    // Register other providers...
    
    // Smart search with automatic language routing
    results, metadata, err := search.SearchSmart(
        "The Matrix",  // Query
        "movie",       // Media type
        0,            // Season (for series)
        0,            // Episode (for series)
        false,        // Specific episode?
    )
    
    // Log metadata at application level
    if metadata != nil {
        log.Printf("Original language: %s", metadata.OriginalLanguage)
        log.Printf("Searching with: EN='%s', FR='%s'", 
            metadata.EnglishTitle, metadata.FrenchTitle)
    }
    
    // Process results by provider
    for provider, providerResults := range results.Results {
        fmt.Printf("%s found %d torrents\n", 
            provider, len(providerResults.MovieTorrents))
    }
    
    // Results are automatically sorted by priority
    for _, torrent := range results.MovieTorrents {
        fmt.Printf("%s - %d seeders\n", torrent.Title, torrent.Seeders)
    }
}
```

## Language-Based Routing Logic

The package implements intelligent routing based on content's original language:

### English Content (original_language = "en")
- Searches all providers **except YGG** (French-only torrent site)
- Uses English title for all searches
- Example: "The Matrix" → Search with "The Matrix" on all providers except YGG

### Non-English Content (original_language ≠ "en")
- Searches YGG with **French title** (if available from TMDB)
- Searches other providers with **English title**
- If no French title exists, YGG is skipped
- Example: "Amélie" (French film) → YGG searches "Le Fabuleux Destin d'Amélie Poulain", others search "Amélie"

### Fallback (TMDB lookup fails)
- Searches all providers except YGG
- Uses original query string
- No title translation

## Core Components

### 1. French Translator

Automatically translates movie and series titles to French for better search results:

```go
translator := translator.NewFrenchTranslator(tmdbAPIKey, cache)
frenchTitle := translator.TranslateTitle("The Matrix", "movie")
// Returns: "Matrix"
```

### 2. Confidence-Based Sorter

Automatically parses and sorts torrents by confidence score:

```go
// Create sorter with debug mode
sorter := sorter.NewTorrentSorter(true, logger)

// Sort results by confidence (automatic parsing)
sorter.SortResults(results)

// Filter by minimum confidence score
highQuality := sorter.FilterByMinConfidence(torrents, 60.0) // Keep only 60%+ confidence
```

The confidence score is calculated by the torrentname parser based on how much metadata it can extract from the torrent name (title, year, resolution, codec, etc.).

### 3. Search Options

```go
options := models.SearchOptions{
    Query:           "Breaking Bad",
    MediaType:       "series",      // "movie" or "series"
    Season:          1,              // For series
    Episode:         1,              // For specific episode
    Language:        "fr",           // Triggers French translation
    SpecificEpisode: true,           // Search for specific episode vs full season
    ResolutionFilter: []string{"1080p", "720p"},
    MaxResults:      50,
}
```

## Implementing a Custom Provider

```go
type MyProvider struct {
    cache torrentsearch.Cache
}

func (p *MyProvider) Search(options models.SearchOptions) (*models.SearchResults, error) {
    // Implement your search logic
    return &models.SearchResults{
        MovieTorrents: []models.TorrentInfo{},
        // ...
    }, nil
}

func (p *MyProvider) GetTorrentHash(torrentID string) (string, error) {
    // Implement hash retrieval
    return "hash", nil
}

func (p *MyProvider) SetCache(cache torrentsearch.Cache) {
    p.cache = cache
}
```

## Cache Interface

Implement this interface for caching:

```go
type Cache interface {
    Get(key string) (interface{}, bool)
    Set(key string, value interface{})
}
```

## Pattern Matching

The package includes intelligent pattern matching for episodes and seasons:

- Episode patterns: `s01e01`, `1x01`, `season 1 episode 1`
- Season patterns: `season 1`, `saison 1`, `s01`
- Complete series indicators: `complete`, `intégrale`, `full series`

## Use Cases

### Searching French Content

```go
// Automatically routes to YGG with French translation
results, err := search.SearchWithFrench(models.SearchOptions{
    Query:     "Avatar",
    MediaType: "movie",
    Language:  "fr",
})
```

### Searching for Specific Episode

```go
options := models.SearchOptions{
    Query:           "Game of Thrones",
    MediaType:       "series",
    Season:          8,
    Episode:         3,
    SpecificEpisode: true,
    Language:        "fr",
}
results, err := search.Search("ygg", options)
```

### Batch Translation

```go
translator := translator.NewFrenchTranslator(apiKey, cache)
titles := []string{"The Matrix", "Inception", "Interstellar"}
translations := translator.TranslateTitles(titles, "movie")
// Returns map[string]string with original -> translated titles
```

## License

See the main project license.