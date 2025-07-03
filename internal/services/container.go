package services

import (
	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/logger"
)

type Container struct {
	TMDB      TMDBService
	AllDebrid AllDebridService
	YGG       YGGService
	EZTV      EZTVService
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
	UnlockLink(link, apiKey string) (string, error)
}

type YGGService interface {
	SearchTorrents(query string, category string, season, episode int) (*models.TorrentResults, error)
	GetTorrentHash(torrentID string) (string, error)
}

type EZTVService interface {
	SearchTorrentsByIMDB(imdbID string, season, episode int) (*models.TorrentResults, error)
	SetConfig(cfg *config.Config)
}