package openapi_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/getkin/kin-openapi/openapi3"
)

type ssItem struct {
	ID string `json:"id"`
}

func ssRoute(method apix.RouteMethod, path string, schemes ...string) *apix.RouteRef {
	ref := &apix.RouteRef{Method: method, Path: path}
	for _, scheme := range schemes {
		apix.WithSecurity(scheme)(ref)
	}
	apix.EnsureResponse(ref, http.StatusOK, reflect.TypeOf(ssItem{}))
	return ref
}

func bearerScheme() openapi3.SecuritySchemes {
	return openapi3.SecuritySchemes{"BearerAuth": {Value: openapi3.NewJWTSecurityScheme()}}
}

// TestBuildFailsForUndeclaredSecuritySchemes: a scheme named by a route or by
// the top-level security but missing from components.securitySchemes fails
// the build, one line per scheme naming its first user. Declared schemes pass.
func TestBuildFailsForUndeclaredSecuritySchemes(t *testing.T) {
	b := openapi.NewBuilder()
	b.SecuritySchemes = bearerScheme()
	b.GlobalSecurity = openapi3.SecurityRequirements{{"Cookie": {}}}

	_, err := b.Build([]*apix.RouteRef{
		ssRoute(apix.MethodPost, "/items", "ApiKey", "BearerAuth"),
		ssRoute(apix.MethodGet, "/items", "ApiKey"),
		ssRoute(apix.MethodDelete, "/items/{id}", "Session"),
		ssRoute(apix.MethodGet, "/health"),
	})
	want := `security scheme "ApiKey" is not declared in components.securitySchemes, but GET /items uses it (and 1 more)
security scheme "Cookie" is not declared in components.securitySchemes, but the top-level security uses it
security scheme "Session" is not declared in components.securitySchemes, but DELETE /items/{id} uses it`
	if err == nil || err.Error() != want {
		t.Fatalf("Build error =\n%v\nwant\n%s", err, want)
	}
}

// TestBuildAcceptsDeclaredSecuritySchemes: with every named scheme declared,
// the document builds, keeps the schemes and validates.
func TestBuildAcceptsDeclaredSecuritySchemes(t *testing.T) {
	b := openapi.NewBuilder()
	b.SecuritySchemes = bearerScheme()

	doc, err := b.Build([]*apix.RouteRef{ssRoute(apix.MethodGet, "/items", "BearerAuth")})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if doc.Components.SecuritySchemes["BearerAuth"] == nil {
		t.Errorf("BearerAuth missing from components.securitySchemes")
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Errorf("document does not validate: %v", err)
	}
}

type ssSchemePlugin struct{ apix.BasePlugin }

func (p *ssSchemePlugin) OnSpecBuild(doc *openapi3.T) error {
	doc.Components.SecuritySchemes = bearerScheme()
	return nil
}

// TestBuildAcceptsSecuritySchemesAPluginDeclares: the check runs after the
// OnSpecBuild hooks, so a scheme a plugin declares counts.
func TestBuildAcceptsSecuritySchemesAPluginDeclares(t *testing.T) {
	apix.RegisterPlugin(&ssSchemePlugin{apix.BasePlugin{PluginName: "ss-scheme-plugin"}})
	t.Cleanup(func() { apix.UnregisterPlugin("ss-scheme-plugin") })

	if _, err := openapi.NewBuilder().Build([]*apix.RouteRef{ssRoute(apix.MethodGet, "/items", "BearerAuth")}); err != nil {
		t.Fatalf("build: %v", err)
	}
}

// TestSecurityRequirementWithoutScopesIsAnEmptyList: WithSecurity without
// scopes is encoded as an empty list, as the OpenAPI schema requires, not as
// null; scopes are kept in order.
func TestSecurityRequirementWithoutScopesIsAnEmptyList(t *testing.T) {
	b := openapi.NewBuilder()
	b.SecuritySchemes = bearerScheme()
	scoped := ssRoute(apix.MethodPost, "/items")
	apix.WithSecurity("BearerAuth", "items:write", "items:read")(scoped)

	doc, err := b.Build([]*apix.RouteRef{ssRoute(apix.MethodGet, "/items", "BearerAuth"), scoped})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for method, want := range map[string]string{
		http.MethodGet:  `[{"BearerAuth":[]}]`,
		http.MethodPost: `[{"BearerAuth":["items:write","items:read"]}]`,
	} {
		got, err := json.Marshal(doc.Paths.Value("/items").GetOperation(method).Security)
		if err != nil {
			t.Fatalf("encode %s security: %v", method, err)
		}
		if string(got) != want {
			t.Errorf("%s /items security = %s, want %s", method, got, want)
		}
	}
}
