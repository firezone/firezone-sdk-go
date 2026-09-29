package firezone_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestResourcesService_Create(t *testing.T) {
	t.Run("happy path with filters", func(t *testing.T) {
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id": "res-1", "name": "postgres-prod", "type": "cidr",
					"address": "10.0.1.0/24", "address_description": "Production Postgres subnet",
					"site_id": "site-1",
					"filters": []map[string]any{
						{"protocol": "tcp", "ports": []string{"5432"}},
					},
				},
			})(w, r)
		}))

		resource, err := client.Resources.Create(context.Background(), &firezone.CreateResourceRequest{
			Name:    "postgres-prod",
			Type:    firezone.ResourceTypeCIDR,
			Address: "10.0.1.0/24",
			SiteID:  "site-1",
			Filters: []firezone.Filter{{Protocol: firezone.FilterProtocolTCP, Ports: []string{"5432"}}},
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		reqResource, ok := gotBody["resource"].(map[string]any)
		if !ok {
			t.Fatalf("body[resource] = %v, want an object", gotBody["resource"])
		}
		if reqResource["type"] != "cidr" {
			t.Errorf("body.resource.type = %v, want cidr", reqResource["type"])
		}

		if resource.Type != firezone.ResourceTypeCIDR {
			t.Errorf("resource.Type = %q, want cidr", resource.Type)
		}
		if len(resource.Filters) != 1 || resource.Filters[0].Protocol != firezone.FilterProtocolTCP {
			t.Errorf("resource.Filters = %+v, want one tcp filter", resource.Filters)
		}
	})

	t.Run("internet type is forbidden", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusForbidden, "Internet Resource cannot be modified"))

		_, err := client.Resources.Create(context.Background(), &firezone.CreateResourceRequest{Name: "internet"})
		if !firezone.IsForbidden(err) {
			t.Fatalf("IsForbidden(err) = false, want true (err: %v)", err)
		}
	})
}

func TestResourcesService_Update(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "res-1", "name": "renamed", "type": "cidr"},
		})(w, r)
	}))

	resource, err := client.Resources.Update(context.Background(), "res-1",
		&firezone.UpdateResourceRequest{Name: "renamed"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/resources/res-1" {
		t.Errorf("request = %s %s, want PATCH /resources/res-1", gotMethod, gotPath)
	}
	// A rename must not carry any other key: an update is a merge, and a
	// stray zero value would overwrite a field the caller never named.
	reqResource, ok := gotBody["resource"].(map[string]any)
	if !ok {
		t.Fatalf("body[\"resource\"] = %v, want an object", gotBody["resource"])
	}
	if len(reqResource) != 1 || reqResource["name"] != "renamed" {
		t.Errorf("body resource = %v, want only name=renamed", reqResource)
	}
	if resource.Name != "renamed" {
		t.Errorf("resource.Name = %q, want renamed", resource.Name)
	}
}

func TestResourcesService_Delete(t *testing.T) {
	client := testutil.NewClient(t, testutil.JSONResponse(http.StatusOK, map[string]any{
		"data": map[string]any{"id": "res-1", "name": "postgres-prod"},
	}))

	if err := client.Resources.Delete(context.Background(), "res-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
}

func TestResourcesService_List_Filters(t *testing.T) {
	tests := []struct {
		name      string
		opts      *firezone.ResourceListOptions
		wantQuery string
	}{
		{name: "by name", opts: &firezone.ResourceListOptions{Name: "postgres-prod"}, wantQuery: "name=postgres-prod"},
		{name: "by type", opts: &firezone.ResourceListOptions{Type: firezone.ResourceTypeDNS}, wantQuery: "type=dns"},
		{name: "by site_id", opts: &firezone.ResourceListOptions{SiteID: "site-1"}, wantQuery: "site_id=site-1"},
		{name: "by address", opts: &firezone.ResourceListOptions{Address: "10.0.0.10"}, wantQuery: "address=10.0.0.10"},
		{name: "by ip_stack", opts: &firezone.ResourceListOptions{IPStack: firezone.IPStackDual}, wantQuery: "ip_stack=dual"},
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

			_, err := client.Resources.List(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

// TestResourcesService_Create_DevicePoolRejected documents the API's
// current refusal to create device pools. The SDK doesn't block it
// client-side - it has no way to know the restriction has been lifted -
// so this pins the error surface callers should branch on.
func TestResourcesService_Create_DevicePoolRejected(t *testing.T) {
	client := testutil.NewClient(t, testutil.ProblemResponse(
		http.StatusUnprocessableEntity, "The request body failed validation."))

	_, err := client.Resources.Create(context.Background(), &firezone.CreateResourceRequest{
		Name: "field-laptops",
		Type: firezone.ResourceTypeDevicePool,
	})
	if !firezone.IsValidation(err) {
		t.Fatalf("IsValidation(err) = false, want true (err: %v)", err)
	}
}

// TestResourcesService_List_FilterByDevicePool checks the current pool type filter.
func TestResourcesService_List_FilterByDevicePool(t *testing.T) {
	var gotQuery string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": []map[string]any{
				{"id": "res-1", "name": "field-laptops", "type": "device_pool"},
			},
			"metadata": map[string]any{"count": 1, "limit": 50},
		})(w, r)
	}))

	page, err := client.Resources.List(context.Background(),
		&firezone.ResourceListOptions{Type: firezone.ResourceTypeDevicePool})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if gotQuery != "type=device_pool" {
		t.Errorf("query = %q, want type=device_pool", gotQuery)
	}
	if len(page.Data) != 1 || page.Data[0].Type != firezone.ResourceTypeDevicePool {
		t.Errorf("page.Data = %+v, want one device_pool Resource", page.Data)
	}
}

func TestResourcesService_DevicePools(t *testing.T) {
	for _, criteria := range []string{
		`{"device":{"field":"id","op":"in","value":[]}}`,
		`{"device":{"field":"id","op":"in","value":["11111111-2222-3333-4444-555555555555"]}}`,
		`{"device":{"field":"actor_id","op":"eq","value":{"subject":"actor_id"}}}`,
		`{"device":{"field":"account_id","op":"eq","value":{"subject":"account_id"}}}`,
		`{"actor_group":{"field":"id","op":"eq","value":"11111111-2222-3333-4444-555555555555"}}`,
	} {
		t.Run(criteria, func(t *testing.T) {
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost || r.Method == http.MethodPatch {
					var body map[string]map[string]json.RawMessage
					decodeJSONBody(t, r, &body)
					var got, want any
					if err := json.Unmarshal(body["resource"]["device_membership_criteria"], &got); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(criteria), &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("criteria = %v, want %v", got, want)
					}
				}
				res := map[string]any{"id": "pool-1", "type": "device_pool", "name": "pool", "device_membership_criteria": json.RawMessage(criteria)}
				var data any = res
				if r.Method == http.MethodGet && r.URL.Path == "/resources" {
					data = []any{res}
				}
				testutil.JSONResponse(http.StatusOK, map[string]any{"data": data})(w, r)
			}))
			ctx := context.Background()
			created, err := client.Resources.Create(ctx, &firezone.CreateResourceRequest{Name: "pool", Type: firezone.ResourceTypeDevicePool, DeviceMembershipCriteria: json.RawMessage(criteria)})
			if err != nil {
				t.Fatal(err)
			}
			updated, err := client.Resources.Update(ctx, created.ID, &firezone.UpdateResourceRequest{DeviceMembershipCriteria: json.RawMessage(criteria)})
			if err != nil {
				t.Fatal(err)
			}
			found, err := client.Resources.Get(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			page, err := client.Resources.List(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, res := range []*firezone.Resource{created, updated, found, &page.Data[0]} {
				if res.Type != firezone.ResourceTypeDevicePool || len(res.DeviceMembershipCriteria) == 0 {
					t.Fatalf("lost device pool fields: %+v", res)
				}
			}
		})
	}
}
