// Package config provides configuration management for the application.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/amaumene/gostremiofr/internal/constants"
)

const (
	// Default configuration file name
	defaultConfigFile = "config.json"
	// Default database path
	defaultDatabasePath = "./cache.db"
)

// Config holds the application configuration.
// It supports loading from environment variables and JSON files.
type Config struct {
	// API Keys
	TMDBAPIKey      string `json:"TMDB_API_KEY"`
	APIKeyAllDebrid string `json:"API_KEY_ALLDEBRID"`
	
	// Content filtering
	ResToShow  []string `json:"RES_TO_SHOW"`  // Allowed resolutions
	LangToShow []string `json:"LANG_TO_SHOW"` // Allowed languages

	// Storage settings
	DatabasePath string        `json:"DATABASE_PATH"`
	CacheSize    int           `json:"CACHE_SIZE"`
	CacheTTL     time.Duration `json:"CACHE_TTL"`

	// Internal maps for fast lookups
	resMap   map[string]bool
	langMap  map[string]bool
	mapsOnce sync.Once
}

// Load reads configuration from environment variables and optional JSON file.
// Environment variables take precedence over file values.
// Returns an error if the configuration is invalid.
func Load() (*Config, error) {
	cfg := &Config{
		CacheSize:    constants.DefaultCacheSize,
		CacheTTL:     time.Duration(constants.DefaultCacheTTL) * time.Hour,
		DatabasePath: getEnvOrDefault("DATABASE_PATH", defaultDatabasePath),
	}

	// Load from environment variables
	cfg.loadFromEnv()

	// Load from config file if exists
	configFile := getEnvOrDefault("CONFIG_FILE", defaultConfigFile)
	if err := cfg.loadFromFile(configFile); err != nil {
		// Ignore file not found errors
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Initialize internal structures
	cfg.InitMaps()

	return cfg, nil
}

// loadFromEnv loads configuration from environment variables.
func (c *Config) loadFromEnv() {
	if tmdbKey := os.Getenv("TMDB_API_KEY"); tmdbKey != "" {
		c.TMDBAPIKey = tmdbKey
	}

	if adKey := os.Getenv("API_KEY_ALLDEBRID"); adKey != "" {
		c.APIKeyAllDebrid = adKey
	}
}

// loadFromFile loads configuration from a JSON file.
func (c *Config) loadFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c)
}

// Validate checks if the configuration is valid.
// Sets default values for missing optional fields.
func (c *Config) Validate() error {
	// TMDB_API_KEY is optional, can be configured via web interface

	// Set default resolutions if none specified
	if len(c.ResToShow) == 0 {
		c.ResToShow = constants.DefaultResolutions
	}

	return nil
}

// InitMaps initializes internal lookup maps for performance.
// This method is idempotent and thread-safe.
func (c *Config) InitMaps() {
	c.mapsOnce.Do(func() {
		// Initialize resolution map
		c.resMap = make(map[string]bool, len(c.ResToShow))
		for _, res := range c.ResToShow {
			c.resMap[strings.ToLower(res)] = true
		}

		// Initialize language map
		c.langMap = make(map[string]bool, len(c.LangToShow))
		for _, lang := range c.LangToShow {
			c.langMap[strings.ToLower(lang)] = true
		}
	})
}


// CreateFromUserData creates a config from user-provided data and existing config.
// User data takes precedence over base config values.
func CreateFromUserData(userConfig map[string]interface{}, baseConfig *Config) *Config {
	cfg := &Config{}

	// Copy from base config if available
	if baseConfig != nil {
		cfg.copyFrom(baseConfig)
	}

	// Apply user overrides
	cfg.applyUserConfig(userConfig)

	// Validate and initialize
	cfg.Validate()
	cfg.InitMaps()

	return cfg
}

// copyFrom copies all fields from another config.
func (c *Config) copyFrom(src *Config) {
	c.TMDBAPIKey = src.TMDBAPIKey
	c.APIKeyAllDebrid = src.APIKeyAllDebrid
	c.ResToShow = append([]string{}, src.ResToShow...)
	c.LangToShow = append([]string{}, src.LangToShow...)
	c.DatabasePath = src.DatabasePath
	c.CacheSize = src.CacheSize
	c.CacheTTL = src.CacheTTL
}

// applyUserConfig applies user-provided configuration overrides.
func (c *Config) applyUserConfig(userConfig map[string]interface{}) {
	// Handle resolution list
	if val, ok := userConfig["RES_TO_SHOW"]; ok {
		if arr, ok := val.([]interface{}); ok {
			c.ResToShow = convertToStringSlice(arr)
		}
	}

	// Handle language list
	if val, ok := userConfig["LANG_TO_SHOW"]; ok {
		if arr, ok := val.([]interface{}); ok {
			c.LangToShow = convertToStringSlice(arr)
		}
	}

	// Handle API keys
	if val, ok := userConfig["TMDB_API_KEY"]; ok {
		if str, ok := val.(string); ok {
			c.TMDBAPIKey = str
		}
	}

	if val, ok := userConfig["API_KEY_ALLDEBRID"]; ok {
		if str, ok := val.(string); ok {
			c.APIKeyAllDebrid = str
		}
	}
}

// convertToStringSlice converts interface slice to string slice.
func convertToStringSlice(arr []interface{}) []string {
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

// getEnvOrDefault returns environment variable value or default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
