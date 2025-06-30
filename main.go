package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize logger
	InitializeLogger()

	// Initialize database
	InitializeDatabase()

	// Create Gin router
	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	})

	// Routes
	setupConfigRoutes(r)
	setupManifestRoutes(r)
	setupStreamRoutes(r)

	// Get port from environment or default to 5000
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	// Start HTTP server
	Logger.Info("✅ HTTP server running on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}