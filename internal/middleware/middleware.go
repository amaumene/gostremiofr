// Package middleware provides HTTP middleware functions for the Gin web framework.
package middleware

import (
	"compress/gzip"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/gin-gonic/gin"
)

// gzipResponseWriter wraps gin.ResponseWriter to provide gzip compression.
type gzipResponseWriter struct {
	gin.ResponseWriter
	gzipWriter *gzip.Writer
}

// Write implements io.Writer interface for gzip compression.
func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	return w.gzipWriter.Write(data)
}

// WriteString writes string data with gzip compression.
func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	return w.gzipWriter.Write([]byte(s))
}

// Gzip returns a middleware that compresses HTTP responses using gzip.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !acceptsGzip(c) {
			c.Next()
			return
		}

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")

		gzipWriter := gzip.NewWriter(c.Writer)
		defer gzipWriter.Close()

		c.Writer = &gzipResponseWriter{
			ResponseWriter: c.Writer,
			gzipWriter:     gzipWriter,
		}

		c.Next()
	}
}

// acceptsGzip checks if client accepts gzip encoding.
func acceptsGzip(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept-Encoding"), "gzip")
}

// CORS returns a middleware that adds CORS headers to responses.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// Logger returns a middleware that logs HTTP requests with status-based log levels.
func Logger(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := buildRequestPath(c)

		c.Next()

		logRequest(log, c, start, path)
	}
}

// buildRequestPath constructs the full request path with query parameters.
func buildRequestPath(c *gin.Context) string {
	path := c.Request.URL.Path
	if raw := c.Request.URL.RawQuery; raw != "" {
		path = path + "?" + raw
	}
	return path
}

// logRequest logs the HTTP request with appropriate log level based on status.
func logRequest(log logger.Logger, c *gin.Context, start time.Time, path string) {
	latency := time.Since(start)
	clientIP := c.ClientIP()
	method := c.Request.Method
	statusCode := c.Writer.Status()

	switch {
	case statusCode >= 500:
		log.Errorf("HTTP request failed - %s %s %d %v %s", clientIP, method, statusCode, latency, path)
	case statusCode >= 400:
		log.Warnf("warning: client error - %s %s %d %v %s", clientIP, method, statusCode, latency, path)
	default:
		log.Infof("HTTP request completed - %s %s %d %v %s", clientIP, method, statusCode, latency, path)
	}
}
