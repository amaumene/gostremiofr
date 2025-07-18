package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/gin-gonic/gin"
)

func (h *Handler) handleManifest(c *gin.Context) {
	manifest := models.Manifest{
		ID:          constants.AddonID,
		Version:     constants.AddonVersion,
		Name:        constants.AddonName,
		Description: constants.AddonDescription,
		Types:       []string{"movie", "series"},
		Resources:   []string{"catalog", "meta", "stream"},
		Catalogs:    h.getDefaultCatalogs(),
		BehaviorHints: models.BehaviorHints{
			Configurable: true,
		},
		IDPrefixes: []string{"tt", "tmdb:"},
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
		ID:          constants.AddonID,
		Version:     constants.AddonVersion,
		Name:        constants.AddonName,
		Description: constants.AddonDescription,
		Types:       []string{"movie", "series"},
		Resources:   []string{"catalog", "meta", "stream"},
		Catalogs:    h.getDefaultCatalogs(),
		BehaviorHints: models.BehaviorHints{
			Configurable: true,
		},
		IDPrefixes: []string{"tt", "tmdb:"},
	}

	c.JSON(http.StatusOK, manifest)
}

func (h *Handler) getDefaultCatalogs() []models.Catalog {
	return []models.Catalog{
		// Movie catalogs
		{
			Type: "movie",
			ID:   "popular",
			Name: "Films populaires",
			Extra: []models.ExtraField{
				{
					Name:    "genre",
					Options: constants.TMDBMovieGenres,
				},
				{Name: "skip"},
			},
		},
		{
			Type: "movie",
			ID:   "trending",
			Name: "Films tendances",
			Extra: []models.ExtraField{
				{Name: "skip"},
			},
		},
		{
			Type: "movie",
			ID:   "search",
			Name: "Rechercher des films",
			Extra: []models.ExtraField{
				{Name: "search", IsRequired: true},
				{Name: "skip"},
			},
		},
		// Series catalogs
		{
			Type: "series",
			ID:   "popular",
			Name: "Séries populaires",
			Extra: []models.ExtraField{
				{
					Name:    "genre",
					Options: constants.TMDBTVGenres,
				},
				{Name: "skip"},
			},
		},
		{
			Type: "series",
			ID:   "trending",
			Name: "Séries tendances",
			Extra: []models.ExtraField{
				{Name: "skip"},
			},
		},
		{
			Type: "series",
			ID:   "search",
			Name: "Rechercher des séries",
			Extra: []models.ExtraField{
				{Name: "search", IsRequired: true},
				{Name: "skip"},
			},
		},
	}
}
