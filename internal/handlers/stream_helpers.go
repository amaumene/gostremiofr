package handlers

import (
	"fmt"
	"strconv"
	"strings"
)

func parseIMDBEpisodeFormat(id string) (string, int, int, bool) {
	if !episodeRegex.MatchString(id) {
		return "", 0, 0, false
	}
	
	matches := episodeRegex.FindStringSubmatch(id)
	if len(matches) != 3 {
		return "", 0, 0, false
	}
	
	imdbID := strings.Split(id, ":")[0]
	season, _ := strconv.Atoi(matches[1])
	episode, _ := strconv.Atoi(matches[2])
	return imdbID, season, episode, true
}

func parseTMDBEpisodeFormat(id string) (string, int, int, bool) {
	if !tmdbEpisodeRegex.MatchString(id) {
		return "", 0, 0, false
	}
	
	matches := tmdbEpisodeRegex.FindStringSubmatch(id)
	if len(matches) != 4 {
		return "", 0, 0, false
	}
	
	tmdbID := fmt.Sprintf("tmdb:%s", matches[1])
	season, _ := strconv.Atoi(matches[2])
	episode, _ := strconv.Atoi(matches[3])
	return tmdbID, season, episode, true
}

func isMovieFormat(id string) bool {
	return imdbIDRegex.MatchString(id) || tmdbIDRegex.MatchString(id)
}