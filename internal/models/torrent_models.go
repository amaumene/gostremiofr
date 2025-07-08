// Package models defines data structures for torrent information and processing.
package models

type TorrentInfo struct {
	ID     string
	Title  string
	Hash   string
	Source string
	Size   int64 // Size in bytes
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

type MagnetInfo struct {
	Hash   string
	Title  string
	Source string
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

type EpisodeFile struct {
	Name          string
	Size          float64
	Link          string
	Source        string
	Season        int
	Episode       int
	Resolution    string
	Language      string
	SeasonTorrent TorrentInfo // Reference to the complete season torrent
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
