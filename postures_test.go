package firezone_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"reflect"
	"testing"
	"time"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

// postureExample is the expression the API's own documentation uses, so
// the SDK is tested against a shape the server is known to accept.
const postureExample = `{
	"and": [
		{"field": "intune.enrolled", "op": "is", "value": true},
		{"field": "intune.compliance_state", "op": "is", "rows": "all", "value": "compliant"},
		{"field": "intune.last_sync_at", "op": "within_last", "value": "PT24H"},
		{"or": [
			{"field": "firezone.last_seen_version", "op": "gte", "value": "@latest"},
			{"not": {"field": "firezone.hostname", "op": "starts_with", "value": "test-"}}
		]}
	]
}`

func examplePosture() firezone.PostureNode {
	return firezone.PostureAnd(
		firezone.PostureCheck("intune.enrolled", firezone.PostureOperatorIs, true),
		firezone.PostureCheck("intune.compliance_state", firezone.PostureOperatorIs, "compliant").
			WithRows(firezone.PostureRowsAll),
		firezone.PostureCheck("intune.last_sync_at", firezone.PostureOperatorWithinLast, "PT24H"),
		firezone.PostureOr(
			firezone.PostureCheck("firezone.last_seen_version", firezone.PostureOperatorGte, "@latest"),
			firezone.PostureNot(firezone.PostureCheck("firezone.hostname", firezone.PostureOperatorStartsWith, "test-")),
		),
	)
}

func TestPostureNode_JSONRoundTrip(t *testing.T) {
	got, err := json.Marshal(examplePosture())
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var want, have any
	if err := json.Unmarshal([]byte(postureExample), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &have); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(have, want) {
		t.Errorf("encoded = %s, want %s", got, postureExample)
	}

	var back firezone.PostureNode
	if err := json.Unmarshal([]byte(postureExample), &back); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	again, err := json.Marshal(back)
	if err != nil {
		t.Fatalf("re-Marshal returned error: %v", err)
	}
	var reencoded any
	if err := json.Unmarshal(again, &reencoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reencoded, want) {
		t.Errorf("round trip = %s, want %s", again, postureExample)
	}
}

func TestPostureNode_ValuelessAndFalseValues(t *testing.T) {
	// A value-less operator must send no value key; a false value must
	// still be sent despite being the zero value.
	exists, err := json.Marshal(firezone.PostureCheck("intune.model", firezone.PostureOperatorExists, nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(exists) != `{"field":"intune.model","op":"exists"}` {
		t.Errorf("exists encoded as %s", exists)
	}
	no, err := json.Marshal(firezone.PostureCheck("intune.jail_broken", firezone.PostureOperatorIs, false))
	if err != nil {
		t.Fatal(err)
	}
	if string(no) != `{"field":"intune.jail_broken","op":"is","value":false}` {
		t.Errorf("false value encoded as %s", no)
	}
}

func TestPostureNode_MarshalRejectsBadShapes(t *testing.T) {
	leaf := firezone.PostureCheck("intune.enrolled", firezone.PostureOperatorIs, true)
	tests := map[string]firezone.PostureNode{
		"empty":              {},
		"empty and":          firezone.PostureAnd(),
		"leaf plus and":      {Field: "a.b", Operator: firezone.PostureOperatorExists, And: []firezone.PostureNode{leaf}},
		"and plus or":        {And: []firezone.PostureNode{leaf}, Or: []firezone.PostureNode{leaf}},
		"field without op":   {Field: "intune.enrolled"},
		"op without field":   {Operator: firezone.PostureOperatorIs},
		"rows on boolean":    {And: []firezone.PostureNode{leaf}, Rows: firezone.PostureRowsAll},
		"bad node in a list": firezone.PostureAnd(leaf, firezone.PostureNode{}),
	}
	for name, node := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := json.Marshal(node); err == nil {
				t.Fatal("Marshal succeeded, want an error")
			}
		})
	}
}

func TestPoliciesService_Create_Postures(t *testing.T) {
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusCreated, map[string]any{
			"data": map[string]any{
				"id": "pol-1", "group_id": "group-1", "resource_id": "res-1",
				"conditions": []any{},
				"postures":   json.RawMessage(postureExample),
			},
		})(w, r)
	}))

	node := examplePosture()
	policy, err := client.Policies.Create(context.Background(), &firezone.CreatePolicyRequest{
		GroupID: "group-1", ResourceID: "res-1", Postures: &node,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	var want any
	if err := json.Unmarshal([]byte(postureExample), &want); err != nil {
		t.Fatal(err)
	}
	sent := gotBody["policy"].(map[string]any)["postures"]
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("body.policy.postures = %v, want %v", sent, want)
	}
	if policy.Postures == nil || len(policy.Postures.And) != 4 {
		t.Fatalf("policy.Postures = %+v, want an and node with 4 children", policy.Postures)
	}
	if got := policy.Postures.And[1].Rows; got != firezone.PostureRowsAll {
		t.Errorf("And[1].Rows = %q, want all", got)
	}
}

func TestPoliciesService_Create_NoPostures(t *testing.T) {
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusCreated, map[string]any{
			"data": map[string]any{"id": "pol-1", "conditions": []any{}, "postures": nil},
		})(w, r)
	}))

	policy, err := client.Policies.Create(context.Background(), &firezone.CreatePolicyRequest{
		GroupID: "group-1", ResourceID: "res-1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, ok := gotBody["policy"].(map[string]any)["postures"]; ok {
		t.Error("body.policy has postures, want it omitted")
	}
	if policy.Postures != nil {
		t.Errorf("policy.Postures = %+v, want nil for a null response", policy.Postures)
	}
}

func TestPoliciesService_Update_Postures(t *testing.T) {
	node := firezone.PostureCheck("intune.enrolled", firezone.PostureOperatorIs, true)
	tests := []struct {
		name    string
		req     firezone.UpdatePolicyRequest
		present bool
		want    any
	}{
		{name: "unchanged", req: firezone.UpdatePolicyRequest{}, present: false},
		{name: "cleared", req: firezone.UpdatePolicyRequest{Postures: firezone.Clear[firezone.PostureNode]()}, present: true, want: nil},
		{name: "replaced", req: firezone.UpdatePolicyRequest{Postures: firezone.Set(node)}, present: true,
			want: map[string]any{"field": "intune.enrolled", "op": "is", "value": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBody map[string]any
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decodeJSONBody(t, r, &gotBody)
				testutil.JSONResponse(http.StatusOK, map[string]any{
					"data": map[string]any{"id": "pol-1", "conditions": []any{}},
				})(w, r)
			}))
			if _, err := client.Policies.Update(context.Background(), "pol-1", &tt.req); err != nil {
				t.Fatalf("Update returned error: %v", err)
			}
			got, ok := gotBody["policy"].(map[string]any)["postures"]
			if ok != tt.present {
				t.Fatalf("postures present = %v, want %v", ok, tt.present)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("postures = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPoliciesService_Create_InvalidPostureNotSent(t *testing.T) {
	called := false
	client := testutil.NewClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	_, err := client.Policies.Create(context.Background(), &firezone.CreatePolicyRequest{
		GroupID: "g", ResourceID: "r", Postures: &firezone.PostureNode{},
	})
	if err == nil {
		t.Fatal("Create succeeded, want an error for an empty posture node")
	}
	var apiErr *firezone.APIError
	if errors.As(err, &apiErr) {
		t.Errorf("err is an APIError (%v), want a client-side error", err)
	}
	if called {
		t.Error("a request was sent for an invalid posture node")
	}
}

func TestPostureFields_Encoding(t *testing.T) {
	tests := []struct {
		name string
		node firezone.PostureNode
		want string
	}{
		{"bool", firezone.PostureIntuneEnrolled.Is(true), `{"field":"intune.enrolled","op":"is","value":true}`},
		{"string list", firezone.PostureIntuneComplianceState.IsIn("compliant", "inGracePeriod"),
			`{"field":"intune.compliance_state","op":"is_in","value":["compliant","inGracePeriod"]}`},
		{"empty list stays a list", firezone.PostureIntuneComplianceState.IsIn(),
			`{"field":"intune.compliance_state","op":"is_in","value":[]}`},
		{"version latest", firezone.PostureFirezoneLastSeenVersion.Gte(firezone.PostureVersionLatest),
			`{"field":"firezone.last_seen_version","op":"gte","value":"@latest"}`},
		{"within last", firezone.PostureIntuneLastSyncAt.WithinLast(24 * time.Hour),
			`{"field":"intune.last_sync_at","op":"within_last","value":"PT24H"}`},
		{"within last mixed", firezone.PostureIntuneLastSyncAt.NotWithinLast(90*time.Minute + 30*time.Second),
			`{"field":"intune.last_sync_at","op":"not_within_last","value":"PT1H30M30S"}`},
		{"before", firezone.PostureIntuneLastSyncAt.Before(time.Date(2026, 1, 1, 2, 0, 0, 0, time.FixedZone("x", 3600))),
			`{"field":"intune.last_sync_at","op":"before","value":"2026-01-01T01:00:00Z"}`},
		{"cidr", firezone.PostureFirezoneIPv4.InCIDR(netip.MustParsePrefix("10.0.0.0/8")),
			`{"field":"firezone.ipv4","op":"is_in_cidr","value":["10.0.0.0/8"]}`},
		{"exists", firezone.PostureIntuneEnrolled.Exists(), `{"field":"intune.enrolled","op":"exists"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.node)
			if err != nil {
				t.Fatalf("Marshal returned error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("encoded = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPostureNode_RowsRejectedForFirezoneFields(t *testing.T) {
	if _, err := json.Marshal(firezone.PostureFirezoneHostname.Is("a").WithRows(firezone.PostureRowsAll)); err == nil {
		t.Error("Marshal succeeded for rows on a firezone field, want an error")
	}
	if _, err := json.Marshal(firezone.PostureIntuneEnrolled.Is(true).WithRows(firezone.PostureRowsAny)); err != nil {
		t.Errorf("Marshal rejected rows on a provider field: %v", err)
	}
}

func TestPostureFields_ConstantsMatchTheirNames(t *testing.T) {
	// A spot check that the generator's naming and typing hold: the
	// constant's value is the dotted field name the API expects.
	if string(firezone.PostureIntuneComplianceState) != "intune.compliance_state" {
		t.Errorf("PostureIntuneComplianceState = %q", firezone.PostureIntuneComplianceState)
	}
	if string(firezone.PostureSentinelOneOSUpToDate) != "sentinelone.os_up_to_date" {
		t.Errorf("PostureSentinelOneOSUpToDate = %q", firezone.PostureSentinelOneOSUpToDate)
	}
}
