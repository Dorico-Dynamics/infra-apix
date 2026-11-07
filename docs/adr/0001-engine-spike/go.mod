module github.com/Infra-Forge/infra-apix/docs/adr/0001-engine-spike

go 1.27.3

require (
	github.com/Infra-Forge/infra-apix v1.9.2
	github.com/danielgtaylor/huma/v2 v2.40.0
	github.com/getkin/kin-openapi v0.149.0
	github.com/go-chi/chi/v5 v5.5.0
	github.com/go-playground/validator/v10 v10.30.5
)

require (
	github.com/gabriel-vasile/mimetype v1.4.16 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// The spike measures this checkout of apix, not a published version.
replace github.com/Infra-Forge/infra-apix => ../../..
