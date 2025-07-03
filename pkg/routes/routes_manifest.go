package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/pkg/helpers"
)

type Manifest struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Types       []string `json:"types"`
	Resources   []string `json:"resources"`
	Catalogs    []string `json:"catalogs"`
	BehaviorHints BehaviorHints `json:"behaviorHints"`
}

type BehaviorHints struct {
	Configurable bool `json:"configurable"`
}

func SetupManifestRoutes(r *gin.Engine) {
	r.GET("/:variables/manifest.json", serveManifest)
}

func serveManifest(c *gin.Context) {
	_, err := helpers.GetConfig(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	manifest := Manifest{
		ID:          "ygg.stremio.ad",
		Version:     "0.0.4",
		Name:        "Ygg + AD",
		Description: "An addon to access YggTorrent torrents cached on AllDebrid (thanks to Ygg API).",
		Types:       []string{"movie", "series"},
		Resources:   []string{"stream"},
		Catalogs:    []string{},
		BehaviorHints: BehaviorHints{
			Configurable: true,
		},
	}

	c.JSON(http.StatusOK, manifest)
}