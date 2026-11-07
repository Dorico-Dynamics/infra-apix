package openapi_test

import (
	"net/http"
	"reflect"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
)

type oidItem struct {
	ID string `json:"id"`
}

func oidRoute(method apix.RouteMethod, path string, params ...string) *apix.RouteRef {
	ref := &apix.RouteRef{Method: method, Path: path, OperationID: apix.DefaultOperationID(method, path)}
	for _, name := range params {
		apix.WithParameter(apix.Parameter{Name: name, In: "path", Required: true})(ref)
	}
	apix.EnsureResponse(ref, http.StatusOK, reflect.TypeOf(oidItem{}))
	return ref
}

// TestBuildFailsForDuplicateOperationIDs: operations that share an operationId
// fail the build, one line per ID naming its first two users in path order and
// counting the others. A path parameter and a literal segment of the same name
// share a default ID in every router syntax: /users/:id and /users/id,
// /teams/{team} and /teams/team. An explicit ID counts too.
func TestBuildFailsForDuplicateOperationIDs(t *testing.T) {
	health := oidRoute(apix.MethodGet, "/health")
	apix.WithOperationID("get_users_id")(health)

	_, err := openapi.NewBuilder().Build([]*apix.RouteRef{
		oidRoute(apix.MethodGet, "/users/:id", "id"),
		oidRoute(apix.MethodGet, "/users/id"),
		oidRoute(apix.MethodGet, "/teams/{team}", "team"),
		oidRoute(apix.MethodGet, "/teams/team"),
		health,
		oidRoute(apix.MethodGet, "/status"),
	})
	want := `operationId "get_teams_team" is used by both GET /teams/team and GET /teams/{team}; give one of them its own with apix.WithOperationID
operationId "get_users_id" is used by both GET /users/id and GET /health (and 1 more); give one of them its own with apix.WithOperationID`
	if err == nil || err.Error() != want {
		t.Fatalf("Build error =\n%v\nwant\n%s", err, want)
	}
}

// TestBuildAcceptsOperationIDsMadeDistinct: WithOperationID on one route of a
// clashing pair resolves it, and the document validates.
func TestBuildAcceptsOperationIDsMadeDistinct(t *testing.T) {
	user := oidRoute(apix.MethodGet, "/users/:id", "id")
	apix.WithOperationID("getUser")(user)

	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{user, oidRoute(apix.MethodGet, "/users/id")})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Errorf("document does not validate: %v", err)
	}
}
