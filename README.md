# GoStremioFR

A high-performance Stremio addon for French content, written in Go. This addon integrates with multiple torrent providers and AllDebrid to provide a seamless streaming experience.

## Features

- 🚀 **High Performance**: Built with Go for optimal speed and low resource usage
- 🔍 **Multiple Torrent Providers**: Supports YGG and EZTV torrent sources
- 🎬 **TMDB Integration**: Automatic metadata enrichment with French titles
- 💾 **Smart Caching**: Built-in LRU cache and SQLite database for faster responses
- 🔐 **Secure API Handling**: Sanitized and validated API keys with masked logging
- 🌐 **AllDebrid Integration**: Stream torrents through AllDebrid for better performance
- 📊 **Intelligent Sorting**: Prioritizes streams by resolution, language, and availability

## Prerequisites

- Go 1.21 or higher
- SQLite3
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
  -e DATABASE_PATH=/data/streams.db \
  -v gostremiofr-data:/data \
  gostremiofr
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | `info` |
| `DATABASE_PATH` | Path to SQLite database | `./streams.db` |
| `PORT` | Server port | `5001` |
| `TMDB_API_KEY` | TMDB API key for metadata | - |
| `API_KEY_ALLDEBRID` | Default AllDebrid API key | - |

### Configuration via Web Interface

1. Navigate to `http://localhost:5001/config`
2. Enter your configuration:
   - **TMDB API Key**: For movie/series metadata
   - **Files to Show**: Number of streams to display (default: 6)
   - **Resolutions**: Preferred resolutions in order (e.g., "1080p,720p,480p")
   - **Languages**: Preferred languages (e.g., "MULTI,FRENCH,VOSTFR")
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
│   ├── handlers/       # HTTP request handlers
│   ├── services/       # External service integrations
│   ├── models/         # Data models
│   ├── cache/          # Caching implementation
│   └── database/       # Database operations
├── pkg/                # Public packages
│   ├── logger/         # Custom logging
│   ├── security/       # Security utilities
│   └── alldebrid/      # AllDebrid client
└── docs/               # Documentation
```

### Key Components

- **Handlers**: Process Stremio requests and coordinate services
- **Services**: 
  - `YGG`: Searches YGG torrent tracker
  - `EZTV`: Searches EZTV for TV series
  - `TMDB`: Fetches movie/series metadata
  - `AllDebrid`: Manages torrent downloads and streaming
- **Cache**: LRU memory cache + SQLite for persistence
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
- **Concurrent Processing**: Parallel torrent searches and hash fetching
- **Database Optimization**: Indexed queries for fast lookups

## Security

- API keys are sanitized and validated before use
- Sensitive data is masked in logs (only first/last 3 characters shown)
- All external inputs are validated
- API keys are transmitted securely (POST requests where possible)

## Troubleshooting

### Common Issues

1. **No streams found**
   - Verify your AllDebrid API key is valid
   - Check if the content is available on supported trackers
   - Ensure your preferred languages/resolutions are configured

2. **Slow responses**
   - Check your internet connection
   - Verify rate limits aren't being hit
   - Consider increasing cache size

3. **Database errors**
   - Ensure write permissions for database directory
   - Check disk space availability

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
- YGG and EZTV communities
- AllDebrid for their service
- TMDB for movie/series metadata

## Support

For issues, questions, or contributions:
- Open an issue on [GitHub](https://github.com/amaumene/gostremiofr/issues)
- Check existing issues before creating new ones
- Provide logs and configuration details when reporting bugs
