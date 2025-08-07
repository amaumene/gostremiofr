// Package database provides data persistence using BoltDB.
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amaumene/gostremiofr/bolthold"
)

const (
	// Default database file permissions
	dbFileMode = 0600
	dbDirMode  = 0755
	
	// Default database filename
	defaultDBFile = "data.db"
)

// TMDBCache represents cached TMDB metadata for movies and TV shows.
type TMDBCache struct {
	IMDBId           string
	Type             string // "movie" or "series"
	Title            string
	Year             int
	OriginalLanguage string
	CreatedAt        time.Time
}

// Magnet represents a magnet link with associated metadata.
type Magnet struct {
	ID           string    // Unique identifier
	Hash         string    // Info hash
	Name         string    // Torrent name
	AddedAt      time.Time // When it was added
	AllDebridID  string    // AllDebrid magnet ID for cleanup
	AllDebridKey string    // API key used (for cleanup)
}

// Database defines the interface for data persistence operations.
type Database interface {
	// GetCachedTMDB retrieves cached TMDB data by IMDB ID
	GetCachedTMDB(imdbId string) (*TMDBCache, error)
	// StoreTMDBCache stores TMDB metadata
	StoreTMDBCache(cache *TMDBCache) error
	// StoreMagnet stores a magnet link
	StoreMagnet(magnet *Magnet) error
	// GetMagnets retrieves all stored magnets
	GetMagnets() ([]Magnet, error)
	// GetOldMagnets retrieves magnets older than specified duration
	GetOldMagnets(olderThan time.Duration) ([]Magnet, error)
	// DeleteMagnet removes a magnet by ID
	DeleteMagnet(id string) error
	// Close closes the database connection
	Close() error
}

// BoltDB implements the Database interface using BoltDB.
type BoltDB struct {
	store *bolthold.Store
}

// BoltTMDBCache is the BoltDB-specific structure for TMDB cache storage.
type BoltTMDBCache struct {
	IMDBId           string `boltholdKey:"IMDBId"`
	Type             string
	Title            string
	Year             int
	OriginalLanguage string
	CreatedAt        time.Time
}

// BoltMagnet is the BoltDB-specific structure for magnet storage.
type BoltMagnet struct {
	ID           string `boltholdKey:"ID"`
	Hash         string `boltholdUnique:"Hash"`
	Name         string
	AddedAt      time.Time
	AllDebridID  string // AllDebrid magnet ID for cleanup
	AllDebridKey string // API key used (for cleanup)
}

// NewBolt creates a new BoltDB database instance.
// If dbPath is empty, uses the default database file in current directory.
func NewBolt(dbPath string) (*BoltDB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(".", defaultDBFile)
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, dbDirMode); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	store, err := bolthold.Open(dbPath, dbFileMode, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt database: %w", err)
	}

	return &BoltDB{store: store}, nil
}

// Close closes the database connection.
func (db *BoltDB) Close() error {
	return db.store.Close()
}

// GetCachedTMDB retrieves cached TMDB data by IMDB ID.
// Returns nil if not found, without error.
func (db *BoltDB) GetCachedTMDB(imdbId string) (*TMDBCache, error) {
	var cache BoltTMDBCache
	err := db.store.Get(imdbId, &cache)
	if err == bolthold.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get TMDB cache: %w", err)
	}

	// Convert from storage format
	return convertToTMDBCache(&cache), nil
}

// convertToTMDBCache converts BoltTMDBCache to TMDBCache.
func convertToTMDBCache(bolt *BoltTMDBCache) *TMDBCache {
	return &TMDBCache{
		IMDBId:           bolt.IMDBId,
		Type:             bolt.Type,
		Title:            bolt.Title,
		Year:             bolt.Year,
		OriginalLanguage: bolt.OriginalLanguage,
		CreatedAt:        bolt.CreatedAt,
	}
}

// StoreTMDBCache stores TMDB metadata in the database.
// Updates existing entries or creates new ones.
func (db *BoltDB) StoreTMDBCache(cache *TMDBCache) error {
	boltCache := &BoltTMDBCache{
		IMDBId:           cache.IMDBId,
		Type:             cache.Type,
		Title:            cache.Title,
		Year:             cache.Year,
		OriginalLanguage: cache.OriginalLanguage,
		CreatedAt:        time.Now(),
	}

	err := db.store.Upsert(cache.IMDBId, boltCache)
	if err != nil {
		return fmt.Errorf("failed to store TMDB cache: %w", err)
	}

	return nil
}

// StoreMagnet stores a magnet link in the database.
// Updates existing entries or creates new ones.
func (db *BoltDB) StoreMagnet(magnet *Magnet) error {
	boltMagnet := &BoltMagnet{
		ID:           magnet.ID,
		Hash:         magnet.Hash,
		Name:         magnet.Name,
		AddedAt:      time.Now(),
		AllDebridID:  magnet.AllDebridID,
		AllDebridKey: magnet.AllDebridKey,
	}

	err := db.store.Upsert(magnet.ID, boltMagnet)
	if err != nil {
		return fmt.Errorf("failed to store magnet: %w", err)
	}

	return nil
}

// GetMagnets retrieves all stored magnets from the database.
func (db *BoltDB) GetMagnets() ([]Magnet, error) {
	var boltMagnets []BoltMagnet
	err := db.store.Find(&boltMagnets, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get magnets: %w", err)
	}

	return convertToMagnets(boltMagnets), nil
}

// convertToMagnets converts slice of BoltMagnet to slice of Magnet.
func convertToMagnets(boltMagnets []BoltMagnet) []Magnet {
	magnets := make([]Magnet, len(boltMagnets))
	for i, bm := range boltMagnets {
		magnets[i] = convertToMagnet(&bm)
	}
	return magnets
}

// convertToMagnet converts BoltMagnet to Magnet.
func convertToMagnet(bolt *BoltMagnet) Magnet {
	return Magnet{
		ID:           bolt.ID,
		Hash:         bolt.Hash,
		Name:         bolt.Name,
		AddedAt:      bolt.AddedAt,
		AllDebridID:  bolt.AllDebridID,
		AllDebridKey: bolt.AllDebridKey,
	}
}

// DeleteMagnet removes a magnet by ID from the database.
// Returns nil if the magnet doesn't exist.
func (db *BoltDB) DeleteMagnet(id string) error {
	err := db.store.Delete(id, BoltMagnet{})
	if err == bolthold.ErrNotFound {
		return nil // Already deleted, not an error
	}
	if err != nil {
		return fmt.Errorf("failed to delete magnet: %w", err)
	}

	return nil
}

// GetOldMagnets returns magnets older than the specified duration.
// Used primarily for cleanup operations.
func (db *BoltDB) GetOldMagnets(olderThan time.Duration) ([]Magnet, error) {
	cutoffTime := time.Now().Add(-olderThan)

	var boltMagnets []BoltMagnet
	err := db.store.Find(&boltMagnets, bolthold.Where("AddedAt").Lt(cutoffTime))
	if err != nil {
		return nil, fmt.Errorf("failed to get old magnets: %w", err)
	}

	return convertToMagnets(boltMagnets), nil
}
