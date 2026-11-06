package echo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/requestbody"
	"github.com/labstack/echo/v4"
)

// RequestDecoder decodes the HTTP request body into dst, enforcing validation rules.
type RequestDecoder func(ctx context.Context, c echo.Context, dst any) error

// ResponseEncoder writes the response payload using the provided status code.
type ResponseEncoder func(ctx context.Context, c echo.Context, status int, payload any, ref *apix.RouteRef) error

// ErrorTransformer transforms handler errors into Echo-compatible errors.
type ErrorTransformer func(err error) error

// Options configures adapter behaviour.
type Options struct {
	Decoder         RequestDecoder
	ResponseEncoder ResponseEncoder
	ErrorHandler    ErrorTransformer
	// UseProblemDetails enables RFC 9457 Problem Details encoding for errors.
	// When enabled, errors implementing StatusCoder will be serialized as
	// application/problem+json instead of plain text.
	UseProblemDetails bool
}

// EchoAdapter integrates apix route registration with echo.Echo.
type EchoAdapter struct {
	e    *echo.Echo
	opts Options
}

// New constructs an EchoAdapter with optional overrides.
func New(e *echo.Echo, opts ...Options) *EchoAdapter {
	adapter := &EchoAdapter{e: e}
	if len(opts) > 0 {
		adapter.opts = opts[0]
	}
	return adapter
}

// Echo exposes the underlying echo.Echo instance.
func (a *EchoAdapter) Echo() *echo.Echo { return a.e }

// Register adds a handler for the provided method and path.
func Register[TReq any, TResp any](a *EchoAdapter, method apix.RouteMethod, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	if a == nil || a.e == nil {
		panic("apix/echo: adapter not initialised")
	}

	ref := &apix.RouteRef{
		Method:         method,
		Path:           path,
		Responses:      make(map[int]*apix.ResponseRef),
		SuccessHeaders: make(map[int][]apix.HeaderRef),
		HandlerType:    reflect.TypeOf(handler),
	}

	reqType := typeOf[TReq]()
	if reqType != nil && !isNoBody(reqType) {
		ref.RequestType = reqType
		if ref.RequestContentType == "" {
			ref.RequestContentType = "application/json"
		}
	}

	for _, opt := range opts {
		opt(ref)
	}

	if ref.SuccessStatus == 0 {
		ref.SuccessStatus = apix.DefaultSuccessStatus(method)
	}

	respType := typeOf[TResp]()
	if respType == nil {
		respType = reflect.TypeOf(struct{}{})
	}
	apix.EnsureResponse(ref, ref.SuccessStatus, respType)

	if ref.OperationID == "" {
		ref.OperationID = apix.DefaultOperationID(ref.Method, ref.Path)
	}

	apix.RegisterRoute(ref)

	a.e.Add(string(method), path, buildEchoHandler(a, handler, ref))
}

// Get registers a GET handler.
func Get[TResp any](a *EchoAdapter, path string, handler apix.HandlerFunc[apix.NoBody, TResp], opts ...apix.RouteOption) {
	Register[apix.NoBody, TResp](a, apix.MethodGet, path, handler, opts...)
}

// Post registers a POST handler.
func Post[TReq any, TResp any](a *EchoAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPost, path, handler, opts...)
}

// Put registers a PUT handler.
func Put[TReq any, TResp any](a *EchoAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPut, path, handler, opts...)
}

// Patch registers a PATCH handler.
func Patch[TReq any, TResp any](a *EchoAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPatch, path, handler, opts...)
}

// Delete registers a DELETE handler.
func Delete[TResp any](a *EchoAdapter, path string, handler apix.HandlerFunc[apix.NoBody, TResp], opts ...apix.RouteOption) {
	Register[apix.NoBody, TResp](a, apix.MethodDelete, path, handler, opts...)
}

func buildEchoHandler[TReq any, TResp any](a *EchoAdapter, handler apix.HandlerFunc[TReq, TResp], ref *apix.RouteRef) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		var reqPtr *TReq
		if ref.RequestType != nil {
			reqVal := new(TReq)
			if err := a.decode(ctx, c, reqVal); err != nil {
				return a.transformError(err)
			}
			reqPtr = reqVal
		}

		resp, err := handler(ctx, reqPtr)
		if err != nil {
			return a.transformError(err)
		}

		return a.encode(ctx, c, ref.SuccessStatus, resp, ref)
	}
}

func (a *EchoAdapter) decode(ctx context.Context, c echo.Context, dst any) error {
	if dec := a.opts.Decoder; dec != nil {
		return dec(ctx, c, dst)
	}
	return defaultDecoder(ctx, c, dst)
}

func (a *EchoAdapter) encode(ctx context.Context, c echo.Context, status int, payload any, ref *apix.RouteRef) error {
	if enc := a.opts.ResponseEncoder; enc != nil {
		return enc(ctx, c, status, payload, ref)
	}
	return defaultEncoder(ctx, c, status, payload)
}

func (a *EchoAdapter) transformError(err error) error {
	if err == nil {
		return nil
	}
	if tr := a.opts.ErrorHandler; tr != nil {
		return tr(err)
	}
	return defaultErrorTransformer(err, a.opts.UseProblemDetails)
}

// defaultErrorTransformer converts apix errors to Echo HTTPError format.
// If useProblemDetails is true, wraps the error with ProblemDetails metadata.
func defaultErrorTransformer(err error, useProblemDetails bool) error {
	// Check for StatusCoder interface (new pattern)
	var statusCoder apix.StatusCoder
	if errors.As(err, &statusCoder) {
		// If Problem Details is enabled, wrap with ProblemDetails
		// Echo will handle the serialization via custom error handler
		if useProblemDetails {
			return apix.ToProblemDetails(err)
		}
		return echo.NewHTTPError(statusCoder.HTTPStatus(), err.Error())
	}
	// Return error as-is for Echo's default error handler
	return err
}

func defaultDecoder(ctx context.Context, c echo.Context, dst any) error {
	if err := requestbody.Decode(c.Request().Body, dst); err != nil {
		return err
	}
	if v := c.Echo().Validator; v != nil {
		return requestbody.Invalid(v.Validate(dst))
	}
	return nil
}

func defaultEncoder(ctx context.Context, c echo.Context, status int, payload any) error {
	if status == http.StatusNoContent || payload == nil || isNoBody(reflect.TypeOf(payload)) {
		return c.NoContent(status)
	}
	return c.JSON(status, payload)
}

func typeOf[T any]() reflect.Type {
	t := reflect.TypeFor[T]()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

var noBodyType = reflect.TypeOf(apix.NoBody{})

func isNoBody(t reflect.Type) bool {
	if t == nil {
		return true
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t == noBodyType
}

// ProblemDetailsErrorHandler returns an Echo error handler that serializes
// ProblemDetails errors as application/problem+json.
// Use this with e.HTTPErrorHandler when UseProblemDetails is enabled.
//
// Example:
//
//	e := echo.New()
//	adapter := echoadapter.New(e, echoadapter.Options{UseProblemDetails: true})
//	e.HTTPErrorHandler = echoadapter.ProblemDetailsErrorHandler(e.HTTPErrorHandler)
func ProblemDetailsErrorHandler(fallback echo.HTTPErrorHandler) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		// Check if this is a ProblemDetails error
		var problem *apix.ProblemDetails
		if errors.As(err, &problem) {
			// Serialize as RFC 9457
			c.Response().Header().Set("Content-Type", "application/problem+json")
			c.Response().WriteHeader(problem.HTTPStatus())
			if encErr := json.NewEncoder(c.Response()).Encode(problem); encErr != nil {
				// Log encoding error if available
				c.Logger().Errorf("failed to encode problem details: %v", encErr)
			}
			return
		}

		// Fall back to default Echo error handler
		if fallback != nil {
			fallback(err, c)
			return
		}

		// No fallback provided - create a safe default Problem Details response
		// Log the original error server-side for debugging
		c.Logger().Errorf("unhandled error (no fallback): %v", err)

		// Return generic message to client (don't leak internal error details)
		defaultProblem := &apix.ProblemDetails{
			Status: http.StatusInternalServerError,
			Title:  "Internal Server Error",
			Detail: "An internal error occurred",
		}
		c.Response().Header().Set("Content-Type", "application/problem+json")
		c.Response().WriteHeader(http.StatusInternalServerError)
		if encErr := json.NewEncoder(c.Response()).Encode(defaultProblem); encErr != nil {
			// Last resort: log the error and ensure status is set
			c.Logger().Errorf("failed to encode default problem details: %v", encErr)
		}
	}
}
