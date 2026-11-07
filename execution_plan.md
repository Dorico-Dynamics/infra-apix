# Execution Plan — apix Go API Documentation Library

## Context & Research Findings

### Why a New Library
- Current Go OpenAPI tooling (swaggo, spec-first generators) relies on comments or YAML edits, allowing AI agents and developers to drift from code.
- We need code-first, deterministic OpenAPI 3.1 generation with runtime + go:generate workflows, aligned with 2025 security/DX standards.

### Reflection Limits & Overrides
- **Spring Boot**: Uses runtime annotations and allows explicit overrides via `@Schema`, `OpenApiCustomiser` when reflection falls short.
- **FastAPI**: Relies on type hints (Pydantic); complex types require explicit Pydantic models or `Annotated` metadata.
- **Go ecosystem**: swaggo uses comment overrides; goa/ogen are DSL/spec-first. Kin-openapi is the building block.
- **adopted strategy**:
  - Route options such as `WithRequestOverride`, `WithExplicitRequestModel` and `WithSuccessHeaders`, and the response options `WithExplicitModel` and `WithResponseExample`, define schemas, content types, headers and examples manually when necessary.
  - Plugins (`apix.RegisterPlugin`), whose hooks see each route, each schema and the built document, mirror Spring's OpenApiCustomiser.
  - Config file (future milestone) to map `method:path` to overrides without code changes.

## Implementation Status

### ✅ Milestone 1 - COMPLETE
1. **Core Registry** ✅ - Thread-safe route metadata storage, deterministic snapshot, success status, security, headers
2. **Echo Adapter** ✅ - Typed registration helpers, custom decoders, validation hooks
3. **Spec Builder** ✅ - OpenAPI 3.1 generation with kin-openapi, deterministic output, 201/401/403 defaults
4. **CLI** ⚠️ - `apix generate` and `spec-guard` commands with DO-NOT-EDIT header. They build the spec from the routes registered in their own process, so they find none of an application's routes; applications write the spec from their own program instead (Programmatic Generation in `docs/OPENAPI_GENERATION.md`)
5. **Runtime Endpoints** ✅ - `/openapi.json` + optional Swagger UI, caching
6. **Tests** ✅ - CI runs the suite with the race detector and fails below 80% total coverage

### ✅ Milestone 2 - COMPLETE
1. **Chi Adapter** ✅ - Full Chi router integration with typed handlers
2. **Gorilla/Mux Adapter** ✅ - Full Mux router integration with typed handlers
3. **Gin Adapter** ✅ - Full Gin framework integration with typed handlers
4. **Fiber Adapter** ✅ - Full Fiber v3 framework integration with typed handlers
5. **Shared Error Schema** ✅ - Standard ErrorResponse with 4xx/5xx helpers
6. **Golden Tests** ✅ - Comprehensive CRUD spec validation and determinism tests
7. **Integration Tests** ✅ - Full integration tests for all 5 framework adapters

### 🚧 Milestone 3 - IN PROGRESS
**Done:**
- Structured examples via the `example` tag, `WithRequestExample` and `WithResponseExample`
- Additional request content types: `WithMultipartFormData` and `WithFormURLEncoded` document them; the default decoders read JSON
- Plugin hooks for custom metadata (`apix.RegisterPlugin`)
- Structured logging (`apix.SetLogger`)
- Migration guide for swaggo users
- Example applications for Echo, Chi, Gin and Gorilla/Mux

**Planned:**
- **Typed Query/Header Parameters** - Struct-based parameter extraction with type inference
- **Middleware Auto-detection** - Automatic security scheme detection from middleware stack
- **DX Extras** - Pagination headers (Link, X-Total-Count), ETag support
- Multipart and form decoding in the adapters
- Metrics
- An example application for Fiber
- A CLI that loads the application's routes

## Planned Work

1. **Typed Query/Header Parameters**
   - Support struct-based query parameter extraction
   - Infer parameter types from struct tags
   - Generate OpenAPI parameter definitions

2. **Pagination Headers**
   - Auto-detect pagination patterns (offset/limit, cursor)
   - Document Link, X-Total-Count headers
   - Helper functions for common pagination schemes

3. **Middleware Auto-detection**
   - Scan middleware stack for JWT/Bearer auth
   - Auto-inject security schemes
   - Support custom middleware detection hooks

## Risks & Mitigations
- Reflection misses → mitigated by override options and plugins ✅
- Agent spec edits → DO-NOT-EDIT header and CI drift checks on the committed spec ⏳: the CLI's `spec-guard` finds no routes, so applications regenerate the spec with their own program and fail on `git diff`
- Performance on large services → the runtime handler builds the document once and keeps it ✅
- Adoption friction → wrappers support incremental migration; documentation and examples prioritized ⏳
