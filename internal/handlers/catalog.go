package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/models"
	"github.com/amaumene/gostremiofr/internal/services"
)

func (h *Handler) handleCatalog(c *gin.Context) {
	configuration := c.Param("configuration")
	catalogType := c.Param("type")
	catalogID := c.Param("id")
	
	// Extract user configuration
	var userConfig map[string]interface{}
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	
	// Extract TMDB API key from user configuration
	tmdbAPIKey := ""
	if val, ok := userConfig["TMDB_API_KEY"]; ok {
		if str, ok := val.(string); ok {
			tmdbAPIKey = str
		}
	}
	// Fall back to config file if not in user configuration
	if tmdbAPIKey == "" && h.config != nil {
		tmdbAPIKey = h.config.TMDBAPIKey
	}
	
	// Update TMDB service with the API key if available
	if tmdbAPIKey != "" && h.services.TMDB != nil {
		if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
			tmdb.SetAPIKey(tmdbAPIKey)
		}
	}
	
	// Parse query parameters
	skip := c.DefaultQuery("skip", "0")
	genre := c.Query("genre")
	search := c.Query("search")
	
	skipInt, err := strconv.Atoi(skip)
	if err != nil {
		skipInt = 0
	}
	
	// Calculate page from skip (assuming 20 items per page)
	page := (skipInt / 20) + 1
	
	h.services.Logger.Infof("[CatalogHandler] processing catalog request - type: %s, id: %s, page: %d", catalogType, catalogID, page)
	
	var metas []models.Meta
	
	// Handle search catalog
	if catalogID == "search" && search != "" {
		metas, err = h.services.TMDB.SearchMulti(search, page)
		if err != nil {
			h.services.Logger.Errorf("[CatalogHandler] search failed: %v", err)
			c.JSON(http.StatusOK, models.CatalogResponse{Metas: []models.Meta{}})
			return
		}
	} else {
		// Handle regular catalogs
		switch catalogID {
		case "popular":
			if catalogType == "movie" {
				metas, err = h.services.TMDB.GetPopularMovies(page, genre)
			} else {
				metas, err = h.services.TMDB.GetPopularSeries(page, genre)
			}
			
		case "trending":
			metas, err = h.services.TMDB.GetTrending(catalogType, "week", page)
			
		case "top_rated":
			// For now, use popular as placeholder
			if catalogType == "movie" {
				metas, err = h.services.TMDB.GetPopularMovies(page, genre)
			} else {
				metas, err = h.services.TMDB.GetPopularSeries(page, genre)
			}
			
		default:
			h.services.Logger.Warnf("[CatalogHandler] unknown catalog ID: %s", catalogID)
			c.JSON(http.StatusOK, models.CatalogResponse{Metas: []models.Meta{}})
			return
		}
		
		if err != nil {
			h.services.Logger.Errorf("[CatalogHandler] failed to fetch catalog: %v", err)
			c.JSON(http.StatusOK, models.CatalogResponse{Metas: []models.Meta{}})
			return
		}
	}
	
	// Filter by type if needed (search returns mixed results)
	if catalogID == "search" && catalogType != "" {
		filtered := make([]models.Meta, 0)
		for _, meta := range metas {
			if meta.Type == catalogType {
				filtered = append(filtered, meta)
			}
		}
		metas = filtered
	}
	
	h.services.Logger.Infof("[CatalogHandler] returning %d items for %s/%s", len(metas), catalogType, catalogID)
	
	c.JSON(http.StatusOK, models.CatalogResponse{Metas: metas})
}

func (h *Handler) handleMeta(c *gin.Context) {
	configuration := c.Param("configuration")
	metaType := c.Param("type")
	metaID := c.Param("id")
	
	// Extract user configuration
	var userConfig map[string]interface{}
	if data, err := base64.StdEncoding.DecodeString(configuration); err == nil {
		json.Unmarshal(data, &userConfig)
	}
	
	// Extract TMDB API key from user configuration
	tmdbAPIKey := ""
	if val, ok := userConfig["TMDB_API_KEY"]; ok {
		if str, ok := val.(string); ok {
			tmdbAPIKey = str
		}
	}
	// Fall back to config file if not in user configuration
	if tmdbAPIKey == "" && h.config != nil {
		tmdbAPIKey = h.config.TMDBAPIKey
	}
	
	// Update TMDB service with the API key if available
	if tmdbAPIKey != "" && h.services.TMDB != nil {
		if tmdb, ok := h.services.TMDB.(*services.TMDB); ok {
			tmdb.SetAPIKey(tmdbAPIKey)
		}
	}
	
	h.services.Logger.Infof("[MetaHandler] fetching metadata - type: %s, id: %s", metaType, metaID)
	
	// Handle TMDB IDs (format: tmdb:12345)
	if strings.HasPrefix(metaID, "tmdb:") {
		tmdbID := strings.TrimPrefix(metaID, "tmdb:")
		meta, err := h.services.TMDB.GetMetadata(metaType, tmdbID)
		if err != nil {
			h.services.Logger.Errorf("[MetaHandler] failed to fetch metadata: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Meta not found"})
			return
		}
		
		c.JSON(http.StatusOK, models.MetaResponse{Meta: *meta})
		return
	}
	
	// For IMDB IDs, we need to find the TMDB ID first
	if strings.HasPrefix(metaID, "tt") {
		// Get basic info to find TMDB ID
		mediaType, title, _, _, err := h.services.TMDB.GetIMDBInfo(metaID)
		if err != nil {
			h.services.Logger.Errorf("[MetaHandler] failed to fetch IMDB info: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Meta not found"})
			return
		}
		
		// For now, return basic meta with IMDB ID
		// In a full implementation, we'd need to map IMDB to TMDB ID
		meta := models.Meta{
			ID:   metaID,
			Type: mediaType,
			Name: title,
		}
		
		c.JSON(http.StatusOK, models.MetaResponse{Meta: meta})
		return
	}
	
	c.JSON(http.StatusNotFound, gin.H{"error": "Invalid meta ID format"})
}