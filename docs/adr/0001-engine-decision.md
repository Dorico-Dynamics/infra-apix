# ADR-0001: The engine behind apix v2: native rewrite or huma v2

## Status

Proposed. Awaiting the maintainer's decision.

## Date

2026-11-07

## Context

apix v1 turns a typed handler, `func(ctx, *TReq) (TResp, error)`, into an
HTTP route on chi, echo v4, gin, gorilla/mux or fiber v3, and builds an
OpenAPI 3.1 document from a global route registry. The next major version has
to do more than v1 can:

- bind and validate typed path, query and header inputs, not only the body;
- document every constraint the runtime enforces (bounds, formats, enums,
  defaults), so the document and the runtime cannot disagree;
- decode JSON, multipart and form bodies strictly: a wrong Content-Type is
  415, a body over its cap is 413, an unknown JSON field is 400;
- answer every error in one platform error format, RFC 9457
  `application/problem+json` with `code`, `request_id`, `correlation_id` and
  field errors `{field, code, message}`, and let one operation use another
  format (RFC 6749 errors on an OAuth2 token endpoint);
- serve on net/http's ServeMux as well as the five routers v1 supports;
- mount typed routes beside existing handlers, under the router's middleware;
- emit valid, deterministic OpenAPI 3.1 and 3.0.3 documents with no dangling
  `$ref`.

v1's handler sees only the decoded body. Path, query and header values reach
it through the request context (a "context locator"), validation is off
unless a Validator is set, and the document carries only `required` from
`validate` tags. v1 decodes JSON only, has no ServeMux adapter and emits no
3.0.3 document.

The question is whether to build these capabilities natively or to adopt
[huma v2](https://github.com/danielgtaylor/huma) as the engine. Either way,
the layers above the engine stay apix's: platform conventions, the CLI,
document serving and the docs UI.

### Options

- **Status quo:** keep the v1 engine and fix only bugs. Every gap in Results
  stays open, so it is not scored.
- **Option A, native rewrite:** grow apix's own engine to close the gaps.
- **Option B, adopt huma v2:** apix v2 is the platform and tooling layers on
  top of huma. The v1 line keeps its engine and gets fixes only.

## Method

The spike is the Go module in [`0001-engine-spike/`](0001-engine-spike/). It
has its own `go.mod`, replaces apix with this checkout, and is outside the
apix module and its CI. It ports six routes into both engines. Each route
covers a case v1 handles badly:

1. An action, `POST /phases/{id}/progress`: If-Match is required (428 without
   it, 412 when stale), and it answers 200, not 201, with the new ETag.
2. A PDF download, `GET /pay-stubs/{payPeriodID}/pdf`, with
   `Content-Disposition`.
3. A multipart statement upload: a file and an enum field, capped at 1 MiB.
4. One handler under `/v1` and `/v2`, with a 32 KiB body cap and
   `Cache-Control: no-store`, on net/http's ServeMux.
5. A cursor-paginated list, and a create that honours `Idempotency-Key`
   (a replay returns the first response; a reused key with another body is
   409).
6. An OAuth2 token endpoint: form-urlencoded, with RFC 6749 errors instead
   of the platform error.

Both engines share the domain code (an HTTP-free store) and the platform
error type. "Glue" is the code each engine needed beyond itself. Each row
runs on a fresh engine. Every response is checked against the engine's own
document with kin-openapi's `openapi3filter`, so a row fails when the runtime
is wrong or when a response disagrees with the document. Many requests are
invalid on purpose, so request validation is not scored. Instead, rows read
the request side of the document directly: bounds, formats, enums, defaults
and required headers.

The error codes and their statuses are:

| Code | Status |
|---|---|
| `validation_failed` | 400 |
| `not_found` | 404 |
| `conflict`, `version_mismatch` | 409 |
| `precondition_failed` | 412 |
| `payload_too_large` | 413 |
| `unsupported_media_type` | 415 |
| `precondition_required` | 428 |
| `internal_error` | 500 |

The field codes are `required`, `invalid_format`, `too_short`, `too_long` and
`out_of_range`.

The routes are modelled on real services, but where a service differs from
the target behaviour, the spike tests the target:

- The first route's model has no such action, so it was built to the
  contract.
- The upload's model enforces no effective cap, and it accepts
  case-insensitive aliases for the format. A strict enum rejects those
  aliases, which is a migration decision for that service.
- The token endpoint's model answers problem+json without `no-store`. The
  spike tests RFC 6749 §5.1 and §5.2.
- The platform catalog has no 412 or 428 codes; the spike adds
  `precondition_failed` and `precondition_required`.

Measured on 2026-11-07 with Go 1.27.3 on darwin/arm64 (Apple M3 Pro), against
apix main at `2c05554` (v1.9.2 plus documentation-only changes), huma
v2.40.0, kin-openapi v0.149.0 and chi v5.5.0.

## Results

### Conformance: apix v1 28 of 38, huma v2 36 of 38

| Row | Checks | apix v1 | huma v2 |
|---|---|---|---|
| A1 | Typed route beside a legacy handler on one chi router, under its middleware | pass | pass |
| R1a | Action POST answers 200 with the new ETag | pass | pass |
| R1b | Stale If-Match → 412 | pass | pass |
| R1c | Missing If-Match → 428; If-Match documented as required | pass | pass |
| R1d | Body field out of range → 400 naming the JSON field; bounds documented | **fail:** no minimum/maximum in the document | pass |
| R1e | Path UUID bound from the template; bad value → 400 naming it; format documented | **fail:** no `format: uuid` | pass |
| R1f | Unknown resource → 404 | pass | pass |
| R2a | PDF with Content-Disposition; raw bytes documented | **fail:** documented as base64 (`format: byte`), so the bytes sent fail the document | pass |
| R2b | Integer path parameter below its minimum or not an integer → 400 naming it; bounds documented | **fail:** no minimum | pass |
| R2c | An error on a binary route is still the platform error | pass | pass |
| R3a | Multipart upload → 200 summary; form documented | pass | pass |
| R3b | Upload over the cap → 413 | pass | pass |
| R3c | JSON sent to the multipart route → 415 | pass | **fail:** 400 `validation_failed` |
| R3d | Form field outside its enum → 400 naming it; enum documented | **fail:** no enum | pass |
| R3e | Missing file part → 400 naming it | pass | pass |
| R4a | v1 and v2 aliases share a handler, with distinct operationIds; no-store sent | pass | pass |
| R4b | Served by net/http's ServeMux | **fail:** no ServeMux adapter | pass |
| R4c | Stale revision → 409 `version_mismatch`, still no-store | pass | pass |
| R4d | JSON body over the route's cap → 413 | pass | pass |
| R4e | A 5xx never echoes internals | pass | pass |
| R5a | `next_cursor` chains pages and is absent on the last | pass | pass |
| R5b | Query parameter out of bounds → 400 naming it; bounds and default documented | **fail:** no bounds or default | pass |
| R5c | A cursor the API did not issue → 400 naming it | pass | pass |
| R5d | Idempotency-Key replay: same 201, body and Location, `X-Idempotency-Replay: true`, one record | pass | pass |
| R5e | Idempotency-Key reused with another body → 409 | pass | pass |
| R6a | Token from a form body → 200 with no-store; form documented | **fail:** no enum for `grant_type` | pass |
| R6b | Unsupported `grant_type` → 400 in RFC 6749 format (a per-operation error format) | pass | pass |
| R6c | Missing parameter → 400 `invalid_request` | pass | pass |
| R6d | Bad client secret → 401 `invalid_client` | pass | pass |
| R6e | JSON sent to the token endpoint → 400 `invalid_request` | pass | **fail:** 200, token issued |
| S1 | Document valid as OpenAPI 3.1 | pass | pass |
| S2 | Valid OpenAPI 3.0.3 rendition | **fail:** none | pass |
| S3 | No dangling `$ref` | pass | pass |
| S4 | Two builds give identical bytes | pass | pass |
| X1 | Malformed JSON → 400 | pass | pass |
| X2 | A JSON route refuses another Content-Type → 415 | **fail:** 201, body decoded | pass |
| X3 | Unknown JSON field → 400 | pass | pass |
| X4 | String shorter or longer than its bounds → 400 naming it as `too_short` or `too_long` | pass | pass |

**apix v1 failures.**
- Six rows (R1d, R1e, R2b, R3d, R5b and R6a) behave correctly at runtime
  through glue but fail on the document. v1's builder reads `json`,
  `description`, `example`, `format` (binary only) and the `required` in
  `validate`, so bounds, formats, enums and defaults never reach the
  document.
- R2a: v1 documents `[]byte` as base64.
- R4b and S2 are missing features.
- X2: the default decoder ignores Content-Type.

Closing these is the rewrite in Option A.

**huma v2 failures.** R3c and R6e have one cause. huma picks the decoder from
the request's Content-Type among the API-wide formats (`api.go:355-373`), not
from the media types the operation documents, and it decodes a request with
no Content-Type as JSON. Option B needs one operation-level check that
compares the request's media type with the operation's documented ones. The
spike leaves it out, so the scoreboard shows huma as shipped plus the glue
below.

### What each engine needed

| | apix v1 | huma v2 |
|---|---|---|
| Glue, code lines | 249 | 171 |
| Routes and their input/output types, code lines | 233 | 253 |
| Context-locator calls | 7 | 0 |
| Adapter instances or APIs, to vary decoding, errors and middleware per route | 4 | 1 |

A code line is a line that is neither blank nor a whole-line comment:
`grep -c -v -E '^[[:space:]]*(//|$)' <file>`.

The apix glue (`glue_apix_test.go`) contains:
- a context locator;
- go-playground validation with JSON field names, and its field codes;
- the platform renderer, set as the ErrorHandler of every adapter instance;
- a raw-bytes encoder;
- path parameter parsing (v1 neither binds nor validates path parameters);
- problem+json and binary response documentation;
- multipart and form decoders with Content-Type and cap checks;
- RFC 6749 rendering;
- per-route cap and no-store middleware.

The huma glue (`glue_huma_test.go`) contains:
- replacements for huma's package-level `NewError` and
  `NewErrorWithContext`, which render the platform error and choose RFC 6749
  per operation from its metadata;
- field errors built from huma's details. These carry a location and an
  English message but no code, so the code is read from huma's exported
  message formats (`validation.Msg*`);
- a transformer that stamps `instance`, `request_id` and `correlation_id`;
- the OAuth2 error schema;
- a binary response declaration (huma documents `[]byte` as base64 JSON);
- a no-store operation middleware;
- a form-urlencoded format (huma ships JSON and CBOR).

The huma config also drops its `CreateHooks`.

### Defaults, before any configuration

| Engine | Case | Status, Content-Type | Body |
|---|---|---|---|
| huma | success | 200 `application/json`, plus a `Link: </schemas/Body.json>; rel="describedBy"` header | adds `"$schema":"http://127.0.0.1:<port>/schemas/Body.json"` |
| huma | validation failure | 422 `application/problem+json` | echoes the value: `{"message":"expected length >= 3","location":"body.name","value":"no"}` |
| huma | malformed JSON | 400 `application/problem+json` | echoes the whole body, secret included: `"value":"{\"name\": \"secret-value\", "` |
| huma | handler returns a plain error | 500 `application/problem+json` | echoes it: `"message":"dial tcp 192.0.2.7:5432: connection refused"` |
| huma | `GET /docs`, `GET /openapi.json` | 200 | served by default |
| apix | `validate:"min=3"` violated, no Validator set | 201 `application/json` | accepted |
| apix | malformed JSON | 400 `text/plain` | `request body is not valid JSON` |
| apix | handler returns a plain error | 500 `text/plain` | `Internal Server Error` |
| apix | handler returns `apix.NotFound` | 404 `text/plain` | `http 404 [NOT_FOUND]: thing not found` |

Neither engine's defaults match the platform contract. huma's leak: the 500
echoes the error, and 400 and 422 echo what the client sent. Under Option B,
apix v2 must own huma's configuration.

### Adapters

| Router | apix v1 | huma v2.40.0 |
|---|---|---|
| chi v5 | yes | `humachi` |
| echo v4 | yes | `humaecho.NewV4` (the package also imports echo v5) |
| echo v5 | no | `humaecho.New` |
| gin | yes | `humagin` |
| gorilla/mux | yes | `humamux` |
| fiber v3 | yes | `humafiber` (the package also imports fiber v2) |
| net/http ServeMux | no | `humago` |
| httprouter, bunrouter, flow | no | yes |

The spike ran apix on chi and huma on ServeMux, and A1 ran both on chi. Parity
across the other adapters is not measured. huma's adapters translate the
router's context into one shared core.

### Footprint

The probes are in [`0001-engine-spike/footprint/`](0001-engine-spike/footprint/),
one module each. Each probe serves one typed POST route and was built with
`CGO_ENABLED=0 go build -trimpath`. The huma probes use `huma.DefaultConfig`,
which also serves the document. Sizes are for darwin/arm64, where Mach-O pads
segments to 16 KiB; module counts do not depend on the platform.

| Probe | Binary bytes | Modules linked | `go.sum` lines |
|---|---|---|---|
| apix v1 + chi | 11,990,034 | 9 | 36 |
| huma v2 + chi | 10,668,098 | 2 | 14 |
| apix v1 + echo v4 | 12,451,906 | 17 | 52 |
| huma v2 + echo v4 | 11,201,954 | 12 (echo v4 and v5) | 36 |

Serving the document through apix's `runtime` package is a separate cost,
independent of this decision. `runtime` imports echo v4 for `RegisterEcho`
and embeds the Swagger UI, so a chi service that imports it links echo v4 and
nine more modules. The `apix-chi-runtime` probe is 20,326,946 bytes with 19
modules.

### Per-request cost

These benchmarks run in process (`httptest.NewRecorder`), with
`-count=6 -benchmem`. The figures are benchstat medians with 95% confidence
intervals. The apix figures include the glue v1 needs to do the same work.

| Benchmark | apix v1 + chi | huma v2 + chi | huma v2 + ServeMux |
|---|---|---|---|
| `UpdateProgress` (UUID path parameter, If-Match, bounded JSON body, ETag) | 3.720 µs ± 1%, 48 allocs | 4.037 µs ± 0% (+8.5%), 49 allocs | 3.855 µs ± 1% (+3.6%), 46 allocs |
| `ListGrants` (bounded query parameter, JSON page) | 2.734 µs ± 1%, 32 allocs | 2.723 µs ± 1% (−0.4%), 29 allocs | 2.573 µs ± 0% (−5.9%), 27 allocs |

### Migration cost

These counts come from three consumer services: two on chi and one on echo,
all on apix v1.8.0. They were taken from non-test Go files on each main
branch on 2026-11-07.

| | Service 1 (chi) | Service 2 (chi) | Service 3 (echo) |
|---|---|---|---|
| Typed route registrations | 98 | 174 | 145 |
| Context-locator calls (request or response writer from the context) | 8 | 35 | 0 |
| `chi.URLParam*` calls | 2 | 8 | 0 |
| `URL.Query()` calls | 4 | 5 | 6 |
| Reads of request-context values that middleware sets | 0 | 19 | 113 |
| `validate:"…"` tags | 61 | 384 | 138 |
| … with constraints beyond `required`/`omitempty` | 29 | 220 | 14 |

Both options change all 417 registrations, because typed inputs replace the
locator calls and manual reads. Context values set by middleware stay as they
are: A1 shows a typed handler reading them under both engines. Option B also
translates the `validate` tags into huma's tags (`minimum`, `maxLength`,
`enum`, `format` and so on), because huma reads no `validate` tag. That is
583 tags, 263 with constraints, and the translation is mechanical, with one
exception. go-playground's `required` also rejects zero values, while JSON
Schema's `required` asks only that the field be present. Sent an empty
title, v1 names the field `required` and huma `too_short`. The migration must
either map that code back or announce the change. Option B also moves request
and response bodies into a `Body` field.

### huma as a dependency

- **Licence:** MIT ("Copyright 2020 Daniel G. Taylor").
- **Go version:** huma's `go.mod` requires Go 1.26.
- **Releases:** 11 in 2026, from v2.34.2 (2026-01-16) to v2.40.0
  (2026-10-27). The longest gap was 90 days.
- **Activity:** 137 contributors. 132 commits landed on main in the 12 months
  to 2026-11-07; the most active contributor wrote 52 of them.
- **Extensions:** per-operation `x-` extensions are supported
  (`Operation.Extensions`). apix v1 has none.

### Weighted evaluation

Scores run from 0 to 5. Measured criteria score five times the passed
fraction; the rest are judgements on the evidence above.

| Criterion | Weight | A: native (v1 today) | B: huma v2 | Basis |
|---|---|---|---|---|
| Conformance coverage | 30% | 3.75 | 4.69 | 24 vs 30 of the 32 route rows (R1a–R6e and X1–X4, without R4b) |
| Error-contract fidelity | 15% | 5 | 5 | Every error body either engine produced has the contract's shape: its members, a type, title and status consistent with its code, the IDs, and field names and codes. Which error a case gets is scored under conformance, where huma's wrong 400 (R3c) and missing errors (R6e for huma, X2 for apix) already count |
| Adapter coverage, stdlib included | 15% | 4.17 | 5 | 5 vs 6 of chi, echo v4, gin, gorilla/mux, fiber v3 and ServeMux |
| Incremental adoption | 10% | 5 | 5 | A1 |
| Spec quality | 10% | 3.75 | 5 | 3 vs 4 of S1–S4 |
| Code we own | 10% | 2 | 4 | A owns the engine: 4,017 non-test lines today[^lines], plus every gap above. B owns about 170 lines of glue, coupled to huma's error constructors and message formats |
| Footprint | 5% | 4 | 5 | B links 2 modules against 9 (chi) and 12 against 17 (echo), in smaller binaries |
| Migration cost | 5% | 4 | 3 | Both change all 417 routes; B also translates 583 tags |
| **Weighted total** | | **3.975** | **4.706** | |

[^lines]: `git ls-files '*.go' ':!*_test.go' ':!examples/**' ':!docs/**' ':!runtime/**' ':!cmd/**' ':!internal/swaggerui/**' ':!openapi/internal/collision/**' | xargs cat | wc -l`
    at `2c05554`. For comparison, huma v2.40.0's root package alone has
    9,392 non-test lines.

Suppose the rewrite took A to full marks on conformance, adapters and spec
quality. Against B with the operation-level media-type check, A would still
total 4.600 to B's 4.800. The gap is the code A must own.

## Decision (proposed)

**Adopt huma v2 as the engine of apix v2 (Option B).**

- apix v2 keeps the platform layer (error contract, request IDs, decode
  policy, conventions) and the tooling layer (CLI, document serving, docs
  UI).
- Services get their API from apix v2, which installs the platform
  configuration. They do not build one with `huma.DefaultConfig`.
- The v1 line keeps its engine and gets fixes only.

Adoption comes with these conditions:

1. Pin huma to an exact version. Bump it only in a PR that runs the
   conformance suite.
2. Move the spike's huma glue into apix v2 and add the operation-level
   media-type check (R3c, R6e).
3. Configure away huma's defaults that conflict with the contract: the
   `$schema` member and `Link` header, 422 for validation, echoed values and
   errors, and the `/docs` and `/schemas` routes (apix serves its own
   document and UI).
4. Grow the spike's rows into a conformance suite that runs in apix CI against
   every adapter.
5. Write the migration codemods: `validate` tags to huma tags, and context
   locators to typed inputs.

## Consequences

### Positive

- v2 gets typed inputs, documented constraints, multipart and form bodies,
  per-operation caps and middleware, ServeMux, OpenAPI 3.0.3 and `x-`
  extensions without building them.
- The probes link fewer modules: 2 against 9 with chi.
- Per-request cost stays within 9% of v1 in both benchmarks.
- v2 work shrinks to glue, the conformance suite and codemods.

### Negative

- apix v2 depends on a third-party engine: its release cadence, its bugs and
  its API changes.
- Every route changes (as in A), and 583 `validate` tags are translated.
- huma's error constructors are process-wide. A process has one error
  policy, and the per-operation choice is made inside it.
- Field codes are derived from huma's message formats.
- `humaecho` and `humafiber` link two major versions of their router.

### Risks

| Risk | Mitigation |
|---|---|
| Upstream stalls or diverges | Pinned version; the conformance suite on every bump; the platform layer keeps huma behind apix where practical; v1 stays until consumers migrate; MIT allows a fork as a last resort |
| A service bypasses apix v2 and gets huma's defaults, which leak 500 messages and echo input | Services get their API from apix v2; a CI check rejects `huma.DefaultConfig` in services |
| A huma bump changes validation messages and so, silently, field codes | Conformance rows pin all five field codes (X4 covers `too_short` and `too_long`) |
| JSON is accepted where the operation documents another media type | The operation-level check (condition 2), covered by R3c and R6e |

If Option A is chosen instead, apix owns the engine and builds typed binding,
constraint documentation, a 3.0.3 rendition, multipart and form decoding, a
ServeMux adapter and per-operation caps. It adds no engine dependency and
keeps `validate` tags.

## Not measured

- Conformance on the echo, gin, gorilla/mux and fiber adapters.
- Real consumer route sets under huma, and document generation for them.
- Streaming (SSE, WebSocket), large bodies and concurrency. The benchmarks
  are in-process microbenchmarks.

## Reproduce

```bash
cd docs/adr/0001-engine-spike
go test ./...   # a failing row is a measurement, not a test failure
SPIKE_SUMMARY=/tmp/summary.md SPIKE_DEFAULTS=/tmp/defaults.txt go test -count=1 ./...
go test -run '^$' -bench . -benchmem -count=6 ./...

cd footprint
for p in apix-chi huma-chi apix-echo huma-echo apix-chi-runtime; do
  (cd "$p" && CGO_ENABLED=0 go build -trimpath -o "/tmp/$p" .)
  echo "$p $(wc -c < "/tmp/$p") bytes, $(go version -m "/tmp/$p" | awk '$1 == "dep"' | wc -l) modules, $(wc -l < "$p/go.sum") go.sum lines"
done
```

`SPIKE_SUMMARY` receives the scoreboard and `SPIKE_DEFAULTS` the defaults
table. The spike and the apix probes resolve apix to this checkout through a
`replace`.
