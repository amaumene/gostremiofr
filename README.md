# GoStremio FR

A high-performance Stremio addon for French torrent streaming, built with Go following best practices.

## Architecture

The project follows Go best practices with a clean architecture:

```
.
├── cmd/
│   └── gostremiofr/        # Application entry point
├── internal/               # Private application code
│   ├── cache/             # LRU cache implementation
│   ├── config/            # Configuration management
│   ├── database/          # Database operations
│   ├── handlers/          # HTTP request handlers
│   ├── middleware/        # HTTP middleware
│   ├── models/            # Data structures
│   └── services/          # Business logic
├── pkg/                   # Public packages
│   ├── logger/            # Logging utilities
│   └── ratelimiter/       # Rate limiting
└── go.mod                 # Go module definition
```

## Features

- **Clean Architecture**: Separation of concerns with dependency injection
- **Performance**: Concurrent torrent searches with goroutines
- **Caching**: Two-level caching (memory + SQLite) for API responses
- **Rate Limiting**: Token bucket rate limiters for external APIs
- **Error Handling**: Proper error propagation and logging
- **Middleware**: Gzip compression, CORS, request logging

## Services

- **TMDB**: Movie/series metadata
- **YGG**: French torrent search
- **Sharewood**: Private tracker integration
- **AllDebrid**: Torrent caching service

## Configuration

Environment variables:
- `PORT`: HTTP server port (default: 5000)
- `DATABASE_PATH`: SQLite database path
- `TMDB_API_KEY`: TMDB API key
- `API_KEY_ALLDEBRID`: AllDebrid API key
- `SHAREWOOD_PASSKEY`: Sharewood passkey
- `DEBUG`: Enable debug logging

## Running

```bash
# Build
go build -o gostremiofr ./cmd/gostremiofr

# Run
./gostremiofr

# Or directly
go run ./cmd/gostremiofr
```

## API Endpoints

- `GET /manifest.json` - Addon manifest
- `GET /:config/manifest.json` - Configured manifest
- `GET /:config/stream/:type/:id.json` - Stream endpoint

## Development

```bash
# Install dependencies
go mod download

# Run tests
go test ./...

# Format code
go fmt ./...

# Lint
golangci-lint run
```