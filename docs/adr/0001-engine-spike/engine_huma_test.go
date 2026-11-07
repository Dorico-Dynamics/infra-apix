package spike

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// newHumaEngine serves the six routes with huma v2 on a net/http ServeMux.
// The code huma needed beyond itself is in glue_huma_test.go.
func newHumaEngine(t testing.TB) engine {
	t.Helper()
	mux := http.NewServeMux()
	api := humago.New(mux, humaConfig())
	registerHumaRoutes(api, newStore())
	doc, err := json.Marshal(api.OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	doc30, err := api.OpenAPI().Downgrade()
	if err != nil {
		t.Fatal(err)
	}
	return engine{name: "huma", handler: mux, doc: doc, doc30: doc30, serveMux: true}
}

// humaConfig is huma's default config with the spike's changes.
func humaConfig() huma.Config {
	installHumaErrors()
	cfg := huma.DefaultConfig("Spike API", "1.0.0")
	// The default config adds a "$schema" member to every JSON response body
	// and a Link header; existing clients would see a changed wire format.
	cfg.CreateHooks = nil
	cfg.Formats["application/x-www-form-urlencoded"] = formFormat
	cfg.Transformers = append(cfg.Transformers, stampRequest)
	return cfg
}

// Route 1: an action, not a create. It needs If-Match (428 without it, 412
// when stale) and answers 200 with the new ETag.

type progressInput struct {
	ID      string `path:"id" format:"uuid" doc:"Phase ID"`
	IfMatch string `header:"If-Match" required:"true" doc:"ETag of the phase as last read, or *"`
	Body    struct {
		Percent float64 `json:"percent" minimum:"0" maximum:"100" doc:"Completion, 0 to 100"`
	}
}

// Resolve answers a missing If-Match with 428 rather than the validation
// error a missing required header gets: the last error carrying a status
// wins.
func (in *progressInput) Resolve(_ huma.Context) []error {
	if in.IfMatch == "" {
		return []error{preconditionRequired("If-Match is required to update a phase.")}
	}
	return nil
}

type progressOutput struct {
	ETag string `header:"ETag" doc:"ETag of the updated phase"`
	Body phaseBody
}

// Route 2: a PDF download.

type payStubInput struct {
	PayPeriodID int64 `path:"payPeriodID" minimum:"1" doc:"Pay period ID"`
}

type payStubOutput struct {
	ContentType        string `header:"Content-Type"`
	ContentDisposition string `header:"Content-Disposition"`
	Body               []byte `contentType:"application/pdf"`
}

// Route 3: a multipart upload with a size cap.

type statementForm struct {
	File   huma.FormFile `form:"file" contentType:"text/csv,text/plain,application/octet-stream" required:"true" doc:"The statement file"`
	Format string        `form:"format" enum:"mt940,camt053,bai2,ofx" required:"true" doc:"Statement format"`
}

type statementInput struct {
	ID      string `path:"id" format:"uuid" doc:"Bank account ID"`
	RawBody huma.MultipartFormFiles[statementForm]
}

type statementOutput struct {
	Body importResult
}

// Route 4: one handler under /v1 and /v2, with a body cap and no-store.

type metadataInput struct {
	ID   string `path:"id" format:"uuid" doc:"Document ID"`
	Body struct {
		ExpectedRevision int64             `json:"expected_revision" minimum:"0" doc:"The revision the caller read"`
		Fields           map[string]string `json:"fields"`
	}
}

type metadataOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body         documentMetadata
}

// Route 5: a cursor-paginated list, and an idempotent create.

type listGrantsInput struct {
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"Page size"`
	Cursor string `query:"cursor" doc:"Opaque cursor from meta.pagination.next_cursor"`
}

type listGrantsOutput struct {
	Body grantPage
}

type createGrantInput struct {
	IdempotencyKey string `header:"Idempotency-Key" doc:"Makes a retried create safe: the same key and body replay the first response"`
	Body           struct {
		Title  string  `json:"title" minLength:"1" maxLength:"200"`
		Amount float64 `json:"amount" exclusiveMinimum:"0"`
	}
}

type createGrantOutput struct {
	Status   int
	Replayed string `header:"X-Idempotency-Replay" doc:"\"true\" when this is the stored response to an earlier request with the same key"`
	Location string `header:"Location"`
	Body     grant
}

// Route 6: the OAuth2 token endpoint, form-urlencoded, with RFC 6749 errors.

type tokenInput struct {
	Body struct {
		GrantType    string `json:"grant_type" enum:"client_credentials,refresh_token,authorization_code"`
		ClientID     string `json:"client_id" minLength:"1"`
		ClientSecret string `json:"client_secret" minLength:"1"`
		Scope        string `json:"scope,omitempty"`
	} `contentType:"application/x-www-form-urlencoded"`
}

type tokenOutput struct {
	CacheControl string `header:"Cache-Control"`
	Pragma       string `header:"Pragma"`
	Body         tokenResponse
}

func registerHumaRoutes(api huma.API, st *store) {
	huma.Register(api, huma.Operation{
		OperationID:   "update-phase-progress",
		Method:        http.MethodPost,
		Path:          "/phases/{id}/progress",
		Summary:       "Update a phase's progress",
		DefaultStatus: http.StatusOK,
		Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusPreconditionFailed, http.StatusPreconditionRequired, http.StatusInternalServerError},
	}, func(_ context.Context, in *progressInput) (*progressOutput, error) {
		p, err := st.updateProgress(in.ID, in.IfMatch, in.Body.Percent)
		switch {
		case errors.Is(err, errNotFound):
			return nil, notFound("The phase does not exist.")
		case errors.Is(err, errPreconditionFailed):
			return nil, preconditionFailed(err.Error())
		case err != nil:
			return nil, err
		}
		out := &progressOutput{ETag: p.etag()}
		out.Body = phaseBody(p)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-pay-stub-pdf",
		Method:      http.MethodGet,
		Path:        "/pay-stubs/{payPeriodID}/pdf",
		Summary:     "Download a pay stub as PDF",
		Responses:   humaBinaryResponse(http.StatusOK, "application/pdf"),
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	}, func(_ context.Context, in *payStubInput) (*payStubOutput, error) {
		p, err := st.payStub(in.PayPeriodID)
		if errors.Is(err, errNotFound) {
			return nil, notFound("No pay stub for pay period " + strconv.FormatInt(in.PayPeriodID, 10) + ".")
		}
		if err != nil {
			return nil, err
		}
		return &payStubOutput{
			ContentType:        "application/pdf",
			ContentDisposition: "attachment; filename=paystub.pdf",
			Body:               p.pdfBytes(),
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:  "upload-bank-statement",
		Method:       http.MethodPost,
		Path:         "/bank-accounts/{id}/upload-statement",
		Summary:      "Import a bank statement",
		MaxBodyBytes: statementMaxBytes,
		Errors:       []int{http.StatusBadRequest, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusInternalServerError},
	}, func(_ context.Context, in *statementInput) (*statementOutput, error) {
		form := in.RawBody.Data()
		data, err := io.ReadAll(form.File)
		if err != nil {
			return nil, err
		}
		res, err := st.importStatement(in.ID, data)
		if errors.Is(err, errNotFound) {
			return nil, notFound("The bank account does not exist.")
		}
		if err != nil {
			return nil, err
		}
		return &statementOutput{Body: res}, nil
	})

	// A group with two prefixes registers the operation once per prefix, with
	// distinct operationIds (v1-update-document-metadata, v2-...).
	docs := huma.NewGroup(api, "/v1", "/v2")
	huma.Register(docs, huma.Operation{
		OperationID:  "update-document-metadata",
		Method:       http.MethodPut,
		Path:         "/documents/{id}/metadata",
		Summary:      "Replace a document's metadata",
		MaxBodyBytes: metadataMaxBytes,
		Middlewares:  huma.Middlewares{noStore},
		Errors:       []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusInternalServerError},
	}, func(_ context.Context, in *metadataInput) (*metadataOutput, error) {
		m, err := st.updateMetadata(in.ID, in.Body.ExpectedRevision, in.Body.Fields)
		switch {
		case errors.Is(err, errNotFound):
			return nil, notFound("The document does not exist.")
		case errors.Is(err, errVersionMismatch):
			return nil, versionMismatch(err.Error())
		case err != nil:
			return nil, err
		}
		return &metadataOutput{CacheControl: "no-store", Body: m}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-grants",
		Method:      http.MethodGet,
		Path:        "/grants",
		Summary:     "List grants",
		Errors:      []int{http.StatusBadRequest, http.StatusInternalServerError},
	}, func(_ context.Context, in *listGrantsInput) (*listGrantsOutput, error) {
		page, next, err := st.listGrants(in.Cursor, in.Limit)
		if errors.Is(err, errInvalidCursor) {
			return nil, validationFailed(fieldError{Field: "cursor", Code: fieldInvalidFormat, Message: err.Error()})
		}
		if err != nil {
			return nil, err
		}
		return &listGrantsOutput{Body: newGrantPage(page, in.Limit, next)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-grant",
		Method:        http.MethodPost,
		Path:          "/grants",
		Summary:       "Create a grant",
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusBadRequest, http.StatusConflict, http.StatusUnsupportedMediaType, http.StatusInternalServerError},
	}, func(_ context.Context, in *createGrantInput) (*createGrantOutput, error) {
		g, status, replayed, err := st.createGrant(in.IdempotencyKey, in.Body.Title, in.Body.Amount)
		if errors.Is(err, errIdempotencyReuse) {
			return nil, conflict(err.Error())
		}
		if err != nil {
			return nil, err
		}
		out := &createGrantOutput{Status: status, Location: "/grants/" + g.ID, Body: g}
		if replayed {
			out.Replayed = "true"
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "issue-token",
		Method:      http.MethodPost,
		Path:        "/oauth2/token",
		Summary:     "Issue an access token",
		Metadata:    map[string]any{errorFormatKey: errorFormatRFC6749},
		Responses:   oauthResponses(api),
	}, func(_ context.Context, in *tokenInput) (*tokenOutput, error) {
		tok, oerr := st.token(in.Body.GrantType, in.Body.ClientID, in.Body.ClientSecret, in.Body.Scope)
		if oerr != nil {
			return nil, oerr
		}
		return &tokenOutput{CacheControl: "no-store", Pragma: "no-cache", Body: tok}, nil
	})
}
