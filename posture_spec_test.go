//go:build spec

package firezone

import (
	"encoding/json"
	"net/netip"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestSpecPostureOperators checks that each typed posture field offers
// exactly the operators the API's leaf schemas accept for that kind of
// attribute - no operator the server would reject, and none it would
// accept that the type leaves out.
func TestSpecPostureOperators(t *testing.T) {
	raw, err := os.ReadFile(findSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas struct {
				Leaf struct {
					OneOf []struct {
						Properties struct {
							Field struct {
								Description string `json:"description"`
							} `json:"field"`
							Op struct {
								Enum []string `json:"enum"`
							} `json:"op"`
						} `json:"properties"`
					} `json:"oneOf"`
				} `json:"PolicyPostureLeaf"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	// The spec's attribute types, folded onto the SDK type that serves
	// them: enum_string takes the string operators, and the three IP
	// types share one SDK type.
	sdkKind := map[string]string{
		"boolean": "bool", "string": "string", "enum_string": "string",
		"integer": "integer", "float": "float", "version": "version",
		"datetime": "timestamp", "ip": "ip", "ipv4": "ip", "ipv6": "ip",
		"json": "json", "string_array": "string_array",
	}
	want := map[string]map[string]bool{}
	for _, l := range doc.Components.Schemas.Leaf.OneOf {
		specType, _, _ := strings.Cut(l.Properties.Field.Description, " ")
		kind, ok := sdkKind[specType]
		if !ok {
			t.Fatalf("spec attribute type %q has no SDK field type; add it to PostureNode's typed fields", specType)
		}
		if want[kind] == nil {
			want[kind] = map[string]bool{}
		}
		for _, op := range l.Properties.Op.Enum {
			want[kind][op] = true
		}
	}

	pfx := netip.MustParsePrefix("10.0.0.0/8")
	got := map[string][]PostureNode{
		"bool": {
			PostureBoolField("x.y").Is(true), PostureBoolField("x.y").Exists(), PostureBoolField("x.y").DoesNotExist(),
		},
		"string": {
			PostureStringField("x.y").Is("a"), PostureStringField("x.y").IsNot("a"),
			PostureStringField("x.y").Contains("a"), PostureStringField("x.y").DoesNotContain("a"),
			PostureStringField("x.y").StartsWith("a"), PostureStringField("x.y").EndsWith("a"),
			PostureStringField("x.y").IsIn("a"), PostureStringField("x.y").IsNotIn("a"),
			PostureStringField("x.y").Matches("a"), PostureStringField("x.y").DoesNotMatch("a"),
			PostureStringField("x.y").Exists(), PostureStringField("x.y").DoesNotExist(),
		},
		"integer": {
			PostureIntegerField("x.y").Eq(1), PostureIntegerField("x.y").Ne(1),
			PostureIntegerField("x.y").Gt(1), PostureIntegerField("x.y").Gte(1),
			PostureIntegerField("x.y").Lt(1), PostureIntegerField("x.y").Lte(1),
			PostureIntegerField("x.y").Exists(), PostureIntegerField("x.y").DoesNotExist(),
		},
		"float": {
			PostureFloatField("x.y").Eq(1), PostureFloatField("x.y").Ne(1),
			PostureFloatField("x.y").Gt(1), PostureFloatField("x.y").Gte(1),
			PostureFloatField("x.y").Lt(1), PostureFloatField("x.y").Lte(1),
			PostureFloatField("x.y").Exists(), PostureFloatField("x.y").DoesNotExist(),
		},
		"version": {
			PostureVersionField("x.y").Is("1"), PostureVersionField("x.y").IsNot("1"),
			PostureVersionField("x.y").Gt("1"), PostureVersionField("x.y").Gte("1"),
			PostureVersionField("x.y").Lt("1"), PostureVersionField("x.y").Lte("1"),
			PostureVersionField("x.y").Exists(), PostureVersionField("x.y").DoesNotExist(),
		},
		"timestamp": {
			PostureTimestampField("x.y").Before(time.Time{}), PostureTimestampField("x.y").After(time.Time{}),
			PostureTimestampField("x.y").WithinLast(time.Hour), PostureTimestampField("x.y").NotWithinLast(time.Hour),
			PostureTimestampField("x.y").Exists(), PostureTimestampField("x.y").DoesNotExist(),
		},
		"ip": {
			PostureIPField("x.y").InCIDR(pfx), PostureIPField("x.y").NotInCIDR(pfx),
			PostureIPField("x.y").Exists(), PostureIPField("x.y").DoesNotExist(),
		},
		"json": {
			PostureJSONField("x.y").IsEmpty(), PostureJSONField("x.y").IsNotEmpty(),
			PostureJSONField("x.y").Exists(), PostureJSONField("x.y").DoesNotExist(),
		},
		"string_array": {
			PostureStringArrayField("x.y").Contains("a"), PostureStringArrayField("x.y").DoesNotContain("a"),
			PostureStringArrayField("x.y").ContainsAnyOf("a"), PostureStringArrayField("x.y").ContainsAllOf("a"),
			PostureStringArrayField("x.y").IsEmpty(), PostureStringArrayField("x.y").IsNotEmpty(),
			PostureStringArrayField("x.y").Exists(), PostureStringArrayField("x.y").DoesNotExist(),
		},
	}

	for kind, ops := range want {
		nodes, ok := got[kind]
		if !ok {
			t.Errorf("no typed field covers spec kind %q", kind)
			continue
		}
		have := map[string]bool{}
		for _, n := range nodes {
			have[string(n.Operator)] = true
		}
		if !reflect.DeepEqual(have, ops) {
			t.Errorf("%s field operators = %v, spec accepts %v", kind, keys(have), keys(ops))
		}
	}
	for kind := range got {
		if _, ok := want[kind]; !ok {
			t.Errorf("typed %s field has no counterpart in the spec", kind)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
