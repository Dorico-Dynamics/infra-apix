package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/logging"
	"github.com/Infra-Forge/infra-apix/internal/pathtemplate"
	"github.com/getkin/kin-openapi/openapi3"
)

// Builder converts registered routes into an OpenAPI 3.1 document.
type Builder struct {
	Info            openapi3.Info
	Servers         openapi3.Servers
	SecuritySchemes openapi3.SecuritySchemes
	GlobalSecurity  openapi3.SecurityRequirements
	Tags            openapi3.Tags

	doc         *openapi3.T
	schemaCache map[reflect.Type]*openapi3.SchemaRef
	// componentTypes records the Go type that took each component name first.
	componentTypes map[string]reflect.Type
	// collisions holds one error per type that maps to a name another type
	// took. Build fails with all of them rather than overwriting the component.
	collisions []error
}

func NewBuilder() *Builder {
	return &Builder{
		Info: openapi3.Info{
			Title:   "API",
			Version: "1.0.0",
		},
	}
}

// Build transforms the route registry snapshot into an OpenAPI document. It
// fails, naming every pair, when two Go types map to one component name, and,
// naming every scheme, when a security requirement names a scheme that
// components.securitySchemes does not declare once plugins have run.
func (b *Builder) Build(routes []*apix.RouteRef) (*openapi3.T, error) {
	paths := openapi3.NewPaths()
	components := &openapi3.Components{
		Schemas:         map[string]*openapi3.SchemaRef{},
		SecuritySchemes: b.SecuritySchemes,
	}
	doc := &openapi3.T{
		OpenAPI:    "3.1.0",
		Info:       &b.Info,
		Servers:    b.Servers,
		Components: components,
		Paths:      paths,
		Tags:       b.Tags,
	}

	if len(b.GlobalSecurity) > 0 {
		doc.Security = b.GlobalSecurity
	}

	b.doc = doc
	b.schemaCache = make(map[reflect.Type]*openapi3.SchemaRef)
	b.componentTypes = make(map[string]reflect.Type)
	defer func() {
		b.doc = nil
		b.schemaCache = nil
		b.componentTypes = nil
		b.collisions = nil
	}()

	for _, route := range routes {
		reported := len(b.collisions)
		if err := b.addRoute(doc, route); err != nil {
			return nil, err
		}
		for i := reported; i < len(b.collisions); i++ {
			b.collisions[i] = fmt.Errorf("%s %s: %w", route.Method, route.Path, b.collisions[i])
		}
	}
	if len(b.collisions) > 0 {
		return nil, errors.Join(b.collisions...)
	}

	sortPaths(doc.Paths)

	// Execute plugin hooks for spec building
	if err := apix.ExecuteOnSpecBuild(doc); err != nil {
		return nil, err
	}
	if err := checkSecuritySchemes(doc); err != nil {
		return nil, err
	}

	// Log spec build completion
	routeCount := len(routes)
	schemaCount := len(doc.Components.Schemas)
	logging.GetLogger().SpecBuilt(routeCount, schemaCount)

	return doc, nil
}

func (b *Builder) addRoute(doc *openapi3.T, ref *apix.RouteRef) error {
	// Normalize path to OpenAPI format (convert :param and *param to {param})
	normalizedPath := normalizePath(ref.Path)

	pathItem := doc.Paths.Value(normalizedPath)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
		doc.Paths.Set(normalizedPath, pathItem)
	}

	op := openapi3.NewOperation()
	op.OperationID = ref.OperationID
	op.Summary = ref.Summary
	op.Description = ref.Description
	op.Deprecated = ref.Deprecated
	op.Tags = ref.Tags

	if len(ref.Parameters) > 0 {
		params := append([]apix.Parameter(nil), ref.Parameters...)
		sort.SliceStable(params, func(i, j int) bool {
			if params[i].In == params[j].In {
				return params[i].Name < params[j].Name
			}
			return params[i].In < params[j].In
		})
		op.Parameters = make(openapi3.Parameters, 0, len(params))
		for _, p := range params {
			paramSchema := &openapi3.Schema{}
			schemaType(paramSchema, p.SchemaType)
			if paramSchema.Type == nil || len(*paramSchema.Type) == 0 {
				schemaType(paramSchema, "string")
			}
			param := &openapi3.Parameter{
				Name:        p.Name,
				In:          p.In,
				Description: p.Description,
				Required:    p.Required,
				Schema:      &openapi3.SchemaRef{Value: paramSchema},
			}
			if p.SchemaType == "" {
				schemaType(param.Schema.Value, "string")
			}
			example, err := exampleValue(p.Example)
			if err != nil {
				return fmt.Errorf("%s %s: parameter %q example: %w", ref.Method, ref.Path, p.Name, err)
			}
			param.Example = example
			op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Value: param})
		}
	}

	if len(ref.Security) > 0 {
		security := openapi3.NewSecurityRequirements()
		for _, sec := range ref.Security {
			// Authenticate writes no scopes as [], which the OpenAPI schema
			// requires; a nil slice would be encoded as null.
			security.With(openapi3.NewSecurityRequirement().Authenticate(sec.Name, sec.Scopes...))
		}
		op.Security = security
	}

	// GET and DELETE content has no defined semantics (RFC 9110 sections 9.3.1
	// and 9.3.5), so a request type on them, which carries query or path
	// values, is not documented as a request body.
	if method := strings.ToUpper(string(ref.Method)); method != http.MethodGet && method != http.MethodDelete {
		if bodyRef, err := b.buildRequestBody(ref); err != nil {
			return err
		} else if bodyRef != nil {
			op.RequestBody = bodyRef
		}
	}

	statusCodes := make([]int, 0, len(ref.Responses))
	for code := range ref.Responses {
		statusCodes = append(statusCodes, code)
	}
	sort.Ints(statusCodes)

	for _, status := range statusCodes {
		respRef := ref.Responses[status]
		oaResp, err := b.buildResponse(ref, status, respRef)
		if err != nil {
			return err
		}
		for _, hdr := range ref.SuccessHeaders[status] {
			oaResp.Headers[hdr.Name] = headerRef(hdr)
		}
		op.AddResponse(status, oaResp)
	}
	if err := checkSuccessHeaders(ref); err != nil {
		return err
	}

	addDXDefaults(ref, op)

	switch strings.ToUpper(string(ref.Method)) {
	case http.MethodGet:
		pathItem.Get = op
	case http.MethodPost:
		pathItem.Post = op
	case http.MethodPut:
		pathItem.Put = op
	case http.MethodPatch:
		pathItem.Patch = op
	case http.MethodDelete:
		pathItem.Delete = op
	default:
		return fmt.Errorf("unsupported method %s", ref.Method)
	}

	// Note: pathItem is already set at normalizedPath on line 80, no need to set again
	return nil
}

func (b *Builder) buildRequestBody(ref *apix.RouteRef) (*openapi3.RequestBodyRef, error) {
	schemaRef, err := b.schemaFromTypes(ref.ExplicitRequestModel, ref.RequestType)
	if err != nil {
		return nil, err
	}
	if schemaRef == nil {
		return nil, nil
	}

	contentType := ref.RequestContentType
	if contentType == "" {
		contentType = "application/json"
	}

	example, err := exampleValue(ref.RequestExample)
	if err != nil {
		return nil, fmt.Errorf("%s %s: request example: %w", ref.Method, ref.Path, err)
	}
	content := openapi3.Content{}
	content[contentType] = &openapi3.MediaType{Schema: schemaRef, Example: example}

	required := ref.BodyRequired
	if !ref.BodyRequired {
		required = ref.RequestType != nil || ref.ExplicitRequestModel != nil
	}

	return &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Required: required,
			Content:  content,
		},
	}, nil
}

func (b *Builder) buildResponse(route *apix.RouteRef, status int, ref *apix.ResponseRef) (*openapi3.Response, error) {
	explicit, inferred := ref.ExplicitModelType, ref.ModelType
	if status == http.StatusNoContent {
		// A 204 has no content (RFC 9110, section 15.3.5): every adapter writes
		// none, whatever the handler returns.
		explicit, inferred = nil, nil
	}
	schemaRef, err := b.schemaFromTypes(explicit, inferred)
	if err != nil {
		return nil, err
	}

	description := ref.Description
	if description == "" {
		description = defaultResponseDescription(status)
	}

	resp := &openapi3.Response{
		Description: &description,
		Content:     openapi3.Content{},
		Headers:     openapi3.Headers{},
	}

	if schemaRef != nil {
		contentType := ref.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		example, err := exampleValue(ref.Example)
		if err != nil {
			return nil, fmt.Errorf("%s %s: response %d example: %w", route.Method, route.Path, status, err)
		}
		resp.Content[contentType] = &openapi3.MediaType{Schema: schemaRef, Example: example}
	}

	for _, hdr := range ref.Headers {
		resp.Headers[hdr.Name] = headerRef(hdr)
	}

	return resp, nil
}

// exampleValue returns an example as the JSON value a handler would send.
// kin-openapi validates an example by walking JSON values, and the YAML encoder
// writes a struct with its Go field names, so anything but a JSON scalar (a
// struct, a typed slice or map, a uint, a time.Time) is stored as its JSON
// form, with integers as int64.
func exampleValue(example any) (any, error) {
	switch v := example.(type) {
	case nil, bool, string, int, int32, int64:
		return example, nil
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v, nil
		}
	}
	data, err := json.Marshal(example)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return plainNumbers(value), nil
}

// plainNumbers replaces each json.Number in a decoded JSON value with an int64
// when it is an integer in range, or a float64 otherwise.
func plainNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
	case []any:
		for i, item := range v {
			v[i] = plainNumbers(item)
		}
	case map[string]any:
		for key, item := range v {
			v[key] = plainNumbers(item)
		}
	}
	return value
}

// schemaFromTypes returns the schema of explicit, or of inferred when explicit
// is nil. A nil type and apix.NoBody have none: the adapters read no request
// body into a NoBody and write no response body for one.
func (b *Builder) schemaFromTypes(explicit reflect.Type, inferred reflect.Type) (*openapi3.SchemaRef, error) {
	t := explicit
	if t == nil {
		t = inferred
	}
	if t == nil || isNoBody(t) {
		return nil, nil
	}
	return b.schemaRefFromType(t)
}

func (b *Builder) schemaRefFromType(t reflect.Type) (*openapi3.SchemaRef, error) {
	nullable := false
	for t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}

	baseRef, err := b.schemaRefNonNull(t)
	if err != nil || baseRef == nil {
		return baseRef, err
	}

	if !nullable {
		return baseRef, nil
	}
	return wrapNullable(baseRef), nil
}

func (b *Builder) schemaRefNonNull(t reflect.Type) (*openapi3.SchemaRef, error) {
	if cached, ok := b.schemaCache[t]; ok {
		if name := componentName(t); name != "" {
			return componentRef(name, cached.Value), nil
		}
		// Each use gets its own copy, so field metadata applied to one use
		// does not reach the others.
		return &openapi3.SchemaRef{Value: copySchema(cached.Value)}, nil
	}

	switch t.Kind() {
	case reflect.Bool:
		return schemaRef(openapi3.NewBoolSchema()), nil
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint8, reflect.Uint16:
		s := openapi3.NewIntegerSchema()
		s.Format = "int32"
		return schemaRef(s), nil
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		// int and uint are 64-bit on amd64 and arm64, and uint32 exceeds int32,
		// so they are int64 on every platform, which keeps the document the
		// same wherever it is built.
		s := openapi3.NewIntegerSchema()
		s.Format = "int64"
		return schemaRef(s), nil
	case reflect.Float32:
		s := openapi3.NewFloat64Schema()
		s.Format = "float"
		return schemaRef(s), nil
	case reflect.Float64:
		s := openapi3.NewFloat64Schema()
		return schemaRef(s), nil
	case reflect.String:
		return schemaRef(openapi3.NewStringSchema()), nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			s := openapi3.NewStringSchema()
			s.Format = "byte"
			return schemaRef(s), nil
		}
		fallthrough
	case reflect.Array:
		if isUUID(t) {
			s := openapi3.NewStringSchema()
			s.Format = "uuid"
			return schemaRef(s), nil
		}
		itemRef, err := b.schemaRefFromType(t.Elem())
		if err != nil {
			return nil, err
		}
		s := openapi3.NewArraySchema()
		s.Items = itemRef
		return schemaRef(s), nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type %s", t.Key())
		}
		valueRef, err := b.schemaRefFromType(t.Elem())
		if err != nil {
			return nil, err
		}
		s := openapi3.NewObjectSchema()
		s.AdditionalProperties = openapi3.AdditionalProperties{Schema: valueRef}
		return schemaRef(s), nil
	case reflect.Interface:
		s := openapi3.NewObjectSchema()
		return schemaRef(s), nil
	case reflect.Struct:
		if t.AssignableTo(timeType) {
			s := openapi3.NewStringSchema()
			s.Format = "date-time"
			return schemaRef(s), nil
		}
		if isUUID(t) {
			s := openapi3.NewStringSchema()
			s.Format = "uuid"
			return schemaRef(s), nil
		}
		if isDecimal(t) {
			s := openapi3.NewStringSchema()
			s.Format = "decimal"
			s.Description = "Decimal number represented as string for precision"
			s.Example = "123.45"
			return schemaRef(s), nil
		}
		return b.buildStructSchema(t)
	default:
		return nil, fmt.Errorf("unsupported type %s", t)
	}
}

func (b *Builder) buildStructSchema(t reflect.Type) (*openapi3.SchemaRef, error) {
	name := componentName(t)

	if name != "" {
		// Names keep only the last import-path element, so a/models.Money and
		// b/models.Money share "models_Money". The schema cache sends each type
		// here once, so a taken name means a second type. Building goes on so
		// that one failed Build names every such type.
		if owner, taken := b.componentTypes[name]; taken {
			b.collisions = append(b.collisions, fmt.Errorf("component name %q is used by both %s and %s; rename one of the types",
				name, qualifiedTypeName(owner), qualifiedTypeName(t)))
		} else {
			b.componentTypes[name] = t
		}

		schema := openapi3.NewObjectSchema()
		schema.Properties = make(map[string]*openapi3.SchemaRef)
		schema.Required = []string{}

		// Register the component before populating it, so a field that leads
		// back to this type resolves to a reference instead of recursing.
		component := &openapi3.SchemaRef{Value: schema}
		b.doc.Components.Schemas[name] = component
		b.schemaCache[t] = component

		if err := b.populateStructSchema(schema, t); err != nil {
			return nil, err
		}

		// Execute plugin hooks for schema generation
		if err := apix.ExecuteOnSchemaGenerate(name, schema); err != nil {
			return nil, err
		}

		// Log schema generation
		logging.GetLogger().SchemaGenerated(name)

		return componentRef(name, schema), nil
	}

	// anonymous/inline struct
	schema := openapi3.NewObjectSchema()
	schema.Properties = make(map[string]*openapi3.SchemaRef)
	schema.Required = []string{}
	if err := b.populateStructSchema(schema, t); err != nil {
		return nil, err
	}
	b.schemaCache[t] = &openapi3.SchemaRef{Value: schema}
	return &openapi3.SchemaRef{Value: copySchema(schema)}, nil
}

func (b *Builder) populateStructSchema(schema *openapi3.Schema, t reflect.Type) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if field.PkgPath != "" && !field.Anonymous {
			continue
		}

		if field.Anonymous {
			ref, err := b.schemaRefFromType(field.Type)
			if err != nil {
				return err
			}
			schema.AllOf = append(schema.AllOf, ref)
			continue
		}

		jsonName, skip := jsonName(field)
		if skip {
			continue
		}
		if schema.Properties == nil {
			schema.Properties = make(map[string]*openapi3.SchemaRef)
		}

		// Check if field is marked as a file upload
		isFile := field.Tag.Get("format") == "binary" || field.Tag.Get("format") == "file"

		var childRef *openapi3.SchemaRef
		var err error

		if isFile {
			// For file uploads, use string schema with binary format
			fileSchema := openapi3.NewStringSchema()
			fileSchema.Format = "binary"
			childRef = &openapi3.SchemaRef{Value: fileSchema}
		} else {
			childRef, err = b.schemaRefFromType(field.Type)
			if err != nil {
				return err
			}
		}

		var example any
		if fieldExample := field.Tag.Get("example"); fieldExample != "" && !isFile {
			example = parseExampleValue(fieldExample, field.Type)
		}
		schema.Properties[jsonName] = withFieldMetadata(childRef, field.Tag.Get("description"), example)

		if isFieldRequired(field) {
			schema.Required = append(schema.Required, jsonName)
		}
	}
	return nil
}

func jsonName(field reflect.StructField) (name string, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	if tag == "" {
		return field.Name, false
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "-" {
		return "", true
	}
	if parts[0] != "" {
		return parts[0], false
	}
	return field.Name, false
}

func isFieldRequired(field reflect.StructField) bool {
	tag := field.Tag.Get("json")
	if strings.Contains(tag, "omitempty") {
		return hasRequiredTag(field)
	}

	if hasRequiredTag(field) {
		return true
	}

	switch field.Type.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return false
	default:
		return true
	}
}

func hasRequiredTag(field reflect.StructField) bool {
	validate := field.Tag.Get("validate")
	if strings.Contains(validate, "required") {
		return true
	}
	binding := field.Tag.Get("binding")
	return strings.Contains(binding, "required")
}

// parseExampleValue converts a string example value to the appropriate type
// based on the field's reflect.Type. For complex types, returns the string as-is.
func parseExampleValue(exampleStr string, fieldType reflect.Type) any {
	// Dereference pointers to get the underlying type
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	switch fieldType.Kind() {
	case reflect.String:
		return exampleStr
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// For integer types, try to parse as int
		var val int64
		if _, err := fmt.Sscanf(exampleStr, "%d", &val); err == nil {
			return val
		}
		return exampleStr
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// For unsigned integer types, try to parse as uint
		var val uint64
		if _, err := fmt.Sscanf(exampleStr, "%d", &val); err == nil {
			return val
		}
		return exampleStr
	case reflect.Float32, reflect.Float64:
		// For float types, try to parse as float
		var val float64
		if _, err := fmt.Sscanf(exampleStr, "%f", &val); err == nil {
			return val
		}
		return exampleStr
	case reflect.Bool:
		// For bool types, try to parse as bool (case-insensitive)
		switch strings.ToLower(exampleStr) {
		case "true":
			return true
		case "false":
			return false
		}
		return exampleStr
	default:
		// For complex types (struct, slice, map, etc.), return string as-is
		// OpenAPI will treat it as a string example
		return exampleStr
	}
}

func headerRef(h apix.HeaderRef) *openapi3.HeaderRef {
	schema := &openapi3.Schema{}
	schemaType(schema, h.SchemaType)
	if schema.Type == nil || len(*schema.Type) == 0 {
		schemaType(schema, "string")
	}
	header := &openapi3.Header{}
	header.Description = h.Description
	header.Required = h.Required
	header.Schema = &openapi3.SchemaRef{Value: schema}
	return &openapi3.HeaderRef{Value: header}
}

// checkSuccessHeaders fails for headers declared with WithSuccessHeaders on a
// status the route has no response for, which would otherwise be dropped.
func checkSuccessHeaders(ref *apix.RouteRef) error {
	statuses := make([]int, 0, len(ref.SuccessHeaders))
	for status, headers := range ref.SuccessHeaders {
		if _, ok := ref.Responses[status]; !ok && len(headers) > 0 {
			statuses = append(statuses, status)
		}
	}
	if len(statuses) == 0 {
		return nil
	}
	sort.Ints(statuses)
	return fmt.Errorf("%s %s: WithSuccessHeaders(%d, ...) names a status the route has no response for", ref.Method, ref.Path, statuses[0])
}

func addDXDefaults(ref *apix.RouteRef, op *openapi3.Operation) {
	if len(ref.Security) > 0 {
		ensureResponse(op, http.StatusUnauthorized, "Unauthorized")
		ensureResponse(op, http.StatusForbidden, "Forbidden")
	}
}

func ensureResponse(op *openapi3.Operation, status int, description string) {
	if existing := op.Responses.Status(status); existing != nil {
		return
	}
	desc := description
	op.Responses.Set(fmt.Sprintf("%d", status), &openapi3.ResponseRef{Value: &openapi3.Response{Description: &desc}})
}

func sortPaths(paths *openapi3.Paths) {
	if paths == nil {
		return
	}
	keys := paths.InMatchingOrder()
	sorted := openapi3.NewPaths()
	for _, k := range keys {
		sorted.Set(k, paths.Value(k))
	}
	*paths = *sorted
}

// checkSecuritySchemes fails for every scheme that a security requirement of
// doc names but components.securitySchemes does not declare. OpenAPI requires
// each name to match a declared scheme; without one, clients and Swagger UI
// cannot tell how to authenticate. Each error names the first user, the
// top-level security or an operation in path order, and counts the others.
func checkSecuritySchemes(doc *openapi3.T) error {
	var declared openapi3.SecuritySchemes
	if doc.Components != nil {
		declared = doc.Components.SecuritySchemes
	}
	first := map[string]string{}
	uses := map[string]int{}
	note := func(where string, reqs openapi3.SecurityRequirements) {
		seen := map[string]bool{}
		for _, req := range reqs {
			for name := range req {
				if _, ok := declared[name]; ok || seen[name] {
					continue
				}
				seen[name] = true
				if uses[name] == 0 {
					first[name] = where
				}
				uses[name]++
			}
		}
	}
	note("the top-level security", doc.Security)
	for _, path := range doc.Paths.InMatchingOrder() {
		ops := doc.Paths.Value(path).Operations()
		for _, method := range slices.Sorted(maps.Keys(ops)) {
			if sec := ops[method].Security; sec != nil {
				note(method+" "+path, *sec)
			}
		}
	}

	errs := make([]error, 0, len(uses))
	for _, name := range slices.Sorted(maps.Keys(uses)) {
		more := ""
		if n := uses[name] - 1; n > 0 {
			more = fmt.Sprintf(" (and %d more)", n)
		}
		errs = append(errs, fmt.Errorf("security scheme %q is not declared in components.securitySchemes, but %s uses it%s", name, first[name], more))
	}
	return errors.Join(errs...)
}

func schemaRef(schema *openapi3.Schema) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: schema}
}

const componentRefPrefix = "#/components/schemas/"

// componentRef refers to a component schema. Value carries the resolved schema
// so the in-memory document validates and can be walked without a loader;
// serialization emits only the $ref.
func componentRef(name string, schema *openapi3.Schema) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Ref: componentRefPrefix + name, Value: schema}
}

// copySchema returns a shallow copy, so per-use edits to top-level fields
// (description, example, nullable) leave the original untouched.
func copySchema(s *openapi3.Schema) *openapi3.Schema {
	if s == nil {
		return nil
	}
	c := *s
	return &c
}

func wrapNullable(ref *openapi3.SchemaRef) *openapi3.SchemaRef {
	if ref == nil {
		return nil
	}
	if ref.Ref != "" {
		return &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Nullable: true,
				AllOf:    []*openapi3.SchemaRef{ref},
			},
		}
	}
	if ref.Value == nil {
		return ref
	}
	clone := copySchema(ref.Value)
	clone.Nullable = true
	return &openapi3.SchemaRef{Value: clone}
}

// withFieldMetadata applies a struct field's description and example to the
// field's schema. A reference to a component is wrapped in allOf, so one
// field's metadata never edits the component that other fields share.
func withFieldMetadata(ref *openapi3.SchemaRef, description string, example any) *openapi3.SchemaRef {
	if ref == nil || (description == "" && example == nil) {
		return ref
	}
	if ref.Ref != "" {
		ref = &openapi3.SchemaRef{Value: &openapi3.Schema{AllOf: []*openapi3.SchemaRef{ref}}}
	}
	if ref.Value == nil {
		return ref
	}
	if description != "" {
		ref.Value.Description = description
	}
	if example != nil {
		ref.Value.Example = example
	}
	return ref
}

func componentName(t reflect.Type) string {
	if t.Name() == "" {
		return ""
	}
	pkg := t.PkgPath()
	if pkg == "" {
		return sanitizeComponentName(t.Name())
	}
	parts := strings.Split(pkg, "/")
	pkgPart := parts[len(parts)-1]
	return sanitizeComponentName(pkgPart + "_" + t.Name())
}

// qualifiedTypeName names a defined type by its full import path, which tells
// apart types that share a component name.
func qualifiedTypeName(t reflect.Type) string {
	return t.PkgPath() + "." + t.Name()
}

func sanitizeComponentName(name string) string {
	replacer := strings.NewReplacer(
		"-", "_",
		".", "_",
		" ", "_",
		"[", "_",
		"]", "_",
		"<", "_",
		">", "_",
		",", "_",
		"*", "_",
		"/", "_",
		"{", "_",
		"}", "_",
	)
	return replacer.Replace(name)
}

var timeType = reflect.TypeOf(time.Time{})

var noBodyType = reflect.TypeOf(apix.NoBody{})

// isNoBody reports whether t is apix.NoBody or a pointer to it.
func isNoBody(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t == noBodyType
}

func isUUID(t reflect.Type) bool {
	return t.PkgPath() == "github.com/google/uuid" && t.Name() == "UUID"
}

func isDecimal(t reflect.Type) bool {
	return t.PkgPath() == "github.com/shopspring/decimal" && t.Name() == "Decimal"
}

// normalizePath converts framework-specific path parameter syntax to OpenAPI format
// Converts :param (Echo, Gin, Fiber) and {param:pattern} (Chi, Mux) to {param},
// leaves {param} (Chi, Mux) unchanged
func normalizePath(path string) string {
	path = pathtemplate.StripPatterns(path)
	var normalized strings.Builder
	i := 0
	for i < len(path) {
		ch := path[i]

		if ch == ':' {
			// Echo/Gin/Fiber style parameter - convert :param to {param}
			normalized.WriteRune('{')
			i++
			// Copy parameter name
			for i < len(path) {
				ch = path[i]
				if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
					normalized.WriteRune(rune(ch))
					i++
				} else {
					break
				}
			}
			normalized.WriteRune('}')
		} else {
			// Regular byte or already in {param} format. A byte, not a rune:
			// rune(ch) would re-encode each byte of a multi-byte character.
			normalized.WriteByte(ch)
			i++
		}
	}

	return normalized.String()
}

func schemaType(schema *openapi3.Schema, t string) {
	if schema == nil {
		return
	}
	if schema.Type == nil {
		schema.Type = &openapi3.Types{}
	}
	if t != "" {
		*schema.Type = openapi3.Types{t}
	}
}

func defaultResponseDescription(status int) string {
	switch status {
	case http.StatusOK:
		return "OK"
	case http.StatusCreated:
		return "Created"
	case http.StatusAccepted:
		return "Accepted"
	case http.StatusNoContent:
		return "No Content"
	case http.StatusBadRequest:
		return "Bad Request"
	case http.StatusUnauthorized:
		return "Unauthorized"
	case http.StatusForbidden:
		return "Forbidden"
	case http.StatusNotFound:
		return "Not Found"
	case http.StatusInternalServerError:
		return "Internal Server Error"
	default:
		return ""
	}
}
