# Product Requirements Document: Go API Documentation Automation Library

> **Status, 2026-11-07.** This is the original product requirements
> document for apix v1. Its goals still describe the project. For what v1
> delivers and what remains, see the README's
> [Status and roadmap](README.md#status-and-roadmap). The remaining work is
> tracked as GitHub issues, listed in the
> [roadmap](https://github.com/Dorico-Dynamics/infra-apix/issues/52).

### Problem Statement

Go teams lack a robust, code-first way to generate OpenAPI documentation. Comment-based tools (swaggo) are brittle and encourage manual YAML editing—now turbocharged by AI agents that “fix” issues by rewriting generated specs. We need deterministic, code-driven docs that keep developers (and agents) out of the spec file.

### Goals

1. **Code-First OpenAPI 3.1 Generation**
   - Derive routes, request/response models, headers, and security information directly from Go handlers.
   - Produce deterministic specs: sorted paths/components, stable operationIds, no per-run differences.
   - Bake in DX/security defaults (201 Location, 401/403 on secured routes, shared error schema, examples).

2. **Framework Integration** (v1)
   - Adapters for Echo, Chi, Gorilla/Mux.
   - Support existing handler signatures with wrapper helpers; no massive refactor required.

3. **Developer Experience**
   - go:generate CLI to write the spec (yaml/json) with DO-NOT-EDIT header.
   - Runtime endpoints: `/openapi.json` (always current) and optional `/swagger/*` behind a feature flag.
   - Clear migration guide (swaggo → typed wrappers).

4. **Guardrails & Enforcement**
   - Prevent manual edits: DO-NOT-EDIT header, CODEOWNERS, CI drift check, pre-commit hook.
   - MCP\`spec_guard\`: auto-regenerate, block completion, and emit guidance if spec is touched.

5. **Security & Compliance**
   - Auto-detect auth middleware; inject security schemes and 401/403 responses.
   - Verify Location/ETag/pagination headers are documented where applicable.

6. **Extensibility**
   - Configurable server URLs, tag mapping, exclude globs, plugin hooks for custom metadata.

### Non-Goals

- Spec-first DSL or full swaggo replacement in v1 (gradual migration supported).
- Automatic annotation of legacy code (MCP covers transitional retrofits).
- Frameworks beyond Echo/Chi/Mux in v1.

### Stakeholders

- Backend engineers (DX, correctness).
- DevSecOps (compliance, security).
- AI toolchain (agents must call safe APIs, not edit specs).

### Success Metrics

- 100% parity between routes and generated spec (CI drift gate passes).
- No manual edits to `openapi.yaml` post-migration.
- Onboard a new route with typed handler and doc generation in <5 minutes.
- Production adoption in a Dorico Dynamics LDA service.

### Milestones

**Milestone 1 (v0.1)**
- Echo adapter with typed helpers (`Get/Post/...` using generics `Handler[TReq,TResp]`).
- Struct tag parsing (`json`, `validate`, `binding`), pointer handling, wrapper detection.
- Spec builder (kin-openapi), deterministic output, 201/401/403 defaults, shared error schema, examples.
- CLI: `apix generate`, runtime `/openapi.json`, optional Swagger UI.
- Integrate into a Dorico Dynamics LDA service (subset of routes); enable CI drift check.

**Milestone 2 (v0.2)**
- Chi & Gorilla/Mux adapters.
- Typed query/header parameter structs with type inference.
- Middleware discovery for security; custom security schemes.
- DX extras: pagination headers, ETag, standard 4xx/5xx responses.
- Golden tests + integration tests.

**Milestone 3 (v0.3)**
- Structured examples via tags/helpers.
- Additional content types (multipart/form-data, form-urlencoded).
- Plugin hooks for custom metadata.
- Observability (logging, metrics).
- Migration guide for swaggo users.

### Risks & Mitigations

- **Reflection limits**: Complex handlers might need explicit overrides → provide `WithRequestModel/WithResponseModel` helpers.
- **Performance**: Large codebases may slow generation → incremental parsing, caching, go:generate usage.
- **Migration friction**: Thousands of routes to convert → wrappers support incremental adoption.
- **AI bypass**: Agents may still edit spec → spec_guard + CI drift + DO-NOT-EDIT header.

### Open Questions

- Should runtime always compute spec, or cache go:generate output? (lean: runtime + optional write-to-file).
- How to add gin/fiber later without duplicated logic?
- Embed Swagger UI assets or rely on external packages? (lean: embed minimal UI).
- Final naming (library + CLI). Working title `apix`.

### Next Steps

1. Approve PRD scope and milestones.
2. Scaffold new Go module (core library + examples).
3. Build Milestone 1 and integrate with a Dorico Dynamics LDA service as proof of concept.
4. Add spec_guard + CI drift enforcement.
5. Gather feedback, iterate toward Milestones 2 & 3.

