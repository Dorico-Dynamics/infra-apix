// Package models is a test fixture. Its types and the ones in
// openapi/internal/collision/a/models share component names: "models_Money"
// and "models_Note".
package models

// Money is a decimal amount with its currency.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// Note is a titled comment.
type Note struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
