# GoStremioFR Architecture Documentation

## Table of Contents
1. [Overview](#overview)
2. [Request Flow](#request-flow)
3. [Core Components](#core-components)
4. [Detailed Function Call Flows](#detailed-function-call-flows)
5. [Data Structures](#data-structures)
6. [Caching Strategy](#caching-strategy)
7. [Error Handling & Timeouts](#error-handling--timeouts)
8. [Configuration](#configuration)

## Overview

GoStremioFR is a Stremio addon that aggregates torrents from multiple French torrent sites (YGG) and international sites (EZTV), processes them through debrid services (AllDebrid), and provides streaming links to Stremio.

### Key Features
- Multi-source torrent aggregation (YGG for French content, EZTV for international)
- TMDB integration for metadata and French title translation
- AllDebrid integration for instant streaming
- Intelligent caching system
- Resolution and language filtering
- Concurrent search with timeout protection

## Request Flow

### 1. Stream Request (`/stream/:type/:id`)

```
User Request → Gin Router → handleStream() → Stream Response
                                ↓
                        Parse Configuration
                                ↓
                        Extract API Keys
                                ↓
                        Get TMDB Info
                                ↓
                    Route by Content Type
                         ↙            ↘
                searchMovieStreams  searchSeriesStreams
```

### 2. Catalog Request (`/catalog/:type/:id`)

```
User Request → Gin Router → handleCatalog() → Catalog Response
                                ↓
                        Parse Configuration
                                ↓
                        Query TMDB API
                                ↓
                    Return Formatted Metas
```

## Core Components

### 1. **Main Application (`cmd/gostremiofr/main.go`)**
- Initializes services
- Sets up HTTP server with Gin
- Configures middleware (CORS, gzip compression)
- Manages graceful shutdown

### 2. **Handlers (`internal/handlers/`)**
- **stream.go**: Core streaming logic
- **catalog.go**: TMDB catalog integration
- **manifest.go**: Addon manifest
- **config.go**: Configuration endpoint

### 3. **Services (`internal/services/`)**
- **ygg.go**: YGG torrent site integration (French content)
- **eztv.go**: EZTV torrent site integration (TV series)
- **apibay.go**: Apibay/The Pirate Bay API integration (International content)
- **tmdb.go**: TMDB API integration
- **alldebrid.go**: AllDebrid service integration
- **torrent_service.go**: Base torrent processing logic

### 4. **Supporting Components**
- **cache**: LRU cache implementation
- **database**: BoltDB for persistent storage
- **ratelimiter**: Token bucket rate limiting
- **logger**: Structured logging

## Detailed Function Call Flows

### A. Movie Stream Request Flow

```go
handleStream(c *gin.Context)
├── Parse configuration from base64
├── Extract API keys (AllDebrid, TMDB)
├── h.services.TMDB.GetIMDBInfo(imdbID) 
│   ├── Check cache
│   ├── Query database
│   └── Call TMDB API if needed
├── searchMovieStreams(title, year, apiKey, userConfig)
│   ├── searchTorrentsOnly(title, "movie", 0, 0, "", year)
│   │   ├── Concurrent searches:
│   │   │   ├── YGG.SearchTorrents(query, "movie", 0, 0)
│   │   │   │   ├── getFrenchTitle(query) // Translates to French
│   │   │   │   ├── API call to YGG
│   │   │   │   └── processTorrents() // Filters & sorts
│   │   │   ├── Apibay.SearchTorrents(query + year, "movie", 0, 0)
│   │   │   │   ├── Build query with year
│   │   │   │   ├── API call to Apibay
│   │   │   │   └── processTorrents() // Filters & sorts
│   │   │   └── (EZTV not called for movies)
│   │   └── Wait for results with 15s timeout
│   └── processResults(results, apiKey, userConfig, year, 0, 0)
│       ├── Process torrents by type
│       ├── Upload magnets to AllDebrid
│       ├── CheckMagnets with retry (8 attempts)
│       └── Create Stream objects
└── Return StreamResponse
```

### B. Series Episode Stream Request Flow

```go
handleStream(c *gin.Context) // for tt7678620:3:48
├── Parse episode info (season: 3, episode: 48)
├── Get TMDB info
├── searchSeriesStreams(title, season, episode, apiKey, imdbID, userConfig)
│   ├── First Search: searchTorrentsWithIMDB(title, "series", 3, 48, apiKey, "tt7678620", userConfig)
│   │   ├── Concurrent searches (15s timeout):
│   │   │   ├── YGG.SearchTorrents(query, "series", 3, 48)
│   │   │   │   ├── getFrenchTitle(query)
│   │   │   │   ├── API call to YGG with query: "name+s03"
│   │   │   │   ├── For S3E48: fetch hashes for matching episodes
│   │   │   │   │   └── Concurrent hash fetching
│   │   │   │   └── processTorrents()
│   │   │   ├── Apibay.SearchTorrents(query, "series", 3, 48)
│   │   │   │   ├── Build query: "name+s03"
│   │   │   │   ├── API call to Apibay
│   │   │   │   └── processTorrents()
│   │   │   └── EZTV.SearchTorrentsByIMDB("tt7678620", 3, 48)
│   │   │       ├── Strip "tt" prefix → "7678620"
│   │   │       ├── API call: https://eztvx.to/api/get-torrents?imdb_id=7678620
│   │   │       └── processTorrents()
│   │   └── Combine results from all sources
│   ├── processResults() → If no working streams found
│   └── Fallback Search: searchTorrentsWithIMDBSpecificEpisode(title, "series", 3, 48, apiKey, "tt7678620", userConfig)
│       ├── Concurrent searches (15s timeout):
│       │   ├── YGG.SearchTorrentsSpecificEpisode(query, "series", 3, 48)
│       │   │   ├── getFrenchTitle(query)
│       │   │   ├── API call with query: "name+s03e48"
│       │   │   └── processTorrents() + fetch hashes
│       │   ├── Apibay.SearchTorrentsSpecificEpisode(query, "series", 3, 48)
│       │   │   ├── Build query: "name+s03e48"
│       │   │   └── processTorrents()
│       │   └── EZTV.SearchTorrentsByIMDB("tt7678620", 3, 48)
│       │       └── (Same as first search - already episode-specific)
│       └── processResults() → Only episode torrents
└── Return first working stream
    ├── Extract torrents with hashes
    ├── Process YGG torrents without hashes
    ├── Upload all magnets to AllDebrid
    ├── CheckMagnets (8 attempts with backoff)
    └── Create streams from ready magnets
```

### C. Torrent Processing Pipeline

```go
ProcessTorrents(torrents, mediaType, season, episode, serviceName, year)
├── For each torrent:
│   ├── Filter 1: Language check
│   │   └── MatchesLanguageFilter(title, language)
│   ├── Filter 2: Year check (movies only)
│   │   └── MatchesYear(title, year)
│   ├── Classification:
│   │   ├── For EZTV: Use provided season/episode
│   │   ├── For YGG: Parse from title
│   │   └── Classify as: movie/episode/season/complete_series
│   ├── Filter 3: Resolution check
│   │   └── MatchesResolutionFilter(title)
│   └── Add to appropriate result category
└── Sort results by priority
    ├── Resolution priority
    ├── Language priority
    └── Size considerations
```

### D. Sequential Torrent Processing Flow (Updated)

```go
processResults()
├── Apply year filtering to movies
├── Create priority-ordered torrent list:
│   ├── For episodes (S3E48): Complete Seasons → Episodes → Complete Series
│   └── For movies: Movies → Episodes → Season Packs → Complete Series
└── processSequentialTorrents() - Process one by one:
    ├── For each torrent in priority order:
    │   ├── Get hash (fetch for YGG if needed)
    │   ├── Upload magnet to AllDebrid
    │   ├── Check status (2 attempts with 3s delay)
    │   ├── If ready:
    │   │   ├── processSingleReadyMagnet():
    │   │   │   ├── For season packs (S3E48 requested):
    │   │   │   │   ├── Parse all episode files
    │   │   │   │   ├── Find target episode file
    │   │   │   │   └── Unlock and return that episode
    │   │   │   └── For regular torrents:
    │   │   │       ├── Find largest video file
    │   │   │       └── Unlock and return it
    │   │   └── RETURN IMMEDIATELY (don't process remaining)
    │   └── If not ready: Continue to next torrent
    └── Return empty if no working torrents found
```

### E. Cache Flow

```go
Cache Operations:
├── Search Cache (24-hour TTL)
│   ├── Key: "torrent_search_YGG_query_series_3_48"
│   └── Value: TorrentResults
├── Hash Cache (24-hour TTL)
│   ├── Key: "torrent_hash_YGG_12345"
│   └── Value: "hash_string"
├── TMDB Cache (24-hour TTL)
│   ├── Key: "tmdb:tt7678620"
│   └── Value: TMDBData
└── Cleanup: Every hour remove expired entries
```

## Data Structures

### Core Models

```go
// Torrent from source
type YggTorrent struct {
    ID     int
    Title  string
    Hash   string  // Empty initially, fetched on demand
    Source string  // "YGG"
}

type EZTVTorrent struct {
    ID      int
    Title   string
    Hash    string  // Provided by EZTV API
    Season  string  // "3"
    Episode string  // "48"
    Source  string  // "EZTV"
}

// Unified torrent info
type TorrentInfo struct {
    ID     string
    Title  string
    Hash   string
    Source string
}

// Results container
type CombinedTorrentResults struct {
    MovieTorrents          []TorrentInfo
    EpisodeTorrents        []TorrentInfo
    CompleteSeasonTorrents []TorrentInfo
    CompleteSeriesTorrents []TorrentInfo
}

// Final stream
type Stream struct {
    Name         string
    Title        string
    Url          string
    BehaviorHints map[string]interface{}
}
```

## Key Implementation Details

### Sequential Torrent Processing Algorithm

```go
// Collect all torrents in priority order
var allTorrents []models.TorrentInfo
if targetSeason > 0 && targetEpisode > 0 {
    // Complete Seasons → Episodes → Complete Series
    // Season packs are prioritized for better quality and completeness
    allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
    allTorrents = append(allTorrents, results.EpisodeTorrents...)
    allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
} else {
    // Movies → Episodes → Season Packs → Complete Series
    allTorrents = append(allTorrents, results.MovieTorrents...)
    allTorrents = append(allTorrents, results.EpisodeTorrents...)
    allTorrents = append(allTorrents, results.CompleteSeasonTorrents...)
    allTorrents = append(allTorrents, results.CompleteSeriesTorrents...)
}

// Process each torrent sequentially
for _, torrent := range allTorrents {
    // 1. Get hash (fetch for YGG if needed)
    hash := getOrFetchHash(torrent)
    
    // 2. Upload magnet to AllDebrid
    err := UploadMagnet(hash, torrent.Title)
    
    // 3. Check status (max 2 attempts)
    for attempt := 1; attempt <= 2; attempt++ {
        status := CheckMagnet(hash)
        if status.Ready {
            // 4. Process ready magnet
            stream := processSingleReadyMagnet(status, torrent)
            if stream != nil {
                return []Stream{stream} // IMMEDIATE RETURN
            }
        }
        sleep(3s)
    }
    // Not ready, continue to next torrent
}
```

### Season Pack Episode Extraction

```go
// Direct parsing from AllDebrid links
for _, link := range seasonTorrent.Links {
    filename := link["filename"]
    if isVideoFile(filename) {
        season, episode := parseEpisodeFromFilename(filename)
        if season == targetSeason && episode == targetEpisode {
            return unlockLink(link["link"])
        }
    }
}
```

## Caching Strategy

### 1. **Memory Cache (LRU)**
- Capacity: 1000 items
- TTL: 24 hours
- Used for: TMDB data, search results, torrent hashes

### 2. **Database Cache (BoltDB)**
- Persistent storage
- Used for: TMDB metadata, magnet information
- Backup for memory cache

### 3. **Cache Keys**
- Search: `torrent_search_{provider}_{query}_{type}_{season}_{episode}`
- Hash: `torrent_hash_{provider}_{torrentID}`
- TMDB: `tmdb:{imdbID}` or `tmdb:search:{query}:{page}`

## Error Handling & Timeouts

### 1. **Request Timeout (30s)**
- Overall request timeout
- Prevents hanging requests
- Logs timeout events

### 2. **Search Timeout (15s)**
- Individual torrent source timeout
- Allows partial results
- Non-blocking concurrent searches

### 3. **Rate Limiter Timeout (5s)**
- Prevents infinite wait
- Token bucket implementation
- Per-service rate limits

### 4. **Retry Logic**
- AllDebrid CheckMagnets: 2 attempts per magnet
- Sequential processing: Try next magnet if current fails
- Season torrent handling: Direct link extraction from ready torrents
- Immediate return on first successful stream

### 5. **Panic Recovery**
- Goroutine panic recovery
- Prevents cascade failures
- Logs panic details

## Configuration

### 1. **User Configuration (Base64 encoded)**
```json
{
  "TMDB_API_KEY": "...",
  "API_KEY_ALLDEBRID": "...",
  "FILES_TO_SHOW": 6,
  "RES_TO_SHOW": ["2160p", "1080p", "720p", "480p"],
  "LANG_TO_SHOW": ["MULTI", "VO", "VFF", "VF", "FRENCH"]
}
```

### 2. **Service Configuration**
- YGG: 10 requests/second, burst 2
- EZTV: 5 requests/second, burst 1
- Apibay: 5 requests/second, burst 2
- TMDB: 20 requests/second, burst 5
- AllDebrid: 10 requests/second, burst 2

### 3. **Environment Variables**
- `PORT`: Server port (default: 5000)
- `LOG_LEVEL`: debug/info/warn/error
- `GIN_MODE`: release/debug
- `DATABASE_DIR`: Database location
- `USE_SSL`: Enable HTTPS

## Apibay Integration Details

### API Format
- **Base URL**: `https://apibay.org/q.php`
- **Parameters**: 
  - `q`: Search query (spaces replaced with `+`)
  - `cat`: Category (`video` for general video search)

### Query Building (Generic Format)
- **Movies**: `{title}+{year}` (e.g., `mission+impossible+1996`)
- **Series (Season Mode)**: `{title}+s{XX}` (e.g., `bluey+s03`) - matches both episodes and complete seasons
- **Series (Episode Mode)**: `{title}+s{XX}e{XX}` (e.g., `bluey+s03e48`) - fallback for specific episode search

### Provider-Specific Behavior
- **YGG**: Translates titles to French before applying generic format
- **Apibay**: Uses generic format directly
- **EZTV**: Uses IMDB ID-based search

### Response Processing
- Returns JSON array of torrents
- Hash is uppercase (converted to lowercase internally)
- Includes seeders/leechers for availability sorting
- Generic torrent processing applies language/resolution filters

## Recent Improvements (v3.0)

### 1. **Sequential Torrent Processing Revolution**
- **Complete Algorithm Redesign**: Changed from bulk magnet processing to sequential torrent processing
- **Priority-Based Processing**: Processes torrents in quality order (resolution → language → size)
- **Immediate Response**: Returns the first working stream without processing remaining torrents
- **No File Limits**: Ignores user's `FilesToShow` setting, processes until success
- **Smart Prioritization**: Complete Seasons → Episodes → Complete Series for episode requests

### 2. **Generic Query Format Implementation**
- **Unified Format**: All providers use consistent query format
- **Movies**: `title+year` (no prefixes)
- **Series**: `title+sXXeXX` or `title+sXX+complete`
- **Provider Adaptation**: YGG translates to French, others use format directly

### 3. **Enhanced Timeout System**
- Request-level: 30 seconds (prevents hanging)
- Search-level: 15 seconds (allows partial results)
- Rate limiter: 5 seconds (prevents deadlock)
- Context propagation throughout the stack

### 4. **Intelligent Season Pack Handling**
- Prioritizes complete season torrents over individual episodes
- Extracts specific episode from complete seasons when needed
- Direct link parsing (no GetEpisodeFiles API call)
- Returns only the requested episode or entire season

### 5. **User-Defined Quality Prioritization**
- **Resolution Priority**: Respects user's `RES_TO_SHOW` order
- **Language Priority**: Respects user's `LANG_TO_SHOW` order (YGG only)
- **Size Tiebreaking**: Larger files preferred within same quality tier
- **No Hardcoded Preferences**: All sorting based on user configuration

### 6. **Source Provider Tracking**
- Stream names show the torrent provider (YGG, EZTV, Apibay)
- Source information preserved through entire processing pipeline
- Users can see which provider supplied each stream

### 7. **Simplified Processing Logic**
- Removed complex batching and file counting
- Single path for all content types (movies, episodes, seasons)
- Consistent error handling across all providers
- Reduced memory usage and API calls

### 8. **Episode-Specific Fallback Search**
- **Two-Phase Search Strategy**: First search for season packs, then specific episodes
- **Smart Fallback**: If no working streams from season search, try episode-specific query
- **Generic Implementation**: Works across all providers (YGG, Apibay, EZTV)
- **Optimized Queries**: `name+s03` → `name+s03e48` for better results
- **No Duplicate Processing**: Fallback only triggers when first search yields no working streams

## Performance Optimizations

### 1. **Concurrent Search Phase**
- Parallel torrent source searches (YGG, EZTV, Apibay)
- 15-second timeout allows partial results
- Non-blocking goroutines with panic recovery

### 2. **Sequential Processing Phase**
- One-by-one torrent processing in quality order
- Hash fetching on-demand for YGG
- Immediate return on first working stream
- No unnecessary API calls after success

### 3. **Efficient Filtering**
- Early filtering by language and resolution
- User-defined quality prioritization
- Year filtering for movies
- Progressive filtering pipeline

### 4. **Smart Caching**
- Cache-first approach for all operations
- 24-hour cache for TMDB data, torrent searches, and hashes
- Automatic cleanup routine prevents memory bloat

### 5. **Resource Optimization**
- No artificial limits on torrent processing
- Only processes torrents until success
- Request timeout protection (30s total)
- Rate limiting prevents API abuse

### 6. **Season Pack Optimization**
- Only best season torrent processed for episodes
- Direct episode extraction from ready torrents
- No redundant GetEpisodeFiles API calls
- Target episode matching with filename parsing

## Common Issues & Solutions

### 1. **EZTV Results Not Appearing**
- Check language filter includes "VO"
- Verify resolution filter includes 720p/1080p
- Ensure IMDB ID is provided for series

### 2. **Hanging Requests**
- 30-second request timeout prevents indefinite hangs
- 15-second search timeout for each source
- Rate limiter timeout (5s) prevents deadlock

### 3. **No Streams Found**
- Torrents are processed sequentially in quality order
- Each torrent gets 2 AllDebrid status check attempts
- System tries all available torrents until one works
- Check AllDebrid account status and API key validity

### 4. **High Episode Numbers**
- Episode 48+ supported
- Pattern matching handles various formats
- Both YGG and EZTV searched for series

### 5. **Season Pack Handling**
- Only the best season torrent is processed
- Specific episode extracted from pack
- Direct link parsing for ready torrents
- No "MAGNET_INVALID_ID" errors