package main

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v4"
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
	e := echo.New()
	api := humaecho.NewV4(e, huma.DefaultConfig("probe", "1.0.0"))
	huma.Post(api, "/items", func(_ context.Context, req *in) (*out, error) {
		o := &out{}
		o.Body.ID = req.Body.Name
		return o, nil
	})
	_ = e.Start(":0")
}
