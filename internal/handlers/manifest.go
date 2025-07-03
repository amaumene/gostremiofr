package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/models"
)

func (h *Handler) handleManifest(c *gin.Context) {
	manifest := models.Manifest{
		ID:          "org.gostremiofr",
		Version:     "1.0.0",
		Name:        "GoStremio FR",
		Description: "French torrent streaming addon",
		Types:       []string{"movie", "series"},
		Resources:   []string{"stream"},
		Catalogs:    []interface{}{},
		BehaviorHints: models.BehaviorHints{
			Configurable: true,
		},
	}
	
	c.JSON(http.StatusOK, manifest)
}

func (h *Handler) handleManifestWithConfig(c *gin.Context) {
	configuration := c.Param("configuration")
	
	var config map[string]string
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &config)
	}
	
	manifest := models.Manifest{
		ID:          "org.gostremiofr.configured",
		Version:     "1.0.0",
		Name:        "GoStremio FR (Configured)",
		Description: "French torrent streaming addon with your configuration",
		Types:       []string{"movie", "series"},
		Resources:   []string{"stream"},
		Catalogs:    []interface{}{},
		BehaviorHints: models.BehaviorHints{
			Configurable: true,
		},
	}
	
	c.JSON(http.StatusOK, manifest)
}