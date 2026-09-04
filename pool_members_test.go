package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestPoolMembersService_List(t *testing.T) {
	var gotMethod, gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": []map[string]any{
				{"id": "client-1", "name": "jane-laptop", "last_seen_at": "2024-01-15T10:30:00Z"},
				{"id": "client-2", "name": "field-tablet-02"},
			},
			"metadata": map[string]any{"count": 2, "limit": 50},
		})(w, r)
	}))

	page, err := client.Resources.PoolMembers("res-1").List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/resources/res-1/pool_members" {
		t.Errorf("request = %s %s, want GET /resources/res-1/pool_members", gotMethod, gotPath)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(page.Data) = %d, want 2", len(page.Data))
	}
	if page.Data[0].Name != "jane-laptop" {
		t.Errorf("Data[0].Name = %q, want jane-laptop", page.Data[0].Name)
	}
	if page.Data[0].LastSeenAt == nil {
		t.Error("Data[0].LastSeenAt = nil, want a timestamp")
	}
	// A Client that has never connected has no last_seen_at, so the
	// field must be a pointer rather than a zero time.Time.
	if page.Data[1].LastSeenAt != nil {
		t.Errorf("Data[1].LastSeenAt = %v, want nil", page.Data[1].LastSeenAt)
	}
}

func TestPoolMembersService_ReplaceAll(t *testing.T) {
	t.Run("replaces membership", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"device_ids": []string{"client-1", "client-2"}},
			})(w, r)
		}))

		ids, err := client.Resources.PoolMembers("res-1").
			ReplaceAll(context.Background(), []string{"client-1", "client-2"})
		if err != nil {
			t.Fatalf("ReplaceAll returned error: %v", err)
		}

		if gotMethod != http.MethodPut || gotPath != "/resources/res-1/pool_members" {
			t.Errorf("request = %s %s, want PUT /resources/res-1/pool_members", gotMethod, gotPath)
		}

		entries, ok := gotBody["pool_members"].([]any)
		if !ok || len(entries) != 2 {
			t.Fatalf("body[pool_members] = %v, want a 2-element list", gotBody["pool_members"])
		}
		first, ok := entries[0].(map[string]any)
		if !ok || first["device_id"] != "client-1" {
			t.Errorf("entries[0] = %v, want {device_id: client-1}", entries[0])
		}
		if len(ids) != 2 {
			t.Errorf("len(ids) = %d, want 2", len(ids))
		}
	})

	t.Run("empty slice clears the pool", func(t *testing.T) {
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"device_ids": []string{}},
			})(w, r)
		}))

		ids, err := client.Resources.PoolMembers("res-1").ReplaceAll(context.Background(), nil)
		if err != nil {
			t.Fatalf("ReplaceAll returned error: %v", err)
		}

		// Clearing the pool has to send an empty list, not omit the key
		// or send null - otherwise the API rejects the body as malformed
		// and the pool is never cleared.
		entries, ok := gotBody["pool_members"].([]any)
		if !ok {
			t.Fatalf("body[pool_members] = %v, want an empty list", gotBody["pool_members"])
		}
		if len(entries) != 0 {
			t.Errorf("body[pool_members] = %v, want empty", entries)
		}
		if len(ids) != 0 {
			t.Errorf("len(ids) = %d, want 0", len(ids))
		}
	})

	t.Run("non-client device id", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(
			http.StatusUnprocessableEntity, "The request body failed validation."))

		_, err := client.Resources.PoolMembers("res-1").
			ReplaceAll(context.Background(), []string{"gateway-1"})
		if !firezone.IsValidation(err) {
			t.Fatalf("IsValidation(err) = false, want true (err: %v)", err)
		}
	})

	t.Run("resource is not a device pool", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(
			http.StatusBadRequest, "Resource type cidr has no pool members"))

		_, err := client.Resources.PoolMembers("res-1").
			ReplaceAll(context.Background(), []string{"client-1"})
		if err == nil {
			t.Fatal("ReplaceAll returned nil error, want a 400")
		}
	})
}

func TestPoolMembersService_Patch(t *testing.T) {
	t.Run("adds and removes", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"device_ids": []string{"client-1"}},
			})(w, r)
		}))

		ids, err := client.Resources.PoolMembers("res-1").
			Patch(context.Background(), []string{"client-1"}, []string{"client-2"})
		if err != nil {
			t.Fatalf("Patch returned error: %v", err)
		}

		if gotMethod != http.MethodPatch || gotPath != "/resources/res-1/pool_members" {
			t.Errorf("request = %s %s, want PATCH /resources/res-1/pool_members", gotMethod, gotPath)
		}

		patchBody, ok := gotBody["pool_members"].(map[string]any)
		if !ok {
			t.Fatalf("body[pool_members] = %v, want an object", gotBody["pool_members"])
		}
		add, ok := patchBody["add"].([]any)
		if !ok || len(add) != 1 || add[0] != "client-1" {
			t.Errorf("body add = %v, want [client-1]", patchBody["add"])
		}
		remove, ok := patchBody["remove"].([]any)
		if !ok || len(remove) != 1 || remove[0] != "client-2" {
			t.Errorf("body remove = %v, want [client-2]", patchBody["remove"])
		}
		if len(ids) != 1 {
			t.Errorf("len(ids) = %d, want 1", len(ids))
		}
	})

	t.Run("add only omits remove", func(t *testing.T) {
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"device_ids": []string{"client-1"}},
			})(w, r)
		}))

		if _, err := client.Resources.PoolMembers("res-1").
			Patch(context.Background(), []string{"client-1"}, nil); err != nil {
			t.Fatalf("Patch returned error: %v", err)
		}

		patchBody, ok := gotBody["pool_members"].(map[string]any)
		if !ok {
			t.Fatalf("body[pool_members] = %v, want an object", gotBody["pool_members"])
		}
		// omitempty on both fields means a one-sided patch sends only the
		// side it touches - an empty remove must not read as "remove
		// nothing explicitly", which is the same thing here but would
		// matter if the API ever distinguished them.
		if _, present := patchBody["remove"]; present {
			t.Errorf("body = %v, want no remove key", patchBody)
		}
	})

	t.Run("not found", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "not found"))

		_, err := client.Resources.PoolMembers("missing").
			Patch(context.Background(), []string{"client-1"}, nil)
		if !firezone.IsNotFound(err) {
			t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
		}
	})
}
