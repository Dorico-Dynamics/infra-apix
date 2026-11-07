package main

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

type in struct {
	Body struct {
		Name string `json:"name"`
	}
}
type out struct {
	Body struct {
		ID string `json:"id"`
	}
}

func main() {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("probe", "1.0.0"))
	huma.Post(api, "/items", func(ctx context.Context, req *in) (*out, error) {
		o := &out{}
		o.Body.ID = req.Body.Name
		return o, nil
	})
	_ = http.ListenAndServe(":0", r)
}
