package main

import (
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GzipResponseWriter wraps gin.ResponseWriter to provide gzip compression
type GzipResponseWriter struct {
	gin.ResponseWriter
	gzipWriter *gzip.Writer
}

func (w *GzipResponseWriter) Write(data []byte) (int, error) {
	return w.gzipWriter.Write(data)
}

func (w *GzipResponseWriter) WriteString(s string) (int, error) {
	return w.gzipWriter.Write([]byte(s))
}

// GzipMiddleware provides gzip compression for responses
func GzipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")

		gzipWriter := gzip.NewWriter(c.Writer)
		defer func() {
			if err := gzipWriter.Close(); err != nil {
				Logger.Errorf("failed to close gzip writer: %v", err)
			}
		}()

		c.Writer = &GzipResponseWriter{
			ResponseWriter: c.Writer,
			gzipWriter:     gzipWriter,
		}

		c.Next()
	}
}

func main() {
	// Initialize logger
	InitializeLogger()

	// Initialize database
	InitializeDatabase()

	// Create Gin router
	r := gin.Default()

	// Add gzip compression middleware
	r.Use(GzipMiddleware())

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	})
	
	// Start cache cleanup routine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			tmdbMemoryCache.CleanExpired()
		}
	}()

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
	Logger.Infof("starting HTTP server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}