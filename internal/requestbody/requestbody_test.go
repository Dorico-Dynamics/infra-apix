package requestbody

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
)

type testMoney struct {
	Amount int `json:"amount"`
}

// testCode decodes from a JSON string through encoding.TextUnmarshaler.
type testCode struct{ value string }

func (c *testCode) UnmarshalText(b []byte) error {
	c.value = string(b)
	return nil
}

type testAudit struct {
	CreatedBy string `json:"created_by"`
}

type testRequest struct {
	testAudit
	Name   string            `json:"name"`
	Budget testMoney         `json:"budget"`
	Items  []testMoney       `json:"items"`
	Tags   []string          `json:"tags"`
	Pair   [2]int            `json:"pair"`
	Ratio  float64           `json:"ratio"`
	Active bool              `json:"active"`
	Count  *int              `json:"count"`
	When   time.Time         `json:"when"`
	Code   testCode          `json:"code"`
	Raw    []byte            `json:"raw"`
	Meta   map[string]string `json:"meta"`
	Reader io.Reader         `json:"reader"`
}

// goNames are fragments of Go type, struct and package names that a client must
// never see in a decode error.
var goNames = []string{
	"Go ", "struct", "testRequest", "testMoney", "testCode", "testAudit", "requestbody",
	"json:", "time.", "io.Reader", "[]string", "[2]int", "*int", "float64", "map[",
}

func TestDecodeAcceptsOneValidValue(t *testing.T) {
	var got testRequest
	body := `{"name":"Ada","created_by":"u1","budget":{"amount":5},"items":[{"amount":7}],"code":"X1"}` + " \n"
	if err := Decode(strings.NewReader(body), &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Name != "Ada" || got.CreatedBy != "u1" || got.Budget.Amount != 5 ||
		len(got.Items) != 1 || got.Items[0].Amount != 7 || got.Code.value != "X1" {
		t.Fatalf("decoded %+v", got)
	}
}

func TestDecodeReportsFailuresWithoutGoNames(t *testing.T) {
	cases := []struct {
		name   string
		body   io.Reader
		detail string
		// legacy is the detail under GOEXPERIMENT=nojsonv2 when it differs; that
		// decoder's field paths are not shown (see fieldPaths).
		legacy string
	}{
		{"nil body", nil, "request body required", ""},
		{"empty body", strings.NewReader(""), "request body required", ""},
		{"whitespace only", strings.NewReader("  \n"), "request body required", ""},
		{"syntax error", strings.NewReader(`{"name": }`), "request body is not valid JSON", ""},
		{"truncated", strings.NewReader(`{"name": "a"`), "request body is not valid JSON", ""},
		{"top-level array", strings.NewReader(`[1, 2]`), "request body must be an object", ""},
		{"string field", strings.NewReader(`{"name": 5}`),
			`field "name" must be a string`, "request body contains a value that must be a string"},
		{"nested integer", strings.NewReader(`{"budget": {"amount": "x"}}`),
			`field "budget.amount" must be an integer`, "request body contains a value that must be an integer"},
		{"embedded struct field", strings.NewReader(`{"created_by": 5}`),
			`field "created_by" must be a string`, "request body contains a value that must be a string"},
		{"array element field", strings.NewReader(`{"items": [{"amount": "x"}]}`),
			`field "items.0.amount" must be an integer`, "request body contains a value that must be an integer"},
		{"map value", strings.NewReader(`{"meta": {"k": 5}}`),
			`field "meta.k" must be a string`, "request body contains a value that must be a string"},
		{"array field", strings.NewReader(`{"tags": "a"}`),
			`field "tags" must be an array`, "request body contains a value that must be an array"},
		{"fixed-size array field", strings.NewReader(`{"pair": "x"}`),
			`field "pair" must be an array`, "request body contains a value that must be an array"},
		{"number field", strings.NewReader(`{"ratio": "x"}`),
			`field "ratio" must be a number`, "request body contains a value that must be a number"},
		{"boolean field", strings.NewReader(`{"active": 1}`),
			`field "active" must be a boolean`, "request body contains a value that must be a boolean"},
		{"pointer field", strings.NewReader(`{"count": "x"}`),
			`field "count" must be an integer`, "request body contains a value that must be an integer"},
		{"text unmarshaler", strings.NewReader(`{"code": 5}`),
			`field "code" must be a string`, "request body contains a value that must be a string"},
		{"bytes field", strings.NewReader(`{"raw": 5}`),
			`field "raw" must be a string`, "request body contains a value that must be a string"},
		{"map field", strings.NewReader(`{"meta": []}`),
			`field "meta" must be an object`, "request body contains a value that must be an object"},
		{"interface field", strings.NewReader(`{"reader": {}}`),
			`field "reader" has an invalid value`, "request body contains an invalid value"},
		{"own unmarshaler", strings.NewReader(`{"when": "yesterday"}`), "request body contains an invalid value", ""},
		{"unknown field", strings.NewReader(`{"nope": 1}`), `unknown field "nope"`, ""},
		{"nested unknown field", strings.NewReader(`{"budget": {"nope": 1}}`), `unknown field "nope"`, ""},
		{"unknown field with a quote", strings.NewReader(`{"a\"b": 1}`), `unknown field "a\"b"`, ""},
		{"trailing value", strings.NewReader(`{"name": "a"} {"name": "b"}`), "unexpected additional JSON content", ""},
		{"trailing bracket", strings.NewReader(`{"name": "a"}}`), "unexpected additional JSON content", ""},
		{"trailing garbage", strings.NewReader(`{"name": "a"} x`), "unexpected additional JSON content", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Decode(tc.body, &testRequest{})

			var reqErr *Error
			if !errors.As(err, &reqErr) {
				t.Fatalf("want *Error, got %T: %v", err, err)
			}
			if reqErr.HTTPStatus() != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", reqErr.HTTPStatus())
			}
			want := tc.detail
			if !fieldPaths && tc.legacy != "" {
				want = tc.legacy
			}
			if reqErr.Error() != want {
				t.Errorf("detail = %q, want %q", reqErr.Error(), want)
			}
			for _, name := range goNames {
				if strings.Contains(reqErr.Error(), name) {
					t.Errorf("detail %q names Go internals (%q)", reqErr.Error(), name)
				}
			}
		})
	}
}

func TestDecodeIntoNonStructValues(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		dst    any
		detail string
	}{
		{"array expected", `"x"`, new([]int), "request body must be an array"},
		{"object expected", `5`, new(map[string]int), "request body must be an object"},
		{"no JSON kind", `{}`, new(io.Reader), "request body contains an invalid value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Decode(strings.NewReader(tc.body), tc.dst)
			if err == nil || err.Error() != tc.detail {
				t.Fatalf("got %v, want %q", err, tc.detail)
			}
		})
	}
}

func TestDecodeReportsABodyOverTheLimit(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		limit int64
	}{
		{"value over the limit", `{"name": "` + strings.Repeat("a", 100) + `"}`, 16},
		{"trailing content over the limit", `{"name": "a"}` + strings.Repeat(" ", 2000), 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := http.MaxBytesReader(nil, io.NopCloser(strings.NewReader(tc.body)), tc.limit)
			err := Decode(body, &testRequest{})

			var reqErr *Error
			if !errors.As(err, &reqErr) {
				t.Fatalf("want *Error, got %T: %v", err, err)
			}
			if reqErr.HTTPStatus() != http.StatusRequestEntityTooLarge || reqErr.Error() != "request body too large" {
				t.Fatalf("got %d %q", reqErr.HTTPStatus(), reqErr.Error())
			}
			if !errors.As(err, new(*http.MaxBytesError)) {
				t.Fatalf("cause not reachable through Unwrap")
			}
		})
	}

	t.Run("a body at the limit is accepted", func(t *testing.T) {
		const body = `{"name": "a"}`
		r := http.MaxBytesReader(nil, io.NopCloser(strings.NewReader(body)), int64(len(body)))
		if err := Decode(r, &testRequest{}); err != nil {
			t.Fatalf("Decode: %v", err)
		}
	})
}

func TestDecodeKeepsTheCauseForLogs(t *testing.T) {
	err := Decode(strings.NewReader(`{"name": }`), &testRequest{})
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("syntax error not reachable through Unwrap: %v", err)
	}

	err = Decode(strings.NewReader(`{"budget": {"amount": "x"}}`), &testRequest{})
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) || !strings.HasSuffix(typeErr.Field, "amount") {
		t.Fatalf("type error not reachable through Unwrap: %v", err)
	}

	var statusCoder apix.StatusCoder
	if !errors.As(err, &statusCoder) {
		t.Fatalf("decode errors must implement apix.StatusCoder")
	}
}

func TestInvalid(t *testing.T) {
	if Invalid(nil) != nil {
		t.Fatalf("Invalid(nil) must be nil")
	}

	t.Run("plain validator error becomes a 422", func(t *testing.T) {
		cause := errors.New("name is required")
		err := Invalid(cause)

		var reqErr *Error
		if !errors.As(err, &reqErr) {
			t.Fatalf("want *Error, got %T", err)
		}
		if reqErr.HTTPStatus() != http.StatusUnprocessableEntity || reqErr.Error() != "name is required" {
			t.Fatalf("got %d %q", reqErr.HTTPStatus(), reqErr.Error())
		}
		if !errors.Is(err, cause) {
			t.Fatalf("cause not reachable through Unwrap")
		}
	})

	t.Run("an error with its own status is kept", func(t *testing.T) {
		for _, own := range []error{
			apix.BadRequest("amount must be positive"),
			fmt.Errorf("validate: %w", apix.NotFound("project not found")),
		} {
			if got := Invalid(own); got != own {
				t.Errorf("Invalid(%v) = %v, want the same error", own, got)
			}
		}
	})
}

func TestExpectedKind(t *testing.T) {
	cases := []struct {
		t    reflect.Type
		want string
	}{
		{nil, ""},
		{reflect.TypeFor[*int](), "an integer"},
		{reflect.TypeFor[uint8](), "an integer"},
		{reflect.TypeFor[**testMoney](), "an object"},
		{reflect.TypeFor[*testCode](), "a string"},
		{reflect.TypeFor[[3]string](), "an array"},
		{reflect.TypeFor[json.RawMessage](), ""},
		{reflect.TypeFor[time.Time](), ""},
		{reflect.TypeFor[any](), ""},
		{reflect.TypeFor[chan int](), ""},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.t), func(t *testing.T) {
			if got := expectedKind(tc.t); got != tc.want {
				t.Fatalf("expectedKind(%v) = %q, want %q", tc.t, got, tc.want)
			}
		})
	}
}
