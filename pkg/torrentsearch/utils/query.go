package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cehbz/torrentname"
)

// BuildSearchQuery builds a standardized search query for torrent providers.
func BuildSearchQuery(query string, mediaType string, season, episode int, specificEpisode bool) string {
	title, year := extractTitleAndYear(query)
	title = formatQueryString(title)

	switch mediaType {
	case "movie":
		return buildMovieQuery(title, year)
	case "series":
		return buildSeriesQuery(title, season, episode, specificEpisode)
	default:
		return title
	}
}

// buildMovieQuery constructs a movie search query.
func buildMovieQuery(title, year string) string {
	if year != "" {
		return fmt.Sprintf("%s+%s", title, year)
	}
	return title
}

// buildSeriesQuery constructs a series search query.
func buildSeriesQuery(title string, season, episode int, specificEpisode bool) string {
	if specificEpisode && episode > 0 {
		return fmt.Sprintf("%s+s%02de%02d", title, season, episode)
	}
	if season > 0 {
		return fmt.Sprintf("%s+s%02d", title, season)
	}
	return title
}

// extractTitleAndYear separates title and year from query string.
func extractTitleAndYear(query string) (string, string) {
	parts := strings.Fields(query)
	if len(parts) <= 1 {
		return query, ""
	}

	lastPart := parts[len(parts)-1]
	if isYear(lastPart) {
		title := strings.Join(parts[:len(parts)-1], " ")
		return title, lastPart
	}

	return query, ""
}

// isYear checks if a string represents a 4-digit year.
func isYear(s string) bool {
	matched, _ := regexp.MatchString(`^\d{4}$`, s)
	return matched
}

var alphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9\s]+`)

// formatQueryString cleans and formats query string for URL usage.
func formatQueryString(query string) string {
	query = strings.TrimSpace(query)
	// Replace non-alphanumeric characters with spaces
	query = alphanumericRegex.ReplaceAllString(query, " ")
	// Replace multiple spaces with single space
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")
	// Convert spaces to + for URL
	query = strings.ReplaceAll(query, " ", "+")
	return strings.Trim(query, "+")
}

// MatchesEpisode checks if filename matches specific season and episode.
func MatchesEpisode(fileName string, season, episode int) bool {
	parsed := torrentname.Parse(fileName)
	return parsed != nil && parsed.Season == season && parsed.Episode == episode
}

// MatchesSeason checks if filename matches specific season.
func MatchesSeason(fileName string, season int) bool {
	parsed := torrentname.Parse(fileName)
	return parsed != nil && parsed.Season == season && parsed.Episode == 0
}