package openapi_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	amodels "github.com/Infra-Forge/infra-apix/openapi/internal/collision/a/models"
	bmodels "github.com/Infra-Forge/infra-apix/openapi/internal/collision/b/models"
)

type cnInvoice struct {
	Total amodels.Money `json:"total"`
}

type cnPayslip struct {
	Net bmodels.Money `json:"net"`
}

func cnRoute(path string, resp reflect.Type) *apix.RouteRef {
	return &apix.RouteRef{
		Method:    apix.MethodGet,
		Path:      path,
		Responses: map[int]*apix.ResponseRef{http.StatusOK: {ModelType: resp}},
	}
}

// TestBuildRejectsTwoTypesWithOneComponentName: a/models.Money and
// b/models.Money both map to "models_Money". Build fails and names both types
// and the route that reached the second one. Before, the second schema
// replaced the first, so every $ref from the first type described the wrong
// schema.
func TestBuildRejectsTwoTypesWithOneComponentName(t *testing.T) {
	_, err := openapi.NewBuilder().Build([]*apix.RouteRef{
		cnRoute("/invoices", reflect.TypeFor[cnInvoice]()),
		cnRoute("/payslips", reflect.TypeFor[cnPayslip]()),
	})
	if err == nil {
		t.Fatal("Build accepted two types with one component name")
	}
	for _, want := range []string{
		"GET /payslips: ",
		`component name "models_Money"`,
		"github.com/Infra-Forge/infra-apix/openapi/internal/collision/a/models.Money",
		"github.com/Infra-Forge/infra-apix/openapi/internal/collision/b/models.Money",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

type cnLedger struct {
	Opening amodels.Money `json:"opening"`
	Note    amodels.Note  `json:"note"`
}

type cnStatement struct {
	Closing bmodels.Money `json:"closing"`
	Carried bmodels.Money `json:"carried"`
	Note    bmodels.Note  `json:"note"`
}

// TestBuildReportsEveryCollisionOnce: one failed Build lists every component
// name that two types share, so a service with several collisions sees them
// all in one run. bmodels.Money is reached twice but reported once.
func TestBuildReportsEveryCollisionOnce(t *testing.T) {
	_, err := openapi.NewBuilder().Build([]*apix.RouteRef{
		cnRoute("/ledgers", reflect.TypeFor[cnLedger]()),
		cnRoute("/statements", reflect.TypeFor[cnStatement]()),
	})
	if err == nil {
		t.Fatal("Build accepted two types per component name")
	}
	lines := strings.Split(err.Error(), "\n")
	want := []string{
		`GET /statements: component name "models_Money" is used by both ` +
			"github.com/Infra-Forge/infra-apix/openapi/internal/collision/a/models.Money and " +
			"github.com/Infra-Forge/infra-apix/openapi/internal/collision/b/models.Money; rename one of the types",
		`GET /statements: component name "models_Note" is used by both ` +
			"github.com/Infra-Forge/infra-apix/openapi/internal/collision/a/models.Note and " +
			"github.com/Infra-Forge/infra-apix/openapi/internal/collision/b/models.Note; rename one of the types",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("error lines:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

// TestBuildCanRunAgainAfterRejectingACollision: a Builder is reused, for
// example by the runtime handler on every request, so a failed Build must not
// leave its component names behind.
func TestBuildCanRunAgainAfterRejectingACollision(t *testing.T) {
	builder := openapi.NewBuilder()
	if _, err := builder.Build([]*apix.RouteRef{
		cnRoute("/invoices", reflect.TypeFor[cnInvoice]()),
		cnRoute("/payslips", reflect.TypeFor[cnPayslip]()),
	}); err == nil {
		t.Fatal("Build accepted two types with one component name")
	}

	doc, err := builder.Build([]*apix.RouteRef{cnRoute("/payslips", reflect.TypeFor[cnPayslip]())})
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	money := doc.Components.Schemas["models_Money"]
	if money == nil || money.Value == nil || money.Value.Properties["currency"] == nil {
		t.Fatalf("models_Money should be b/models.Money, got %+v", money)
	}
}
