package spike

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	apixchi "github.com/Infra-Forge/infra-apix/chi"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/go-chi/chi/v5"
)

// newApixEngine serves the six routes with apix v1 on chi, the way its
// consumers do today. apix has no net/http ServeMux adapter, so route 4 is
// served by chi too. The code apix needed beyond itself is in
// glue_apix_test.go, apart from the list route's query parsing, which is in
// its handler. apix keeps routes in a process-wide registry, so the registry
// is reset first and the document is built straight after registration.
func newApixEngine(t testing.TB) engine {
	t.Helper()
	apix.ResetRegistry()
	r := chi.NewRouter()
	r.Use(withExchange)
	registerApixRoutes(r, newStore())
	b := openapi.NewBuilder()
	b.Info.Title = "Spike API"
	b.Info.Version = "1.0.0"
	doc, err := b.Build(apix.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return engine{name: "apix", handler: r, doc: raw}
}

// Request bodies. apix v1 reads the json, validate (required only), format
// (binary only), example and description tags into the document; the
// validate tags reach the validator at runtime.

type apixProgressRequest struct {
	Percent *float64 `json:"percent" validate:"required,gte=0,lte=100" description:"Completion, 0 to 100"`
}

type apixStatementRequest struct {
	File   []byte `json:"file" format:"binary" validate:"required" description:"The statement file"`
	Format string `json:"format" validate:"required,oneof=mt940 camt053 bai2 ofx" description:"Statement format"`
}

type apixMetadataRequest struct {
	ExpectedRevision *int64            `json:"expected_revision" validate:"required,gte=0" description:"The revision the caller read"`
	Fields           map[string]string `json:"fields" validate:"required"`
}

type apixGrantRequest struct {
	Title  string  `json:"title" validate:"required,min=1,max=200"`
	Amount float64 `json:"amount" validate:"required,gt=0"`
}

type apixTokenRequest struct {
	GrantType    string `json:"grant_type" validate:"required,oneof=client_credentials"`
	ClientID     string `json:"client_id" validate:"required"`
	ClientSecret string `json:"client_secret" validate:"required"`
	Scope        string `json:"scope,omitempty"`
}

func registerApixRoutes(r chi.Router, st *store) {
	jsonOptions := apixchi.Options{
		Validator:       playgroundValidator{},
		ErrorHandler:    renderApixError,
		ResponseEncoder: encodeApixResponse,
	}
	jsonRoutes := apixchi.New(r, jsonOptions)
	cappedRoutes := apixchi.New(r.With(maxBody(metadataMaxBytes), noStoreHeader), jsonOptions)
	uploads := apixchi.New(r, apixchi.Options{
		Decoder:         decodeStatement,
		ErrorHandler:    renderApixError,
		ResponseEncoder: encodeApixResponse,
	})
	oauth := apixchi.New(r, apixchi.Options{
		Decoder:         decodeTokenForm,
		ErrorHandler:    renderOAuthError,
		ResponseEncoder: encodeApixResponse,
	})
	uuidPath := func(name, desc string) apix.RouteOption {
		return apix.WithParameter(apix.Parameter{Name: name, In: "path", Required: true, Description: desc})
	}

	// Route 1.
	apixchi.Post(jsonRoutes, "/phases/{id}/progress", func(ctx context.Context, req *apixProgressRequest) (phaseBody, error) {
		id, err := pathUUID(ctx, "id")
		if err != nil {
			return phaseBody{}, err
		}
		ifMatch := requestOf(ctx).Header.Get("If-Match")
		if ifMatch == "" {
			return phaseBody{}, preconditionRequired("If-Match is required to update a phase.")
		}
		p, err := st.updateProgress(id, ifMatch, *req.Percent)
		switch {
		case errors.Is(err, errNotFound):
			return phaseBody{}, notFound("The phase does not exist.")
		case errors.Is(err, errPreconditionFailed):
			return phaseBody{}, preconditionFailed(err.Error())
		case err != nil:
			return phaseBody{}, err
		}
		responseHeader(ctx).Set("ETag", p.etag())
		return phaseBody(p), nil
	},
		apix.WithOperationID("update-phase-progress"),
		apix.WithSummary("Update a phase's progress"),
		apix.WithSuccessStatus(http.StatusOK),
		uuidPath("id", "Phase ID"),
		apix.WithParameter(apix.Parameter{Name: "If-Match", In: "header", Required: true, Description: "ETag of the phase as last read, or *"}),
		apix.WithSuccessHeaders(http.StatusOK, apix.HeaderRef{Name: "ETag", Description: "ETag of the updated phase"}),
		documentProblems(http.StatusBadRequest, http.StatusNotFound, http.StatusPreconditionFailed, http.StatusPreconditionRequired, http.StatusInternalServerError),
	)

	// Route 2.
	apixchi.Get(jsonRoutes, "/pay-stubs/{payPeriodID}/pdf", func(ctx context.Context, _ *apix.NoBody) ([]byte, error) {
		id, err := pathInt(ctx, "payPeriodID", 1)
		if err != nil {
			return nil, err
		}
		p, err := st.payStub(id)
		if errors.Is(err, errNotFound) {
			return nil, notFound("No pay stub for pay period " + strconv.FormatInt(id, 10) + ".")
		}
		if err != nil {
			return nil, err
		}
		responseHeader(ctx).Set("Content-Disposition", "attachment; filename=paystub.pdf")
		return p.pdfBytes(), nil
	},
		apix.WithOperationID("get-pay-stub-pdf"),
		apix.WithSummary("Download a pay stub as PDF"),
		apix.WithParameter(apix.Parameter{Name: "payPeriodID", In: "path", Required: true, SchemaType: "integer", Description: "Pay period ID"}),
		binaryResponse(http.StatusOK, "application/pdf"),
		apix.WithSuccessHeaders(http.StatusOK, apix.HeaderRef{Name: "Content-Disposition"}),
		documentProblems(http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError),
	)

	// Route 3.
	apixchi.Post(uploads, "/bank-accounts/{id}/upload-statement", func(ctx context.Context, req *apixStatementRequest) (importResult, error) {
		id, err := pathUUID(ctx, "id")
		if err != nil {
			return importResult{}, err
		}
		res, err := st.importStatement(id, req.File)
		if errors.Is(err, errNotFound) {
			return importResult{}, notFound("The bank account does not exist.")
		}
		return res, err
	},
		apix.WithOperationID("upload-bank-statement"),
		apix.WithSummary("Import a bank statement"),
		apix.WithMultipartFormData(),
		apix.WithSuccessStatus(http.StatusOK),
		uuidPath("id", "Bank account ID"),
		documentProblems(http.StatusBadRequest, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusInternalServerError),
	)

	// Route 4: registered once per prefix, with distinct operationIds.
	updateMetadata := func(ctx context.Context, req *apixMetadataRequest) (documentMetadata, error) {
		id, err := pathUUID(ctx, "id")
		if err != nil {
			return documentMetadata{}, err
		}
		m, err := st.updateMetadata(id, *req.ExpectedRevision, req.Fields)
		switch {
		case errors.Is(err, errNotFound):
			return documentMetadata{}, notFound("The document does not exist.")
		case errors.Is(err, errVersionMismatch):
			return documentMetadata{}, versionMismatch(err.Error())
		}
		return m, err
	}
	for _, v := range []string{"v1", "v2"} {
		apixchi.Put(cappedRoutes, "/"+v+"/documents/{id}/metadata", updateMetadata,
			apix.WithOperationID(v+"-update-document-metadata"),
			apix.WithSummary("Replace a document's metadata"),
			uuidPath("id", "Document ID"),
			apix.WithSuccessHeaders(http.StatusOK, apix.HeaderRef{Name: "Cache-Control"}),
			documentProblems(http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusInternalServerError),
		)
	}

	// Route 5: list. Its query parsing and validation are handler code.
	apixchi.Get(jsonRoutes, "/grants", func(ctx context.Context, _ *apix.NoBody) (grantPage, error) {
		q := requestOf(ctx).URL.Query()
		limit := pageDefault
		if raw := q.Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > pageMax {
				return grantPage{}, validationFailed(fieldError{Field: "limit", Code: fieldOutOfRange, Message: "must be an integer from 1 to " + strconv.Itoa(pageMax)})
			}
			limit = n
		}
		page, next, err := st.listGrants(q.Get("cursor"), limit)
		if errors.Is(err, errInvalidCursor) {
			return grantPage{}, validationFailed(fieldError{Field: "cursor", Code: fieldInvalidFormat, Message: err.Error()})
		}
		if err != nil {
			return grantPage{}, err
		}
		return newGrantPage(page, limit, next), nil
	},
		apix.WithOperationID("list-grants"),
		apix.WithSummary("List grants"),
		apix.WithParameter(apix.Parameter{Name: "limit", In: "query", SchemaType: "integer", Description: "Page size, 1 to 100 (default 50)"}),
		apix.WithParameter(apix.Parameter{Name: "cursor", In: "query", Description: "Opaque cursor from meta.pagination.next_cursor"}),
		documentProblems(http.StatusBadRequest, http.StatusInternalServerError),
	)

	// Route 5: idempotent create.
	apixchi.Post(jsonRoutes, "/grants", func(ctx context.Context, req *apixGrantRequest) (grant, error) {
		g, _, replayed, err := st.createGrant(requestOf(ctx).Header.Get("Idempotency-Key"), req.Title, req.Amount)
		if errors.Is(err, errIdempotencyReuse) {
			return grant{}, conflict(err.Error())
		}
		if err != nil {
			return grant{}, err
		}
		h := responseHeader(ctx)
		h.Set("Location", "/grants/"+g.ID)
		if replayed {
			h.Set("X-Idempotency-Replay", "true")
		}
		return g, nil
	},
		apix.WithOperationID("create-grant"),
		apix.WithSummary("Create a grant"),
		apix.WithParameter(apix.Parameter{Name: "Idempotency-Key", In: "header", Description: "Makes a retried create safe: the same key and body replay the first response"}),
		apix.WithSuccessHeaders(http.StatusCreated,
			apix.HeaderRef{Name: "Location"},
			apix.HeaderRef{Name: "X-Idempotency-Replay", Description: `"true" when this is the stored response to an earlier request with the same key`}),
		documentProblems(http.StatusBadRequest, http.StatusConflict, http.StatusUnsupportedMediaType, http.StatusInternalServerError),
	)

	// Route 6.
	apixchi.Post(oauth, "/oauth2/token", func(ctx context.Context, req *apixTokenRequest) (tokenResponse, error) {
		tok, oerr := st.token(req.GrantType, req.ClientID, req.ClientSecret, req.Scope)
		if oerr != nil {
			return tokenResponse{}, oerr
		}
		h := responseHeader(ctx)
		h.Set("Cache-Control", "no-store")
		h.Set("Pragma", "no-cache")
		return tok, nil
	},
		apix.WithOperationID("issue-token"),
		apix.WithSummary("Issue an access token"),
		apix.WithFormURLEncoded(),
		apix.WithSuccessStatus(http.StatusOK),
		apix.WithSuccessHeaders(http.StatusOK, apix.HeaderRef{Name: "Cache-Control"}, apix.HeaderRef{Name: "Pragma"}),
		apix.WithCustomErrorResponse(http.StatusBadRequest, oauthError{}, "RFC 6749 error"),
		apix.WithCustomErrorResponse(http.StatusUnauthorized, oauthError{}, "RFC 6749 error"),
	)
}
