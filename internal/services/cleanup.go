package services

import (
	"context"
	"sync"
	"time"

	"github.com/amaumene/gostremiofr/internal/database"
	"github.com/amaumene/gostremiofr/pkg/logger"
	"github.com/amaumene/gostremiofr/pkg/security"
)

const (
	// Default cleanup settings
	defaultCleanupInterval = 1 * time.Hour
	defaultRetentionPeriod = 4 * time.Hour
)

// CleanupService manages periodic cleanup of old resources
type CleanupService struct {
	db              database.Database
	allDebrid       *AllDebrid
	logger          logger.Logger
	interval        time.Duration
	retentionPeriod time.Duration
	mu              sync.Mutex
	running         bool
	stopChan        chan struct{}
	validator       *security.APIKeyValidator
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(db database.Database, allDebrid *AllDebrid) *CleanupService {
	return &CleanupService{
		db:              db,
		allDebrid:       allDebrid,
		logger:          logger.New(),
		interval:        defaultCleanupInterval,
		retentionPeriod: defaultRetentionPeriod,
		stopChan:        make(chan struct{}),
		validator:       security.NewAPIKeyValidator(),
	}
}

// SetRetentionPeriod sets how long to keep magnets before cleanup
func (c *CleanupService) SetRetentionPeriod(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.retentionPeriod = duration
}

// SetInterval sets how often cleanup runs
func (c *CleanupService) SetInterval(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interval = duration
}

// Start begins the cleanup service
func (c *CleanupService) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = true
	c.mu.Unlock()

	c.logger.Infof("starting cleanup service with interval: %v, retention: %v", c.interval, c.retentionPeriod)

	// Run initial cleanup
	c.performCleanup()

	// Start periodic cleanup
	go c.cleanupLoop(ctx)

	return nil
}

// Stop stops the cleanup service
func (c *CleanupService) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.running = false
	close(c.stopChan)
	c.logger.Infof("cleanup service stopped")
}

// cleanupLoop runs periodic cleanup
func (c *CleanupService) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.Stop()
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.performCleanup()
		}
	}
}

// performCleanup executes the cleanup process
func (c *CleanupService) performCleanup() {
	c.logger.Infof("starting cleanup process")

	oldMagnets, err := c.fetchOldMagnets()
	if err != nil || oldMagnets == nil {
		return
	}

	magnetsByKey := c.groupMagnetsByAPIKey(oldMagnets)
	c.cleanupAllDebridConcurrently(magnetsByKey)
	
	cleaned := c.deleteMagnetsFromDatabase(oldMagnets)
	c.logger.Infof("cleanup completed: %d magnets removed from database", cleaned)
}

// cleanupAllDebridMagnets removes magnets from AllDebrid
func (c *CleanupService) cleanupAllDebridMagnets(apiKey string, magnets []database.Magnet) {
	// Validate API key
	if !c.validator.IsValidAllDebridKey(apiKey) {
		c.logger.Warnf("skipping cleanup for invalid API key: %s", c.validator.MaskAPIKey(apiKey))
		return
	}

	for _, magnet := range magnets {
		if magnet.AllDebridID == "" {
			continue
		}

		err := c.allDebrid.DeleteMagnet(magnet.AllDebridID, apiKey)
		if err != nil {
			c.logger.Warnf("failed to delete magnet %s from AllDebrid: %v", magnet.AllDebridID, err)
			// Continue with other magnets even if one fails
		} else {
			c.logger.Debugf("deleted magnet %s from AllDebrid", magnet.AllDebridID)
		}

		// Small delay between deletions to avoid rate limiting
		time.Sleep(100 * time.Millisecond)
	}
}

// CleanupNow performs immediate cleanup (useful for testing or manual trigger)
func (c *CleanupService) CleanupNow() {
	c.performCleanup()
}
