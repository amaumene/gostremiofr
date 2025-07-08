// Package models defines data structures for TMDB API responses.
package models

type TMDBData struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	Year  int    `json:"year"`
}

type TMDBFindResponse struct {
	MovieResults []TMDBMovie `json:"movie_results"`
	TVResults    []TMDBTV    `json:"tv_results"`
}

type TMDBMovie struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Overview      string  `json:"overview"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	ReleaseDate   string  `json:"release_date"`
	VoteAverage   float64 `json:"vote_average"`
	VoteCount     int     `json:"vote_count"`
	GenreIDs      []int   `json:"genre_ids"`
	Popularity    float64 `json:"popularity"`
}

type TMDBTV struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	OriginalName string  `json:"original_name"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	FirstAirDate string  `json:"first_air_date"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	GenreIDs     []int   `json:"genre_ids"`
	Popularity   float64 `json:"popularity"`
}

type TMDBMovieResponse struct {
	Page         int         `json:"page"`
	Results      []TMDBMovie `json:"results"`
	TotalPages   int         `json:"total_pages"`
	TotalResults int         `json:"total_results"`
}

type TMDBTVResponse struct {
	Page         int      `json:"page"`
	Results      []TMDBTV `json:"results"`
	TotalPages   int      `json:"total_pages"`
	TotalResults int      `json:"total_results"`
}

type TMDBGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TMDBGenreResponse struct {
	Genres []TMDBGenre `json:"genres"`
}

type TMDBMovieDetails struct {
	ID                  int         `json:"id"`
	IMDBId              string      `json:"imdb_id"`
	Title               string      `json:"title"`
	OriginalTitle       string      `json:"original_title"`
	Overview            string      `json:"overview"`
	PosterPath          string      `json:"poster_path"`
	BackdropPath        string      `json:"backdrop_path"`
	ReleaseDate         string      `json:"release_date"`
	Runtime             int         `json:"runtime"`
	VoteAverage         float64     `json:"vote_average"`
	VoteCount           int         `json:"vote_count"`
	Genres              []TMDBGenre `json:"genres"`
	ProductionCountries []struct {
		ISO  string `json:"iso_3166_1"`
		Name string `json:"name"`
	} `json:"production_countries"`
	SpokenLanguages []struct {
		ISO  string `json:"iso_639_1"`
		Name string `json:"name"`
	} `json:"spoken_languages"`
	Credits struct {
		Cast []struct {
			Name      string `json:"name"`
			Character string `json:"character"`
			Order     int    `json:"order"`
		} `json:"cast"`
		Crew []struct {
			Name       string `json:"name"`
			Department string `json:"department"`
			Job        string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
}

type TMDBTVDetails struct {
	ID               int          `json:"id"`
	Name             string       `json:"name"`
	OriginalName     string       `json:"original_name"`
	Overview         string       `json:"overview"`
	PosterPath       string       `json:"poster_path"`
	BackdropPath     string       `json:"backdrop_path"`
	FirstAirDate     string       `json:"first_air_date"`
	EpisodeRunTime   []int        `json:"episode_run_time"`
	VoteAverage      float64      `json:"vote_average"`
	VoteCount        int          `json:"vote_count"`
	Genres           []TMDBGenre  `json:"genres"`
	OriginCountry    []string     `json:"origin_country"`
	OriginalLanguage string       `json:"original_language"`
	NumberOfSeasons  int          `json:"number_of_seasons"`
	NumberOfEpisodes int          `json:"number_of_episodes"`
	Seasons          []TMDBSeason `json:"seasons"`
	Credits          struct {
		Cast []struct {
			Name      string `json:"name"`
			Character string `json:"character"`
			Order     int    `json:"order"`
		} `json:"cast"`
		Crew []struct {
			Name       string `json:"name"`
			Department string `json:"department"`
			Job        string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	ExternalIds struct {
		IMDBId string `json:"imdb_id"`
	} `json:"external_ids"`
}

type TMDBSeason struct {
	ID           int    `json:"id"`
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	AirDate      string `json:"air_date"`
	EpisodeCount int    `json:"episode_count"`
	PosterPath   string `json:"poster_path"`
}

type TMDBSeasonDetails struct {
	ID           int           `json:"id"`
	SeasonNumber int           `json:"season_number"`
	Name         string        `json:"name"`
	Overview     string        `json:"overview"`
	AirDate      string        `json:"air_date"`
	Episodes     []TMDBEpisode `json:"episodes"`
	PosterPath   string        `json:"poster_path"`
}

type TMDBEpisode struct {
	ID            int     `json:"id"`
	EpisodeNumber int     `json:"episode_number"`
	SeasonNumber  int     `json:"season_number"`
	Name          string  `json:"name"`
	Overview      string  `json:"overview"`
	AirDate       string  `json:"air_date"`
	StillPath     string  `json:"still_path"`
	VoteAverage   float64 `json:"vote_average"`
	VoteCount     int     `json:"vote_count"`
	Runtime       int     `json:"runtime"`
}

type TMDBConfiguration struct {
	Images TMDBImageConfig `json:"images"`
}

type TMDBImageConfig struct {
	BaseURL       string   `json:"base_url"`
	SecureBaseURL string   `json:"secure_base_url"`
	PosterSizes   []string `json:"poster_sizes"`
	BackdropSizes []string `json:"backdrop_sizes"`
}
