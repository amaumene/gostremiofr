package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Initialize test database in memory
	var err error
	DB, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		panic(err)
	}
	createTables()

	// Initialize logger for tests
	InitializeLogger()

	// Run tests
	code := m.Run()

	// Cleanup
	DB.Close()
	os.Exit(code)
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	
	// Add CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	})
	
	// Setup routes
	setupConfigRoutes(r)
	setupManifestRoutes(r)
	setupStreamRoutes(r)
	
	return r
}

func TestConfigPage(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/config", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Configuration Stremio Addon")
	assert.Contains(t, w.Body.String(), "generateConfig()")
	assert.Contains(t, w.Body.String(), "getConfigFromURL")
	assert.Contains(t, w.Body.String(), `id="tmdb"`)
	assert.Contains(t, w.Body.String(), `id="alldebrid"`)
}

func TestConfigurePageWithVariables(t *testing.T) {
	router := setupTestRouter()

	// Create test configuration
	config := Config{
		TMDBAPIKey:       "test-tmdb-key",
		APIKeyAllDebrid:  "test-alldebrid-key",
		FilesToShow:      5,
		ResToShow:        []string{"1080p", "720p"},
		LangToShow:       []string{"MULTi", "FRENCH"},
		CodecsToShow:     []string{"h264", "h265"},
		SharewoodPasskey: "test-sharewood-key",
	}

	configJSON, _ := json.Marshal(config)
	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/"+encodedConfig+"/configure", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Configuration Stremio Addon")
	assert.Contains(t, w.Body.String(), "generateConfig()")
	assert.Contains(t, w.Body.String(), "getConfigFromURL")
}

func TestManifestEndpoint(t *testing.T) {
	router := setupTestRouter()

	// Create test configuration
	config := Config{
		TMDBAPIKey:      "test-key",
		APIKeyAllDebrid: "test-key",
		FilesToShow:     5,
		ResToShow:       []string{"1080p"},
		LangToShow:      []string{"FRENCH"},
		CodecsToShow:    []string{"h264"},
	}

	configJSON, _ := json.Marshal(config)
	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/"+encodedConfig+"/manifest.json", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var manifest Manifest
	err := json.Unmarshal(w.Body.Bytes(), &manifest)
	require.NoError(t, err)

	assert.Equal(t, "ygg.stremio.ad", manifest.ID)
	assert.Equal(t, "0.0.4", manifest.Version)
	assert.Equal(t, "Ygg + AD", manifest.Name)
	assert.Contains(t, manifest.Types, "movie")
	assert.Contains(t, manifest.Types, "series")
	assert.Contains(t, manifest.Resources, "stream")
	assert.True(t, manifest.BehaviorHints.Configurable)
}

func TestInvalidConfigManifest(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/invalid-base64/manifest.json", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestStreamEndpointParameterParsing(t *testing.T) {
	router := setupTestRouter()

	// Create test configuration
	config := Config{
		TMDBAPIKey:      "test-key",
		APIKeyAllDebrid: "test-key",
		FilesToShow:     5,
		ResToShow:       []string{"1080p"},
		LangToShow:      []string{"FRENCH"},
		CodecsToShow:    []string{"h264"},
	}

	configJSON, _ := json.Marshal(config)
	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	// Test movie endpoint with .json suffix
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/"+encodedConfig+"/stream/movie/tt1234567.json", nil)
	router.ServeHTTP(w, req)

	// Should return 200 even if TMDB fails (returns empty streams)
	assert.Equal(t, http.StatusOK, w.Code)

	var response StreamResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
}

func TestHelperFunctions(t *testing.T) {
	t.Run("FormatSize", func(t *testing.T) {
		assert.Equal(t, "1.00 GB", FormatSize(1073741824))
		assert.Equal(t, "0.50 GB", FormatSize(536870912))
		assert.Equal(t, "0.00 GB", FormatSize(0))
	})

	t.Run("ParseFileName", func(t *testing.T) {
		parsed := ParseFileName("Movie.2023.1080p.BluRay.x264-GROUP")
		assert.Equal(t, "1080p", parsed.Resolution)
		assert.Equal(t, "x264", parsed.Codec)
		assert.Equal(t, "BluRay", parsed.Source)

		parsed = ParseFileName("Series.S01E01.720p.WEB-DL.h265")
		assert.Equal(t, "720p", parsed.Resolution)
		assert.Equal(t, "h265", parsed.Codec)
		assert.Equal(t, "WEB-DL", parsed.Source)

		parsed = ParseFileName("Unknown.File.Name")
		assert.Equal(t, "?", parsed.Resolution)
		assert.Equal(t, "?", parsed.Codec)
		assert.Equal(t, "?", parsed.Source)
	})

	t.Run("PadString", func(t *testing.T) {
		assert.Equal(t, "01", PadString("1", 2))
		assert.Equal(t, "10", PadString("10", 2))
		assert.Equal(t, "100", PadString("100", 2))
	})

	t.Run("StringToInt", func(t *testing.T) {
		assert.Equal(t, 123, StringToInt("123"))
		assert.Equal(t, 0, StringToInt("invalid"))
		assert.Equal(t, 0, StringToInt(""))
	})
}

func TestGetConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("ValidConfig", func(t *testing.T) {
		config := Config{
			TMDBAPIKey:      "test-key",
			APIKeyAllDebrid: "test-key",
			FilesToShow:     5,
		}

		configJSON, _ := json.Marshal(config)
		encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "variables", Value: encodedConfig}}

		parsedConfig, err := GetConfig(c)
		require.NoError(t, err)
		assert.Equal(t, "test-key", parsedConfig.TMDBAPIKey)
		assert.Equal(t, "test-key", parsedConfig.APIKeyAllDebrid)
		assert.Equal(t, 5, parsedConfig.FilesToShow)
	})

	t.Run("InvalidBase64", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "variables", Value: "invalid-base64"}}

		_, err := GetConfig(c)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid configuration")
	})

	t.Run("MissingVariables", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		_, err := GetConfig(c)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "configuration missing")
	})
}

func TestDatabaseOperations(t *testing.T) {
	t.Run("TMDBCache", func(t *testing.T) {
		// Test storing TMDB data
		err := StoreTMDB("tt1234567", "movie", "Test Movie", "Test Movie FR")
		require.NoError(t, err)

		// Test retrieving TMDB data
		cached, err := GetCachedTMDB("tt1234567")
		require.NoError(t, err)
		require.NotNil(t, cached)
		assert.Equal(t, "movie", cached.Type)
		assert.Equal(t, "Test Movie", cached.Title)
		assert.Equal(t, "Test Movie FR", cached.FrenchTitle)

		// Test non-existent data
		cached, err = GetCachedTMDB("tt9999999")
		require.NoError(t, err)
		assert.Nil(t, cached)
	})

	t.Run("MagnetOperations", func(t *testing.T) {
		// Test storing magnet
		err := StoreMagnet("12345", "abcdef123456", "Test Magnet")
		require.NoError(t, err)

		// Test getting all magnets
		magnets, err := GetAllMagnets()
		require.NoError(t, err)
		assert.Len(t, magnets, 1)
		assert.Equal(t, "12345", magnets[0].ID)
		assert.Equal(t, "abcdef123456", magnets[0].Hash)
		assert.Equal(t, "Test Magnet", magnets[0].Name)

		// Test deleting magnet
		err = DeleteMagnet("12345")
		require.NoError(t, err)

		magnets, err = GetAllMagnets()
		require.NoError(t, err)
		assert.Len(t, magnets, 0)
	})

	t.Run("InvalidMagnetData", func(t *testing.T) {
		// Test storing magnet with empty data
		err := StoreMagnet("", "hash", "name")
		assert.Error(t, err)

		err = StoreMagnet("id", "", "name")
		assert.Error(t, err)

		err = StoreMagnet("id", "hash", "")
		assert.Error(t, err)
	})
}

func TestTorrentProcessing(t *testing.T) {
	config := &Config{
		ResToShow:    []string{"1080p", "720p"},
		LangToShow:   []string{"MULTi", "FRENCH"},
		CodecsToShow: []string{"h264", "h265"},
	}

	t.Run("YggTorrentProcessing", func(t *testing.T) {
		torrents := []YggTorrent{
			{ID: 1, Title: "Movie.2023.1080p.MULTi.h264-GROUP", Source: "YGG"},
			{ID: 2, Title: "Series.S01.COMPLETE.720p.FRENCH.h265-GROUP", Source: "YGG"},
			{ID: 3, Title: "Series.S01E01.1080p.MULTi.h264-GROUP", Source: "YGG"},
		}

		// Test movie processing - should filter only matching torrents
		results := ProcessTorrents(torrents, "movie", "", "", config)
		// Only the movie torrent should match all filters (resolution, language, codec)
		movieCount := 0
		for _, torrent := range results.MovieTorrents {
			if torrent.Title == "Movie.2023.1080p.MULTi.h264-GROUP" {
				movieCount++
			}
		}
		assert.Equal(t, 1, movieCount)

		// Test series processing
		results = ProcessTorrents(torrents, "series", "1", "1", config)
		assert.Len(t, results.CompleteSeriesTorrents, 1)
		assert.Len(t, results.EpisodeTorrents, 1)
		assert.Contains(t, results.CompleteSeriesTorrents[0].Title, "COMPLETE")
		assert.Contains(t, results.EpisodeTorrents[0].Title, "S01E01")
	})
}

func TestMatchesEpisode(t *testing.T) {
	tests := []struct {
		title    string
		season   string
		episode  string
		expected bool
	}{
		{"Series.S01E01.1080p", "1", "1", true},
		{"Series.S01.E01.1080p", "1", "1", true},
		{"Series.S01E02.1080p", "1", "1", false},
		{"Series.S02E01.1080p", "1", "1", false},
		{"Movie.2023.1080p", "1", "1", false},
		{"", "", "", false},
	}

	for _, test := range tests {
		result := matchesEpisode(test.title, test.season, test.episode)
		assert.Equal(t, test.expected, result, "Failed for title: %s, season: %s, episode: %s", test.title, test.season, test.episode)
	}
}

func TestRouteParameterParsing(t *testing.T) {
	router := setupTestRouter()

	config := Config{
		TMDBAPIKey:      "test-key",
		APIKeyAllDebrid: "test-key",
		FilesToShow:     1,
		ResToShow:       []string{"1080p"},
		LangToShow:      []string{"FRENCH"},
		CodecsToShow:    []string{"h264"},
	}

	configJSON, _ := json.Marshal(config)
	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	tests := []struct {
		path     string
		expected int
	}{
		{"/" + encodedConfig + "/stream/movie/tt1234567.json", http.StatusOK},
		{"/" + encodedConfig + "/stream/series/tt1234567:1:1.json", http.StatusOK},
		{"/" + encodedConfig + "/stream/movie/tt1234567", http.StatusOK},
	}

	for _, test := range tests {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", test.path, nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, test.expected, w.Code, "Failed for path: %s", test.path)
	}
}

func TestCORSHeaders(t *testing.T) {
	router := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/config", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestAllDebridStructures(t *testing.T) {
	t.Run("AllDebridMagnetUnmarshaling", func(t *testing.T) {
		// Test that we can unmarshal AllDebrid response with numeric ID
		jsonData := `{
			"status": "success",
			"data": {
				"magnets": [{
					"id": 12345,
					"hash": "abcdef123456",
					"name": "Test Magnet",
					"size": 1073741824,
					"ready": true
				}]
			}
		}`

		var response AllDebridUploadResponse
		err := json.Unmarshal([]byte(jsonData), &response)
		require.NoError(t, err)
		assert.Equal(t, "success", response.Status)
		assert.Len(t, response.Data.Magnets, 1)
		assert.Equal(t, 12345, response.Data.Magnets[0].ID)
		assert.Equal(t, "abcdef123456", response.Data.Magnets[0].Hash)
		assert.True(t, response.Data.Magnets[0].Ready)
	})

	t.Run("ProcessedMagnetStructure", func(t *testing.T) {
		processed := ProcessedMagnet{
			Hash:   "abcdef123456",
			Ready:  "✅ Ready",
			Name:   "Test Magnet",
			Size:   1073741824,
			ID:     12345,
			Source: "YGG",
		}

		jsonData, err := json.Marshal(processed)
		require.NoError(t, err)
		assert.Contains(t, string(jsonData), `"id":12345`)
		assert.Contains(t, string(jsonData), `"ready":"✅ Ready"`)
	})
}

// Integration test that verifies the full request flow
func TestFullRequestFlow(t *testing.T) {
	router := setupTestRouter()

	// 1. Test config page loads
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/config", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. Test manifest with valid config
	config := Config{
		TMDBAPIKey:      "test-key",
		APIKeyAllDebrid: "test-key",
		FilesToShow:     2,
		ResToShow:       []string{"1080p"},
		LangToShow:      []string{"FRENCH"},
		CodecsToShow:    []string{"h264"},
	}

	configJSON, _ := json.Marshal(config)
	encodedConfig := base64.StdEncoding.EncodeToString(configJSON)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/"+encodedConfig+"/manifest.json", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 3. Test configure page with the same config
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/"+encodedConfig+"/configure", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "getConfigFromURL")

	// 4. Test stream endpoint (will fail on external APIs but should parse correctly)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/"+encodedConfig+"/stream/movie/tt1234567.json", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}