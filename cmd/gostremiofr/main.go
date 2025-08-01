// Package main is the entry point for the GoStremioFR application.
package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/pkg/ssl"
	"github.com/gin-gonic/gin"
)

// gzipResponseWriter wraps gin.ResponseWriter to provide gzip compression
type gzipResponseWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

// Write implements io.Writer interface for gzip compression
func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	return w.writer.Write(data)
}

// WriteString writes string data with gzip compression
func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	return w.writer.Write([]byte(s))
}

// gzipMiddleware provides gzip compression for HTTP responses
func gzipMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !acceptsGzip(c) {
			c.Next()
			return
		}

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")

		gz := gzip.NewWriter(c.Writer)
		defer func() {
			if err := gz.Close(); err != nil {
				logger.Errorf("failed to close gzip writer: %v", err)
			}
		}()

		c.Writer = &gzipResponseWriter{
			ResponseWriter: c.Writer,
			writer:         gz,
		}

		c.Next()
	}
}

// acceptsGzip checks if client accepts gzip encoding
func acceptsGzip(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept-Encoding"), "gzip")
}

// setupRouter creates and configures the Gin router
func setupRouter() *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(gzipMiddleware())
	r.Use(corsMiddleware())

	return r
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	}
}

// startBackgroundServices initializes background tasks
func startBackgroundServices(ctx context.Context) {
	tmdbCache.StartCleanup(ctx)

	if container == nil || container.Cleanup == nil {
		return
	}

	configureCleanupRetention()
	if err := container.Cleanup.Start(ctx); err != nil {
		logger.Errorf("failed to start cleanup service: %v", err)
	}
}

// configureCleanupRetention sets retention period from environment
func configureCleanupRetention() {
	hours := os.Getenv("ALLDEBRID_RETENTION_HOURS")
	if hours == "" {
		return
	}

	duration, err := time.ParseDuration(hours + "h")
	if err != nil {
		logger.Warnf("invalid retention hours %q: %v", hours, err)
		return
	}

	container.Cleanup.SetRetentionPeriod(duration)
	logger.Infof("AllDebrid retention period set to %v", duration)
}

// getServerPort returns the configured server port
func getServerPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return constants.DefaultPort
}

// startHTTPSServer starts the server with TLS
func startHTTPSServer(r *gin.Engine, port string) error {
	sslManager := ssl.NewLocalIPCertificate(logger)
	if err := sslManager.Setup(); err != nil {
		return fmt.Errorf("SSL setup failed: %w", err)
	}

	cert, key := sslManager.GetCertificatePaths()
	host := sslManager.GetHostname()

	logger.Infof("starting HTTPS server on port %s", port)
	logger.Infof("accessible at https://%s:%s", host, port)

	return http.ListenAndServeTLS(":"+port, cert, key, r)
}

// startHTTPServer starts the server without TLS
func startHTTPServer(r *gin.Engine, port string) error {
	logger.Infof("starting HTTP server on port %s", port)
	return http.ListenAndServe(":"+port, r)
}

// runServer starts the appropriate server based on configuration
func runServer(r *gin.Engine, port string) {
	useSSL := strings.ToLower(os.Getenv("USE_SSL")) == "true"

	if !useSSL {
		log.Fatal(startHTTPServer(r, port))
		return
	}

	if err := startHTTPSServer(r, port); err != nil {
		logger.Errorf("HTTPS server failed: %v", err)
		logger.Info("falling back to HTTP")
		log.Fatal(startHTTPServer(r, port))
	}
}

func main() {
	// Initialize application components
	initLogger()
	initDatabase()
	initServices()

	// Setup HTTP router
	router := setupRouter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background services
	startBackgroundServices(ctx)

	// Register HTTP routes
	httpHandler.RegisterRoutes(router)

	// Start server
	port := getServerPort()
	runServer(router, port)
}