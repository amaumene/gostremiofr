package models

type Manifest struct {
	ID            string        `json:"id"`
	Version       string        `json:"version"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Types         []string      `json:"types"`
	Resources     []string      `json:"resources"`
	Catalogs      []Catalog     `json:"catalogs"`
	BehaviorHints BehaviorHints `json:"behaviorHints"`
	IDPrefixes    []string      `json:"idPrefixes,omitempty"`
	Background    string        `json:"background,omitempty"`
	Logo          string        `json:"logo,omitempty"`
	ContactEmail  string        `json:"contactEmail,omitempty"`
}

type BehaviorHints struct {
	Configurable          bool `json:"configurable"`
	ConfigurationRequired bool `json:"configurationRequired,omitempty"`
}

type Catalog struct {
	Type  string       `json:"type"`
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Extra []ExtraField `json:"extra,omitempty"`
}

type ExtraField struct {
	Name       string   `json:"name"`
	Options    []string `json:"options,omitempty"`
	IsRequired bool     `json:"isRequired,omitempty"`
}

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

type Video struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Released  string `json:"released,omitempty"`
	Overview  string `json:"overview,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

type CatalogResponse struct {
	Metas []Meta `json:"metas"`
}

type MetaResponse struct {
	Meta Meta `json:"meta"`
}
