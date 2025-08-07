// Application initialization and setup.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amaumene/gostremiofr/internal/adapters"
	"github.com/amaumene/gostremiofr/internal/cache"
	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/internal/handlers"
	"github.com/amaumene/gostremiofr/internal/services"
	log "github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/providers"
)

// Global application components
var (
	logger      log.Logger
	db          database.Database
	tmdbCache   *cache.LRUCache
	httpHandler *handlers.Handler
	container   *services.Container
)

// initLogger initializes the application logger.
func initLogger() {
	logger = log.New()
}

// initDatabase initializes the BoltDB database.
func initDatabase() {
	dbPath := getDatabasePath()
	
	var err error
	db, err = database.NewBolt(dbPath)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize database: %v", err))
	}
}

// getDatabasePath returns the database file path.
func getDatabasePath() string {
	dir := os.Getenv("DATABASE_DIR")
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "data.db")
}

// initServices creates and initializes all application services.
func initServices() {
	tmdbCache = createCache()
	container = createServiceContainer(tmdbCache, db)
	httpHandler = handlers.New(container, nil)
}

// createCache creates a new LRU cache instance.
func createCache() *cache.LRUCache {
	const (
		cacheSize = 5000
		cacheTTL  = 24 * time.Hour
	)
	return cache.New(cacheSize, cacheTTL)
}

// createServiceContainer creates and configures the service container.
func createServiceContainer(c *cache.LRUCache, d database.Database) *services.Container {
	// Initialize services
	tmdb := services.NewTMDB("", c)
	
	// Configure AllDebrid service
	allDebrid := services.NewAllDebrid("")
	allDebrid.SetDB(d)
	
	// Create cleanup service
	cleanup := services.NewCleanupService(d, allDebrid)
	
	// Create torrentsearch with native providers
	torrentSearch := createTorrentSearch(c)
	
	return &services.Container{
		TMDB:          tmdb,
		AllDebrid:     allDebrid,
		Cache:         c,
		DB:            d,
		Logger:        log.New(),
		TorrentSorter: services.NewTorrentSorter(nil),
		Cleanup:       cleanup,
		TorrentSearch: torrentSearch,
	}
}

// createTorrentSearch creates the smart torrentsearch with providers.
func createTorrentSearch(c *cache.LRUCache) *torrentsearch.TorrentSearch {
	// Create cache adapter
	cacheAdapter := adapters.NewCacheAdapter(c)
	
	// Initialize torrentsearch
	search := torrentsearch.New(cacheAdapter)
	
	// Don't set TMDB key here - it will be set per request from client
	
	// Register native providers directly
	yggProvider := providers.NewYGGProvider()
	yggProvider.SetCache(cacheAdapter)
	search.RegisterProvider(providers.ProviderYGG, yggProvider)
	
	torrentsCSVProvider := providers.NewTorrentsCSVProvider()
	torrentsCSVProvider.SetCache(cacheAdapter)
	search.RegisterProvider(providers.ProviderTorrentsCSV, torrentsCSVProvider)
	
	apibayProvider := providers.NewApiBayProvider()
	apibayProvider.SetCache(cacheAdapter)
	search.RegisterProvider(providers.ProviderApiBay, apibayProvider)
	
	return search
}