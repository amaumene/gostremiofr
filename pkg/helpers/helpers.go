package helpers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

// Config represents the configuration structure
type Config struct {
	TMDBAPIKey       string   `json:"TMDB_API_KEY"`
	APIKeyAlldebrid  string   `json:"API_KEY_ALLDEBRID"`
	ResolutionToShow []string `json:"RESOLUTION_TO_SHOW"`
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

