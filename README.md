# GoStremioFR

A high-performance Stremio addon for French content, written in Go. This addon integrates with multiple torrent providers and AllDebrid to provide a seamless streaming experience.

## Features

- 🚀 **High Performance**: Built with Go for optimal speed and low resource usage
- 🔍 **Multiple Torrent Providers**: Supports YGG and TorrentsCSV torrent sources
- 🎬 **TMDB Integration**: Automatic metadata enrichment with French titles
- 📚 **Built-in Catalogs**: Self-sufficient with popular, trending, and search catalogs
- 📺 **Full Series Support**: Complete episode listings with season/episode metadata
- 💾 **Smart Caching**: Built-in LRU cache and BoltDB database for faster responses
- 🔐 **Secure API Handling**: Sanitized and validated API keys with masked logging
- 🌐 **AllDebrid Integration**: Stream torrents through AllDebrid for better performance
- 📊 **Intelligent Sorting**: Prioritizes streams by resolution and size
- 🏷️ **Source Tracking**: Stream results show the original torrent provider (YGG, TorrentsCSV)
- 🇫🇷 **French-Focused**: Catalogs optimized for French content via YGG integration
- ⚡ **Sequential Processing**: Processes torrents one-by-one in quality order until a working stream is found
- 📦 **Season Pack Support**: Intelligently extracts specific episodes from complete season torrents
- ⏱️ **Advanced Timeout Handling**: Request-level, search-level, and rate limiter timeouts prevent hanging
- 🎯 **Smart Prioritization**: Automatically prioritizes complete seasons over individual episodes for better quality
- 🔄 **Episode Fallback Search**: Two-phase search strategy - first searches for season packs, then specific episodes if needed

## Prerequisites

- Go 1.21 or higher
- AllDebrid account (for streaming)
- TMDB API key (optional, for metadata)

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/amaumene/gostremiofr.git
cd gostremiofr

# Build the application
go build -o gostremiofr ./cmd/gostremiofr

# Run the server
./gostremiofr
```

### Using Docker

```bash
# Build the Docker image
docker build -t gostremiofr .

# Run the container
docker run -p 5001:5001 \
  -e LOG_LEVEL=info \
  -e DATABASE_DIR=/data \
  -e USE_SSL=false \
  -v gostremiofr-data:/data \
  gostremiofr
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | `info` |
| `DATABASE_DIR` | Directory for BoltDB database | `.` |
| `PORT` | Server port | `5001` |
| `TMDB_API_KEY` | TMDB API key for metadata | - |
| `API_KEY_ALLDEBRID` | Default AllDebrid API key | - |
| `USE_SSL` | Enable SSL using local-ip.sh certificates | `false` |
| `GIN_MODE` | Gin framework mode (debug, release, test) | `release` |

### Configuration via Web Interface

1. Navigate to:
   - HTTP: `http://localhost:5001/config`
   - HTTPS (if USE_SSL=true): `https://[your-ip-with-dashes].local-ip.sh:5001/config`

2. Enter your configuration:
   - **TMDB API Key**: For movie/series metadata
   - **Resolutions**: Preferred resolutions in order (e.g., "2160p,1080p,720p,480p")
   - **AllDebrid API Key**: Your AllDebrid API key

3. Generate the configuration and use the provided manifest URL in Stremio

## Usage

### Adding to Stremio

1. Start the GoStremioFR server
2. Open Stremio
3. Go to Settings → Addons
4. Use one of these methods:
   - **With configuration**: Use the URL from the web interface
   - **Direct**: `http://localhost:5001/manifest.json`

### API Endpoints

- `GET /config` - Configuration interface
- `GET /{config}/manifest.json` - Addon manifest
- `GET /{config}/catalog/{type}/{id}.json` - Browse catalogs (popular, trending, search)
- `GET /{config}/meta/{type}/{id}.json` - Get detailed metadata
- `GET /{config}/stream/{type}/{id}.json` - Stream endpoint
- `GET /health` - Health check endpoint

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Stremio   │────▶│  GoStremioFR │────▶│   Torrent   │
│   Client    │     │    Server    │     │  Providers  │
└─────────────┘     └──────────────┘     └─────────────┘
                            │                     │
                            ▼                     │
                    ┌──────────────┐             │
                    │   AllDebrid  │◀────────────┘
                    │     API      │
                    └──────────────┘
```

### Project Structure

```
gostremiofr/
├── cmd/gostremiofr/    # Application entry point
├── internal/            # Internal packages
│   ├── config/         # Configuration management
│   ├── constants/      # Application constants
│   ├── database/       # Database operations
│   ├── cache/          # Caching implementation
│   ├── middleware/     # HTTP middleware (auth, CORS, etc.)
│   ├── handlers/       # HTTP request handlers
│   │   └── stream_helpers.go     # Stream parsing helper functions
│   ├── services/       # External service integrations
│   │   ├── alldebrid.go          # AllDebrid service implementation
│   │   ├── alldebrid_helpers.go  # AllDebrid helper functions
│   │   ├── tmdb.go               # TMDB service implementation
│   │   ├── tmdb_helpers.go       # TMDB helper functions
│   │   ├── ygg.go                # YGG torrent service
│   │   ├── ygg_helpers.go        # YGG helper functions
│   │   ├── torrentscsv.go        # TorrentsCSV torrent service
│   │   ├── torrent_service.go    # Base torrent service
│   │   ├── torrent_service_helpers.go # Torrent service helpers
│   │   ├── cleanup.go            # Cleanup service
│   │   └── cleanup_helpers.go    # Cleanup helper functions
│   └── models/         # Data models (organized by domain)
│       ├── common.go             # Common models and interfaces
│       ├── stream_models.go      # Stremio stream responses
│       ├── torrent_models.go     # Torrent processing models
│       ├── tmdb_models.go        # TMDB API responses
│       └── stremio_models.go     # Stremio protocol models
├── pkg/                # Public packages
│   ├── logger/         # Custom logging
│   ├── httputil/       # HTTP utilities and client
│   ├── security/       # Security utilities
│   ├── ratelimiter/    # Rate limiting utilities
│   ├── alldebrid/      # AllDebrid API client
│   └── ssl/            # SSL certificate utilities
└── docs/               # Documentation
```

### Key Components

- **Handlers**: Process Stremio requests and coordinate services
  - Stream handlers with dedicated helper functions for parsing
- **Services**: 
  - `YGG`: Searches YGG torrent tracker (French content)
  - `TorrentsCSV`: Searches TorrentsCSV API (International content)
  - `TMDB`: Fetches movie/series metadata
  - `AllDebrid`: Manages torrent downloads and streaming
  - `TorrentService`: Base service with common torrent processing logic
  - Each service has accompanying `*_helpers.go` file for utility functions
- **Cache**: LRU memory cache + BoltDB embedded database for persistence
- **Middleware**: HTTP middleware for authentication, CORS, and request handling
- **Security**: API key validation and sanitization

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/services/...
```

### Building for Production

```bash
# Build with optimizations
go build -ldflags="-w -s" -o gostremiofr ./cmd/gostremiofr

# Cross-compile for different platforms
GOOS=linux GOARCH=amd64 go build -o gostremiofr-linux-amd64 ./cmd/gostremiofr
GOOS=darwin GOARCH=amd64 go build -o gostremiofr-darwin-amd64 ./cmd/gostremiofr
GOOS=windows GOARCH=amd64 go build -o gostremiofr-windows-amd64.exe ./cmd/gostremiofr
```

### Code Style

- Follow standard Go formatting (`go fmt`)
- Add tests for new functionality
- Update documentation as needed
- Use meaningful commit messages

## Logging

The application uses structured logging with the following levels:

- **DEBUG**: Detailed information for debugging (API calls, torrent processing)
- **INFO**: High-level operations (server start, request summaries)
- **WARN**: Non-critical issues (invalid API keys with fallbacks)
- **ERROR**: Operation failures affecting functionality
- **FATAL**: Unrecoverable errors requiring shutdown

Set the log level using the `LOG_LEVEL` environment variable.

## Performance Considerations

- **Caching**: TMDB results are cached for 24 hours to reduce API calls
- **Rate Limiting**: Built-in rate limiters for all external APIs
- **Concurrent Torrent Search**: Parallel searches across YGG and TorrentsCSV with 15-second timeout
- **Database Optimization**: Indexed queries for fast lookups
- **Sequential Torrent Processing**: Processes best torrents one-by-one until a working stream is found
- **Smart Season Pack Handling**: Extracts only requested episodes from complete seasons
- **Request Timeouts**: 30-second overall timeout with multiple timeout layers
- **Immediate Response**: Returns the first working stream without processing remaining torrents
- **Quality Prioritization**: User-defined resolution preferences with size-based tiebreaking

## Security

- API keys are sanitized and validated before use
- Sensitive data is masked in logs (only first/last 3 characters shown)
- All external inputs are validated
- API keys are transmitted securely (POST requests where possible)

### SSL/HTTPS Support

GoStremioFR supports automatic SSL certificate provisioning using [local-ip.sh](https://local-ip.sh):

1. **Enable SSL**: Set `USE_SSL=true` environment variable
2. **Automatic Setup**: The server will:
   - Detect your local IP address
   - Download a valid SSL certificate from local-ip.sh
   - Configure HTTPS automatically
3. **Access**: Your addon will be available at `https://[your-ip-with-dashes].local-ip.sh:5001`

Example:
```bash
# Run with SSL enabled
USE_SSL=true ./gostremiofr

# The server will display something like:
# [App] starting HTTPS server on port 5001
# [App] accessible at https://192-168-1-100.local-ip.sh:5001
```

Benefits:
- Valid SSL certificates without configuration
- Works on local networks
- Automatically renewed certificates
- No certificate warnings in browsers

## Troubleshooting

### Common Issues

1. **No streams found**
   - Verify your AllDebrid API key is valid
   - Check if the content is available on supported trackers
   - Ensure your preferred resolutions are configured

2. **Slow responses**
   - Check your internet connection
   - Verify rate limits aren't being hit
   - Consider increasing cache size

3. **Database errors**
   - Ensure write permissions for database directory (`DATABASE_DIR`)
   - Check disk space availability
   - BoltDB database file will be created as `data.db` in the specified directory

### Debug Mode

Enable debug logging for detailed information:

```bash
LOG_LEVEL=debug ./gostremiofr
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the GPL-3.0 License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Stremio team for the addon protocol
- YGG community for French content
- AllDebrid for their service
- TMDB for movie/series metadata

## Support

For issues, questions, or contributions:
- Open an issue on [GitHub](https://github.com/amaumene/gostremiofr/issues)
- Check existing issues before creating new ones
- Provide logs and configuration details when reporting bugs
