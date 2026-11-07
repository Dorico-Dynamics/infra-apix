# Contributing to apix

Thank you for your interest in contributing to `apix`! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Code Style](#code-style)
- [Submitting Changes](#submitting-changes)
- [Release Process](#release-process)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code:

- Be respectful and inclusive
- Welcome newcomers and help them learn
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Prerequisites

- Go 1.27.3 or later
- Git
- Make (optional, but recommended)
- Basic understanding of OpenAPI 3.1 specification

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork:

```bash
git clone https://github.com/YOUR_USERNAME/apix.git
cd apix
```

3. Add upstream remote:

```bash
git remote add upstream https://github.com/Dorico-Dynamics/infra-apix.git
```

## Development Setup

### Install Dependencies

```bash
go mod download
```

### Run Tests

```bash
# Run all tests
make test

# Run tests with coverage
make cover

# Generate coverage report
make cover-html
```

### Run Examples

```bash
# Echo example
cd examples/dorico-dynamics
go run main.go

# Chi example
cd examples/dorico-dynamics-chi
go run main.go

# Gin example
cd examples/dorico-dynamics-gin
go run main.go

# Mux example
cd examples/dorico-dynamics-mux
go run main.go
```

## Project Structure

```
apix/
├── chi/              # Chi framework adapter
├── cmd/
│   └── apix/         # CLI tool
├── docs/             # Documentation
├── echo/             # Echo framework adapter
├── examples/         # Example applications
├── fiber/            # Fiber framework adapter
├── gin/              # Gin framework adapter
├── mux/              # Gorilla/Mux adapter
├── openapi/          # OpenAPI builder and encoding
├── runtime/          # Runtime server for serving specs
├── tests/            # Integration and golden tests
├── errors.go         # Error types
├── registry.go       # Route registry
└── README.md         # Main documentation
```

### Key Components

**Core (`registry.go`):**
- Route registration and metadata storage
- Thread-safe global registry
- Route options and configuration

**Adapters (`echo/`, `chi/`, `mux/`, `gin/`, `fiber/`):**
- Framework-specific route registration
- Request/response handling
- Type-safe handler wrappers

**OpenAPI Builder (`openapi/`):**
- Schema generation from Go types
- OpenAPI 3.1 document construction
- YAML/JSON encoding

**Runtime (`runtime/`):**
- HTTP server for serving specs
- Swagger UI integration
- Caching and validation

**CLI (`cmd/apix/`):**
- `generate` command for static spec generation
- `spec-guard` command for drift detection

## Making Changes

### Create a Branch

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

### Branch Naming

- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test improvements

### Commit Messages

Follow conventional commits format:

```
type(scope): subject

body (optional)

footer (optional)
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `ci`: CI and release workflows
- `build`: Toolchain and dependencies
- `chore`: Maintenance tasks

**Examples:**

```
feat(chi): add support for middleware detection

fix(openapi): handle nullable nested structs correctly

docs(readme): update installation instructions

test(echo): add integration tests for error handling
```

## Testing

### Test Requirements

- **Minimum 80% code coverage** for all new code
- All tests must pass before submitting PR
- Add tests for bug fixes to prevent regressions
- Include integration tests for new features

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Check coverage
go tool cover -func=coverage.out

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html
```

### Writing Tests

**Unit Tests:**

```go
func TestRouteRegistration(t *testing.T) {
    apix.ResetRegistry()
    
    ref := &apix.RouteRef{
        Method: apix.MethodPost,
        Path:   "/api/users",
    }
    
    apix.RegisterRoute(ref)
    
    routes := apix.Snapshot()
    if len(routes) != 1 {
        t.Errorf("expected 1 route, got %d", len(routes))
    }
}
```

**Integration Tests:**

```go
func TestEchoAdapter(t *testing.T) {
    apix.ResetRegistry()
    
    e := echo.New()
    adapter := echoadapter.New(e)
    
    echoadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create user"),
    )
    
    routes := apix.Snapshot()
    if len(routes) != 1 {
        t.Fatalf("expected 1 route, got %d", len(routes))
    }
    
    if routes[0].Summary != "Create user" {
        t.Errorf("expected summary 'Create user', got %q", routes[0].Summary)
    }
}
```

**Golden Tests:**

For OpenAPI spec generation, use golden tests:

```go
func TestOpenAPIGeneration(t *testing.T) {
    // Setup routes
    apix.ResetRegistry()
    // ... register routes ...
    
    // Generate spec
    builder := openapi.NewBuilder()
    doc, err := builder.Build(apix.Snapshot())
    if err != nil {
        t.Fatal(err)
    }
    
    // Compare with golden file
    data, _, _ := openapi.EncodeDocument(doc, "yaml")
    golden := filepath.Join("testdata", "expected.yaml")
    
    if *update {
        os.WriteFile(golden, data, 0644)
    }
    
    expected, _ := os.ReadFile(golden)
    if !bytes.Equal(data, expected) {
        t.Error("spec does not match golden file")
    }
}
```

## Code Style

### Go Style Guide

Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) and standard Go conventions:

- Use `gofmt` for formatting
- Use `go vet` for static analysis
- Follow Go naming conventions
- Write clear, self-documenting code
- Add comments for exported functions and types

### Formatting

```bash
# Format code
go fmt ./...

# Run go vet
go vet ./...
```

### Documentation

- Add godoc comments for all exported types and functions
- Include examples in documentation
- Update README.md for user-facing changes
- Add entries to CHANGELOG.md

**Example:**

```go
// WithSummary sets the operation summary (short description).
// The summary appears in API documentation and Swagger UI.
//
// Example:
//
//	apix.WithSummary("Create a new user")
func WithSummary(summary string) RouteOption {
    return func(ref *RouteRef) {
        ref.Summary = summary
    }
}
```

## Submitting Changes

### Before Submitting

1. **Run tests:**
   ```bash
   make test
   ```

2. **Check coverage:**
   ```bash
   make cover
   ```

3. **Format code:**
   ```bash
   go fmt ./...
   ```

4. **Run linter:**
   ```bash
   go vet ./...
   ```

5. **Update documentation:**
   - Update README.md if needed
   - Add/update examples
   - Update API documentation

### Pull Request Process

1. **Update your branch:**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Push to your fork:**
   ```bash
   git push origin feature/your-feature-name
   ```

3. **Create Pull Request:**
   - Go to GitHub and create a PR
   - Fill out the PR template
   - Link related issues
   - Add screenshots/examples if applicable

4. **PR Requirements:**
   - All tests pass
   - Coverage remains above 80%
   - Code is formatted
   - Documentation is updated
   - Commits follow conventional format

5. **Review Process:**
   - The maintainer reviews your PR
   - Address feedback and comments
   - Make requested changes
   - Only the maintainer approves and merges PRs

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Tests added/updated
- [ ] All tests pass
- [ ] Coverage above 80%

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No new warnings
```

## Release Process

Releases are automated via GitHub Actions using semantic versioning.

### Version Bumping

Each merge to `main` releases at most one version, with the largest bump that
any commit since the last tag calls for. Scopes count: `fix(openapi): …` is a
fix. Only plain `vMAJOR.MINOR.PATCH` tags count as the last tag; pre-release
and build tags such as `v1.7.0-rc.1` or `v1.7.0+meta` are skipped.

- `feat!:` (any type with `!`) or a `BREAKING CHANGE:` footer → major. From v1
  on, the module path in `go.mod` must already end in the new `/vN`; otherwise
  the release fails instead of tagging a version Go cannot fetch.
- `feat:` → minor (v1.6.10 → v1.7.0)
- `fix:` → patch (v1.6.10 → v1.6.11)
- Any other type → no release.

Releases run one at a time. Merges that land while a release runs are released
together by the next run. The rules live in `.github/scripts/next-version.sh`,
and `next-version_test.sh` beside it covers them.

### Release Workflow

1. Merge PR to `main`
2. GitHub Actions automatically:
   - Runs tests
   - Determines version from commits
   - Creates git tag
   - Creates GitHub release, whose notes link the version's pkg.go.dev page

pkg.go.dev adds a version once it is fetched through proxy.golang.org or requested on pkg.go.dev; the workflow does neither.

### Manual Tags

Do not tag releases by hand. The workflow computes the next version from the latest `vMAJOR.MINOR.PATCH` tag, so a hand-made tag changes every version after it.

## Development Tips

### Debugging

```go
// Enable verbose logging
log.SetFlags(log.LstdFlags | log.Lshortfile)

// Print route registry
routes := apix.Snapshot()
for _, r := range routes {
    log.Printf("%s %s - %s", r.Method, r.Path, r.Summary)
}
```

### Testing Locally

```bash
# Test with local changes: each example's go.mod replaces the module with ../..
cd examples/dorico-dynamics
go run .
```

### Common Issues

**Import cycle:**
- Keep adapters independent
- Use interfaces for dependencies

**Test failures:**
- Always call `apix.ResetRegistry()` in tests
- Use table-driven tests for multiple cases

**Coverage drops:**
- Add tests for new code
- Test error paths
- Test edge cases

## Questions?

- **Issues**: https://github.com/Dorico-Dynamics/infra-apix/issues
- **Website**: [Dorico Dynamics LDA](https://doricod.co.mz)

Thank you for contributing to apix! 🚀

