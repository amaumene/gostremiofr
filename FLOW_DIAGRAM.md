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
│           searchTorrentsWithIMDB()                           │
│  Concurrent searches with 15s timeout                       │
├─────────────────────┬──────────────────────────────────────┤
│                     │                                       │
▼                     ▼                                       │
┌──────────────┐     ┌──────────────┐                       │
│     YGG      │     │    EZTV      │                       │
│              │     │              │                       │
│ 1. Get French│     │ 1. Strip "tt"│                       │
│    title     │     │ 2. API call  │                       │
│ 2. API call  │     │ 3. Returns   │                       │
│ 3. For S3E48:│     │    torrents  │                       │
│    fetch hash│     │    with hash │                       │
│ 4. Filter &  │     │ 4. Filter &  │                       │
│    sort      │     │    sort      │                       │
└──────┬───────┘     └──────┬───────┘                       │
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
│  1. Collect Magnets:                                       │
│     - EZTV torrents already have hashes                    │
│     - YGG torrents: fetch missing hashes                   │
│                                                             │
│  2. Upload to AllDebrid:                                   │
│     - Group by provider                                    │
│     - Upload each magnet                                   │
│     - Wait 2 seconds                                       │
│                                                             │
│  3. Check Magnet Status (8 attempts):                      │
│     - Attempt 1: Check immediately                         │
│     - Not ready? Wait 2s, check again                      │
│     - Progressive backoff up to 14s                        │
│                                                             │
│  4. Create Streams:                                        │
│     - Only from ready magnets                              │
│     - Sort by resolution, language                         │
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
│ from YGG/EZTV   │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────┐
│ Filter 1: Language                  │
│ - Check against LANG_TO_SHOW        │
│ - EZTV = "VO", YGG = parse title   │
│ - Pass if matches or no filter     │
└────────┬────────────────────────────┘
         │ Passed
         ▼
┌─────────────────────────────────────┐
│ Filter 2: Year (Movies only)        │
│ - Extract year from title           │
│ - Allow ±1 year tolerance           │
│ - Pass if matches or no year       │
└────────┬────────────────────────────┘
         │ Passed
         ▼
┌─────────────────────────────────────┐
│ Classification                      │
│ - EZTV: Use API season/episode      │
│ - YGG: Parse from title patterns    │
│ - Classify as:                      │
│   • movie                           │
│   • episode (S3E48)                 │
│   • season (Season 3 complete)     │
│   • complete_series                 │
└────────┬────────────────────────────┘
         │ Classified
         ▼
┌─────────────────────────────────────┐
│ Filter 3: Resolution                │
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
│ Extract Magnets                      │
│ - EZTV: Use provided hash            │
│ - YGG: Fetch hash if missing         │
│ - Limit to 30 magnets                │
└────────┬─────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ Group by Provider                    │
│ - ygg: [magnet1, magnet2...]         │
│ - eztv: [magnet3, magnet4...]        │
└────────┬─────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ For Each Provider Group:             │
│                                      │
│ 1. Upload Phase:                     │
│    POST /magnet/upload               │
│    - Send each magnet                │
│    - Log success/failure             │
│                                      │
│ 2. Wait 2 seconds                    │
│                                      │
│ 3. Check Phase (8 attempts):        │
│    POST /magnet/status               │
│    - Send magnet hashes              │
│    - Check if ready                  │
│    - If not ready:                   │
│      • Attempt 1-8: wait n*2 sec    │
│      • Retry check                   │
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
│ - YGG/EZTV/TMDB/AllDebrid          │
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
│ │ YGG Search    │ EZTV Search│ │
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
├── LANG_TO_SHOW: ["MULTI", "VO", "VFF", "VF", "FRENCH"]
│   └── Affects: Torrent filtering in ProcessTorrents()
│       - EZTV torrents tagged as "VO"
│       - YGG torrents parsed from title
│
├── RES_TO_SHOW: ["2160p", "1080p", "720p", "480p"]
│   └── Affects: Resolution filtering after classification
│       - Can filter out all EZTV if only 4K selected
│
├── FILES_TO_SHOW: 6
│   └── Affects: 
│       - Max YGG torrents to fetch hashes for
│       - Max streams returned to user
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