package main

import (
	"fmt"
	"log"

	"github.com/amaumene/gostremiofr/pkg/torrentsearch"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/models"
	"github.com/amaumene/gostremiofr/pkg/torrentsearch/providers"
)

type SimpleCache struct {
	data map[string]interface{}
}

func NewSimpleCache() *SimpleCache {
	return &SimpleCache{
		data: make(map[string]interface{}),
	}
}

func (c *SimpleCache) Get(key string) (interface{}, bool) {
	val, exists := c.data[key]
	return val, exists
}

func (c *SimpleCache) Set(key string, value interface{}) {
	c.data[key] = value
}

func main() {
	cache := NewSimpleCache()
	
	search := torrentsearch.New(cache)
	
	// Language routing is now handled automatically by the search package
	
	yggProvider := providers.NewYGGProvider()
	search.RegisterProvider(providers.ProviderYGG, yggProvider)
	
	searchOptions := models.SearchOptions{
		Query:     "The Matrix",
		MediaType: "movie",
		Language:  "fr",
	}
	
	results, err := search.Search(providers.ProviderYGG, searchOptions)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Found %d movie torrents:\n", len(results.MovieTorrents))
	for i, torrent := range results.MovieTorrents {
		if i >= 5 {
			break
		}
		fmt.Printf("  - %s (Size: %.2f GB, Seeders: %d)\n", 
			torrent.Title, 
			float64(torrent.Size)/(1024*1024*1024),
			torrent.Seeders)
	}
	
	seriesOptions := models.SearchOptions{
		Query:           "Breaking Bad",
		MediaType:       "series",
		Season:          1,
		Episode:         1,
		Language:        "fr",
		SpecificEpisode: true,
	}
	
	seriesResults, err := search.Search(providers.ProviderYGG, seriesOptions)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("\nFound %d episode torrents for S01E01:\n", len(seriesResults.EpisodeTorrents))
	for i, torrent := range seriesResults.EpisodeTorrents {
		if i >= 5 {
			break
		}
		fmt.Printf("  - %s (Size: %.2f GB, Seeders: %d)\n",
			torrent.Title,
			float64(torrent.Size)/(1024*1024*1024),
			torrent.Seeders)
	}
}