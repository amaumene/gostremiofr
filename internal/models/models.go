package models


type Stream struct {
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

type StreamResponse struct {
	Streams []Stream `json:"streams"`
}

type TorrentInfo struct {
	ID     string
	Title  string
	Hash   string
	Source string
}

type YggTorrent struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Size     int64  `json:"size"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Hash     string `json:"hash,omitempty"`
	Source   string `json:"source,omitempty"`
}

type TorrentResults struct {
	CompleteSeriesTorrents []TorrentInfo
	CompleteSeasonTorrents []TorrentInfo
	EpisodeTorrents        []TorrentInfo
	MovieTorrents          []TorrentInfo
}

type CombinedTorrentResults struct {
	CompleteSeriesTorrents []TorrentInfo
	CompleteSeasonTorrents []TorrentInfo
	EpisodeTorrents        []TorrentInfo
	MovieTorrents          []TorrentInfo
}

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
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Overview      string   `json:"overview"`
	PosterPath    string   `json:"poster_path"`
	BackdropPath  string   `json:"backdrop_path"`
	ReleaseDate   string   `json:"release_date"`
	VoteAverage   float64  `json:"vote_average"`
	VoteCount     int      `json:"vote_count"`
	GenreIDs      []int    `json:"genre_ids"`
	Popularity    float64  `json:"popularity"`
}

type TMDBTV struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	OriginalName  string   `json:"original_name"`
	Overview      string   `json:"overview"`
	PosterPath    string   `json:"poster_path"`
	BackdropPath  string   `json:"backdrop_path"`
	FirstAirDate  string   `json:"first_air_date"`
	VoteAverage   float64  `json:"vote_average"`
	VoteCount     int      `json:"vote_count"`
	GenreIDs      []int    `json:"genre_ids"`
	Popularity    float64  `json:"popularity"`
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
	ID               int           `json:"id"`
	IMDBId           string        `json:"imdb_id"`
	Title            string        `json:"title"`
	OriginalTitle    string        `json:"original_title"`
	Overview         string        `json:"overview"`
	PosterPath       string        `json:"poster_path"`
	BackdropPath     string        `json:"backdrop_path"`
	ReleaseDate      string        `json:"release_date"`
	Runtime          int           `json:"runtime"`
	VoteAverage      float64       `json:"vote_average"`
	VoteCount        int           `json:"vote_count"`
	Genres           []TMDBGenre   `json:"genres"`
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
	ID               int           `json:"id"`
	Name             string        `json:"name"`
	OriginalName     string        `json:"original_name"`
	Overview         string        `json:"overview"`
	PosterPath       string        `json:"poster_path"`
	BackdropPath     string        `json:"backdrop_path"`
	FirstAirDate     string        `json:"first_air_date"`
	EpisodeRunTime   []int         `json:"episode_run_time"`
	VoteAverage      float64       `json:"vote_average"`
	VoteCount        int           `json:"vote_count"`
	Genres           []TMDBGenre   `json:"genres"`
	OriginCountry    []string      `json:"origin_country"`
	OriginalLanguage string        `json:"original_language"`
	NumberOfSeasons  int           `json:"number_of_seasons"`
	NumberOfEpisodes int           `json:"number_of_episodes"`
	Seasons          []TMDBSeason  `json:"seasons"`
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
	ID             int     `json:"id"`
	EpisodeNumber  int     `json:"episode_number"`
	SeasonNumber   int     `json:"season_number"`
	Name           string  `json:"name"`
	Overview       string  `json:"overview"`
	AirDate        string  `json:"air_date"`
	StillPath      string  `json:"still_path"`
	VoteAverage    float64 `json:"vote_average"`
	VoteCount      int     `json:"vote_count"`
	Runtime        int     `json:"runtime"`
}

type MagnetInfo struct {
	Hash   string
	Title  string
	Source string
}

type AllDebridMagnet struct {
	Hash       string  `json:"hash"`
	Status     string  `json:"status"`
	StatusCode int     `json:"statusCode"`
	Filename   string  `json:"filename"`
	Size       float64 `json:"size"`
	ID         int64   `json:"id"`
	Links      []interface{} `json:"links"`
}

type ProcessedMagnet struct {
	Hash   string
	Ready  bool
	Name   string
	Size   float64
	ID     int64
	Source string
	Links  []interface{}
}

type VideoFile struct {
	Name   string
	Size   float64
	Link   string
	Source string
}

type FileInfo struct {
	Name   string     `json:"name"`
	Size   float64    `json:"size"`
	Link   string     `json:"link"`
	Files  []FileInfo `json:"files"`
	Source string     `json:"source"`
}

type ParsedFileName struct {
	Resolution string `json:"resolution"`
	Codec      string `json:"codec"`
	Source     string `json:"source"`
}

type Priority struct {
	Resolution int
	Language   int
}

type Manifest struct {
	ID            string         `json:"id"`
	Version       string         `json:"version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Types         []string       `json:"types"`
	Resources     []string       `json:"resources"`
	Catalogs      []Catalog      `json:"catalogs"`
	BehaviorHints BehaviorHints  `json:"behaviorHints"`
}

type BehaviorHints struct {
	Configurable bool `json:"configurable"`
}

// Catalog represents a catalog definition in the manifest
type Catalog struct {
	Type  string       `json:"type"`
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Extra []ExtraField `json:"extra,omitempty"`
}

// ExtraField represents additional filtering options for catalogs
type ExtraField struct {
	Name       string   `json:"name"`
	Options    []string `json:"options,omitempty"`
	IsRequired bool     `json:"isRequired,omitempty"`
}

// Meta represents a media item in catalog responses
type Meta struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Poster      string   `json:"poster,omitempty"`
	Background  string   `json:"background,omitempty"`
	Logo        string   `json:"logo,omitempty"`
	Description string   `json:"description,omitempty"`
	ReleaseInfo string   `json:"releaseInfo,omitempty"`
	IMDBRating  float64  `json:"imdbRating,omitempty"`
	Runtime     string   `json:"runtime,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Cast        []string `json:"cast,omitempty"`
	Director    []string `json:"director,omitempty"`
	Writer      []string `json:"writer,omitempty"`
	Country     string   `json:"country,omitempty"`
	Language    string   `json:"language,omitempty"`
	Videos      []Video  `json:"videos,omitempty"`
}

// Video represents an episode in a series
type Video struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Released  string `json:"released,omitempty"`
	Overview  string `json:"overview,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

// CatalogResponse represents the response for catalog endpoints
type CatalogResponse struct {
	Metas []Meta `json:"metas"`
}

// MetaResponse represents the response for metadata endpoints
type MetaResponse struct {
	Meta Meta `json:"meta"`
}

// TMDBConfiguration for image URLs
type TMDBConfiguration struct {
	Images TMDBImageConfig `json:"images"`
}

type TMDBImageConfig struct {
	BaseURL       string   `json:"base_url"`
	SecureBaseURL string   `json:"secure_base_url"`
	PosterSizes   []string `json:"poster_sizes"`
	BackdropSizes []string `json:"backdrop_sizes"`
}