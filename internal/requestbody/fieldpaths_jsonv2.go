//go:build goexperiment.jsonv2

package requestbody

// fieldPaths reports whether json.UnmarshalTypeError.Field is the JSON path of
// the rejected value. The jsonv2-based encoding/json, the default in Go 1.27,
// reports the path as the client wrote it, such as "items.0.amount".
const fieldPaths = true
