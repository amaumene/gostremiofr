package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/handlers"
)

func SetupStreamRoutes(r *gin.Engine, handler *handlers.Handler) {
	// Map the route format to match the internal handler's expectations
	r.GET("/:variables/stream/:type/:id", func(c *gin.Context) {
		// Get the parameters
		variables := c.Param("variables")
		streamType := c.Param("type")
		id := c.Param("id")
		
		// Update the URL path to match the expected format with .json extension
		c.Request.URL.Path = "/" + variables + "/stream/" + streamType + "/" + id + ".json"
		
		// Set up the parameters for the internal handler
		c.Params = gin.Params{
			{Key: "configuration", Value: variables},
			{Key: "type", Value: streamType},
			{Key: "id", Value: id},
		}
		
		// Delegate to the internal handler
		handler.HandleStream(c)
	})
}