package main

import (
	"context"
	"net/http"

	chiadapter "github.com/Infra-Forge/infra-apix/chi"
	"github.com/go-chi/chi/v5"
)

type in struct {
	Name string `json:"name"`
}

type out struct {
	ID string `json:"id"`
}

func main() {
	r := chi.NewRouter()
	chiadapter.Post(chiadapter.New(r), "/items", func(_ context.Context, req *in) (out, error) { return out{ID: req.Name}, nil })
	_ = http.ListenAndServe(":0", r)
}
