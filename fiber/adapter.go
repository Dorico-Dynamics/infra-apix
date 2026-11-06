package fiber

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/errorhandler"
	"github.com/Infra-Forge/infra-apix/internal/requestbody"
	"github.com/gofiber/fiber/v3"
)

// RequestDecoder decodes the HTTP request body into dst, enforcing validation rules.
type RequestDecoder func(ctx context.Context, c fiber.Ctx, dst any) error

// ResponseEncoder writes the response payload using the provided status code.
type ResponseEncoder func(ctx context.Context, c fiber.Ctx, status int, payload any, ref *apix.RouteRef) error

// ErrorHandler handles errors from the typed handler.
type ErrorHandler func(ctx context.Context, c fiber.Ctx, err error) error

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

// FiberAdapter integrates apix route registration with fiber.App.
type FiberAdapter struct {
	app  *fiber.App
	opts Options
}

// New constructs a FiberAdapter with optional overrides.
func New(app *fiber.App, opts ...Options) *FiberAdapter {
	adapter := &FiberAdapter{app: app}
	if len(opts) > 0 {
		adapter.opts = opts[0]
	}
	return adapter
}

// App exposes the underlying fiber.App instance.
func (a *FiberAdapter) App() *fiber.App { return a.app }

// Register adds a handler for the provided method and path.
func Register[TReq any, TResp any](a *FiberAdapter, method apix.RouteMethod, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	if a == nil || a.app == nil {
		panic("apix/fiber: adapter not initialised")
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

	a.app.Add([]string{string(method)}, path, buildFiberHandler(a, handler, ref))
}

// Get registers a GET handler.
func Get[TResp any](a *FiberAdapter, path string, handler apix.HandlerFunc[apix.NoBody, TResp], opts ...apix.RouteOption) {
	Register[apix.NoBody, TResp](a, apix.MethodGet, path, handler, opts...)
}

// Post registers a POST handler.
func Post[TReq any, TResp any](a *FiberAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPost, path, handler, opts...)
}

// Put registers a PUT handler.
func Put[TReq any, TResp any](a *FiberAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPut, path, handler, opts...)
}

// Patch registers a PATCH handler.
func Patch[TReq any, TResp any](a *FiberAdapter, path string, handler apix.HandlerFunc[TReq, TResp], opts ...apix.RouteOption) {
	Register[TReq, TResp](a, apix.MethodPatch, path, handler, opts...)
}

// Delete registers a DELETE handler.
func Delete[TResp any](a *FiberAdapter, path string, handler apix.HandlerFunc[apix.NoBody, TResp], opts ...apix.RouteOption) {
	Register[apix.NoBody, TResp](a, apix.MethodDelete, path, handler, opts...)
}

func buildFiberHandler[TReq any, TResp any](a *FiberAdapter, handler apix.HandlerFunc[TReq, TResp], ref *apix.RouteRef) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx := c.Context()

		var reqPtr *TReq
		if ref.RequestType != nil {
			reqVal := new(TReq)
			if err := a.decode(ctx, c, reqVal); err != nil {
				return a.handleError(ctx, c, err)
			}
			reqPtr = reqVal
		}

		resp, err := handler(ctx, reqPtr)
		if err != nil {
			return a.handleError(ctx, c, err)
		}

		if err := a.encode(ctx, c, ref.SuccessStatus, resp, ref); err != nil {
			return a.handleError(ctx, c, err)
		}
		return nil
	}
}

func (a *FiberAdapter) decode(ctx context.Context, c fiber.Ctx, dst any) error {
	if dec := a.opts.Decoder; dec != nil {
		return dec(ctx, c, dst)
	}
	return defaultDecoder(ctx, c, dst)
}

func (a *FiberAdapter) encode(ctx context.Context, c fiber.Ctx, status int, payload any, ref *apix.RouteRef) error {
	if enc := a.opts.ResponseEncoder; enc != nil {
		return enc(ctx, c, status, payload, ref)
	}
	return defaultEncoder(ctx, c, status, payload)
}

func (a *FiberAdapter) handleError(ctx context.Context, c fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	if handler := a.opts.ErrorHandler; handler != nil {
		return handler(ctx, c, err)
	}
	return defaultErrorHandler(ctx, c, err, a.opts.UseProblemDetails)
}

func defaultDecoder(ctx context.Context, c fiber.Ctx, dst any) error {
	return requestbody.Decode(bytes.NewReader(c.Body()), dst)
}

func defaultEncoder(ctx context.Context, c fiber.Ctx, status int, payload any) error {
	if status == http.StatusNoContent || payload == nil || isNoBody(reflect.TypeOf(payload)) {
		return c.SendStatus(status)
	}
	return c.Status(status).JSON(payload)
}

func defaultErrorHandler(ctx context.Context, c fiber.Ctx, err error, useProblemDetails bool) error {
	// An error without a 4xx or 5xx status is logged, and the client gets a
	// generic 500.
	status, err := errorhandler.Resolve(c.Method(), c.Path(), err)

	// If Problem Details is enabled, serialize as RFC 9457
	if useProblemDetails {
		// Fiber's JSON() overrides Content-Type, so we need to marshal manually
		data, marshalErr := json.Marshal(apix.ToProblemDetails(err))
		if marshalErr == nil {
			c.Set("Content-Type", "application/problem+json")
			return c.Status(status).Send(data)
		}
		// Fall back to plain error response if marshaling fails
	}

	return c.Status(status).JSON(fiber.Map{"error": err.Error()})
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
