package apix

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
)

// ErrorResponse is the standard error response schema for 4xx/5xx responses.
type ErrorResponse struct {
	Code    string `json:"code" description:"Error code identifier"`
	Message string `json:"message" description:"Human-readable error message"`
	Details any    `json:"details,omitempty" description:"Additional error details"`
}

var errorResponseType = reflect.TypeOf(ErrorResponse{})

// StatusCoder is an interface for errors that can provide an HTTP status code.
// Framework adapters check for this interface to determine the appropriate HTTP response status.
// HTTPStatus must return a 4xx or 5xx status: the adapters log an error with
// any other status, such as the 0 of a ProblemDetails without Status, and
// answer a generic 500 instead.
//
// Example:
//
//	type MyError struct {
//	    message string
//	}
//
//	func (e *MyError) Error() string { return e.message }
//	func (e *MyError) HTTPStatus() int { return http.StatusBadRequest }
type StatusCoder interface {
	error
	HTTPStatus() int
}

// HTTPError is the exported error type that implements StatusCoder.
// It provides HTTP status code, message, and optional error code for structured error responses.
//
// Use the convenience constructors (NotFound, BadRequest, etc.) or WithStatus for custom status codes.
type HTTPError struct {
	Status  int    // HTTP status code
	Message string // Human-readable error message
	Code    string // Optional error code identifier (e.g., "RESOURCE_NOT_FOUND")
	Err     error  // Optional wrapped error for error chain inspection
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("http %d [%s]: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("http %d: %s", e.Status, e.Message)
}

// HTTPStatus implements the StatusCoder interface.
func (e *HTTPError) HTTPStatus() int {
	return e.Status
}

// Unwrap implements error unwrapping for errors.Is and errors.As.
func (e *HTTPError) Unwrap() error {
	return e.Err
}

// Convenience constructors for common HTTP errors

// NotFound creates a 404 Not Found error.
//
// Example:
//
//	return apix.NotFound("user not found")
func NotFound(message string) error {
	return &HTTPError{
		Status:  http.StatusNotFound,
		Message: message,
		Code:    "NOT_FOUND",
	}
}

// BadRequest creates a 400 Bad Request error.
//
// Example:
//
//	return apix.BadRequest("invalid email format")
func BadRequest(message string) error {
	return &HTTPError{
		Status:  http.StatusBadRequest,
		Message: message,
		Code:    "BAD_REQUEST",
	}
}

// Conflict creates a 409 Conflict error.
//
// Example:
//
//	return apix.Conflict("user already exists")
func Conflict(message string) error {
	return &HTTPError{
		Status:  http.StatusConflict,
		Message: message,
		Code:    "CONFLICT",
	}
}

// Unauthorized creates a 401 Unauthorized error.
//
// Example:
//
//	return apix.Unauthorized("authentication required")
func Unauthorized(message string) error {
	return &HTTPError{
		Status:  http.StatusUnauthorized,
		Message: message,
		Code:    "UNAUTHORIZED",
	}
}

// Forbidden creates a 403 Forbidden error.
//
// Example:
//
//	return apix.Forbidden("insufficient permissions")
func Forbidden(message string) error {
	return &HTTPError{
		Status:  http.StatusForbidden,
		Message: message,
		Code:    "FORBIDDEN",
	}
}

// UnprocessableEntity creates a 422 Unprocessable Entity error.
//
// Example:
//
//	return apix.UnprocessableEntity("validation failed")
func UnprocessableEntity(message string) error {
	return &HTTPError{
		Status:  http.StatusUnprocessableEntity,
		Message: message,
		Code:    "UNPROCESSABLE_ENTITY",
	}
}

// InternalServerError creates a 500 Internal Server Error.
//
// Example:
//
//	return apix.InternalServerError("database connection failed")
func InternalServerError(message string) error {
	return &HTTPError{
		Status:  http.StatusInternalServerError,
		Message: message,
		Code:    "INTERNAL_SERVER_ERROR",
	}
}

// WithStatus creates an HTTPError with a custom status code.
// Use this for status codes not covered by the convenience constructors.
//
// Example:
//
//	return apix.WithStatus(http.StatusTeapot, "I'm a teapot")
func WithStatus(status int, message string) error {
	return &HTTPError{
		Status:  status,
		Message: message,
	}
}

// WrapError wraps an existing error with HTTP status information.
// The wrapped error can be inspected using errors.Is and errors.As.
//
// Example:
//
//	if err := db.Query(); err != nil {
//	    return apix.WrapError(err, http.StatusInternalServerError, "database query failed")
//	}
func WrapError(err error, status int, message string) error {
	return &HTTPError{
		Status:  status,
		Message: message,
		Err:     err,
	}
}

// WithStandardErrors adds standard 4xx/5xx error responses to a route.
// This includes 400, 422, 500 with the shared ErrorResponse schema.
func WithStandardErrors() RouteOption {
	return func(r *RouteRef) {
		EnsureResponse(r, http.StatusBadRequest, errorResponseType,
			WithDescriptionResponse("Bad Request - Invalid input"))
		EnsureResponse(r, http.StatusUnprocessableEntity, errorResponseType,
			WithDescriptionResponse("Unprocessable Entity - Validation failed"))
		EnsureResponse(r, http.StatusInternalServerError, errorResponseType,
			WithDescriptionResponse("Internal Server Error"))
	}
}

// WithErrorResponse adds a specific error response with the standard ErrorResponse schema.
func WithErrorResponse(status int, description string) RouteOption {
	return func(r *RouteRef) {
		EnsureResponse(r, status, errorResponseType, WithDescriptionResponse(description))
	}
}

// WithCustomErrorResponse adds a custom error response with a specific model type.
func WithCustomErrorResponse(status int, model any, description string) RouteOption {
	return func(r *RouteRef) {
		modelType := typeOf(model)
		EnsureResponse(r, status, modelType, WithDescriptionResponse(description))
	}
}

// Common error response helpers

// WithBadRequestError adds a 400 Bad Request error response.
func WithBadRequestError(description string) RouteOption {
	if description == "" {
		description = "Bad Request - Invalid input"
	}
	return WithErrorResponse(http.StatusBadRequest, description)
}

// WithUnauthorizedError adds a 401 Unauthorized error response.
func WithUnauthorizedError(description string) RouteOption {
	if description == "" {
		description = "Unauthorized - Authentication required"
	}
	return WithErrorResponse(http.StatusUnauthorized, description)
}

// WithForbiddenError adds a 403 Forbidden error response.
func WithForbiddenError(description string) RouteOption {
	if description == "" {
		description = "Forbidden - Insufficient permissions"
	}
	return WithErrorResponse(http.StatusForbidden, description)
}

// WithNotFoundError adds a 404 Not Found error response.
func WithNotFoundError(description string) RouteOption {
	if description == "" {
		description = "Not Found - Resource does not exist"
	}
	return WithErrorResponse(http.StatusNotFound, description)
}

// WithConflictError adds a 409 Conflict error response.
func WithConflictError(description string) RouteOption {
	if description == "" {
		description = "Conflict - Resource already exists"
	}
	return WithErrorResponse(http.StatusConflict, description)
}

// WithValidationError adds a 422 Unprocessable Entity error response.
func WithValidationError(description string) RouteOption {
	if description == "" {
		description = "Unprocessable Entity - Validation failed"
	}
	return WithErrorResponse(http.StatusUnprocessableEntity, description)
}

// WithInternalServerError adds a 500 Internal Server Error response.
func WithInternalServerError(description string) RouteOption {
	if description == "" {
		description = "Internal Server Error"
	}
	return WithErrorResponse(http.StatusInternalServerError, description)
}

// WithServiceUnavailableError adds a 503 Service Unavailable error response.
func WithServiceUnavailableError(description string) RouteOption {
	if description == "" {
		description = "Service Unavailable - Temporary outage"
	}
	return WithErrorResponse(http.StatusServiceUnavailable, description)
}

// ProblemDetails implements RFC 9457 (Problem Details for HTTP APIs).
// It provides a machine-readable format for HTTP API error responses.
//
// See: https://www.rfc-editor.org/rfc/rfc9457.html
//
// Example:
//
//	problem := &apix.ProblemDetails{
//	    Type:     "https://example.com/probs/out-of-credit",
//	    Title:    "You do not have enough credit.",
//	    Status:   403,
//	    Detail:   "Your current balance is 30, but that costs 50.",
//	    Instance: "/account/12345/msgs/abc",
//	}
type ProblemDetails struct {
	// Type is a URI reference that identifies the problem type.
	// When dereferenced, it SHOULD provide human-readable documentation.
	// Defaults to "about:blank" if not specified.
	Type string `json:"type,omitempty"`

	// Title is a short, human-readable summary of the problem type.
	// It SHOULD NOT change from occurrence to occurrence of the problem.
	Title string `json:"title,omitempty"`

	// Status is the HTTP status code for this occurrence of the problem.
	// This is advisory; the actual HTTP response status is authoritative.
	Status int `json:"status,omitempty"`

	// Detail is a human-readable explanation specific to this occurrence.
	Detail string `json:"detail,omitempty"`

	// Instance is a URI reference that identifies the specific occurrence.
	// It may or may not yield further information if dereferenced.
	Instance string `json:"instance,omitempty"`

	// Extensions holds additional problem-specific extension members.
	// These are serialized at the top level alongside the standard members.
	Extensions map[string]any `json:"-"`
}

// Error implements the error interface.
func (p *ProblemDetails) Error() string {
	if p.Detail != "" {
		return p.Detail
	}
	if p.Title != "" {
		return p.Title
	}
	return fmt.Sprintf("http %d", p.Status)
}

// HTTPStatus implements the StatusCoder interface.
func (p *ProblemDetails) HTTPStatus() int {
	return p.Status
}

// MarshalJSON implements custom JSON marshaling to include extension members.
func (p *ProblemDetails) MarshalJSON() ([]byte, error) {
	// Create a map with standard members
	m := make(map[string]any)

	if p.Type != "" {
		m["type"] = p.Type
	} else {
		m["type"] = "about:blank"
	}

	if p.Title != "" {
		m["title"] = p.Title
	}

	if p.Status != 0 {
		m["status"] = p.Status
	}

	if p.Detail != "" {
		m["detail"] = p.Detail
	}

	if p.Instance != "" {
		m["instance"] = p.Instance
	}

	// Add extension members (skip reserved field names to prevent overwrites)
	reservedFields := map[string]bool{
		"type":     true,
		"title":    true,
		"status":   true,
		"detail":   true,
		"instance": true,
	}

	for k, v := range p.Extensions {
		// Skip extension keys that collide with reserved RFC 9457 fields
		if reservedFields[k] {
			continue
		}
		m[k] = v
	}

	return json.Marshal(m)
}

// ToProblemDetails converts an HTTPError to RFC 9457 ProblemDetails format.
// If the error is not an HTTPError, it returns a generic 500 problem.
func ToProblemDetails(err error) *ProblemDetails {
	if err == nil {
		return nil
	}

	// Check if it's already a ProblemDetails (handles wrapped errors)
	var pd *ProblemDetails
	if errors.As(err, &pd) {
		return pd
	}

	// Check if it's an HTTPError (handles wrapped errors)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		result := &ProblemDetails{
			Status: httpErr.Status,
			Detail: httpErr.Message,
		}

		// Set title based on status code
		result.Title = http.StatusText(httpErr.Status)

		// Use error code as type if available
		if httpErr.Code != "" {
			result.Type = fmt.Sprintf("about:blank#%s", httpErr.Code)
		}

		return result
	}

	// Try to extract status from StatusCoder interface (handles wrapped errors)
	status := http.StatusInternalServerError
	var statusCoder StatusCoder
	if errors.As(err, &statusCoder) {
		status = statusCoder.HTTPStatus()
	}

	return &ProblemDetails{
		Status: status,
		Title:  http.StatusText(status),
		Detail: err.Error(),
	}
}

// NewProblemDetails creates a new ProblemDetails with the given status and detail.
// This is a convenience constructor for simple cases.
//
// Example:
//
//	return apix.NewProblemDetails(403, "Insufficient credit", "Your balance is 30, but that costs 50")
func NewProblemDetails(status int, title, detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: status,
		Title:  title,
		Detail: detail,
	}
}

// WithExtension adds an extension member to the ProblemDetails.
// Extension members are serialized at the top level alongside standard members.
//
// Example:
//
//	problem := apix.NewProblemDetails(403, "Out of credit", "Insufficient balance")
//	problem.WithExtension("balance", 30).WithExtension("cost", 50)
func (p *ProblemDetails) WithExtension(key string, value any) *ProblemDetails {
	if p.Extensions == nil {
		p.Extensions = make(map[string]any)
	}
	p.Extensions[key] = value
	return p
}

// WithType sets the Type URI for the ProblemDetails.
func (p *ProblemDetails) WithType(typeURI string) *ProblemDetails {
	p.Type = typeURI
	return p
}

// WithInstance sets the Instance URI for the ProblemDetails.
func (p *ProblemDetails) WithInstance(instance string) *ProblemDetails {
	p.Instance = instance
	return p
}
