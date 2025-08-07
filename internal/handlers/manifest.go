package handlers

import (
	"net/http"

	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/gin-gonic/gin"
)

func (h *Handler) handleManifest(c *gin.Context) {
	manifest := h.createManifest()
	c.JSON(http.StatusOK, manifest)
}

func (h *Handler) handleManifestWithConfig(c *gin.Context) {
	manifest := h.createManifest()
	c.JSON(http.StatusOK, manifest)
}

func (h *Handler) createManifest() models.Manifest {
	return models.Manifest{
		ID:          constants.AddonID,
		Version:     constants.AddonVersion,
		Name:        constants.AddonName,
		Description: constants.AddonDescription,
		Types:       []string{"movie", "series"},
		Resources:   []string{"catalog", "meta", "stream"},
		Catalogs:    h.getDefaultCatalogs(),
		BehaviorHints: models.BehaviorHints{
			Configurable:    true,
			ConfigurationRequired: true,  // Require configuration since we need API keys
		},
		IDPrefixes: []string{"tt", "tmdb:"},  // Only accept IMDB (tt) and TMDB IDs
		Background: constants.AddonBackground,
		Logo:       constants.AddonLogo,
	}
}

func (h *Handler) getDefaultCatalogs() []models.Catalog {
	var catalogs []models.Catalog
	catalogs = append(catalogs, h.getMovieCatalogs()...)
	catalogs = append(catalogs, h.getSeriesCatalogs()...)
	return catalogs
}

func (h *Handler) getMovieCatalogs() []models.Catalog {
	return []models.Catalog{
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
	}
}

func (h *Handler) getSeriesCatalogs() []models.Catalog {
	return []models.Catalog{
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
