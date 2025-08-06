// Package main contains application initialization and setup
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
	log "github.com/amaumene/gostremiofr/pkg/logger"
)

// Global application components
var (
	logger      log.Logger
	db          database.Database
	tmdbCache   *cache.LRUCache
	httpHandler *handlers.Handler
	container   *services.Container
)

// initLogger initializes the application logger with configured log level
func initLogger() {
	logger = log.New()

	level := getLogLevel()
	validateLogLevel(level)
}

// getLogLevel retrieves log level from environment or returns default
func getLogLevel() string {
	level := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if level == "" {
		return "info"
	}
	return level
}

// validateLogLevel checks if the provided log level is valid
func validateLogLevel(level string) {
	validLevels := map[string]bool{
		"debug":   true,
		"info":    true,
		"warn":    true,
		"warning": true,
		"error":   true,
	}

	if !validLevels[level] {
		logger.Warnf("unknown log level %q, defaulting to info", level)
	}
}

// initDatabase initializes the BoltDB database
func initDatabase() {
	dbPath := getDatabasePath()
	
	var err error
	db, err = database.NewBolt(dbPath)
	if err != nil {
		logger.Fatalf("failed to initialize database: %v", err)
	}

	logger.Info("database initialized successfully")
}

// getDatabasePath returns the database file path
func getDatabasePath() string {
	dir := os.Getenv("DATABASE_DIR")
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "data.db")
}

// initServices creates and initializes all application services
func initServices() {
	tmdbCache = createCache()
	container = createServiceContainer(tmdbCache, db)
	httpHandler = handlers.New(container, nil)
	
	logger.Info("services initialized successfully")
}

// createCache creates a new LRU cache instance
func createCache() *cache.LRUCache {
	const (
		cacheSize = 5000
		cacheTTL  = 24 * time.Hour
	)
	return cache.New(cacheSize, cacheTTL)
}

// createServiceContainer creates and configures the service container
func createServiceContainer(c *cache.LRUCache, d database.Database) *services.Container {
	// Initialize services
	tmdb := services.NewTMDB("", c)
	ygg := services.NewYGG(d, c, tmdb)
	torrentsCSV := services.NewTorrentsCSV(d, c)
	
	// Configure AllDebrid service
	allDebrid := services.NewAllDebrid("")
	allDebrid.SetDB(d)
	
	// Create cleanup service
	cleanup := services.NewCleanupService(d, allDebrid)
	
	return &services.Container{
		TMDB:          tmdb,
		AllDebrid:     allDebrid,
		YGG:           ygg,
		TorrentsCSV:   torrentsCSV,
		Cache:         c,
		DB:            d,
		Logger:        log.New(),
		TorrentSorter: services.NewTorrentSorter(nil),
		Cleanup:       cleanup,
	}
}