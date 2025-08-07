// Package services provides dependency injection container for application services.
package services

import (
	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch"
)

// Container holds all application services for dependency injection.
type Container struct {
	TMDB           TMDBService
	AllDebrid      AllDebridService
	Cache          *cache.LRUCache
	DB             database.Database
	Logger         logger.Logger
	TorrentSorter  *TorrentSorter
	Cleanup        *CleanupService
	TorrentSearch  *torrentsearch.TorrentSearch
}

// TMDBService defines the interface for TMDB API operations.
type TMDBService interface {
	GetIMDBInfo(imdbID string) (string, string, string, int, string, error)
	GetTMDBInfo(tmdbID string) (string, string, string, int, string, error)
	GetTMDBInfoWithType(tmdbID, mediaType string) (string, string, string, int, string, error)
	GetPopularMovies(page int, genreID string) ([]models.Meta, error)
	GetPopularSeries(page int, genreID string) ([]models.Meta, error)
	GetTrending(mediaType string, timeWindow string, page int) ([]models.Meta, error)
	SearchMulti(query string, page int) ([]models.Meta, error)
	GetMetadata(mediaType, tmdbID string) (*models.Meta, error)
}

// AllDebridService defines the interface for AllDebrid API operations.
type AllDebridService interface {
	CheckMagnets(magnets []models.MagnetInfo, apiKey string) ([]models.ProcessedMagnet, error)
	UploadMagnet(hash, title, apiKey string) error
	GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error)
	UnlockLink(link, apiKey string) (string, error)
	DeleteMagnet(magnetID, apiKey string) error
}
