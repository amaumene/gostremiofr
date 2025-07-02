package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	TMDBAPIKey       string   `json:"TMDB_API_KEY"`
	APIKeyAllDebrid  string   `json:"API_KEY_ALLEDBRID"`
	FilesToShow      int      `json:"FILES_TO_SHOW"`
	ResToShow        []string `json:"RES_TO_SHOW"`
	LangToShow       []string `json:"LANG_TO_SHOW"`
	CodecsToShow     []string `json:"CODECS_TO_SHOW"`
	SharewoodPasskey string   `json:"SHAREWOOD_PASSKEY"`
	
	DatabasePath string        `json:"DATABASE_PATH"`
	CacheSize    int           `json:"CACHE_SIZE"`
	CacheTTL     time.Duration `json:"CACHE_TTL"`
	
	resMap    map[string]bool
	langMap   map[string]bool
	codecMap  map[string]bool
	mapsOnce  sync.Once
}

func Load() (*Config, error) {
	cfg := &Config{
		FilesToShow:  10,
		CacheSize:    1000,
		CacheTTL:     1 * time.Hour,
		DatabasePath: getEnvOrDefault("DATABASE_PATH", "./cache.db"),
	}
	
	if tmdbKey := os.Getenv("TMDB_API_KEY"); tmdbKey != "" {
		cfg.TMDBAPIKey = tmdbKey
	}
	
	if adKey := os.Getenv("API_KEY_ALLDEBRID"); adKey != "" {
		cfg.APIKeyAllDebrid = adKey
	}
	
	if swKey := os.Getenv("SHAREWOOD_PASSKEY"); swKey != "" {
		cfg.SharewoodPasskey = swKey
	}
	
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "config.json"
	}
	
	if data, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}
	
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	cfg.initMaps()
	
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.TMDBAPIKey == "" {
		return errors.New("TMDB_API_KEY is required")
	}
	
	if c.FilesToShow <= 0 {
		c.FilesToShow = 10
	}
	
	if len(c.ResToShow) == 0 {
		c.ResToShow = []string{"4k", "1080p", "720p"}
	}
	
	if len(c.LangToShow) == 0 {
		c.LangToShow = []string{"multi", "french", "english"}
	}
	
	if len(c.CodecsToShow) == 0 {
		c.CodecsToShow = []string{"h264", "h265", "x264", "x265", "hevc", "av1"}
	}
	
	return nil
}

func (c *Config) initMaps() {
	c.mapsOnce.Do(func() {
		c.resMap = make(map[string]bool)
		for _, res := range c.ResToShow {
			c.resMap[strings.ToLower(res)] = true
		}
		
		c.langMap = make(map[string]bool)
		for _, lang := range c.LangToShow {
			c.langMap[strings.ToLower(lang)] = true
		}
		
		c.codecMap = make(map[string]bool)
		for _, codec := range c.CodecsToShow {
			c.codecMap[strings.ToLower(codec)] = true
		}
	})
}

func (c *Config) IsResolutionAllowed(res string) bool {
	return c.resMap[strings.ToLower(res)]
}

func (c *Config) IsLanguageAllowed(lang string) bool {
	return c.langMap[strings.ToLower(lang)]
}

func (c *Config) IsCodecAllowed(codec string) bool {
	return c.codecMap[strings.ToLower(codec)]
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}