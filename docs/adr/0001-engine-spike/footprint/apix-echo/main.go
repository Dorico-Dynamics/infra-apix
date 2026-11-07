package main

import (
	"context"

	echoadapter "github.com/Infra-Forge/infra-apix/echo"
	"github.com/labstack/echo/v4"
)

type in struct {
	Name string `json:"name"`
}

type out struct {
	ID string `json:"id"`
}

func main() {
	e := echo.New()
	echoadapter.Post(echoadapter.New(e), "/items", func(_ context.Context, req *in) (out, error) { return out{ID: req.Name}, nil })
	_ = e.Start(":0")
}
