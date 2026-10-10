// Command genposture generates posture_fields_gen.go: one typed constant
// per field the Firezone API accepts in a Policy's posture expression.
//
// The field list is the API's own registry, published in the description
// of the PolicyPostureNode schema as one table per provider, so this
// reads the vendored OpenAPI spec rather than keeping a second copy by
// hand. Run it with `mise run generate`; `mise run generate-check` fails
// if the committed file is stale.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"sort"
	"strings"
)

// goType maps the type column of the spec's field table to the SDK's
// field type. enum_string takes the string operators, so it shares
// PostureStringField; the three IP types share PostureIPField.
var goType = map[string]string{
	"boolean":      "PostureBoolField",
	"string":       "PostureStringField",
	"enum_string":  "PostureStringField",
	"integer":      "PostureIntegerField",
	"float":        "PostureFloatField",
	"version":      "PostureVersionField",
	"datetime":     "PostureTimestampField",
	"ip":           "PostureIPField",
	"ipv4":         "PostureIPField",
	"ipv6":         "PostureIPField",
	"json":         "PostureJSONField",
	"string_array": "PostureStringArrayField",
}

// initialisms are the words in a field name that need more than a
// leading capital: the acronyms Go spells in capitals, and one brand.
var initialisms = map[string]string{
	"id": "ID", "ip": "IP", "ipv4": "IPv4", "ipv6": "IPv6", "os": "OS",
	"mdm": "MDM", "dns": "DNS", "uuid": "UUID", "udid": "UDID", "imei": "IMEI",
	"meid": "MEID", "iccid": "ICCID", "mac": "MAC", "tpm": "TPM", "vm": "VM",
	"pac": "PAC", "pcr": "PCR", "pcr0": "PCR0", "sid": "SID", "ssv": "SSV",
	"sip": "SIP", "cpu": "CPU", "eas": "EAS", "ad": "AD", "rbac": "RBAC",
	"gb": "GB", "pe": "PE", "api": "API", "url": "URL",

	// Not an initialism, but a brand name Go would otherwise flatten.
	"sentinelone": "SentinelOne",
}

type field struct {
	name      string // "intune.compliance_state"
	specType  string // "enum_string"
	platforms string // "windows, macos"
	ident     string // "PostureIntuneComplianceState"
}

func main() {
	spec := flag.String("spec", "testdata/openapi.json", "OpenAPI document to read")
	out := flag.String("out", "posture_fields_gen.go", "file to write")
	flag.Parse()

	fields, err := load(*spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genposture:", err)
		os.Exit(1)
	}
	src, err := render(fields)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genposture:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "genposture:", err)
		os.Exit(1)
	}
}

var tableRow = regexp.MustCompile("(?m)^\\| `([a-z0-9_.]+)` \\| (\\w+) \\| ([^|]+) \\|$")

func load(path string) ([]field, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Description string `json:"description"`
				OneOf       []struct {
					Properties struct {
						Field struct {
							Description string   `json:"description"`
							Enum        []string `json:"enum"`
						} `json:"field"`
					} `json:"properties"`
				} `json:"oneOf"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	node, ok := doc.Components.Schemas["PolicyPostureNode"]
	if !ok {
		return nil, fmt.Errorf("%s has no PolicyPostureNode schema", path)
	}
	leaf, ok := doc.Components.Schemas["PolicyPostureLeaf"]
	if !ok {
		return nil, fmt.Errorf("%s has no PolicyPostureLeaf schema", path)
	}

	// The leaf schema is the validator's view of the same registry. A
	// field the table lists with a type the leaf schemas don't agree on
	// means the description and the schema drifted - stop, don't guess.
	leafType := map[string]string{}
	for _, l := range leaf.OneOf {
		t, _, _ := strings.Cut(l.Properties.Field.Description, " ")
		for _, f := range l.Properties.Field.Enum {
			leafType[f] = t
		}
	}

	var fields []field
	seen := map[string]string{}
	for _, m := range tableRow.FindAllStringSubmatch(node.Description, -1) {
		f := field{name: m[1], specType: m[2], platforms: strings.TrimSpace(m[3])}
		if _, ok := goType[f.specType]; !ok {
			return nil, fmt.Errorf("field %s has type %q, which genposture does not know", f.name, f.specType)
		}
		if got := leafType[f.name]; got != f.specType {
			return nil, fmt.Errorf("field %s is %q in the table but %q in the leaf schemas", f.name, f.specType, got)
		}
		f.ident = identFor(f.name)
		if prev, dup := seen[f.ident]; dup {
			return nil, fmt.Errorf("fields %s and %s both generate %s", prev, f.name, f.ident)
		}
		seen[f.ident] = f.name
		fields = append(fields, f)
	}
	if len(fields) != len(leafType) {
		return nil, fmt.Errorf("the description lists %d fields but the leaf schemas accept %d", len(fields), len(leafType))
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].name < fields[j].name })
	return fields, nil
}

func identFor(name string) string {
	var b strings.Builder
	b.WriteString("Posture")
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == '_' }) {
		if up, ok := initialisms[w]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

func provider(f field) string {
	p, _, _ := strings.Cut(f.name, ".")
	return p
}

func render(fields []field) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/genposture from testdata/openapi.json; DO NOT EDIT.\n\n")
	b.WriteString("package firezone\n")
	for i, f := range fields {
		if i == 0 || provider(f) != provider(fields[i-1]) {
			fmt.Fprintf(&b, "\n// Fields of the %q provider.\nconst (\n", provider(f))
		}
		fmt.Fprintf(&b, "\t// %s is the %s field %q. Platforms: %s.\n", f.ident, f.specType, f.name, f.platforms)
		fmt.Fprintf(&b, "\t%s %s = %q\n", f.ident, goType[f.specType], f.name)
		if i == len(fields)-1 || provider(f) != provider(fields[i+1]) {
			b.WriteString(")\n")
		}
	}
	return format.Source(b.Bytes())
}
