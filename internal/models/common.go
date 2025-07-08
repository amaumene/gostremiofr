package models

// CastMember represents a cast member in credits
type CastMember struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	Order     int    `json:"order"`
}

// CrewMember represents a crew member in credits
type CrewMember struct {
	Name       string `json:"name"`
	Department string `json:"department"`
	Job        string `json:"job"`
}

// Credits represents cast and crew information
type Credits struct {
	Cast []CastMember `json:"cast"`
	Crew []CrewMember `json:"crew"`
}

// ProductionCountry represents a production country
type ProductionCountry struct {
	ISO  string `json:"iso_3166_1"`
	Name string `json:"name"`
}

// SpokenLanguage represents a spoken language
type SpokenLanguage struct {
	ISO  string `json:"iso_639_1"`
	Name string `json:"name"`
}

// ExternalIds represents external IDs for media
type ExternalIds struct {
	IMDBId string `json:"imdb_id"`
}