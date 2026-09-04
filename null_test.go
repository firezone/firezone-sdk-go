package firezone_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

// captureBody runs call against a stub server and returns the exact
// bytes it sent. Asserting on the raw body (rather than a decoded map)
// is the whole point of these tests: a decoded map cannot tell an
// omitted field from one sent as JSON null, and that distinction is the
// difference between "leave this alone" and "clear it".
func captureBody(t *testing.T, response any, call func(c *firezone.Client) error) string {
	t.Helper()

	var body string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		body = string(b)
		testutil.JSONResponse(http.StatusOK, map[string]any{"data": response})(w, r)
	}))

	if err := call(client); err != nil {
		t.Fatalf("request returned error: %v", err)
	}
	return body
}

func TestNull_Marshal(t *testing.T) {
	type wrapper struct {
		Field *firezone.Null[string] `json:"field,omitempty"`
	}

	tests := []struct {
		name  string
		value *firezone.Null[string]
		want  string
	}{
		{name: "nil pointer omits the field", value: nil, want: `{}`},
		{name: "Clear sends JSON null", value: firezone.Clear[string](), want: `{"field":null}`},
		{name: "Set sends the value", value: firezone.Set("x"), want: `{"field":"x"}`},
		{
			// On the wire these stay distinct. The server happens to
			// treat both as a clear (it replaces an empty string with
			// the field's default), but that is its choice to make - the
			// SDK's job is to send what the caller asked for.
			name:  "Set of the empty string sends an empty string, not null",
			value: firezone.Set(""),
			want:  `{"field":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(wrapper{Field: tt.value})
			if err != nil {
				t.Fatalf("Marshal returned error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNull_MarshalNamedStringType(t *testing.T) {
	type wrapper struct {
		Field *firezone.Null[firezone.IPStack] `json:"field,omitempty"`
	}

	got, err := json.Marshal(wrapper{Field: firezone.Set(firezone.IPStackDual)})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if want := `{"field":"dual"}`; string(got) != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}

func TestNull_Unmarshal(t *testing.T) {
	type wrapper struct {
		Field *firezone.Null[string] `json:"field,omitempty"`
	}

	// A non-pointer Null does see the null, which is what UnmarshalJSON
	// is there for.
	t.Run("a non-pointer Null decodes null to an invalid Null", func(t *testing.T) {
		var v struct {
			Field firezone.Null[string] `json:"field"`
		}
		if err := json.Unmarshal([]byte(`{"field":null}`), &v); err != nil {
			t.Fatalf("Unmarshal returned error: %v", err)
		}
		if v.Field.Valid {
			t.Errorf("Field.Valid = true, want false")
		}
	})

	tests := []struct {
		name      string
		in        string
		wantSet   bool
		wantValid bool
		wantValue string
	}{
		{name: "absent leaves the pointer nil", in: `{}`, wantSet: false},
		// encoding/json sets a pointer field to nil on JSON null without
		// consulting the pointee's UnmarshalJSON, so a *Null cannot
		// preserve the null/absent distinction when decoding. That only
		// matters for round-tripping a request struct, which the SDK
		// never does - see the Null doc comment.
		{name: "null leaves the pointer nil", in: `{"field":null}`, wantSet: false},
		{
			name: "a value decodes to a valid Null", in: `{"field":"x"}`,
			wantSet: true, wantValid: true, wantValue: "x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w wrapper
			if err := json.Unmarshal([]byte(tt.in), &w); err != nil {
				t.Fatalf("Unmarshal returned error: %v", err)
			}
			if (w.Field != nil) != tt.wantSet {
				t.Fatalf("Field != nil = %v, want %v", w.Field != nil, tt.wantSet)
			}
			if w.Field == nil {
				return
			}
			if w.Field.Valid != tt.wantValid {
				t.Errorf("Field.Valid = %v, want %v", w.Field.Valid, tt.wantValid)
			}
			if w.Field.Value != tt.wantValue {
				t.Errorf("Field.Value = %q, want %q", w.Field.Value, tt.wantValue)
			}
		})
	}
}

func TestUpdateResourceRequest_Body(t *testing.T) {
	response := map[string]any{"id": "res-1", "name": "postgres", "type": "cidr"}

	tests := []struct {
		name string
		req  *firezone.UpdateResourceRequest
		want string
	}{
		{
			name: "omitted nullable fields are absent from the body",
			req:  &firezone.UpdateResourceRequest{Name: "renamed"},
			want: `{"resource":{"name":"renamed"}}`,
		},
		{
			name: "Clear sends null for each nullable field",
			req: &firezone.UpdateResourceRequest{
				Address:            firezone.Clear[string](),
				AddressDescription: firezone.Clear[string](),
				IPStack:            firezone.Clear[firezone.IPStack](),
				SiteID:             firezone.Clear[string](),
			},
			want: `{"resource":{"address":null,"address_description":null,"ip_stack":null,"site_id":null}}`,
		},
		{
			name: "Set sends each nullable field's value",
			req: &firezone.UpdateResourceRequest{
				Address:            firezone.Set("10.0.1.0/24"),
				AddressDescription: firezone.Set("Production subnet"),
				IPStack:            firezone.Set(firezone.IPStackIPv4Only),
				SiteID:             firezone.Set("site-1"),
			},
			want: `{"resource":{"address":"10.0.1.0/24","address_description":"Production subnet",` +
				`"ip_stack":"ipv4_only","site_id":"site-1"}}`,
		},
		{
			name: "a nil Filters pointer leaves the filters alone",
			req:  &firezone.UpdateResourceRequest{Name: "renamed"},
			want: `{"resource":{"name":"renamed"}}`,
		},
		{
			name: "a pointer to an empty Filters slice clears every filter",
			req:  &firezone.UpdateResourceRequest{Filters: &[]firezone.Filter{}},
			want: `{"resource":{"filters":[]}}`,
		},
		{
			name: "a populated Filters pointer replaces the filters",
			req: &firezone.UpdateResourceRequest{
				Filters: &[]firezone.Filter{
					{Protocol: firezone.FilterProtocolTCP, Ports: []string{"5432"}},
				},
			},
			want: `{"resource":{"filters":[{"protocol":"tcp","ports":["5432"]}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureBody(t, response, func(c *firezone.Client) error {
				_, err := c.Resources.Update(context.Background(), "res-1", tt.req)
				return err
			})
			if got != tt.want {
				t.Errorf("request body =\n\t%s\nwant\n\t%s", got, tt.want)
			}
		})
	}
}

func TestUpdatePolicyRequest_Body(t *testing.T) {
	response := map[string]any{"id": "pol-1", "group_id": "g", "resource_id": "r"}

	tests := []struct {
		name string
		req  *firezone.UpdatePolicyRequest
		want string
	}{
		{
			name: "an omitted Description is absent from the body",
			req:  &firezone.UpdatePolicyRequest{GroupID: "group-1"},
			want: `{"policy":{"group_id":"group-1"}}`,
		},
		{
			name: "Clear sends a null Description",
			req:  &firezone.UpdatePolicyRequest{Description: firezone.Clear[string]()},
			want: `{"policy":{"description":null}}`,
		},
		{
			name: "Set sends the Description",
			req:  &firezone.UpdatePolicyRequest{Description: firezone.Set("prod access")},
			want: `{"policy":{"description":"prod access"}}`,
		},
		{
			name: "a nil Conditions pointer leaves the conditions alone",
			req:  &firezone.UpdatePolicyRequest{GroupID: "group-1"},
			want: `{"policy":{"group_id":"group-1"}}`,
		},
		{
			name: "a pointer to an empty Conditions slice clears every condition",
			req:  &firezone.UpdatePolicyRequest{Conditions: &[]firezone.Condition{}},
			want: `{"policy":{"conditions":[]}}`,
		},
		{
			name: "a populated Conditions pointer replaces the conditions",
			req: &firezone.UpdatePolicyRequest{
				Conditions: &[]firezone.Condition{{
					Property: firezone.ConditionPropertyRemoteIPLocationRegion,
					Operator: firezone.ConditionOperatorIsIn,
					Values:   []string{"US"},
				}},
			},
			want: `{"policy":{"conditions":[{"property":"remote_ip_location_region",` +
				`"operator":"is_in","values":["US"]}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureBody(t, response, func(c *firezone.Client) error {
				_, err := c.Policies.Update(context.Background(), "pol-1", tt.req)
				return err
			})
			if got != tt.want {
				t.Errorf("request body =\n\t%s\nwant\n\t%s", got, tt.want)
			}
		})
	}
}

func TestUpdateActorRequest_Body(t *testing.T) {
	response := map[string]any{"id": "actor-1", "name": "Alice", "type": "account_user"}

	tests := []struct {
		name string
		req  *firezone.UpdateActorRequest
		want string
	}{
		{
			name: "an omitted Email is absent from the body",
			req:  &firezone.UpdateActorRequest{Name: "Alice"},
			want: `{"actor":{"name":"Alice"}}`,
		},
		{
			name: "Clear sends a null Email",
			req:  &firezone.UpdateActorRequest{Email: firezone.Clear[string]()},
			want: `{"actor":{"email":null}}`,
		},
		{
			name: "Set sends the Email",
			req:  &firezone.UpdateActorRequest{Email: firezone.Set("alice@example.com")},
			want: `{"actor":{"email":"alice@example.com"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureBody(t, response, func(c *firezone.Client) error {
				_, err := c.Actors.Update(context.Background(), "actor-1", tt.req)
				return err
			})
			if got != tt.want {
				t.Errorf("request body =\n\t%s\nwant\n\t%s", got, tt.want)
			}
		})
	}
}
