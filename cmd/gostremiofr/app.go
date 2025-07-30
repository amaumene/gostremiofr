package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/handlers"
	"github.com/amaumene/gostremiofr/internal/services"
	"github.com/amaumene/gostremiofr/pkg/logger"
)

var (
	Logger           logger.Logger
	DB               database.Database
	tmdbMemoryCache  *cache.LRUCache
	handler          *handlers.Handler
	serviceContainer *services.Container
)

func InitializeLogger() {
	Logger = logger.New()

	// Get log level from environment
	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "info"
	}

	// Log level validation (for user feedback)
	switch logLevel {
	case "debug", "info", "warn", "warning", "error":
		// Valid log levels
	default:
		Logger.Warnf("[App] warning: unknown log level '%s', defaulting to info", os.Getenv("LOG_LEVEL"))
	}
}

func InitializeDatabase() {
	var err error

	// Get database directory from environment variable, default to current directory
	dbDir := os.Getenv("DATABASE_DIR")
	if dbDir == "" {
		dbDir = "."
	}
	dbPath := filepath.Join(dbDir, "data.db")

	DB, err = database.NewBolt(dbPath)
	if err != nil {
		Logger.Fatalf("failed to initialize database: %v", err)
	}

	Logger.Infof("[App] BoltHold database initialized successfully")
}

func InitializeServices() {
	// Initialize cache with larger capacity for better performance with large series
	tmdbMemoryCache = cache.New(5000, 24*time.Hour)

	// Initialize services
	tmdbService := services.NewTMDB("", tmdbMemoryCache) // empty API key for now
	yggService := services.NewYGG(DB, tmdbMemoryCache, tmdbService)
	apibayService := services.NewApibay(DB, tmdbMemoryCache)
	allDebridService := services.NewAllDebrid("") // empty API key for now
	allDebridService.SetDB(DB) // Set database for cleanup tracking

	// Initialize cleanup service
	cleanupService := services.NewCleanupService(DB, allDebridService)

	// Initialize services container
	serviceContainer = &services.Container{
		TMDB:          tmdbService,
		AllDebrid:     allDebridService,
		YGG:           yggService,
		Apibay:        apibayService,
		Cache:         tmdbMemoryCache,
		DB:            DB,
		Logger:        logger.New(),
		TorrentSorter: services.NewTorrentSorter(nil), // Will be updated with config later
		Cleanup:       cleanupService,
	}

	// Initialize handler
	handler = handlers.New(serviceContainer, nil)

	Logger.Infof("[App] services initialized successfully")
}
