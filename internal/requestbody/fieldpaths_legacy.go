//go:build !goexperiment.jsonv2

package requestbody

// fieldPaths reports whether json.UnmarshalTypeError.Field is the JSON path of
// the rejected value. The legacy encoding/json (GOEXPERIMENT=nojsonv2) puts the
// Go names of embedded structs into it, so it is not shown to clients.
const fieldPaths = false
