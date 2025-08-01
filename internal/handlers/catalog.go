package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/internal/services"
	"github.com/gin-gonic/gin"
)

func (h *Handler) extractTMDBAPIKey(configuration string) string {
	userConfig := decodeUserConfig(configuration)
	return h.extractTMDBKey(userConfig)
}

func (h *Handler) updateTMDBService(tmdbAPIKey string) {
	if tmdbAPIKey != "" && h.services.TMDB != nil {
		if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
			tmdb.SetAPIKey(tmdbAPIKey)
		}
	}
}

func (h *Handler) handleCatalog(c *gin.Context) {
	configuration := c.Param("configuration")
	catalogType := c.Param("type")
	catalogID := c.Param("id")
	genre := c.Query("genre")
	search := c.Query("search")

	skipInt, _ := strconv.Atoi(c.DefaultQuery("skip", "0"))
	page := (skipInt / 20) + 1

	tmdbAPIKey := h.extractTMDBAPIKey(configuration)
	h.updateTMDBService(tmdbAPIKey)

	h.services.Logger.Debugf("catalog request: %s/%s (page %d)", catalogType, catalogID, page)

	metas, err := h.fetchCatalogMetas(catalogType, catalogID, search, genre, page)
	if err != nil {
		h.services.Logger.Errorf("catalog fetch failed: %v", err)
		c.JSON(http.StatusOK, models.CatalogResponse{Metas: []models.Meta{}})
		return
	}

	metas = h.filterMetasByType(metas, catalogID, catalogType)
	h.services.Logger.Debugf("returning %d items for %s/%s", len(metas), catalogType, catalogID)
	c.JSON(http.StatusOK, models.CatalogResponse{Metas: metas})
}

func (h *Handler) fetchCatalogMetas(catalogType, catalogID, search, genre string, page int) ([]models.Meta, error) {
	if catalogID == "search" && search != "" {
		return h.services.TMDB.SearchMulti(search, page)
	}

	switch catalogID {
	case "popular":
		if catalogType == "movie" {
			return h.services.TMDB.GetPopularMovies(page, genre)
		}
		return h.services.TMDB.GetPopularSeries(page, genre)

	case "trending":
		return h.services.TMDB.GetTrending(catalogType, "week", page)

	case "top_rated":
		if catalogType == "movie" {
			return h.services.TMDB.GetPopularMovies(page, genre)
		}
		return h.services.TMDB.GetPopularSeries(page, genre)

	default:
		return []models.Meta{}, nil
	}
}

func (h *Handler) filterMetasByType(metas []models.Meta, catalogID, catalogType string) []models.Meta {
	if catalogID != "search" || catalogType == "" {
		return metas
	}

	filtered := make([]models.Meta, 0)
	for _, meta := range metas {
		if meta.Type == catalogType {
			filtered = append(filtered, meta)
		}
	}
	return filtered
}

func (h *Handler) fetchTMDBMeta(metaType, tmdbID string) (*models.Meta, error) {
	return h.services.TMDB.GetMetadata(metaType, tmdbID)
}

func (h *Handler) fetchIMDBMeta(metaID string) (*models.Meta, error) {
	mediaType, title, _, _, _, err := h.services.TMDB.GetIMDBInfo(metaID)
	if err != nil {
		return nil, err
	}

	return &models.Meta{
		ID:   metaID,
		Type: mediaType,
		Name: title,
	}, nil
}

func (h *Handler) handleMeta(c *gin.Context) {
	configuration := c.Param("configuration")
	metaType := c.Param("type")
	metaID := c.Param("id")

	tmdbAPIKey := h.extractTMDBAPIKey(configuration)
	h.updateTMDBService(tmdbAPIKey)

	h.services.Logger.Debugf("fetching metadata: %s/%s", metaType, metaID)

	meta, err := h.fetchMeta(metaType, metaID)
	if err != nil {
		h.handleMetaError(c, err)
		return
	}

	c.JSON(http.StatusOK, models.MetaResponse{Meta: *meta})
}

func (h *Handler) fetchMeta(metaType, metaID string) (*models.Meta, error) {
	if strings.HasPrefix(metaID, "tmdb:") {
		tmdbID := strings.TrimPrefix(metaID, "tmdb:")
		return h.fetchTMDBMeta(metaType, tmdbID)
	} else if strings.HasPrefix(metaID, "tt") {
		return h.fetchIMDBMeta(metaID)
	}
	return nil, fmt.Errorf("Invalid meta ID format")
}

func (h *Handler) handleMetaError(c *gin.Context, err error) {
	if err.Error() == "Invalid meta ID format" {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	} else {
		h.services.Logger.Errorf("metadata fetch failed: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta not found"})
	}
}
