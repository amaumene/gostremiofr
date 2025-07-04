package main

import (
	"compress/gzip"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/pkg/ssl"
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
				Logger.Errorf("[App] failed to close gzip writer: %v", err)
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

	// Initialize services
	InitializeServices()

	// Set Gin mode based on environment
	if os.Getenv("GIN_MODE") == "" {
		// Default to release mode if not specified
		gin.SetMode(gin.ReleaseMode)
	}

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

	// Register all routes through the handler
	handler.RegisterRoutes(r)

	// Get port from environment or default
	port := os.Getenv("PORT")
	if port == "" {
		port = constants.DefaultPort
	}

	// Check if SSL is enabled
	useSSL := strings.ToLower(os.Getenv("USE_SSL")) == "true"
	
	if useSSL {
		// Setup SSL certificate from local-ip.sh
		sslManager := ssl.NewLocalIPCertificate(Logger)
		if err := sslManager.Setup(); err != nil {
			Logger.Errorf("[App] failed to setup SSL: %v", err)
			Logger.Infof("[App] falling back to HTTP")
			useSSL = false
		} else {
			// Get certificate paths
			certPath, keyPath := sslManager.GetCertificatePaths()
			hostname := sslManager.GetHostname()
			
			Logger.Infof("[App] starting HTTPS server on port %s", port)
			Logger.Infof("[App] accessible at https://%s:%s", hostname, port)
			
			// Start HTTPS server
			log.Fatal(http.ListenAndServeTLS(":"+port, certPath, keyPath, r))
		}
	}
	
	if !useSSL {
		// Start HTTP server
		Logger.Infof("[App] starting HTTP server on port %s", port)
		log.Fatal(http.ListenAndServe(":"+port, r))
	}
}