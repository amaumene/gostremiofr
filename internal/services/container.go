package services

import (
	"github.com/gostremiofr/gostremiofr/internal/cache"
	"github.com/gostremiofr/gostremiofr/internal/database"
	"github.com/gostremiofr/gostremiofr/internal/models"
	"github.com/gostremiofr/gostremiofr/pkg/logger"
)

type Container struct {
	TMDB      TMDBService
	AllDebrid AllDebridService
	YGG       YGGService
	Sharewood SharewoodService
	Cache     *cache.LRUCache
	DB        *database.DB
	Logger    logger.Logger
}

type TMDBService interface {
	GetIMDBInfo(imdbID string) (string, string, string, error)
}

type AllDebridService interface {
	CheckMagnets(magnets []models.MagnetInfo, apiKey string) ([]models.ProcessedMagnet, error)
	UploadMagnet(hash, title, apiKey string) error
	GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error)
}

type YGGService interface {
	SearchTorrents(query string, category string) (*models.TorrentResults, error)
	GetTorrentHash(torrentID string) (string, error)
}

type SharewoodService interface {
	SearchTorrents(query string, mediaType string, season, episode int) (*models.SharewoodResults, error)
}