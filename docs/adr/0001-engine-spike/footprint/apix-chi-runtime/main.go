package main

import (
	"context"
	"net/http"

	chiadapter "github.com/Infra-Forge/infra-apix/chi"
	"github.com/Infra-Forge/infra-apix/runtime"
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
	docs, err := runtime.NewHandler(runtime.Config{Title: "probe", Version: "1.0.0", SpecPath: "/openapi.json"})
	if err != nil {
		panic(err)
	}
	r.Handle("/openapi.json", docs)
	_ = http.ListenAndServe(":0", r)
}
