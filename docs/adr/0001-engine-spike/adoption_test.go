package spike

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	apixchi "github.com/Infra-Forge/infra-apix/chi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// tenantKey is the context key a legacy middleware stores the tenant under.
type tenantKey struct{}

type typedBody struct {
	ID     string `json:"id"`
	Tenant string `json:"tenant"`
}

// adoptions mount one typed route, GET /typed/{id}, on an existing chi router.
var adoptions = map[string]func(r chi.Router){
	"apix": func(r chi.Router) {
		apix.ResetRegistry()
		apixchi.Get(apixchi.New(r), "/typed/{id}", func(ctx context.Context, _ *apix.NoBody) (typedBody, error) {
			return typedBody{ID: chi.URLParamFromCtx(ctx, "id"), Tenant: ctx.Value(tenantKey{}).(string)}, nil
		}, apix.WithParameter(apix.Parameter{Name: "id", In: "path", Required: true}))
	},
	"huma": func(r chi.Router) {
		api := humachi.New(r, humaConfig())
		huma.Get(api, "/typed/{id}", func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{ Body typedBody }, error) {
			return &struct{ Body typedBody }{Body: typedBody{ID: in.ID, Tenant: ctx.Value(tenantKey{}).(string)}}, nil
		})
	},
}

// TestIncrementalAdoption mounts a typed route beside a legacy handler on one
// chi router, under the router's middleware, and checks that both answer,
// that the middleware ran for both, and that the typed handler reads what the
// middleware put in the request context.
func TestIncrementalAdoption(t *testing.T) {
	for _, name := range engineNames() {
		t.Run(name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Legacy-Middleware", "ran")
					next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), tenantKey{}, "t-1")))
				})
			})
			r.Get("/legacy", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "legacy") })
			adoptions[name](r)
			srv := httptest.NewServer(r)
			defer srv.Close()

			var why []string
			for _, path := range []string{"/legacy", "/typed/42"} {
				resp, err := srv.Client().Get(srv.URL + path)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Legacy-Middleware") != "ran" {
					why = append(why, path+": status "+resp.Status+", middleware header "+resp.Header.Get("X-Legacy-Middleware"))
					continue
				}
				if path == "/typed/42" {
					var got typedBody
					if err := json.Unmarshal(body, &got); err != nil || got != (typedBody{ID: "42", Tenant: "t-1"}) {
						why = append(why, path+": body "+string(body))
					}
				}
			}
			res := result{}
			if len(why) > 0 {
				res.runtime = why[0]
				t.Log(why)
			}
			board.record("A1 typed route beside a legacy handler on one chi router, under its middleware", name, res)
		})
	}
}
