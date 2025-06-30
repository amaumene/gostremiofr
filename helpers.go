package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type Config struct {
	TMDBAPIKey       string   `json:"TMDB_API_KEY"`
	APIKeyAllDebrid  string   `json:"API_KEY_ALLEDBRID"`
	FilesToShow      int      `json:"FILES_TO_SHOW"`
	ResToShow        []string `json:"RES_TO_SHOW"`
	LangToShow       []string `json:"LANG_TO_SHOW"`
	CodecsToShow     []string `json:"CODECS_TO_SHOW"`
	SharewoodPasskey string   `json:"SHAREWOOD_PASSKEY"`
}

type ParsedFileName struct {
	Resolution string `json:"resolution"`
	Codec      string `json:"codec"`
	Source     string `json:"source"`
}

func FormatSize(bytes int64) string {
	gb := float64(bytes) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.2f GB", gb)
}

func ParseFileName(fileName string) ParsedFileName {
	var result ParsedFileName

	// Resolution patterns
	resolutionRegex := regexp.MustCompile(`(?i)(4k|\d{3,4}p)`)
	if match := resolutionRegex.FindString(fileName); match != "" {
		result.Resolution = match
	} else {
		result.Resolution = "?"
	}

	// Codec patterns
	codecRegex := regexp.MustCompile(`(?i)(h\.264|h\.265|x\.264|x\.265|h264|h265|x264|x265|AV1|HEVC)`)
	if match := codecRegex.FindString(fileName); match != "" {
		result.Codec = match
	} else {
		result.Codec = "?"
	}

	// Source patterns
	sourceRegex := regexp.MustCompile(`(?i)(BluRay|WEB[-]?DL|WEB|HDRip|DVDRip|BRRip)`)
	if match := sourceRegex.FindString(fileName); match != "" {
		result.Source = match
	} else {
		result.Source = "?"
	}

	return result
}

func GetConfig(c *gin.Context) (*Config, error) {
	variables := c.Param("variables")
	if variables == "" {
		return nil, fmt.Errorf("configuration missing in URL")
	}

	decoded, err := base64.StdEncoding.DecodeString(variables)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration in URL: %v", err)
	}

	var config Config
	if err := json.Unmarshal(decoded, &config); err != nil {
		return nil, fmt.Errorf("invalid configuration format: %v", err)
	}

	return &config, nil
}

func PadString(s string, length int) string {
	if len(s) >= length {
		return s
	}
	return strings.Repeat("0", length-len(s)) + s
}

func StringToInt(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}