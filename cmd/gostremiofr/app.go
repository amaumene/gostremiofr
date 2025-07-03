package main

import (
	"os"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/handlers"
	"github.com/amaumene/gostremiofr/internal/services"
	"github.com/amaumene/gostremiofr/pkg/logger"
)

var (
	Logger          logger.Logger
	DB              *database.DB
	tmdbMemoryCache *cache.LRUCache
	handler         *handlers.Handler
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
	
	// Get database path from environment variable, default to current directory
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./streams.db"
	}

	DB, err = database.New(dbPath)
	if err != nil {
		Logger.Fatalf("failed to initialize database: %v", err)
	}

	Logger.Infof("[App] database initialized successfully")
}

func InitializeServices() {
	// Initialize cache
	tmdbMemoryCache = cache.New(1000, 24*time.Hour)

	// Initialize services
	tmdbService := services.NewTMDB("", tmdbMemoryCache) // empty API key for now
	yggService := services.NewYGG(DB, tmdbMemoryCache)
	eztvService := services.NewEZTV(DB, tmdbMemoryCache)
	allDebridService := services.NewAllDebrid("")  // empty API key for now
	
	// Initialize services container
	serviceContainer := &services.Container{
		TMDB:      tmdbService,
		AllDebrid: allDebridService,
		YGG:       yggService,
		EZTV:      eztvService,
		Cache:     tmdbMemoryCache,
		DB:        DB,
		Logger:    logger.New(),
	}

	// Initialize handler
	handler = handlers.New(serviceContainer, nil)

	Logger.Infof("[App] services initialized successfully")
}