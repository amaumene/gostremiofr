// Package models defines data structures for AllDebrid API responses.
package models

type AllDebridMagnet struct {
	Hash       string        `json:"hash"`
	Status     string        `json:"status"`
	StatusCode int           `json:"statusCode"`
	Filename   string        `json:"filename"`
	Size       float64       `json:"size"`
	ID         int64         `json:"id"`
	Links      []interface{} `json:"links"`
}
