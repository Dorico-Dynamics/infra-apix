// Package models is a test fixture. Its types and the ones in
// openapi/internal/collision/b/models share component names: "models_Money"
// and "models_Note".
package models

// Money is an amount in minor units.
type Money struct {
	Minor int64 `json:"minor"`
}

// Note is free text kept with a record.
type Note struct {
	Text string `json:"text"`
}
