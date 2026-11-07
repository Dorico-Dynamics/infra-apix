package apix

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/Infra-Forge/infra-apix/internal/generation"
	"github.com/Infra-Forge/infra-apix/internal/logging"
	"github.com/Infra-Forge/infra-apix/internal/pathtemplate"
)

// RouteMethod represents an HTTP method for an endpoint.
type RouteMethod string

const (
	MethodGet    RouteMethod = http.MethodGet
	MethodPost   RouteMethod = http.MethodPost
	MethodPut    RouteMethod = http.MethodPut
	MethodPatch  RouteMethod = http.MethodPatch
	MethodDelete RouteMethod = http.MethodDelete
)

// HandlerFunc is the canonical typed handler signature. The request pointer may be nil when the route has no body.
type HandlerFunc[TReq any, TResp any] func(ctx context.Context, req *TReq) (TResp, error)

// NoBody is the request or response type of a handler without a body. The
// adapters decode no request into it (the handler receives nil) and write no
// response body for it, and the OpenAPI document shows no content for it.
type NoBody struct{}

// RouteRef stores metadata captured at registration time for later OpenAPI generation.
type RouteRef struct {
	Method      RouteMethod
	Path        string
	OperationID string
	Summary     string
	Description string
	Tags        []string
	Deprecated  bool

	// Request models
	RequestType          reflect.Type
	RequestContentType   string
	ExplicitRequestModel reflect.Type
	RequestExample       any

	// Responses keyed by HTTP status code.
	Responses map[int]*ResponseRef

	// Security requirements (e.g., BearerAuth).
	Security []SecurityRequirement

	// Custom headers expected in success responses.
	SuccessHeaders map[int][]HeaderRef
	SuccessStatus  int

	// Request body requirements
	BodyRequired bool

	// Parameter metadata (path/query/header)
	Parameters []Parameter

	// Underlying handler reflection info (for debugging / advanced extensions).
	HandlerType reflect.Type
}

// ResponseRef describes a response schema.
type ResponseRef struct {
	ModelType         reflect.Type
	ExplicitModelType reflect.Type
	Description       string
	ContentType       string
	Example           any
	Headers           []HeaderRef
}

// HeaderRef models a documented HTTP header.
type HeaderRef struct {
	Name        string
	Description string
	SchemaType  string
	Required    bool
}

// Parameter captures query/path/header metadata for endpoints.
type Parameter struct {
	Name        string
	In          string
	Description string
	Required    bool
	SchemaType  string
	Example     any
}

// SecurityRequirement mirrors OpenAPI security requirement.
type SecurityRequirement struct {
	Name   string
	Scopes []string
}

type routeRegistry struct {
	mu     sync.RWMutex
	routes []*RouteRef
}

var globalRegistry = &routeRegistry{}

// ResetRegistry clears registry content (primarily for tests/CLI runs).
func ResetRegistry() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.routes = nil
	generation.Advance()
}

// RegisterRoute registers a new route metadata entry.
// Executes plugin hooks before adding the route to the registry.
func RegisterRoute(ref *RouteRef) {
	// Execute plugin hooks before registration
	if err := executeOnRouteRegister(ref); err != nil {
		panic(fmt.Sprintf("apix: plugin hook failed during route registration: %v", err))
	}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	if ref.SuccessStatus == 0 {
		ref.SuccessStatus = DefaultSuccessStatus(ref.Method)
	}
	globalRegistry.routes = append(globalRegistry.routes, ref)
	generation.Advance()

	// Log route registration
	logging.GetLogger().RouteRegistered(string(ref.Method), ref.Path, "summary", ref.Summary)
}

// Snapshot returns a copy of registered routes sorted by path+method for deterministic output.
func Snapshot() []*RouteRef {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	out := make([]*RouteRef, len(globalRegistry.routes))
	copy(out, globalRegistry.routes)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return strings.Compare(string(out[i].Method), string(out[j].Method)) < 0
		}
		return strings.Compare(out[i].Path, out[j].Path) < 0
	})
	return out
}

// Options -------------------------------------------------------------------

type RouteOption func(*RouteRef)

type ResponseOption func(*ResponseRef)

// WithSummary sets the endpoint summary.
func WithSummary(summary string) RouteOption {
	return func(r *RouteRef) { r.Summary = summary }
}

// WithDescription sets the endpoint description.
func WithDescription(desc string) RouteOption {
	return func(r *RouteRef) { r.Description = desc }
}

// WithTags appends tags.
func WithTags(tags ...string) RouteOption {
	return func(r *RouteRef) {
		r.Tags = append(r.Tags, tags...)
	}
}

// WithOperationID overrides the generated operation ID.
func WithOperationID(id string) RouteOption {
	return func(r *RouteRef) { r.OperationID = id }
}

// WithDeprecated marks the operation as deprecated.
func WithDeprecated() RouteOption {
	return func(r *RouteRef) { r.Deprecated = true }
}

// WithSecurity adds a security requirement.
func WithSecurity(name string, scopes ...string) RouteOption {
	return func(r *RouteRef) {
		r.Security = append(r.Security, SecurityRequirement{Name: name, Scopes: scopes})
	}
}

// WithSuccessHeaders documents headers for the given status code.
func WithSuccessHeaders(status int, headers ...HeaderRef) RouteOption {
	return func(r *RouteRef) {
		if r.SuccessHeaders == nil {
			r.SuccessHeaders = make(map[int][]HeaderRef)
		}
		r.SuccessHeaders[status] = append(r.SuccessHeaders[status], headers...)
	}
}

// WithSuccessStatus overrides the default success HTTP status code.
func WithSuccessStatus(status int) RouteOption {
	return func(r *RouteRef) {
		if status >= 100 {
			r.SuccessStatus = status
			EnsureResponse(r, status, nil)
		}
	}
}

// WithExplicitRequestModel overrides the inferred request type.
func WithExplicitRequestModel(model any, contentType string) RouteOption {
	return func(r *RouteRef) {
		if model == nil {
			return
		}
		r.ExplicitRequestModel = typeOf(model)
		r.RequestContentType = contentType
	}
}

// WithRequestOverride allows overriding request schema and example when reflection fails.
func WithRequestOverride(model any, contentType string, example any) RouteOption {
	return func(r *RouteRef) {
		if model != nil {
			r.ExplicitRequestModel = typeOf(model)
		}
		if contentType != "" {
			r.RequestContentType = contentType
		}
		if example != nil {
			r.RequestExample = example
		}
	}
}

// WithRequestExample sets an example value for the request body.
// The example will be included in the OpenAPI spec for documentation.
func WithRequestExample(example any) RouteOption {
	return func(r *RouteRef) {
		if example != nil {
			r.RequestExample = example
		}
	}
}

// WithMultipartFormData sets the request content type to multipart/form-data.
// Use this for file upload endpoints.
func WithMultipartFormData() RouteOption {
	return func(r *RouteRef) {
		r.RequestContentType = "multipart/form-data"
	}
}

// WithFormURLEncoded sets the request content type to application/x-www-form-urlencoded.
// Use this for traditional HTML form submissions.
func WithFormURLEncoded() RouteOption {
	return func(r *RouteRef) {
		r.RequestContentType = "application/x-www-form-urlencoded"
	}
}

// WithParameter adds a query/path/header parameter definition.
func WithParameter(param Parameter) RouteOption {
	return func(r *RouteRef) {
		r.Parameters = append(r.Parameters, param)
	}
}

// WithBodyRequired explicitly marks request body requirement.
func WithBodyRequired(required bool) RouteOption {
	return func(r *RouteRef) {
		r.BodyRequired = required
	}
}

// Response helpers -----------------------------------------------------------

// WithDescriptionResponse sets response description.
func WithDescriptionResponse(desc string) ResponseOption {
	return func(resp *ResponseRef) { resp.Description = desc }
}

// WithContentType sets response content type.
func WithContentType(ct string) ResponseOption {
	return func(resp *ResponseRef) { resp.ContentType = ct }
}

// WithExplicitModel overrides inferred response model type.
func WithExplicitModel(model any) ResponseOption {
	return func(resp *ResponseRef) {
		if model != nil {
			resp.ExplicitModelType = typeOf(model)
		}
	}
}

// WithHeaders attaches headers to the response.
func WithHeaders(headers ...HeaderRef) ResponseOption {
	return func(resp *ResponseRef) {
		resp.Headers = append(resp.Headers, headers...)
	}
}

// WithResponseExample sets an example value for the response.
// The example will be included in the OpenAPI spec for documentation.
func WithResponseExample(example any) ResponseOption {
	return func(resp *ResponseRef) {
		if example != nil {
			resp.Example = example
		}
	}
}

// RegisterResponse registers a response schema for the route.
func RegisterResponse(r *RouteRef, status int, modelType reflect.Type, opts ...ResponseOption) {
	if r.Responses == nil {
		r.Responses = make(map[int]*ResponseRef)
	}
	resp := &ResponseRef{
		ModelType:   modelType,
		ContentType: "application/json",
	}
	for _, opt := range opts {
		opt(resp)
	}
	r.Responses[status] = resp
}

// EnsureResponse populates the response entry if it does not yet exist.
func EnsureResponse(r *RouteRef, status int, modelType reflect.Type, opts ...ResponseOption) {
	if r.Responses == nil {
		r.Responses = make(map[int]*ResponseRef)
	}
	if existing, ok := r.Responses[status]; ok && existing != nil {
		if modelType != nil && existing.ModelType == nil {
			existing.ModelType = modelType
		}
		if existing.ContentType == "" {
			existing.ContentType = "application/json"
		}
		return
	}
	RegisterResponse(r, status, modelType, opts...)
}

func typeOf(v any) reflect.Type {
	t := reflect.TypeOf(v)
	if t == nil {
		panic("apix: nil type override")
	}
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}
	return t
}

// Derived defaults -----------------------------------------------------------

// defaultOperationID generates a stable ID combining method and path.
func defaultOperationID(method RouteMethod, path string) string {
	segments := strings.Split(strings.Trim(pathtemplate.StripPatterns(path), "/"), "/")
	for i, segment := range segments {
		// Echo, Gin and Fiber write a path parameter as :id, chi and mux as
		// {id} or {id:pattern}; the builder documents each as {id}.
		segments[i] = strings.TrimPrefix(segment, ":")
	}
	normalized := strings.Join(segments, "_")
	normalized = strings.ReplaceAll(normalized, "{", "")
	normalized = strings.ReplaceAll(normalized, "}", "")
	if normalized == "" {
		normalized = "root"
	}
	return fmt.Sprintf("%s_%s", strings.ToLower(string(method)), normalized)
}

// DefaultOperationID exposes the stable ID generator.
func DefaultOperationID(method RouteMethod, path string) string {
	return defaultOperationID(method, path)
}

func DefaultSuccessStatus(method RouteMethod) int {
	switch method {
	case MethodPost:
		return http.StatusCreated
	case MethodDelete:
		return http.StatusNoContent
	default:
		return http.StatusOK
	}
}
