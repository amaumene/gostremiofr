// Package ssl provides SSL/TLS certificate management for local development.
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

	"github.com/amaumene/gostremiofr/pkg/httputil"
	"github.com/amaumene/gostremiofr/pkg/logger"
)

const (
	// Certificate download URLs
	certificateURL = "https://local-ip.sh/server.pem"
	privateKeyURL  = "https://local-ip.sh/server.key"
	
	// File permissions
	keyFileMode = 0600
	dirMode     = 0755
	
	// Certificate validity period
	certValidityDays = 30
	
	// Network constants
	dnsServer = "8.8.8.8:80"
	networkType = "udp"
	
	// HTTP client timeout
	httpTimeout = 30 * time.Second
)

// LocalIPCertificate manages SSL certificates for local IP addresses using local-ip.sh service.
type LocalIPCertificate struct {
	logger   logger.Logger
	cacheDir string
	certPath string
	keyPath  string
	hostname string
}

// NewLocalIPCertificate creates a new LocalIPCertificate instance.
// Certificates are cached in a temporary directory.
func NewLocalIPCertificate(logger logger.Logger) *LocalIPCertificate {
	cacheDir := filepath.Join(os.TempDir(), "gostremiofr-ssl")
	return &LocalIPCertificate{
		logger:   logger,
		cacheDir: cacheDir,
	}
}

// Setup downloads and sets up the SSL certificate from local-ip.sh.
// It automatically detects the local IP address and constructs the appropriate hostname.
// Certificates are cached and reused if they are still valid.
func (l *LocalIPCertificate) Setup() error {
	l.logger.Info("setting up local-ip.sh certificate")

	// Get local IP address
	ip, err := l.getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %w", err)
	}

	// Construct hostname and paths
	l.hostname = l.constructHostname(ip)
	l.certPath = filepath.Join(l.cacheDir, "server.pem")
	l.keyPath = filepath.Join(l.cacheDir, "server.key")

	l.logger.Infof("using hostname: %s", l.hostname)

	// Check if valid certificates already exist
	if l.certificatesExist() && l.certificatesValid() {
		l.logger.Info("valid certificates already exist")
		return nil
	}

	// Create cache directory
	if err := os.MkdirAll(l.cacheDir, dirMode); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download certificates
	if err := l.downloadCertificates(); err != nil {
		return fmt.Errorf("failed to download certificates: %w", err)
	}

	l.logger.Info("certificates downloaded successfully")
	return nil
}

// constructHostname creates a hostname from an IP address for local-ip.sh
func (l *LocalIPCertificate) constructHostname(ip string) string {
	return strings.ReplaceAll(ip, ".", "-") + ".local-ip.sh"
}

// GetTLSConfig returns the TLS configuration with the loaded certificates.
func (l *LocalIPCertificate) GetTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(l.certPath, l.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}

// GetHostname returns the configured hostname.
func (l *LocalIPCertificate) GetHostname() string {
	return l.hostname
}

// GetCertificatePaths returns the paths to the certificate and key files.
func (l *LocalIPCertificate) GetCertificatePaths() (certPath, keyPath string) {
	return l.certPath, l.keyPath
}

// getLocalIP returns the local IP address by establishing a UDP connection.
// This method works reliably across different network configurations.
func (l *LocalIPCertificate) getLocalIP() (string, error) {
	conn, err := net.Dial(networkType, dnsServer)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// certificatesExist checks if both certificate and key files exist.
func (l *LocalIPCertificate) certificatesExist() bool {
	_, certErr := os.Stat(l.certPath)
	_, keyErr := os.Stat(l.keyPath)
	return certErr == nil && keyErr == nil
}

// certificatesValid checks if the certificates are still valid.
// Certificates are considered valid if they are less than 30 days old.
func (l *LocalIPCertificate) certificatesValid() bool {
	info, err := os.Stat(l.certPath)
	if err != nil {
		return false
	}

	age := time.Since(info.ModTime())
	return age < certValidityDays*24*time.Hour
}

// downloadCertificates downloads the certificate and key from local-ip.sh.
// The private key is automatically secured with appropriate file permissions.
func (l *LocalIPCertificate) downloadCertificates() error {
	client := httputil.NewHTTPClient(httpTimeout)

	// Download certificate
	l.logger.Debugf("downloading certificate from %s", certificateURL)
	if err := l.downloadFile(client, certificateURL, l.certPath); err != nil {
		return fmt.Errorf("failed to download certificate: %w", err)
	}

	// Download private key
	l.logger.Debugf("downloading private key from %s", privateKeyURL)
	if err := l.downloadFile(client, privateKeyURL, l.keyPath); err != nil {
		return fmt.Errorf("failed to download private key: %w", err)
	}

	// Set appropriate permissions for the private key
	if err := os.Chmod(l.keyPath, keyFileMode); err != nil {
		return fmt.Errorf("failed to set key permissions: %w", err)
	}

	return nil
}

// downloadFile downloads a file from URL to destination.
// Certificate verification is skipped for local-ip.sh downloads.
func (l *LocalIPCertificate) downloadFile(client *http.Client, url, dest string) error {
	// Configure transport with custom TLS settings
	client.Transport = l.createTransport()

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

// createTransport creates an HTTP transport that skips certificate verification
func (l *LocalIPCertificate) createTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

// Cleanup removes the cached certificates and temporary directory.
func (l *LocalIPCertificate) Cleanup() error {
	l.logger.Info("cleaning up certificates")
	return os.RemoveAll(l.cacheDir)
}
