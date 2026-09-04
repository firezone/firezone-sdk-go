//go:build spec

// Package firezone's OpenAPI conformance check.
//
// Every field this SDK decodes is an assumption about what the server
// sends. Unit tests can't test those assumptions - they assert against
// fixtures written by the same person who wrote the struct tag, so a
// misspelled field passes every test and silently decodes as a zero
// value forever. This test checks the struct tags against Firezone's
// published OpenAPI spec instead, which is the only thing that actually
// knows.
//
// Run it with `mise run spec-check`. It checks against the spec vendored
// at testdata/openapi.json; point it at another copy with
//
//	FIREZONE_OPENAPI=/path/to/openapi.json mise run spec-check
//
// to check against an unreleased spec in a local monorepo checkout.
package firezone

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// schemaFor maps an SDK type to its OpenAPI schema when the names
// differ. Types not listed here are matched by identical name.
//
// A value may name a property inside a schema with a dotted path, which
// is how the response envelopes are reached: the payload this SDK
// decodes is the "data" property of a *Response schema, not a schema of
// its own.
var schemaFor = map[string]string{
	"ClientDevice": "Client",
	// Not the bare Gateway schema: the provision response is a Gateway
	// plus the one-time token, which the bare schema does not carry.
	"ProvisionedGateway":  "GatewayProvisionResponse.data",
	"Condition":           "PolicyCondition",
	"Filter":              "ResourceFilter",
	"GroupMember":         "Membership",
	"RotatedGatewayToken": "GatewayTokenResponse.data",

	// Unexported types decode server responses too, and get no less
	// scrutiny for being lowercase - a typo in one of these is the same
	// silent zero value as in any exported read model.
	"membershipActorIDs":  "MembershipResponse.data",
	"poolMemberDeviceIDs": "PoolMemberResponse.data",
	"pageMetadataBody":    "PaginationMetadata",
	"problemDetailsBody":  "ValidationProblemDetails",
}

// skipTypes are SDK types with no OpenAPI counterpart, each with the
// reason. A type that is neither here nor resolvable to a schema fails
// the test - that is deliberate, so a new resource can't be added
// without someone deciding which bucket it belongs in.
var skipTypes = map[string]string{
	"AuthProvider": "embedded base type; its fields are checked via each concrete provider",
	// The envelopes are the wrapper the spec describes per response
	// schema rather than as a schema of their own; every *Response
	// schema's "data"/"metadata" pair is what they mirror.
	"dataEnvelope": "generic {\"data\": ...} wrapper, not a schema in its own right",
	"listEnvelope": "generic {\"data\": ..., \"metadata\": ...} wrapper, not a schema in its own right",
}

// TestSpecConformance fails when the SDK declares a JSON field the
// server never sends. Fields the server sends that the SDK omits are
// reported but not fatal: not exposing a field is a deliberate scope
// choice, while decoding one that doesn't exist is always a bug.
func TestSpecConformance(t *testing.T) {
	specPath := findSpec(t)
	schemas := loadSchemas(t, specPath)
	structs := parseStructs(t)

	var missingSchema []string

	for _, name := range sortedKeys(structs) {
		if reason, ok := skipTypes[name]; ok {
			t.Logf("skip %s: %s", name, reason)
			continue
		}
		if !isReadModel(name) {
			continue
		}
		// Types with no JSON tags decode nothing - services, the client
		// itself, errors built by hand. There is no assumption to check.
		if len(structs[name]) == 0 {
			continue
		}

		schemaName := name
		if alias, ok := schemaFor[name]; ok {
			schemaName = alias
		}
		props, ok := schemas[schemaName]
		if !ok {
			missingSchema = append(missingSchema, name)
			continue
		}

		fields := structs[name]
		var ghosts []string
		for _, f := range fields {
			if _, ok := props[f]; !ok {
				ghosts = append(ghosts, f)
			}
		}
		if len(ghosts) > 0 {
			sort.Strings(ghosts)
			t.Errorf("%s (schema %q): declares %d field(s) absent from the spec, which will always decode as zero: %s",
				name, schemaName, len(ghosts), strings.Join(ghosts, ", "))
		}

		// Informational: spec fields this type doesn't expose.
		var uncovered []string
		for p := range props {
			if !contains(fields, p) {
				uncovered = append(uncovered, p)
			}
		}
		if len(uncovered) > 0 {
			sort.Strings(uncovered)
			t.Logf("%s: %d spec field(s) not exposed by the SDK: %s",
				name, len(uncovered), strings.Join(uncovered, ", "))
		}
	}

	if len(missingSchema) > 0 {
		sort.Strings(missingSchema)
		t.Errorf("no OpenAPI schema found for: %s\n"+
			"Add each to schemaFor (if the schema is named differently) or to skipTypes (with a reason).",
			strings.Join(missingSchema, ", "))
	}
}

// vendoredSpec is the checked-in copy of the OpenAPI document. Having
// it in the repo is what lets these checks run in CI: before it was
// vendored they searched for a sibling monorepo checkout and skipped
// when they found none, so the one test that can catch a wrong struct
// tag never ran anywhere except on the machine of someone who happened
// to have both repos cloned.
const vendoredSpec = "testdata/openapi.json"

// findSpec locates the OpenAPI document: the vendored copy by default,
// or an explicit override for checking against an unreleased spec in a
// local monorepo checkout.
//
// The default is the vendored copy rather than a discovered checkout so
// that a local run and a CI run check against the same bytes. Refreshing
// the vendored spec is deliberate - see testdata/README.md.
func findSpec(t *testing.T) string {
	path := vendoredSpec
	if p := os.Getenv("FIREZONE_OPENAPI"); p != "" {
		path = p
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("OpenAPI spec %s: %v\n"+
			"The spec is vendored at %s; restore it, or point FIREZONE_OPENAPI at a copy.",
			path, err, vendoredSpec)
	}
	return path
}

// loadSchemas reads components.schemas into schema name -> property set.
//
// Names may carry a dotted suffix ("MembershipResponse.data") selecting
// a property's own schema, which is how a payload the spec only
// describes inside a response envelope becomes checkable.
func loadSchemas(t *testing.T, path string) map[string]map[string]struct{} {
	raw := loadRawSchemas(t, path)

	out := make(map[string]map[string]struct{}, len(raw.all))
	for name, schema := range raw.all {
		out[name] = propertyNames(raw, schema)
		for prop, sub := range schema.Properties {
			resolved := raw.resolve(sub, 0)
			if len(resolved.Properties) > 0 || len(resolved.AllOf) > 0 {
				out[name+"."+prop] = propertyNames(raw, resolved)
			}
		}
	}
	t.Logf("loaded %d schemas from %s", len(raw.all), path)
	return out
}

func propertyNames(raw rawSpec, schema *rawSchema) map[string]struct{} {
	props := make(map[string]struct{}, len(schema.Properties))
	for p := range schema.Properties {
		props[p] = struct{}{}
	}
	for _, branch := range schema.AllOf {
		for p := range propertyNames(raw, raw.resolve(branch, 0)) {
			props[p] = struct{}{}
		}
	}
	return props
}

// forEachStruct walks every struct type declared in the package's
// non-test files, so the two struct walks share one parse.
func forEachStruct(t *testing.T, fn func(name string, st *ast.StructType)) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				if st, ok := ts.Type.(*ast.StructType); ok {
					fn(ts.Name.Name, st)
				}
				return true
			})
		}
	}
}

// parseStructs returns every struct in the package as its list of JSON
// field names, with embedded structs flattened in - without that, a type
// that embeds a shared base looks like it is missing all of the base's
// fields.
func parseStructs(t *testing.T) map[string][]string {
	own := map[string][]string{}    // type -> its own json tags
	embeds := map[string][]string{} // type -> embedded type names

	forEachStruct(t, func(name string, st *ast.StructType) {
		if _, seen := own[name]; !seen {
			own[name] = nil
		}
		for _, f := range st.Fields.List {
			// An embedded field has no names.
			if len(f.Names) == 0 {
				if id, ok := f.Type.(*ast.Ident); ok {
					embeds[name] = append(embeds[name], id.Name)
				}
				continue
			}
			if f.Tag == nil {
				continue
			}
			tag := reflect.StructTag(strings.Trim(f.Tag.Value, "`"))
			jt := tag.Get("json")
			if jt == "" || jt == "-" {
				continue
			}
			if fname := strings.Split(jt, ",")[0]; fname != "" {
				own[name] = append(own[name], fname)
			}
		}
	})

	// Flatten embedded types (depth-limited; the SDK nests one level).
	resolved := make(map[string][]string, len(own))
	var expand func(string, int) []string
	expand = func(name string, depth int) []string {
		if depth > 8 {
			t.Fatalf("embedding cycle at %s", name)
		}
		fields := append([]string(nil), own[name]...)
		for _, e := range embeds[name] {
			if _, ok := own[e]; ok {
				fields = append(fields, expand(e, depth+1)...)
			}
		}
		return fields
	}
	for name := range own {
		resolved[name] = expand(name, 0)
	}
	t.Logf("parsed %d struct types from package source", len(resolved))
	return resolved
}

// isReadModel reports whether a type decodes a server response.
//
// Request bodies are excluded by their presence in requestSchemaFor
// rather than by a naming rule, so an unexported body type can't slip
// past both walks by not looking like a request. Option structs never
// reach the wire as JSON at all.
func isReadModel(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := requestSchemaFor[name]; ok {
		return false
	}
	return !strings.HasSuffix(name, "Options")
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// requestSpec locates an SDK request body in the OpenAPI document and
// records how the endpoint treats it.
type requestSpec struct {
	// schema is the OpenAPI request schema name.
	schema string
	// merge marks an endpoint that merges the body into the existing
	// record, so an omitted field and a field sent as null mean
	// different things. Only these need [firezone.Null] typing; a create
	// body has nothing to clear, and a full-replace body sends the whole
	// collection every time.
	merge bool
}

// requestSchemaFor maps every SDK request body to its OpenAPI schema.
// A request type absent from this table fails TestSpecRequestConformance,
// so no request body can be added without someone deciding which schema
// it answers to.
var requestSchemaFor = map[string]requestSpec{
	"CreateActorRequest":      {schema: "ActorCreateRequest"},
	"UpdateActorRequest":      {schema: "ActorUpdateRequest", merge: true},
	"UpdateClientRequest":     {schema: "ClientPutRequest", merge: true},
	"ProvisionGatewayRequest": {schema: "GatewayCreateRequest"},
	"UpdateGatewayRequest":    {schema: "GatewayUpdateRequest", merge: true},
	"CreateGroupRequest":      {schema: "GroupCreateRequest"},
	"UpdateGroupRequest":      {schema: "GroupUpdateRequest", merge: true},
	"membershipEntry":         {schema: "MembershipPutRequest"},
	"membershipPatchBody":     {schema: "MembershipPatchRequest"},
	"CreatePolicyRequest":     {schema: "PolicyCreateRequest"},
	"UpdatePolicyRequest":     {schema: "PolicyUpdateRequest", merge: true},
	"poolMemberEntry":         {schema: "PoolMemberPutRequest"},
	"poolMemberPatchBody":     {schema: "PoolMemberPatchRequest"},
	"CreateResourceRequest":   {schema: "ResourceCreateRequest"},
	"UpdateResourceRequest":   {schema: "ResourceUpdateRequest", merge: true},
	"CreateSiteRequest":       {schema: "SiteCreateRequest"},
	"UpdateSiteRequest":       {schema: "SiteUpdateRequest", merge: true},
}

// TestSpecRequestConformance checks every request body this SDK sends
// against the schema the server validates it with.
//
// The read-model check (TestSpecConformance) covers the decode side. The
// encode side is the half that fails silently: the server casts a
// request body with Ecto's changeset cast, which drops keys it doesn't
// recognise without comment - so a misspelled tag on a create body
// returns 201 Created with the field quietly missing, and every unit
// test still passes because the fixtures were written by the same hand
// as the tag.
//
// Four rules, in increasing specificity:
//
//   - Every field the SDK sends must exist in the schema (otherwise the
//     server drops it).
//   - Every field the spec marks required must be present and sent
//     unconditionally (otherwise the call 422s).
//   - On a merge-patch body, a nullable field must be *Null[T] so it can
//     be cleared as well as set.
//   - On a merge-patch body, an array field must be a pointer to a slice
//     so an empty list can be sent at all.
func TestSpecRequestConformance(t *testing.T) {
	specPath := findSpec(t)
	schemas := loadRequestSchemas(t, specPath)
	fields := parseStructFieldTypes(t)

	for _, name := range sortedKeys(fields) {
		req, ok := requestSchemaFor[name]
		if !ok {
			continue
		}
		schema, ok := schemas[req.schema]
		if !ok {
			t.Errorf("%s: OpenAPI schema %q not found", name, req.schema)
			continue
		}

		declared := map[string]structField{}
		for _, f := range fields[name] {
			declared[f.jsonName] = f

			prop, ok := schema.props[f.jsonName]
			if !ok {
				t.Errorf("%s.%s (json %q): absent from schema %q; the server drops unrecognised keys, "+
					"so this field would be silently discarded",
					name, f.goName, f.jsonName, req.schema)
				continue
			}
			if !req.merge {
				continue
			}
			switch {
			case prop.Nullable && !strings.HasPrefix(f.goType, "*Null["):
				t.Errorf("%s.%s (json %q): spec marks it nullable, so it must be *Null[T] to be clearable; got %s",
					name, f.goName, f.jsonName, f.goType)
			case prop.Type == "array" && !strings.HasPrefix(f.goType, "*[]"):
				t.Errorf("%s.%s (json %q): spec type is array, so it must be *[]T for an empty list to be sendable; got %s",
					name, f.goName, f.jsonName, f.goType)
			}
		}

		for _, want := range schema.required {
			f, ok := declared[want]
			if !ok {
				t.Errorf("%s: schema %q requires %q, which this type cannot send",
					name, req.schema, want)
				continue
			}
			if f.omitEmpty {
				t.Errorf("%s.%s (json %q): schema %q requires it, so it must not be omitempty - "+
					"a zero value would drop it and the call would 422",
					name, f.goName, f.jsonName, req.schema)
			}
		}
	}
}

// TestSpecRequestWrapperKeys checks the key each request body is nested
// under against the wrapper property the spec declares.
//
// The key is a bare string at the wrapBody call site rather than part of
// the request type, so it is invisible to the checks above. A wrong one
// is a 400 on every call to that endpoint - loud, but only once someone
// runs it against a real server, which no test here does.
func TestSpecRequestWrapperKeys(t *testing.T) {
	specPath := findSpec(t)
	schemas := loadRequestSchemas(t, specPath)

	calls := parseWrapBodyCalls(t)
	if len(calls) == 0 {
		t.Fatal("found no wrapBody calls to check; the AST walk is broken, not the code")
	}

	for _, c := range calls {
		req, ok := requestSchemaFor[c.requestType]
		if !ok {
			t.Errorf("%s: wrapBody(%q, ...) wraps %s, which is absent from requestSchemaFor",
				c.pos, c.key, c.requestType)
			continue
		}
		schema := schemas[req.schema]
		if c.key != schema.wrapper {
			t.Errorf("%s: wrapBody(%q, ...) wraps %s, but schema %q nests the body under %q",
				c.pos, c.key, c.requestType, req.schema, schema.wrapper)
		}
	}
	t.Logf("checked %d wrapBody call sites", len(calls))
}

// rawSchema is the subset of an OpenAPI schema these checks read.
type rawSchema struct {
	Ref      string   `json:"$ref"`
	Type     string   `json:"type"`
	Nullable bool     `json:"nullable"`
	Required []string `json:"required"`
	// AllOf composes a schema from several others. The spec uses it in
	// exactly one place - the provisioned-gateway payload, which is a
	// Gateway plus the one-time token - but a composed schema whose
	// branches went unread would look like it had no fields at all.
	AllOf      []*rawSchema          `json:"allOf"`
	Items      *rawSchema            `json:"items"`
	Properties map[string]*rawSchema `json:"properties"`
}

// rawSpec is the parsed document plus the $ref resolution it needs.
type rawSpec struct {
	all map[string]*rawSchema
}

func (r rawSpec) resolve(s *rawSchema, depth int) *rawSchema {
	if s == nil {
		return &rawSchema{}
	}
	if s.Ref == "" || depth > 8 {
		return s
	}
	target, ok := r.all[strings.TrimPrefix(s.Ref, "#/components/schemas/")]
	if !ok {
		return s
	}
	return r.resolve(target, depth+1)
}

func loadRawSchemas(t *testing.T, path string) rawSpec {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]*rawSchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatalf("spec %s has no components.schemas", path)
	}
	return rawSpec{all: doc.Components.Schemas}
}

// specProperty is the part of an OpenAPI property these checks read.
type specProperty struct {
	Type     string
	Nullable bool
}

// requestSchema is one request body's shape: the key it is nested under,
// its fields, and which of them the server requires.
type requestSchema struct {
	wrapper  string
	props    map[string]specProperty
	required []string
}

// loadRequestSchemas reads each request schema and unwraps the single
// property the API nests bodies under (e.g. {"resource": {...}}).
//
// A wrapper holding an array (the membership and pool member PUT
// bodies) is unwrapped one step further, to the element schema - that is
// the shape the SDK's Go type describes.
func loadRequestSchemas(t *testing.T, path string) map[string]requestSchema {
	raw := loadRawSchemas(t, path)

	out := map[string]requestSchema{}
	for name, schema := range raw.all {
		if len(schema.Properties) != 1 {
			continue
		}
		var wrapper string
		var inner *rawSchema
		for k, v := range schema.Properties {
			wrapper, inner = k, raw.resolve(v, 0)
		}
		if inner.Type == "array" {
			inner = raw.resolve(inner.Items, 0)
		}
		if len(inner.Properties) == 0 {
			continue
		}

		props := make(map[string]specProperty, len(inner.Properties))
		for f, v := range inner.Properties {
			resolved := raw.resolve(v, 0)
			props[f] = specProperty{Type: resolved.Type, Nullable: resolved.Nullable || v.Nullable}
		}
		out[name] = requestSchema{wrapper: wrapper, props: props, required: inner.Required}
	}
	return out
}

// structField is one JSON-tagged field, with the Go type it is declared
// as - the tag alone can't tell whether a nullable field is reachable.
type structField struct {
	goName    string
	goType    string
	jsonName  string
	omitEmpty bool
}

// parseStructFieldTypes returns each struct's JSON-tagged fields along
// with their Go types. parseStructs deliberately returns only tag names
// (it checks a different property); this keeps the type information that
// nullability needs.
func parseStructFieldTypes(t *testing.T) map[string][]structField {
	out := map[string][]structField{}
	forEachStruct(t, func(name string, st *ast.StructType) {
		for _, f := range st.Fields.List {
			if len(f.Names) == 0 || f.Tag == nil {
				continue
			}
			jt := reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("json")
			if jt == "" || jt == "-" {
				continue
			}
			parts := strings.Split(jt, ",")
			if parts[0] == "" {
				continue
			}
			out[name] = append(out[name], structField{
				goName:    f.Names[0].Name,
				goType:    types.ExprString(f.Type),
				jsonName:  parts[0],
				omitEmpty: contains(parts[1:], "omitempty"),
			})
		}
	})
	return out
}

// wrapBodyCall is one wrapBody call site: the key it nests the body
// under, and the request type it wraps.
type wrapBodyCall struct {
	key         string
	requestType string
	pos         string
}

// parseWrapBodyCalls finds every wrapBody call and works out which
// request type it wraps, from the AST alone.
//
// The second argument takes three shapes across the call sites: a
// parameter (wrapBody(key, req)), a composite literal
// (wrapBody(key, membershipPatchBody{...})), and a slice built earlier
// in the function (entries := make([]membershipEntry, ...)). An argument
// in none of those shapes fails the test rather than being skipped, so
// a new call site can't quietly go unchecked.
func parseWrapBodyCalls(t *testing.T) []wrapBodyCall {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	var calls []wrapBodyCall
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					return true
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "wrapBody" || len(call.Args) != 2 {
						return true
					}

					pos := fset.Position(call.Pos()).String()
					key, ok := stringLit(call.Args[0])
					if !ok {
						t.Errorf("%s: wrapBody's key is not a string literal; this check can't verify it", pos)
						return true
					}
					typ, ok := wrappedType(fn, call.Args[1])
					if !ok {
						t.Errorf("%s: cannot determine the type wrapBody(%q, ...) wraps", pos, key)
						return true
					}
					calls = append(calls, wrapBodyCall{key: key, requestType: typ, pos: pos})
					return true
				})
				return false
			})
		}
	}
	return calls
}

// wrappedType names the request type an expression carries, stripping
// pointer and slice decoration to the underlying struct name.
func wrappedType(fn *ast.FuncDecl, arg ast.Expr) (string, bool) {
	switch a := arg.(type) {
	case *ast.CompositeLit:
		return baseTypeName(a.Type)
	case *ast.Ident:
		// A parameter carries its type in the signature.
		for _, p := range fn.Type.Params.List {
			for _, n := range p.Names {
				if n.Name == a.Name {
					return baseTypeName(p.Type)
				}
			}
		}
		// Otherwise it was built in the body: entries := make([]T, ...).
		var found string
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			if id, ok := assign.Lhs[0].(*ast.Ident); !ok || id.Name != a.Name {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "make" {
				return true
			}
			if name, ok := baseTypeName(call.Args[0]); ok {
				found = name
			}
			return true
		})
		return found, found != ""
	}
	return "", false
}

// baseTypeName strips *, [] and package qualifiers down to a type name.
func baseTypeName(e ast.Expr) (string, bool) {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name, true
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.ArrayType:
		return baseTypeName(t.Elt)
	case *ast.SelectorExpr:
		return t.Sel.Name, true
	}
	return "", false
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	return v, err == nil
}

// TestSpecReadModelNullability checks that a read-model field the spec
// marks nullable can actually represent null.
//
// String fields are exempt: decoding null to "" is idiomatic Go and
// loses nothing anyone acts on. Numbers and booleans are not, because
// their zero values are legitimate values - a null session lifetime
// decoding to 0 reads as "sessions expire immediately" rather than "not
// configured", which is how this rule came to exist. The acceptance
// tests found that one against a real portal; this catches the next one
// without needing a live server.
//
// Embedded fields are flattened in, so a field on a shared base type is
// checked against every concrete schema that carries it. That matters
// here: the spec marks the session lifetimes nullable on only one of the
// five auth providers even though the underlying column is identical for
// all of them, so the rule has to fire on any schema that admits null.
func TestSpecReadModelNullability(t *testing.T) {
	specPath := findSpec(t)
	raw := loadRawSchemas(t, specPath)
	fields := flattenedFieldTypes(t)

	for _, name := range sortedKeys(fields) {
		if _, skipped := skipTypes[name]; skipped {
			continue
		}
		if !isReadModel(name) {
			continue
		}
		schemaName := name
		if alias, ok := schemaFor[name]; ok {
			schemaName = alias
		}
		schema, ok := raw.all[schemaName]
		if !ok {
			// TestSpecConformance already reports an unmapped read
			// model; no need to say it twice.
			continue
		}

		for _, f := range fields[name] {
			prop, ok := schema.Properties[f.jsonName]
			if !ok {
				continue
			}
			resolved := raw.resolve(prop, 0)
			nullable := resolved.Nullable || prop.Nullable
			switch {
			case !nullable,
				resolved.Type == "string",
				resolved.Type == "array",
				strings.HasPrefix(f.goType, "*"),
				strings.HasPrefix(f.goType, "[]"):
				continue
			}
			t.Errorf("%s.%s (json %q): spec marks it nullable and its type is %s, "+
				"so null would decode to the zero value and be indistinguishable from a real one; "+
				"use *%s",
				name, f.goName, f.jsonName, resolved.Type, f.goType)
		}
	}
}

// flattenedFieldTypes returns each struct's JSON-tagged fields with
// their Go types, with embedded structs' fields folded into the types
// that embed them.
func flattenedFieldTypes(t *testing.T) map[string][]structField {
	own := parseStructFieldTypes(t)
	embeds := map[string][]string{}

	forEachStruct(t, func(name string, st *ast.StructType) {
		for _, f := range st.Fields.List {
			if len(f.Names) != 0 {
				continue
			}
			if id, ok := f.Type.(*ast.Ident); ok {
				embeds[name] = append(embeds[name], id.Name)
			}
		}
	})

	out := make(map[string][]structField, len(own))
	var expand func(string, int) []structField
	expand = func(name string, depth int) []structField {
		if depth > 8 {
			t.Fatalf("embedding cycle at %s", name)
		}
		fields := append([]structField(nil), own[name]...)
		for _, e := range embeds[name] {
			if _, ok := own[e]; ok {
				fields = append(fields, expand(e, depth+1)...)
			}
		}
		return fields
	}
	for name := range own {
		out[name] = expand(name, 0)
	}
	return out
}
