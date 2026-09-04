package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestPoliciesService_Create(t *testing.T) {
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusCreated, map[string]any{
			"data": map[string]any{
				"id": "pol-1", "group_id": "group-1", "resource_id": "res-1",
				"description": "eng access", "flow_log_uploads_enabled": true,
				"conditions": []map[string]any{
					{"property": "remote_ip_location_region", "operator": "is_in", "values": []string{"US", "CA"}},
				},
			},
		})(w, r)
	}))

	policy, err := client.Policies.Create(context.Background(), &firezone.CreatePolicyRequest{
		GroupID:    "group-1",
		ResourceID: "res-1",
		Conditions: []firezone.Condition{
			{Property: firezone.ConditionPropertyRemoteIPLocationRegion, Operator: firezone.ConditionOperatorIsIn, Values: []string{"US", "CA"}},
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	reqPolicy, ok := gotBody["policy"].(map[string]any)
	if !ok {
		t.Fatalf("body[policy] = %v, want an object", gotBody["policy"])
	}
	if reqPolicy["group_id"] != "group-1" {
		t.Errorf("body.policy.group_id = %v, want group-1", reqPolicy["group_id"])
	}

	if len(policy.Conditions) != 1 {
		t.Fatalf("len(policy.Conditions) = %d, want 1", len(policy.Conditions))
	}
	if policy.Conditions[0].Operator != firezone.ConditionOperatorIsIn {
		t.Errorf("policy.Conditions[0].Operator = %q, want is_in", policy.Conditions[0].Operator)
	}
}

func TestPoliciesService_Create_TimeRangeCondition(t *testing.T) {
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusCreated, map[string]any{
			"data": map[string]any{
				"id": "pol-1", "group_id": "group-1", "resource_id": "res-1",
				"flow_log_uploads_enabled": true,
				"conditions": []map[string]any{
					{
						"property": "current_utc_datetime",
						"operator": "is_in_day_of_week_time_ranges",
						"values":   []string{"M/09:00-17:00/America/New_York"},
					},
				},
			},
		})(w, r)
	}))

	policy, err := client.Policies.Create(context.Background(), &firezone.CreatePolicyRequest{
		GroupID:    "group-1",
		ResourceID: "res-1",
		Conditions: []firezone.Condition{{
			Property: firezone.ConditionPropertyCurrentUTCDatetime,
			Operator: firezone.ConditionOperatorIsInDayOfWeekTimeRanges,
			Values:   []string{"M/09:00-17:00/America/New_York"},
		}},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	reqPolicy, ok := gotBody["policy"].(map[string]any)
	if !ok {
		t.Fatalf("body[\"policy\"] = %v, want an object", gotBody["policy"])
	}
	conds, ok := reqPolicy["conditions"].([]any)
	if !ok || len(conds) != 1 {
		t.Fatalf("body policy.conditions = %v, want 1 condition", reqPolicy["conditions"])
	}
	cond, ok := conds[0].(map[string]any)
	if !ok {
		t.Fatalf("condition = %v, want an object", conds[0])
	}
	if cond["property"] != "current_utc_datetime" {
		t.Errorf("condition.property = %v, want current_utc_datetime", cond["property"])
	}
	if cond["operator"] != "is_in_day_of_week_time_ranges" {
		t.Errorf("condition.operator = %v, want is_in_day_of_week_time_ranges", cond["operator"])
	}
	// The timezone segment is required and contains a slash itself, so
	// it must survive JSON encoding intact - this is the value shape
	// most likely to get mangled by a well-meaning "cleanup".
	vals, ok := cond["values"].([]any)
	if !ok || len(vals) != 1 || vals[0] != "M/09:00-17:00/America/New_York" {
		t.Errorf("condition.values = %v, want [M/09:00-17:00/America/New_York]", cond["values"])
	}

	if len(policy.Conditions) != 1 ||
		policy.Conditions[0].Property != firezone.ConditionPropertyCurrentUTCDatetime ||
		policy.Conditions[0].Operator != firezone.ConditionOperatorIsInDayOfWeekTimeRanges {
		t.Errorf("policy.Conditions = %+v, want one current_utc_datetime condition", policy.Conditions)
	}
}

func TestPoliciesService_DisableEnable(t *testing.T) {
	t.Run("disable", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "pol-1", "group_id": "g", "resource_id": "r", "is_disabled": true},
			})(w, r)
		}))

		policy, err := client.Policies.Disable(context.Background(), "pol-1")
		if err != nil {
			t.Fatalf("Disable returned error: %v", err)
		}
		if gotMethod != http.MethodPatch || gotPath != "/policies/pol-1" {
			t.Errorf("request = %s %s, want PATCH /policies/pol-1", gotMethod, gotPath)
		}

		reqPolicy, ok := gotBody["policy"].(map[string]any)
		if !ok {
			t.Fatalf("body[\"policy\"] = %v, want an object", gotBody["policy"])
		}
		if reqPolicy["is_disabled"] != true {
			t.Errorf("body policy.is_disabled = %v, want true", reqPolicy["is_disabled"])
		}
		// Disable must not send a zero-valued GroupID/ResourceID, which
		// would repoint the Policy at nothing.
		if len(reqPolicy) != 1 {
			t.Errorf("body policy = %v, want only is_disabled", reqPolicy)
		}
		if !policy.IsDisabled {
			t.Error("policy.IsDisabled = false, want true")
		}
	})

	t.Run("enable", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "pol-1", "group_id": "g", "resource_id": "r", "is_disabled": false},
			})(w, r)
		}))

		policy, err := client.Policies.Enable(context.Background(), "pol-1")
		if err != nil {
			t.Fatalf("Enable returned error: %v", err)
		}
		if gotMethod != http.MethodPatch || gotPath != "/policies/pol-1" {
			t.Errorf("request = %s %s, want PATCH /policies/pol-1", gotMethod, gotPath)
		}

		reqPolicy, ok := gotBody["policy"].(map[string]any)
		if !ok {
			t.Fatalf("body[\"policy\"] = %v, want an object", gotBody["policy"])
		}
		if reqPolicy["is_disabled"] != false {
			t.Errorf("body policy.is_disabled = %v, want false", reqPolicy["is_disabled"])
		}
		if policy.IsDisabled {
			t.Error("policy.IsDisabled = true, want false")
		}
	})

	t.Run("not found", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "not found"))
		_, err := client.Policies.Disable(context.Background(), "missing")
		if !firezone.IsNotFound(err) {
			t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
		}
	})
}

func TestPoliciesService_List_Filters(t *testing.T) {
	tests := []struct {
		name      string
		opts      *firezone.PolicyListOptions
		wantQuery string
	}{
		{name: "by group_id", opts: &firezone.PolicyListOptions{GroupID: "group-1"}, wantQuery: "group_id=group-1"},
		{name: "by resource_id", opts: &firezone.PolicyListOptions{ResourceID: "res-1"}, wantQuery: "resource_id=res-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery string
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				testutil.JSONResponse(http.StatusOK, map[string]any{
					"data":     []map[string]any{},
					"metadata": map[string]any{"count": 0, "limit": 50, "next_page": "", "prev_page": ""},
				})(w, r)
			}))

			_, err := client.Policies.List(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}
