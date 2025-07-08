package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Database interface for abstracting different database implementations
type Database interface {
	Close() error
	GetCachedTMDB(imdbId string) (*TMDBCache, error)
	StoreTMDBCache(cache *TMDBCache) error
	StoreMagnet(magnet *Magnet) error
	GetMagnets() ([]Magnet, error)
	DeleteMagnet(id string) error
}

type DB struct {
	conn *sql.DB

	stmtGetTMDB      *sql.Stmt
	stmtStoreTMDB    *sql.Stmt
	stmtStoreMagnet  *sql.Stmt
	stmtGetMagnets   *sql.Stmt
	stmtDeleteMagnet *sql.Stmt
	mu               sync.RWMutex
}

type TMDBCache struct {
	IMDBId    string    `json:"imdb_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Year      int       `json:"year"`
	CreatedAt time.Time `json:"created_at"`
}

type Magnet struct {
	ID      string    `json:"id"`
	Hash    string    `json:"hash"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"added_at"`
}

func New(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = filepath.Join(".", "streams.db")
	}

	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{conn: conn}

	if err := db.createTables(); err != nil {
		conn.Close()
		return nil, err
	}

	if err := db.prepareStatements(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.stmtGetTMDB != nil {
		db.stmtGetTMDB.Close()
	}
	if db.stmtStoreTMDB != nil {
		db.stmtStoreTMDB.Close()
	}
	if db.stmtStoreMagnet != nil {
		db.stmtStoreMagnet.Close()
	}
	if db.stmtGetMagnets != nil {
		db.stmtGetMagnets.Close()
	}
	if db.stmtDeleteMagnet != nil {
		db.stmtDeleteMagnet.Close()
	}

	return db.conn.Close()
}

func (db *DB) createTables() error {
	tmdbTable := `
	CREATE TABLE IF NOT EXISTS tmdb_cache (
		imdb_id TEXT PRIMARY KEY,
		type TEXT,
		title TEXT,
		year INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	magnetsTable := `
	CREATE TABLE IF NOT EXISTS magnets (
		id TEXT PRIMARY KEY NOT NULL,
		hash TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	tmdbIndex := `CREATE INDEX IF NOT EXISTS idx_tmdb_created_at ON tmdb_cache(created_at)`
	magnetsIndex := `CREATE INDEX IF NOT EXISTS idx_magnets_added_at ON magnets(added_at, id)`

	if _, err := db.conn.Exec(tmdbTable); err != nil {
		return fmt.Errorf("failed to create tmdb_cache table: %w", err)
	}

	if _, err := db.conn.Exec(magnetsTable); err != nil {
		return fmt.Errorf("failed to create magnets table: %w", err)
	}

	if _, err := db.conn.Exec(tmdbIndex); err != nil {
		return fmt.Errorf("failed to create tmdb_cache index: %w", err)
	}

	if _, err := db.conn.Exec(magnetsIndex); err != nil {
		return fmt.Errorf("failed to create magnets index: %w", err)
	}

	// Migration: Add year column if it doesn't exist
	migrationSQL := `ALTER TABLE tmdb_cache ADD COLUMN year INTEGER DEFAULT 0`
	db.conn.Exec(migrationSQL) // Ignore error if column already exists

	return nil
}

func (db *DB) prepareStatements() error {
	var err error

	db.stmtGetTMDB, err = db.conn.Prepare("SELECT type, title, year FROM tmdb_cache WHERE imdb_id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare get TMDB statement: %w", err)
	}

	db.stmtStoreTMDB, err = db.conn.Prepare("INSERT OR REPLACE INTO tmdb_cache (imdb_id, type, title, year) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare store TMDB statement: %w", err)
	}

	db.stmtStoreMagnet, err = db.conn.Prepare("INSERT OR REPLACE INTO magnets (id, hash, name, added_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)")
	if err != nil {
		return fmt.Errorf("failed to prepare store magnet statement: %w", err)
	}

	db.stmtGetMagnets, err = db.conn.Prepare("SELECT id, hash, name, added_at FROM magnets ORDER BY added_at ASC")
	if err != nil {
		return fmt.Errorf("failed to prepare get magnets statement: %w", err)
	}

	db.stmtDeleteMagnet, err = db.conn.Prepare("DELETE FROM magnets WHERE id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare delete magnet statement: %w", err)
	}

	return nil
}

func (db *DB) GetCachedTMDB(imdbId string) (*TMDBCache, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var cache TMDBCache
	cache.IMDBId = imdbId

	err := db.stmtGetTMDB.QueryRow(imdbId).Scan(&cache.Type, &cache.Title, &cache.Year)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get TMDB cache: %w", err)
	}

	return &cache, nil
}

func (db *DB) StoreTMDBCache(cache *TMDBCache) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtStoreTMDB.Exec(cache.IMDBId, cache.Type, cache.Title, cache.Year)
	if err != nil {
		return fmt.Errorf("failed to store TMDB cache: %w", err)
	}

	return nil
}

func (db *DB) StoreMagnet(magnet *Magnet) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtStoreMagnet.Exec(magnet.ID, magnet.Hash, magnet.Name)
	if err != nil {
		return fmt.Errorf("failed to store magnet: %w", err)
	}

	return nil
}

func (db *DB) GetMagnets() ([]Magnet, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.stmtGetMagnets.Query()
	if err != nil {
		return nil, fmt.Errorf("failed to get magnets: %w", err)
	}
	defer rows.Close()

	var magnets []Magnet
	for rows.Next() {
		var m Magnet
		if err := rows.Scan(&m.ID, &m.Hash, &m.Name, &m.AddedAt); err != nil {
			return nil, fmt.Errorf("failed to scan magnet: %w", err)
		}
		magnets = append(magnets, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating magnets: %w", err)
	}

	return magnets, nil
}

func (db *DB) DeleteMagnet(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.stmtDeleteMagnet.Exec(id)
	if err != nil {
		return fmt.Errorf("failed to delete magnet: %w", err)
	}

	return nil
}

func (db *DB) CleanupOldRecords(maxAge time.Duration) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	query := "DELETE FROM tmdb_cache WHERE created_at < ?"
	if _, err := db.conn.Exec(query, cutoff); err != nil {
		return fmt.Errorf("failed to cleanup TMDB cache: %w", err)
	}

	query = "DELETE FROM magnets WHERE added_at < ?"
	if _, err := db.conn.Exec(query, cutoff); err != nil {
		return fmt.Errorf("failed to cleanup magnets: %w", err)
	}

	return nil
}
