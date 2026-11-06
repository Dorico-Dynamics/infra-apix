package apix_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
)

// TestSetLoggerReceivesRouteAndBuildEvents: route registration and document
// builds reach the logger passed to SetLogger, and nothing does after
// SetLogger(nil).
func TestSetLoggerReceivesRouteAndBuildEvents(t *testing.T) {
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { apix.SetLogger(nil); apix.ResetRegistry() })

	apix.ResetRegistry()
	apix.RegisterRoute(&apix.RouteRef{
		Method:        apix.MethodGet,
		Path:          "/v1/items",
		Summary:       "list items",
		SuccessStatus: http.StatusOK,
		Responses: map[int]*apix.ResponseRef{
			http.StatusOK: {ModelType: reflect.TypeOf(sampleResp{})},
		},
	})
	if _, err := openapi.NewBuilder().Build(apix.Snapshot()); err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, want := range []string{
		`level=INFO msg="route registered" method=GET path=/v1/items summary="list items"`,
		`level=DEBUG msg="schema generated" type=`,
		`level=INFO msg="OpenAPI spec built" routes=1 schemas=`,
	} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("missing %q in %q", want, logs.String())
		}
	}

	apix.SetLogger(nil)
	logs.Reset()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/v1/other"})
	if logs.Len() != 0 {
		t.Errorf("the removed logger received %q", logs.String())
	}
}
