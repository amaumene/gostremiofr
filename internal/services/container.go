package services

import (
	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/pkg/logger"
)

type Container struct {
	TMDB          TMDBService
	AllDebrid     AllDebridService
	YGG           YGGService
	Apibay        ApibayService
	Cache         *cache.LRUCache
	DB            database.Database
	Logger        logger.Logger
	TorrentSorter *TorrentSorter
}

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

type AllDebridService interface {
	CheckMagnets(magnets []models.MagnetInfo, apiKey string) ([]models.ProcessedMagnet, error)
	UploadMagnet(hash, title, apiKey string) error
	GetVideoFiles(magnetID, apiKey string) ([]models.VideoFile, error)
	UnlockLink(link, apiKey string) (string, error)
}

type YGGService interface {
	SearchTorrents(query string, category string, season, episode int) (*models.TorrentResults, error)
	SearchTorrentsSpecificEpisode(query string, category string, season, episode int) (*models.TorrentResults, error)
	GetTorrentHash(torrentID string) (string, error)
	SetConfig(cfg *config.Config)
}

type ApibayService interface {
	SearchTorrents(query string, mediaType string, season, episode int) (*models.TorrentResults, error)
	SearchTorrentsSpecificEpisode(query string, mediaType string, season, episode int) (*models.TorrentResults, error)
	SetConfig(cfg *config.Config)
}
