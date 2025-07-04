package ssl

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amaumene/gostremiofr/pkg/logger"
)

type LocalIPCertificate struct {
	logger    logger.Logger
	cacheDir  string
	certPath  string
	keyPath   string
	hostname  string
}

// NewLocalIPCertificate creates a new LocalIPCertificate instance
func NewLocalIPCertificate(logger logger.Logger) *LocalIPCertificate {
	cacheDir := filepath.Join(os.TempDir(), "gostremiofr-ssl")
	return &LocalIPCertificate{
		logger:   logger,
		cacheDir: cacheDir,
	}
}

// Setup downloads and sets up the SSL certificate from local-ip.sh
func (l *LocalIPCertificate) Setup() error {
	l.logger.Infof("[SSL] setting up local-ip.sh certificate")

	// Get local IP address
	ip, err := l.getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}

	// Construct hostname
	l.hostname = strings.ReplaceAll(ip, ".", "-") + ".local-ip.sh"
	l.certPath = filepath.Join(l.cacheDir, "server.pem")
	l.keyPath = filepath.Join(l.cacheDir, "server.key")

	l.logger.Infof("[SSL] using hostname: %s", l.hostname)

	// Check if certificates already exist and are valid
	if l.certificatesExist() && l.certificatesValid() {
		l.logger.Infof("[SSL] valid certificates already exist")
		return nil
	}

	// Create cache directory
	if err := os.MkdirAll(l.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download certificates
	if err := l.downloadCertificates(); err != nil {
		return fmt.Errorf("failed to download certificates: %w", err)
	}

	l.logger.Infof("[SSL] certificates downloaded successfully")
	return nil
}

// GetTLSConfig returns the TLS configuration
func (l *LocalIPCertificate) GetTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(l.certPath, l.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}

// GetHostname returns the configured hostname
func (l *LocalIPCertificate) GetHostname() string {
	return l.hostname
}

// GetCertificatePaths returns the paths to the certificate and key files
func (l *LocalIPCertificate) GetCertificatePaths() (certPath, keyPath string) {
	return l.certPath, l.keyPath
}

// getLocalIP returns the local IP address
func (l *LocalIPCertificate) getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// certificatesExist checks if certificate files exist
func (l *LocalIPCertificate) certificatesExist() bool {
	_, certErr := os.Stat(l.certPath)
	_, keyErr := os.Stat(l.keyPath)
	return certErr == nil && keyErr == nil
}

// certificatesValid checks if the certificates are still valid
func (l *LocalIPCertificate) certificatesValid() bool {
	// For local-ip.sh certificates, we'll check if they're less than 30 days old
	// as they might have expiration dates
	info, err := os.Stat(l.certPath)
	if err != nil {
		return false
	}

	age := time.Since(info.ModTime())
	return age < 30*24*time.Hour
}

// downloadCertificates downloads the certificate and key from local-ip.sh
func (l *LocalIPCertificate) downloadCertificates() error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Download certificate
	certURL := "https://local-ip.sh/server.pem"
	l.logger.Debugf("[SSL] downloading certificate from %s", certURL)
	
	if err := l.downloadFile(client, certURL, l.certPath); err != nil {
		return fmt.Errorf("failed to download certificate: %w", err)
	}

	// Download private key
	keyURL := "https://local-ip.sh/server.key"
	l.logger.Debugf("[SSL] downloading private key from %s", keyURL)
	
	if err := l.downloadFile(client, keyURL, l.keyPath); err != nil {
		return fmt.Errorf("failed to download private key: %w", err)
	}

	// Set appropriate permissions for the private key
	if err := os.Chmod(l.keyPath, 0600); err != nil {
		return fmt.Errorf("failed to set key permissions: %w", err)
	}

	return nil
}

// downloadFile downloads a file from URL to destination
func (l *LocalIPCertificate) downloadFile(client *http.Client, url, dest string) error {
	// Create a custom transport that skips certificate verification for local-ip.sh
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client.Transport = transport

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create the file
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// Cleanup removes the cached certificates
func (l *LocalIPCertificate) Cleanup() error {
	l.logger.Infof("[SSL] cleaning up certificates")
	return os.RemoveAll(l.cacheDir)
}