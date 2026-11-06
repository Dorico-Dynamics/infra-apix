package apix_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
)

func TestHTTPErrorImplementsError(t *testing.T) {
	err := &apix.HTTPError{
		Status:  http.StatusNotFound,
		Message: "resource not found",
		Code:    "NOT_FOUND",
	}

	if err.Error() == "" {
		t.Fatal("HTTPError.Error() should return non-empty string")
	}

	expected := "http 404 [NOT_FOUND]: resource not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestHTTPErrorWithoutCode(t *testing.T) {
	err := &apix.HTTPError{
		Status:  http.StatusBadRequest,
		Message: "invalid input",
	}

	expected := "http 400: invalid input"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestHTTPErrorImplementsStatusCoder(t *testing.T) {
	err := &apix.HTTPError{
		Status:  http.StatusConflict,
		Message: "conflict",
	}

	var statusCoder apix.StatusCoder
	if !errors.As(err, &statusCoder) {
		t.Fatal("HTTPError should implement StatusCoder interface")
	}

	if statusCoder.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, statusCoder.HTTPStatus())
	}
}

func TestHTTPErrorUnwrap(t *testing.T) {
	innerErr := errors.New("database error")
	err := &apix.HTTPError{
		Status:  http.StatusInternalServerError,
		Message: "failed to query database",
		Err:     innerErr,
	}

	if !errors.Is(err, innerErr) {
		t.Error("HTTPError should unwrap to inner error")
	}

	unwrapped := errors.Unwrap(err)
	if unwrapped != innerErr {
		t.Error("Unwrap should return inner error")
	}
}

func TestNotFoundConstructor(t *testing.T) {
	err := apix.NotFound("user not found")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("NotFound should return HTTPError")
	}

	if httpErr.Status != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Status)
	}

	if httpErr.Message != "user not found" {
		t.Errorf("expected message %q, got %q", "user not found", httpErr.Message)
	}

	if httpErr.Code != "NOT_FOUND" {
		t.Errorf("expected code %q, got %q", "NOT_FOUND", httpErr.Code)
	}
}

func TestBadRequestConstructor(t *testing.T) {
	err := apix.BadRequest("invalid email")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("BadRequest should return HTTPError")
	}

	if httpErr.Status != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Status)
	}
}

func TestConflictConstructor(t *testing.T) {
	err := apix.Conflict("user already exists")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("Conflict should return HTTPError")
	}

	if httpErr.Status != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, httpErr.Status)
	}
}

func TestUnauthorizedConstructor(t *testing.T) {
	err := apix.Unauthorized("authentication required")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("Unauthorized should return HTTPError")
	}

	if httpErr.Status != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Status)
	}
}

func TestForbiddenConstructor(t *testing.T) {
	err := apix.Forbidden("insufficient permissions")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("Forbidden should return HTTPError")
	}

	if httpErr.Status != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Status)
	}
}

func TestUnprocessableEntityConstructor(t *testing.T) {
	err := apix.UnprocessableEntity("validation failed")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("UnprocessableEntity should return HTTPError")
	}

	if httpErr.Status != http.StatusUnprocessableEntity {
		t.Errorf("expected status %d, got %d", http.StatusUnprocessableEntity, httpErr.Status)
	}
}

func TestInternalServerErrorConstructor(t *testing.T) {
	err := apix.InternalServerError("database connection failed")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("InternalServerError should return HTTPError")
	}

	if httpErr.Status != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, httpErr.Status)
	}
}

func TestWithStatusConstructor(t *testing.T) {
	err := apix.WithStatus(http.StatusTeapot, "I'm a teapot")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("WithStatus should return HTTPError")
	}

	if httpErr.Status != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, httpErr.Status)
	}

	if httpErr.Message != "I'm a teapot" {
		t.Errorf("expected message %q, got %q", "I'm a teapot", httpErr.Message)
	}

	// WithStatus should not set a code
	if httpErr.Code != "" {
		t.Errorf("expected empty code, got %q", httpErr.Code)
	}
}

func TestWrapErrorConstructor(t *testing.T) {
	innerErr := errors.New("connection timeout")
	err := apix.WrapError(innerErr, http.StatusServiceUnavailable, "service unavailable")

	var httpErr *apix.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatal("WrapError should return HTTPError")
	}

	if httpErr.Status != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, httpErr.Status)
	}

	if httpErr.Message != "service unavailable" {
		t.Errorf("expected message %q, got %q", "service unavailable", httpErr.Message)
	}

	if !errors.Is(err, innerErr) {
		t.Error("WrapError should preserve error chain")
	}
}

// customError is a test type that implements StatusCoder
type customError struct {
	status int
}

func (e *customError) Error() string {
	return "custom error"
}

func (e *customError) HTTPStatus() int {
	return e.status
}

func TestStatusCoderInterface(t *testing.T) {
	// Test that any type implementing StatusCoder works
	err := &customError{status: http.StatusBadGateway}

	var statusCoder apix.StatusCoder
	if !errors.As(err, &statusCoder) {
		t.Fatal("customError should implement StatusCoder")
	}

	if statusCoder.HTTPStatus() != http.StatusBadGateway {
		t.Errorf("expected status %d, got %d", http.StatusBadGateway, statusCoder.HTTPStatus())
	}
}

func TestProblemDetailsBasic(t *testing.T) {
	problem := &apix.ProblemDetails{
		Type:     "https://example.com/probs/out-of-credit",
		Title:    "You do not have enough credit.",
		Status:   http.StatusForbidden,
		Detail:   "Your current balance is 30, but that costs 50.",
		Instance: "/account/12345/msgs/abc",
	}

	if problem.Error() != "Your current balance is 30, but that costs 50." {
		t.Errorf("unexpected error message: %s", problem.Error())
	}

	if problem.HTTPStatus() != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, problem.HTTPStatus())
	}
}

func TestProblemDetailsMarshalJSON(t *testing.T) {
	problem := &apix.ProblemDetails{
		Type:   "https://example.com/probs/out-of-credit",
		Title:  "Out of Credit",
		Status: http.StatusForbidden,
		Detail: "Your balance is insufficient.",
	}

	data, err := problem.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result["type"] != "https://example.com/probs/out-of-credit" {
		t.Errorf("unexpected type: %v", result["type"])
	}

	if result["title"] != "Out of Credit" {
		t.Errorf("unexpected title: %v", result["title"])
	}

	if result["status"] != float64(http.StatusForbidden) {
		t.Errorf("unexpected status: %v", result["status"])
	}

	if result["detail"] != "Your balance is insufficient." {
		t.Errorf("unexpected detail: %v", result["detail"])
	}
}

func TestProblemDetailsWithExtensions(t *testing.T) {
	problem := apix.NewProblemDetails(
		http.StatusForbidden,
		"Out of Credit",
		"Your balance is insufficient.",
	).WithExtension("balance", 30).WithExtension("cost", 50)

	data, err := problem.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result["balance"] != float64(30) {
		t.Errorf("unexpected balance: %v", result["balance"])
	}

	if result["cost"] != float64(50) {
		t.Errorf("unexpected cost: %v", result["cost"])
	}
}

func TestProblemDetailsDefaultType(t *testing.T) {
	problem := &apix.ProblemDetails{
		Status: http.StatusNotFound,
		Detail: "Resource not found",
	}

	data, err := problem.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result["type"] != "about:blank" {
		t.Errorf("expected default type 'about:blank', got %v", result["type"])
	}
}

func TestProblemDetailsReservedExtensionKeys(t *testing.T) {
	// Test that extensions with reserved field names are ignored
	problem := apix.NewProblemDetails(
		http.StatusBadRequest,
		"Bad Request",
		"Invalid input provided",
	).
		// Try to overwrite reserved fields via extensions
		WithExtension("type", "https://malicious.com/override").
		WithExtension("title", "Malicious Title").
		WithExtension("status", 999).
		WithExtension("detail", "Malicious Detail").
		WithExtension("instance", "/malicious/path").
		// Add a valid extension to verify non-reserved keys still work
		WithExtension("custom_field", "valid value")

	data, err := problem.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify reserved fields were NOT overwritten by extensions
	if result["type"] != "about:blank" {
		t.Errorf("extension should not overwrite 'type', got %v", result["type"])
	}

	if result["title"] != "Bad Request" {
		t.Errorf("extension should not overwrite 'title', got %v", result["title"])
	}

	if result["status"] != float64(http.StatusBadRequest) {
		t.Errorf("extension should not overwrite 'status', got %v", result["status"])
	}

	if result["detail"] != "Invalid input provided" {
		t.Errorf("extension should not overwrite 'detail', got %v", result["detail"])
	}

	// Instance is empty in the original, so it should not appear
	if _, exists := result["instance"]; exists {
		t.Errorf("extension should not add 'instance' when it was empty, got %v", result["instance"])
	}

	// Verify valid extension was added
	if result["custom_field"] != "valid value" {
		t.Errorf("valid extension should be added, got %v", result["custom_field"])
	}
}

func TestToProblemDetailsFromHTTPError(t *testing.T) {
	httpErr := apix.NotFound("user not found")

	problem := apix.ToProblemDetails(httpErr)

	if problem.Status != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, problem.Status)
	}

	if problem.Detail != "user not found" {
		t.Errorf("expected detail 'user not found', got %q", problem.Detail)
	}

	if problem.Title != "Not Found" {
		t.Errorf("expected title 'Not Found', got %q", problem.Title)
	}

	if problem.Type != "about:blank#NOT_FOUND" {
		t.Errorf("expected type 'about:blank#NOT_FOUND', got %q", problem.Type)
	}
}

func TestToProblemDetailsFromGenericError(t *testing.T) {
	err := errors.New("something went wrong")

	problem := apix.ToProblemDetails(err)

	if problem.Status != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, problem.Status)
	}

	if problem.Detail != "something went wrong" {
		t.Errorf("expected detail 'something went wrong', got %q", problem.Detail)
	}

	if problem.Title != "Internal Server Error" {
		t.Errorf("expected title 'Internal Server Error', got %q", problem.Title)
	}
}

func TestToProblemDetailsFromProblemDetails(t *testing.T) {
	original := &apix.ProblemDetails{
		Type:   "https://example.com/probs/test",
		Status: http.StatusTeapot,
		Detail: "I'm a teapot",
	}

	problem := apix.ToProblemDetails(original)

	if problem != original {
		t.Error("ToProblemDetails should return the same instance for ProblemDetails")
	}
}

func TestProblemDetailsFluentAPI(t *testing.T) {
	problem := apix.NewProblemDetails(
		http.StatusForbidden,
		"Access Denied",
		"You don't have permission",
	).WithType("https://api.example.com/errors/access-denied").
		WithInstance("/users/123").
		WithExtension("required_role", "admin").
		WithExtension("user_role", "user")

	if problem.Type != "https://api.example.com/errors/access-denied" {
		t.Errorf("unexpected type: %s", problem.Type)
	}

	if problem.Instance != "/users/123" {
		t.Errorf("unexpected instance: %s", problem.Instance)
	}

	if problem.Extensions["required_role"] != "admin" {
		t.Errorf("unexpected required_role: %v", problem.Extensions["required_role"])
	}

	if problem.Extensions["user_role"] != "user" {
		t.Errorf("unexpected user_role: %v", problem.Extensions["user_role"])
	}
}
