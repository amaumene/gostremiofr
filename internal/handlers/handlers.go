package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/gostremiofr/gostremiofr/internal/config"
	"github.com/gostremiofr/gostremiofr/internal/services"
)

type Handler struct {
	services *services.Container
	config   *config.Config
}

func New(services *services.Container, config *config.Config) *Handler {
	return &Handler{
		services: services,
		config:   config,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/", h.handleHome)
	r.GET("/configure", h.handleConfigure)
	r.GET("/:configuration/configure", h.handleConfigureWithParams)
	r.GET("/manifest.json", h.handleManifest)
	r.GET("/:configuration/manifest.json", h.handleManifestWithConfig)
	r.GET("/:configuration/stream/:type/:id.json", h.handleStream)
}

func (h *Handler) handleHome(c *gin.Context) {
	c.String(200, "Welcome to GoStremio addon!")
}

func (h *Handler) handleConfigure(c *gin.Context) {
	c.Redirect(302, "/configure")
}

func (h *Handler) handleConfigureWithParams(c *gin.Context) {
	configuration := c.Param("configuration")
	c.JSON(200, gin.H{
		"configuration": configuration,
		"message": "Configuration received",
	})
}