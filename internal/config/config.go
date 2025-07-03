package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	TMDBAPIKey       string   `json:"TMDB_API_KEY"`
	APIKeyAllDebrid  string   `json:"API_KEY_ALLDEBRID"`
	FilesToShow      int      `json:"FILES_TO_SHOW"`
	ResToShow        []string `json:"RES_TO_SHOW"`
	LangToShow       []string `json:"LANG_TO_SHOW"`
	
	DatabasePath string        `json:"DATABASE_PATH"`
	CacheSize    int           `json:"CACHE_SIZE"`
	CacheTTL     time.Duration `json:"CACHE_TTL"`
	
	resMap    map[string]bool
	langMap   map[string]bool
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
	
	cfg.InitMaps()
	
	return cfg, nil
}

func (c *Config) Validate() error {
	// TMDB_API_KEY is now optional, configured via web interface
	
	if c.FilesToShow <= 0 {
		c.FilesToShow = 10
	}
	
	if len(c.ResToShow) == 0 {
		c.ResToShow = []string{"4k", "1080p", "720p"}
	}
	
	if len(c.LangToShow) == 0 {
		c.LangToShow = []string{"multi", "french", "english"}
	}
	
	
	return nil
}

func (c *Config) InitMaps() {
	c.mapsOnce.Do(func() {
		c.resMap = make(map[string]bool)
		for _, res := range c.ResToShow {
			c.resMap[strings.ToLower(res)] = true
		}
		
		c.langMap = make(map[string]bool)
		for _, lang := range c.LangToShow {
			c.langMap[strings.ToLower(lang)] = true
		}
	})
}

func (c *Config) IsResolutionAllowed(res string) bool {
	return c.resMap[strings.ToLower(res)]
}

func (c *Config) IsLanguageAllowed(lang string) bool {
	return c.langMap[strings.ToLower(lang)]
}

func (c *Config) GetResolutionPriority(res string) int {
	resLower := strings.ToLower(res)
	for i, allowedRes := range c.ResToShow {
		if strings.ToLower(allowedRes) == resLower {
			// Higher index = lower priority (reverse order for sorting)
			return len(c.ResToShow) - i
		}
	}
	return 0 // Not in list = lowest priority
}


// CreateFromUserData creates a config from user-provided data and existing config
func CreateFromUserData(userConfig map[string]interface{}, baseConfig *Config) *Config {
	cfg := &Config{
		FilesToShow: 6, // Default from user config
	}
	
	// Copy from existing config if available
	if baseConfig != nil {
		*cfg = *baseConfig
	}
	
	// Override with user configuration
	if val, ok := userConfig["FILES_TO_SHOW"]; ok {
		if floatVal, ok := val.(float64); ok {
			cfg.FilesToShow = int(floatVal)
		}
	}
	
	if val, ok := userConfig["RES_TO_SHOW"]; ok {
		if arr, ok := val.([]interface{}); ok {
			cfg.ResToShow = make([]string, len(arr))
			for i, v := range arr {
				if str, ok := v.(string); ok {
					cfg.ResToShow[i] = str
				}
			}
		}
	}
	
	if val, ok := userConfig["LANG_TO_SHOW"]; ok {
		if arr, ok := val.([]interface{}); ok {
			cfg.LangToShow = make([]string, len(arr))
			for i, v := range arr {
				if str, ok := v.(string); ok {
					cfg.LangToShow[i] = str
				}
			}
		}
	}
	
	if val, ok := userConfig["TMDB_API_KEY"]; ok {
		if str, ok := val.(string); ok {
			cfg.TMDBAPIKey = str
		}
	}
	
	if val, ok := userConfig["API_KEY_ALLDEBRID"]; ok {
		if str, ok := val.(string); ok {
			cfg.APIKeyAllDebrid = str
		}
	}
	
	// Validate and initialize the config
	cfg.Validate()
	cfg.InitMaps()
	
	return cfg
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}