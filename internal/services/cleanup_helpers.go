package services

import (
	"sync"

	"github.com/amaumene/gostremiofr/internal/database"
)

func (c *CleanupService) fetchOldMagnets() ([]database.Magnet, error) {
	oldMagnets, err := c.db.GetOldMagnets(c.retentionPeriod)
	if err != nil {
		c.logger.Errorf("failed to get old magnets: %v", err)
		return nil, err
	}
	
	if len(oldMagnets) == 0 {
		c.logger.Debugf("no old magnets to clean up")
		return nil, nil
	}
	
	c.logger.Infof("found %d magnets to clean up", len(oldMagnets))
	return oldMagnets, nil
}

func (c *CleanupService) groupMagnetsByAPIKey(magnets []database.Magnet) map[string][]database.Magnet {
	magnetsByKey := make(map[string][]database.Magnet)
	for _, magnet := range magnets {
		if magnet.AllDebridID != "" && magnet.AllDebridKey != "" {
			magnetsByKey[magnet.AllDebridKey] = append(magnetsByKey[magnet.AllDebridKey], magnet)
		}
	}
	return magnetsByKey
}

func (c *CleanupService) cleanupAllDebridConcurrently(magnetsByKey map[string][]database.Magnet) {
	var wg sync.WaitGroup
	for apiKey, magnets := range magnetsByKey {
		wg.Add(1)
		go func(key string, mags []database.Magnet) {
			defer wg.Done()
			c.cleanupAllDebridMagnets(key, mags)
		}(apiKey, magnets)
	}
	wg.Wait()
}

func (c *CleanupService) deleteMagnetsFromDatabase(magnets []database.Magnet) int {
	cleaned := 0
	for _, magnet := range magnets {
		if err := c.db.DeleteMagnet(magnet.ID); err != nil {
			c.logger.Errorf("failed to delete magnet %s from database: %v", magnet.ID, err)
		} else {
			cleaned++
		}
	}
	return cleaned
}