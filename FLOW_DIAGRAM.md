# GoStremioFR Flow Diagrams

## Stream Request Flow - Visual Representation

### Series Episode Request Example: `/stream/series/tt7678620:3:48.json`

```
┌─────────────────┐
│ Stremio Client  │
│ Requests S3E48  │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    handleStream()                            │
│  1. Parse ID: tt7678620:3:48                               │
│  2. Extract: imdbID=tt7678620, season=3, episode=48        │
│  3. Decode base64 config & extract API keys                │
│  4. 30-second timeout context created                      │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                  TMDB.GetIMDBInfo()                          │
│  1. Check memory cache (24h TTL)                           │
│  2. Check database cache                                   │
│  3. If miss: API call to TMDB                             │
│  4. Returns: type="series", title="Bluey"                 │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│              searchSeriesStreams()                           │
│  Routes to searchTorrentsWithIMDB()                        │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                performParallelSearch()                       │
│  Concurrent searches with 15s timeout                       │
├─────────────────────┬──────────────────────────────────────┤
│                     │                                       │
▼                     ▼                                       │
┌──────────────┐     ┌──────────────┐                       │
│     YGG      │     │    Apibay    │                       │
│              │     │              │                       │
│ 1. Get French│     │ 1. Generic   │                       │
│    title     │     │    query     │                       │
│ 2. API call  │     │ 2. API call  │                       │
│ 3. For S3E48:│     │ 3. Returns   │                       │
│    fetch hash│     │    torrents  │                       │
│ 4. Filter &  │     │    with hash │                       │
│    sort      │     │ 4. Filter &  │                       │
└──────┬───────┘     │    sort      │                       │
       │             └──────┬───────┘                       │
       │                    │                                │
       └────────────────────┴────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│              Combined Results                                │
│  - EpisodeTorrents: [S3E48 matches]                        │
│  - CompleteSeasonTorrents: [Season 3 packs]                │
│  - CompleteSeriesTorrents: [Complete series]               │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    processResults()                          │
│                                                             │
│  1. Sort all torrents by priority:                        │
│     - Resolution priority (from user config)               │
│     - Size as tie-breaker (larger is better)              │
│                                                             │
│  2. Collect Magnets:                                       │
│     - Apibay torrents already have hashes                  │
│     - YGG torrents: fetch missing hashes                   │
│                                                             │
│  2. Upload to AllDebrid:                                   │
│     - Group by provider                                    │
│     - Upload each magnet                                   │
│     - Wait 2 seconds                                       │
│                                                             │
│  3. Sequential Processing:                                  │
│     - Process torrents one by one in quality order         │
│     - Check each magnet (2 attempts with 3s delay)         │
│     - Return first working stream immediately               │
│                                                             │
│  4. Create Streams:                                        │
│     - Only from ready magnets                              │
│     - Return first working stream immediately              │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│ Stream Response │
│   JSON Array    │
└─────────────────┘
```

## Torrent Processing Pipeline

```
┌─────────────────┐
│ Raw Torrents    │
│ from YGG/Apibay │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────┐
│ Filter 1: Year (Movies only)        │
│ - Extract year from title           │
│ - Allow ±1 year tolerance           │
│ - Pass if matches or no year       │
└────────┬────────────────────────────┘
         │ Passed
         ▼
┌─────────────────────────────────────┐
│ Classification                      │
│ - Generic classification logic      │
│ - Parse from title patterns         │
│ - Classify as:                      │
│   • movie                           │
│   • episode (S3E48)                 │
│   • season (Season 3 complete)     │
│   • complete_series                 │
└────────┬────────────────────────────┘
         │ Classified
         ▼
┌─────────────────────────────────────┐
│ Filter 2: Resolution                │
│ - Parse resolution from title       │
│ - Check against RES_TO_SHOW         │
│ - Pass if matches or no filter     │
└────────┬────────────────────────────┘
         │ Passed all filters
         ▼
┌─────────────────────────────────────┐
│ Add to Result Category              │
│ - MovieTorrents[]                   │
│ - EpisodeTorrents[]                 │
│ - CompleteSeasonTorrents[]          │
│ - CompleteSeriesTorrents[]          │
└─────────────────────────────────────┘
```

## AllDebrid Magnet Processing

```
┌──────────────────┐
│ Torrent Results  │
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ Sort & Extract Magnets               │
│ - Sort by priority (resolution+size) │
│ - Apibay: Use provided hash          │
│ - YGG: Fetch hash if missing         │
│ - Sequential processing (no limits)  │
└────────┬─────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ Sequential Processing                │
│ - Process torrents one by one        │
│ - In quality order (res, size)       │
└────────┬─────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ For Each Torrent (in order):        │
│                                      │
│ 1. Upload Phase:                     │
│    POST /magnet/upload               │
│    - Send single magnet              │
│    - Log success/failure             │
│                                      │
│ 2. Wait 2 seconds                    │
│                                      │
│ 3. Check Phase (2 attempts):        │
│    POST /magnet/status               │
│    - Check if ready                  │
│    - If ready: process & return      │
│    - If not ready: wait 3s, retry   │
│    - If still not ready: next torrent│
└────────┬─────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ Process Ready Magnets                │
│ - Extract video links                │
│ - Create Stream objects              │
│ - Add quality/size metadata          │
└──────────────────────────────────────┘
```

## Cache Hierarchy

```
┌─────────────────┐
│ Request Handler │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────┐
│ Check Memory Cache (LRU)            │
│ - Capacity: 1000 items              │
│ - TTL: 24 hours                     │
│ - Hit? Return immediately           │
└────────┬────────────────────────────┘
         │ Miss
         ▼
┌─────────────────────────────────────┐
│ Check Database (BoltDB)             │
│ - Persistent storage                │
│ - Hit? Update memory cache & return │
└────────┬────────────────────────────┘
         │ Miss
         ▼
┌─────────────────────────────────────┐
│ Fetch from External API             │
│ - YGG/Apibay/TMDB/AllDebrid         │
│ - Update both caches                │
│ - Return result                     │
└─────────────────────────────────────┘
```

## Error Handling Flow

```
┌──────────────┐
│ User Request │
└──────┬───────┘
       │
       ▼
┌────────────────────────────────┐
│ 30s Request Timeout Context    │◄──── Timeout Protection
├────────────────────────────────┤
│                                │
│ ┌────────────────────────────┐ │
│ │ 15s Search Timeout         │ │◄──── Partial Results
│ ├────────────────────────────┤ │
│ │ YGG Search  │ Apibay Search│ │
│ │ - Panic recovery           │ │◄──── Crash Protection
│ │ - Error logging            │ │
│ └────────────────────────────┘ │
│                                │
│ ┌────────────────────────────┐ │
│ │ 5s Rate Limiter Timeout    │ │◄──── Deadlock Prevention
│ └────────────────────────────┘ │
│                                │
│ ┌────────────────────────────┐ │
│ │ AllDebrid Retry Logic      │ │
│ │ - 8 attempts               │ │◄──── Resilience
│ │ - Progressive backoff      │ │
│ └────────────────────────────┘ │
└────────────────────────────────┘
```

## Configuration Impact on Flow

```
User Config
├── RES_TO_SHOW: ["2160p", "1080p", "720p", "480p"]
│   └── Affects: Resolution filtering after classification
│       - Priority order for torrent sorting
│       - Size-based tie-breaking within same resolution
│
├── API_KEY_ALLDEBRID: "..."
│   └── Required for:
│       - Magnet upload
│       - Status checking
│       - Stream generation
│
└── TMDB_API_KEY: "..."
    └── Used for:
        - Getting content metadata
        - French title translation
        - Catalog browsing
```

## v5.0 Flow Improvements

### Simplified Torrent Sorting Algorithm

```
┌────────────────────────────────────────┐
│ Before v5.0 (Complex Sorting)         │
├────────────────────────────────────────┤
│ 1. Language priority (YGG vs others)  │
│ 2. Provider preference (YGG > Apibay) │
│ 3. Resolution priority                 │
│ 4. Size as tie-breaker               │
│ Problem: Apibay processed first!      │
└────────────────────────────────────────┘
         │
         ▼ SIMPLIFIED
┌────────────────────────────────────────┐
│ v5.0 (Clean Sorting)                  │
├────────────────────────────────────────┤
│ 1. Resolution priority (user config)  │
│ 2. Size as tie-breaker (larger better)│
│ Result: YGG season packs prioritized! │
└────────────────────────────────────────┘
```

### Resource Management Improvements

```
┌──────────────────────────────────┐
│ HTTP Client Connections         │
├──────────────────────────────────┤
│ Before: Default connections      │
│ - No connection pooling          │
│ - Potential resource exhaustion  │
│                                  │
│ v5.0: Connection Pooling         │
│ - MaxIdleConns: 10               │
│ - MaxIdleConnsPerHost: 2         │
│ - IdleConnTimeout: 30s           │
│ - Applied to all HTTP clients    │
└──────────────────────────────────┘
```

### Code Quality Impact

```
┌──────────────────────────────────┐
│ Removed Complexity (~300 lines) │
├──────────────────────────────────┤
│ - Unused language filter logic  │
│ - Legacy compatibility functions │
│ - Redundant search methods      │
│ - Obsolete interface methods    │
│                                 │
│ Result: Simpler, maintainable   │
│ codebase following KISS principle│
└──────────────────────────────────┘
```