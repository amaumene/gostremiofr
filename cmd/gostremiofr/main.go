// Package main is the entry point for the GoStremioFR application.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/internal/constants"
	"github.com/amaumene/gostremiofr/internal/middleware"
	"github.com/amaumene/gostremiofr/pkg/ssl"
	"github.com/gin-gonic/gin"
)

// setupRouter creates and configures the Gin router
func setupRouter() *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(middleware.Gzip())
	r.Use(middleware.CORS())

	return r
}

// startBackgroundServices initializes background tasks
func startBackgroundServices(ctx context.Context) {
	tmdbCache.StartCleanup(ctx)
	startCleanupService(ctx)
}

// startCleanupService starts the cleanup service if available
func startCleanupService(ctx context.Context) {
	if container == nil || container.Cleanup == nil {
		return
	}

	configureCleanupRetention()
	container.Cleanup.Start(ctx)
}

// configureCleanupRetention sets retention period from environment
func configureCleanupRetention() {
	hours := os.Getenv("ALLDEBRID_RETENTION_HOURS")
	if hours == "" {
		return
	}

	duration, err := time.ParseDuration(hours + "h")
	if err != nil {
		return
	}

	container.Cleanup.SetRetentionPeriod(duration)
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
	return http.ListenAndServeTLS(":"+port, cert, key, r)
}

// startHTTPServer starts the server without TLS
func startHTTPServer(r *gin.Engine, port string) error {
	return http.ListenAndServe(":"+port, r)
}

// runServer starts the appropriate server based on configuration
func runServer(r *gin.Engine, port string) {
	if shouldUseSSL() {
		runSSLServer(r, port)
	} else {
		log.Fatal(startHTTPServer(r, port))
	}
}

// shouldUseSSL determines if SSL should be used
func shouldUseSSL() bool {
	return strings.ToLower(os.Getenv("USE_SSL")) == "true"
}

// runSSLServer attempts to start HTTPS, falls back to HTTP on failure
func runSSLServer(r *gin.Engine, port string) {
	if err := startHTTPSServer(r, port); err != nil {
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