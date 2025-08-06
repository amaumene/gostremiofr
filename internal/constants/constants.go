// Package constants defines application-wide constants and default values.
package constants

const (
	// Addon metadata
	AddonID          = "gostremiofr.stremio.addon"
	AddonVersion     = "1.0.0"
	AddonName        = "GoStremioFR"
	AddonDescription = "French torrent addon with YGG, TorrentsCSV, AllDebrid integration and TMDB catalogs"

	// Default configuration values
	DefaultPort     = "5000"
	DefaultLogLevel = "info"

	// Cache settings
	DefaultCacheSize = 1000
	DefaultCacheTTL  = 24 // hours

	// Rate limiting
	TMDBRateLimit      = 20 // requests per second
	TMDBRateBurst      = 5  // burst capacity
	AllDebridRateLimit = 10 // requests per second
	AllDebridRateBurst = 2  // burst capacity
)

// TMDBMovieGenres contains TMDB genre IDs for movies.
var TMDBMovieGenres = []string{
	"28",    // Action
	"12",    // Adventure
	"16",    // Animation
	"35",    // Comedy
	"80",    // Crime
	"99",    // Documentary
	"18",    // Drama
	"10751", // Family
	"14",    // Fantasy
	"36",    // History
	"27",    // Horror
	"10402", // Music
	"9648",  // Mystery
	"10749", // Romance
	"878",   // Science Fiction
	"10770", // TV Movie
	"53",    // Thriller
	"10752", // War
	"37",    // Western
}

// TMDBTVGenres contains TMDB genre IDs for TV series.
var TMDBTVGenres = []string{
	"10759", // Action & Adventure
	"16",    // Animation
	"35",    // Comedy
	"80",    // Crime
	"99",    // Documentary
	"18",    // Drama
	"10751", // Family
	"10762", // Kids
	"9648",  // Mystery
	"10763", // News
	"10764", // Reality
	"10765", // Sci-Fi & Fantasy
	"10766", // Soap
	"10767", // Talk
	"10768", // War & Politics
	"37",    // Western
}

// DefaultResolutions lists supported resolutions in order of preference.
var DefaultResolutions = []string{
	"2160p",
	"1080p",
	"720p",
	"480p",
}
