package middleware

import (
	"compress/gzip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gostremiofr/gostremiofr/pkg/logger"
)

type gzipResponseWriter struct {
	gin.ResponseWriter
	gzipWriter *gzip.Writer
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	return w.gzipWriter.Write(data)
}

func (w *gzipResponseWriter) WriteString(s string) (int, error) {
	return w.gzipWriter.Write([]byte(s))
}

func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
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

func Logger(log logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		
		if raw != "" {
			path = path + "?" + raw
		}

		switch {
		case statusCode >= 500:
			log.Errorf("%s %s %d %v %s", clientIP, method, statusCode, latency, path)
		case statusCode >= 400:
			log.Warnf("%s %s %d %v %s", clientIP, method, statusCode, latency, path)
		default:
			log.Infof("%s %s %d %v %s", clientIP, method, statusCode, latency, path)
		}
	}
}