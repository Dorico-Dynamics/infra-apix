package gin

import (
	"context"
	"net/http"
	"reflect"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/errorhandler"
	"github.com/Infra-Forge/infra-apix/internal/requestbody"
	"github.com/gin-gonic/gin"
)

// RequestDecoder decodes the HTTP request body into dst, enforcing validation rules.
type RequestDecoder func(ctx context.Context, c *gin.Context, dst any) error

// ResponseEncoder writes the response payload using the provided status code.
type ResponseEncoder func(ctx context.Context, c *gin.Context, status int, payload any, ref *apix.RouteRef) error

// ErrorHandler handles errors from the typed handler.
type ErrorHandler func(ctx context.Context, c *gin.Context, err error)

// Options configures adapter behaviour.
type Options struct {
	Decoder         RequestDecoder
	ResponseEncoder ResponseEncoder
	ErrorHandler    ErrorHandler
	// UseProblemDetails enables RFC 9457 Problem Details encoding for errors.
	// When enabled, errors implementing StatusCoder will be serialized as
	// application/problem+json instead of plain text.
	UseProblemDetails bool
}

// GinAdapter integrates apix route registration with gin.Engine.
type GinAdapter struct {
	e    *gin.Engine
	opts Options
}

// New constructs a GinAdapter with optional overrides.
func New(e *gin.Engine, opts ...Options) *GinAdapter {
	adapter := &GinAdapter{e: e}
	if len(opts) > 0 {
		adapter.opts = opts[0]
	}
	return adapter
}

// Engine exposes the underlying gin.Engine instance.
func (a *GinAdapter) Engine() *gin.Engine { return a.e }

// Register adds a handler for the provided method and path.
func Register[TReq any, TResp any](a *GinAdapter, method apix.RouteMethod, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	if a == nil || a.e == nil {
		panic("apix/gin: adapter not initialised")
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

	a.e.Handle(string(method), path, buildGinHandler(a, handler, ref))
}

// Get registers a GET handler.
func Get[TResp any](a *GinAdapter, path string, handler apix.HandlerFunc[apix.NoBody, TResp], opts ...apix.RouteOption) {
	Register[apix.NoBody, TResp](a, apix.MethodGet, path, handler, opts...)
}

// Post registers a POST handler.
func Post[TReq any, TResp any](a *GinAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPost, path, handler, opts...)
}

// Put registers a PUT handler.
func Put[TReq any, TResp any](a *GinAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPut, path, handler, opts...)
}

// Patch registers a PATCH handler.
func Patch[TReq any, TResp any](a *GinAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPatch, path, handler, opts...)
}

// Delete registers a DELETE handler.
func Delete[TResp any](a *GinAdapter, path string, handler apix.HandlerFunc[apix.NoBody, TResp], opts ...apix.RouteOption) {
	Register[apix.NoBody, TResp](a, apix.MethodDelete, path, handler, opts...)
}

func buildGinHandler[TReq any, TResp any](a *GinAdapter, handler apix.HandlerFunc[TReq, TResp], ref *apix.RouteRef) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		var reqPtr *TReq
		if ref.RequestType != nil {
			reqVal := new(TReq)
			if err := a.decode(ctx, c, reqVal); err != nil {
				a.handleError(ctx, c, err)
				return
			}
			reqPtr = reqVal
		}

		resp, err := handler(ctx, reqPtr)
		if err != nil {
			a.handleError(ctx, c, err)
			return
		}

		if err := a.encode(ctx, c, ref.SuccessStatus, resp, ref); err != nil {
			a.handleError(ctx, c, err)
		}
	}
}

func (a *GinAdapter) decode(ctx context.Context, c *gin.Context, dst any) error {
	if dec := a.opts.Decoder; dec != nil {
		return dec(ctx, c, dst)
	}
	return defaultDecoder(ctx, c, dst)
}

func (a *GinAdapter) encode(ctx context.Context, c *gin.Context, status int, payload any, ref *apix.RouteRef) error {
	if enc := a.opts.ResponseEncoder; enc != nil {
		return enc(ctx, c, status, payload, ref)
	}
	return defaultEncoder(ctx, c, status, payload)
}

func (a *GinAdapter) handleError(ctx context.Context, c *gin.Context, err error) {
	if err == nil {
		return
	}
	if handler := a.opts.ErrorHandler; handler != nil {
		handler(ctx, c, err)
		return
	}
	defaultErrorHandler(ctx, c, err, a.opts.UseProblemDetails)
}

func defaultDecoder(ctx context.Context, c *gin.Context, dst any) error {
	// Gin has built-in validation via ShouldBind, but we use manual JSON decoding for consistency
	// Users can provide custom decoder if they want Gin's validation
	return requestbody.Decode(c.Request.Body, dst)
}

func defaultEncoder(ctx context.Context, c *gin.Context, status int, payload any) error {
	if status == http.StatusNoContent || payload == nil || isNoBody(reflect.TypeOf(payload)) {
		c.Status(status)
		return nil
	}
	c.JSON(status, payload)
	return nil
}

func defaultErrorHandler(ctx context.Context, c *gin.Context, err error, useProblemDetails bool) {
	// An error without a 4xx or 5xx status is logged, and the client gets a
	// generic 500.
	status, err := errorhandler.Resolve(ctx, errorhandler.RequestOf(c.Request), err)

	// If Problem Details is enabled, serialize as RFC 9457
	if useProblemDetails {
		c.Header("Content-Type", "application/problem+json")
		c.JSON(status, apix.ToProblemDetails(err))
		return
	}

	c.JSON(status, gin.H{"error": err.Error()})
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
