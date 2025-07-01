package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

// Prepared statements for optimized queries
var (
	stmtGetTMDB    *sql.Stmt
	stmtStoreTMDB  *sql.Stmt
	stmtStoreMagnet *sql.Stmt
	stmtGetMagnets *sql.Stmt
	stmtDeleteMagnet *sql.Stmt
	stmtMutex      sync.RWMutex
)

type TMDBCache struct {
	IMDBId      string    `json:"imdb_id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	FrenchTitle string    `json:"french_title"`
	CreatedAt   time.Time `json:"created_at"`
}

type Magnet struct {
	ID      string    `json:"id"`
	Hash    string    `json:"hash"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"added_at"`
}

func InitializeDatabase() {
	var err error
	
	// Get database path from environment variable, default to current directory
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(".", "streams.db")
	} else {
		// Ensure the directory exists
		dbDir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			Logger.Fatalf("failed to create database directory: %v", err)
		}
	}
	
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		Logger.Fatalf("failed to open database: %v", err)
	}

	// Configure connection pool
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)

	// Create tables if they don't exist
	createTables()
	
	// Initialize prepared statements
	initPreparedStatements()
}

func createTables() {
	tmdbTable := `
	CREATE TABLE IF NOT EXISTS tmdb_cache (
		imdb_id TEXT PRIMARY KEY,
		type TEXT,
		title TEXT,
		french_title TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	magnetsTable := `
	CREATE TABLE IF NOT EXISTS magnets (
		id TEXT PRIMARY KEY NOT NULL,
		hash TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	// Create indexes for better performance
	tmdbIndex := `CREATE INDEX IF NOT EXISTS idx_tmdb_created_at ON tmdb_cache(created_at)`
	magnetsIndex := `CREATE INDEX IF NOT EXISTS idx_magnets_added_at ON magnets(added_at, id)`

	if _, err := DB.Exec(tmdbTable); err != nil {
		Logger.Fatalf("failed to create tmdb_cache table: %v", err)
	}

	if _, err := DB.Exec(magnetsTable); err != nil {
		Logger.Fatalf("failed to create magnets table: %v", err)
	}

	if _, err := DB.Exec(tmdbIndex); err != nil {
		Logger.Fatalf("failed to create tmdb_cache index: %v", err)
	}

	if _, err := DB.Exec(magnetsIndex); err != nil {
		Logger.Fatalf("failed to create magnets index: %v", err)
	}
}

func initPreparedStatements() {
	var err error
	
	stmtGetTMDB, err = DB.Prepare("SELECT type, title, french_title FROM tmdb_cache WHERE imdb_id = ?")
	if err != nil {
		Logger.Fatalf("failed to prepare get TMDB statement: %v", err)
	}
	
	stmtStoreTMDB, err = DB.Prepare("INSERT OR REPLACE INTO tmdb_cache (imdb_id, type, title, french_title) VALUES (?, ?, ?, ?)")
	if err != nil {
		Logger.Fatalf("failed to prepare store TMDB statement: %v", err)
	}
	
	stmtStoreMagnet, err = DB.Prepare("INSERT OR REPLACE INTO magnets (id, hash, name, added_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)")
	if err != nil {
		Logger.Fatalf("failed to prepare store magnet statement: %v", err)
	}
	
	stmtGetMagnets, err = DB.Prepare("SELECT id, hash, name, added_at FROM magnets ORDER BY added_at ASC")
	if err != nil {
		Logger.Fatalf("failed to prepare get magnets statement: %v", err)
	}
	
	stmtDeleteMagnet, err = DB.Prepare("DELETE FROM magnets WHERE id = ?")
	if err != nil {
		Logger.Fatalf("failed to prepare delete magnet statement: %v", err)
	}
}

func GetCachedTMDB(imdbId string) (*TMDBCache, error) {
	var cache TMDBCache
	stmtMutex.RLock()
	err := stmtGetTMDB.QueryRow(imdbId).Scan(&cache.Type, &cache.Title, &cache.FrenchTitle)
	stmtMutex.RUnlock()

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &cache, nil
}

func StoreTMDB(imdbId, mediaType, title, frenchTitle string) error {
	stmtMutex.Lock()
	_, err := stmtStoreTMDB.Exec(imdbId, mediaType, title, frenchTitle)
	stmtMutex.Unlock()
	return err
}

func StoreMagnet(id, hash, name string) error {
	if id == "" || hash == "" || name == "" {
		return fmt.Errorf("id, hash, and name must not be empty")
	}

	stmtMutex.Lock()
	_, err := stmtStoreMagnet.Exec(id, hash, name)
	stmtMutex.Unlock()
	return err
}

func GetAllMagnets() ([]Magnet, error) {
	stmtMutex.RLock()
	rows, err := stmtGetMagnets.Query()
	stmtMutex.RUnlock()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var magnets []Magnet
	for rows.Next() {
		var magnet Magnet
		err := rows.Scan(&magnet.ID, &magnet.Hash, &magnet.Name, &magnet.AddedAt)
		if err != nil {
			return nil, err
		}
		magnets = append(magnets, magnet)
	}

	return magnets, nil
}

func DeleteMagnet(id string) error {
	stmtMutex.Lock()
	_, err := stmtDeleteMagnet.Exec(id)
	stmtMutex.Unlock()
	return err
}