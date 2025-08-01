package models

// Stream represents a single playable stream in Stremio format.
type Stream struct {
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url"`
}

// StreamResponse is the response format for stream endpoints.
type StreamResponse struct {
	Streams []Stream `json:"streams"`
}
