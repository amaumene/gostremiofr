package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

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

	// Create tables if they don't exist
	createTables()
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

	if _, err := DB.Exec(tmdbTable); err != nil {
		Logger.Fatalf("failed to create tmdb_cache table: %v", err)
	}

	if _, err := DB.Exec(magnetsTable); err != nil {
		Logger.Fatalf("failed to create magnets table: %v", err)
	}
}

func GetCachedTMDB(imdbId string) (*TMDBCache, error) {
	var cache TMDBCache
	err := DB.QueryRow(
		"SELECT type, title, french_title FROM tmdb_cache WHERE imdb_id = ?",
		imdbId,
	).Scan(&cache.Type, &cache.Title, &cache.FrenchTitle)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &cache, nil
}

func StoreTMDB(imdbId, mediaType, title, frenchTitle string) error {
	_, err := DB.Exec(
		"INSERT OR REPLACE INTO tmdb_cache (imdb_id, type, title, french_title) VALUES (?, ?, ?, ?)",
		imdbId, mediaType, title, frenchTitle,
	)
	return err
}

func StoreMagnet(id, hash, name string) error {
	if id == "" || hash == "" || name == "" {
		return fmt.Errorf("id, hash, and name must not be empty")
	}

	_, err := DB.Exec(
		"INSERT OR REPLACE INTO magnets (id, hash, name, added_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)",
		id, hash, name,
	)
	return err
}

func GetAllMagnets() ([]Magnet, error) {
	rows, err := DB.Query("SELECT id, hash, name, added_at FROM magnets ORDER BY added_at ASC")
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
	_, err := DB.Exec("DELETE FROM magnets WHERE id = ?", id)
	return err
}