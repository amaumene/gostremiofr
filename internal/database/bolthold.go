package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amaumene/gostremiofr/bolthold"
)

// TMDBCache represents cached TMDB data
type TMDBCache struct {
	IMDBId           string
	Type             string
	Title            string
	Year             int
	OriginalLanguage string
	CreatedAt        time.Time
}

// Magnet represents a magnet link
type Magnet struct {
	ID      string
	Hash    string
	Name    string
	AddedAt time.Time
}

// Database interface for database operations
type Database interface {
	GetCachedTMDB(imdbId string) (*TMDBCache, error)
	StoreTMDBCache(cache *TMDBCache) error
	StoreMagnet(magnet *Magnet) error
	GetMagnets() ([]Magnet, error)
	DeleteMagnet(id string) error
	Close() error
}

type BoltDB struct {
	store *bolthold.Store
}

type BoltTMDBCache struct {
	IMDBId           string `boltholdKey:"IMDBId"`
	Type             string
	Title            string
	Year             int
	OriginalLanguage string
	CreatedAt        time.Time
}

type BoltMagnet struct {
	ID      string `boltholdKey:"ID"`
	Hash    string `boltholdUnique:"Hash"`
	Name    string
	AddedAt time.Time
}

func NewBolt(dbPath string) (*BoltDB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(".", "data.db")
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	store, err := bolthold.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt database: %w", err)
	}

	return &BoltDB{store: store}, nil
}

func (db *BoltDB) Close() error {
	return db.store.Close()
}

func (db *BoltDB) GetCachedTMDB(imdbId string) (*TMDBCache, error) {
	var cache BoltTMDBCache
	err := db.store.Get(imdbId, &cache)
	if err == bolthold.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get TMDB cache: %w", err)
	}

	// Convert BoltTMDBCache to TMDBCache for compatibility
	return &TMDBCache{
		IMDBId:           cache.IMDBId,
		Type:             cache.Type,
		Title:            cache.Title,
		Year:             cache.Year,
		OriginalLanguage: cache.OriginalLanguage,
		CreatedAt:        cache.CreatedAt,
	}, nil
}

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

func (db *BoltDB) StoreMagnet(magnet *Magnet) error {
	boltMagnet := &BoltMagnet{
		ID:      magnet.ID,
		Hash:    magnet.Hash,
		Name:    magnet.Name,
		AddedAt: time.Now(),
	}

	err := db.store.Upsert(magnet.ID, boltMagnet)
	if err != nil {
		return fmt.Errorf("failed to store magnet: %w", err)
	}

	return nil
}

func (db *BoltDB) GetMagnets() ([]Magnet, error) {
	var boltMagnets []BoltMagnet
	err := db.store.Find(&boltMagnets, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get magnets: %w", err)
	}

	// Convert BoltMagnet to Magnet for compatibility
	magnets := make([]Magnet, len(boltMagnets))
	for i, bm := range boltMagnets {
		magnets[i] = Magnet{
			ID:      bm.ID,
			Hash:    bm.Hash,
			Name:    bm.Name,
			AddedAt: bm.AddedAt,
		}
	}

	return magnets, nil
}

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
