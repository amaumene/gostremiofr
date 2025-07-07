package helpers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

// Config represents the configuration structure
type Config struct {
	TMDBAPIKey        string   `json:"TMDB_API_KEY"`
	APIKeyAlldebrid   string   `json:"API_KEY_ALLDEBRID"`
	LangToShow        []string `json:"LANG_TO_SHOW"`
	SourcePriority    []string `json:"SOURCE_PRIORITY"`
	ResolutionToShow  []string `json:"RESOLUTION_TO_SHOW"`
	TimeToKeepTorrent int      `json:"TIME_TO_KEEP_TORRENT"`
}

func GetConfig(c *gin.Context) (*Config, error) {
	variables := c.Param("variables")
	if variables == "" {
		return nil, fmt.Errorf("configuration missing in URL")
	}

	decoded, err := base64.StdEncoding.DecodeString(variables)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration in URL: %v", err)
	}

	var config Config
	if err := json.Unmarshal(decoded, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %v", err)
	}

	return &config, nil
}

// IsResolutionAllowed checks if a resolution is allowed
func (c *Config) IsResolutionAllowed(resolution string) bool {
	if len(c.ResolutionToShow) == 0 {
		return true
	}
	for _, allowed := range c.ResolutionToShow {
		if allowed == resolution {
			return true
		}
	}
	return false
}


// IsLanguageAllowed checks if a language is allowed
func (c *Config) IsLanguageAllowed(language string) bool {
	if len(c.LangToShow) == 0 {
		return true
	}
	for _, allowed := range c.LangToShow {
		if allowed == language {
			return true
		}
	}
	return false
}