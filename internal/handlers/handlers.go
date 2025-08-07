// Package handlers implements HTTP request handlers for the Stremio addon API.
package handlers

import (
	"strings"

	"github.com/amaumene/gostremiofr/internal/config"
	"github.com/amaumene/gostremiofr/internal/services"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for the Stremio addon.
type Handler struct {
	services *services.Container
	config   *config.Config
}

// New creates a new Handler with the provided services and configuration.
func New(services *services.Container, config *config.Config) *Handler {
	return &Handler{
		services: services,
		config:   config,
	}
}

// RegisterRoutes registers all HTTP routes for the Stremio addon.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Home route
	r.GET("/", h.handleHome)

	// Configuration routes
	r.GET("/config", h.handleConfig)
	r.GET("/configure", h.handleConfig) // Alias for compatibility
	r.GET("/:configuration/configure", h.handleConfigWithParams)

	// Manifest routes
	r.GET("/manifest.json", h.handleManifest)
	r.GET("/:configuration/manifest.json", h.handleManifestWithConfig)

	// Catalog routes - handle both with and without .json in the handler
	r.GET("/:configuration/catalog/:type/:id", h.handleCatalogWrapper)
	r.GET("/:configuration/catalog/:type/:id/*extra", h.handleCatalogWrapper)

	// Meta routes - handle both with and without .json in the handler
	r.GET("/:configuration/meta/:type/:id", h.handleMetaWrapper)

	// Stream routes - handle both with and without .json in the handler
	r.GET("/:configuration/stream/:type/:id", h.handleStreamWrapper)
}

// HandleStream is an exported wrapper for the internal handleStream method.
func (h *Handler) HandleStream(c *gin.Context) {
	h.handleStream(c)
}

func (h *Handler) handleHome(c *gin.Context) {
	c.String(200, "Welcome to GoStremioFR! Visit /config to configure the addon.")
}

// Wrapper functions to handle .json extension
func (h *Handler) handleCatalogWrapper(c *gin.Context) {
	// Strip .json extension from ID if present
	stripJSONExtension(c, "id")

	// Handle extra path parameters (e.g., /catalog/movie/search/search=term.json)
	extra := c.Param("extra")
	if extra != "" {
		// Remove leading slash
		extra = strings.TrimPrefix(extra, "/")
		// Remove .json extension if present
		extra = strings.TrimSuffix(extra, ".json")

		// Parse path-based parameters (e.g., "search=term&genre=28")
		params := strings.Split(extra, "&")
		for _, param := range params {
			parts := strings.SplitN(param, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				// Add as query parameter so the handler can access it via c.Query()
				c.Request.URL.RawQuery = c.Request.URL.RawQuery + "&" + key + "=" + value
			}
		}

		// Clean up the query string
		if strings.HasPrefix(c.Request.URL.RawQuery, "&") {
			c.Request.URL.RawQuery = strings.TrimPrefix(c.Request.URL.RawQuery, "&")
		}
	}

	h.handleCatalog(c)
}

func (h *Handler) handleMetaWrapper(c *gin.Context) {
	// Strip .json extension from ID if present
	stripJSONExtension(c, "id")
	h.handleMeta(c)
}

func (h *Handler) handleStreamWrapper(c *gin.Context) {
	// Strip .json extension from ID if present
	stripJSONExtension(c, "id")
	h.handleStream(c)
}
