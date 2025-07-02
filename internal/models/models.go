package models

import "time"

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
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Size   int64  `json:"size"`
	Hash   string `json:"hash,omitempty"`
	Source string `json:"source,omitempty"`
}

type SharewoodTorrent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	InfoHash    string    `json:"infoHash"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	Seeders     int       `json:"seeders"`
	Leechers    int       `json:"leechers"`
	Language    string    `json:"language"`
	DownloadURL string    `json:"downloadUrl"`
	CreatedAt   time.Time `json:"createdAt"`
	Source      string    `json:"source"`
}

type TorrentResults struct {
	CompleteSeriesTorrents []TorrentInfo
	CompleteSeasonTorrents []TorrentInfo
	EpisodeTorrents        []TorrentInfo
	MovieTorrents          []TorrentInfo
}

type SharewoodResults struct {
	CompleteSeriesTorrents []SharewoodTorrent
	CompleteSeasonTorrents []SharewoodTorrent
	EpisodeTorrents        []SharewoodTorrent
	MovieTorrents          []SharewoodTorrent
}

type CombinedTorrentResults struct {
	CompleteSeriesTorrents []TorrentInfo
	CompleteSeasonTorrents []TorrentInfo
	EpisodeTorrents        []TorrentInfo
	MovieTorrents          []TorrentInfo
}

type TMDBData struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	FrenchTitle string `json:"french_title"`
}

type TMDBFindResponse struct {
	MovieResults []TMDBMovie `json:"movie_results"`
	TVResults    []TMDBTV    `json:"tv_results"`
}

type TMDBMovie struct {
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
}

type TMDBTV struct {
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
}

type MagnetInfo struct {
	Hash   string
	Title  string
	Source string
}

type AllDebridMagnet struct {
	Hash   string  `json:"hash"`
	Ready  bool    `json:"ready"`
	Name   string  `json:"name"`
	Size   float64 `json:"size"`
	ID     string  `json:"id"`
}

type ProcessedMagnet struct {
	Hash   string
	Ready  bool
	Name   string
	Size   float64
	ID     string
	Source string
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
	Codec      int
}

type Manifest struct {
	ID            string         `json:"id"`
	Version       string         `json:"version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Types         []string       `json:"types"`
	Resources     []string       `json:"resources"`
	Catalogs      []interface{}  `json:"catalogs"`
	BehaviorHints BehaviorHints  `json:"behaviorHints"`
}

type BehaviorHints struct {
	Configurable bool `json:"configurable"`
}