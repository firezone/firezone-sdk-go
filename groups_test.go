package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestGroupsService_Get(t *testing.T) {
	t.Run("unsynced group", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "group-1", "name": "Engineering"},
		}))

		group, err := client.Groups.Get(context.Background(), "group-1")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if group.IsSynced() {
			t.Error("group.IsSynced() = true, want false (no directory_id)")
		}
	})

	t.Run("synced group", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "group-1", "name": "Engineering", "directory_id": "dir-1", "idp_id": "idp-abc"},
		}))

		group, err := client.Groups.Get(context.Background(), "group-1")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if !group.IsSynced() {
			t.Error("group.IsSynced() = false, want true (directory_id set)")
		}
	})
}

func TestGroupsService_Update(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "group-1", "name": "renamed"},
		})(w, r)
	}))

	group, err := client.Groups.Update(context.Background(), "group-1", &firezone.UpdateGroupRequest{Name: "renamed"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/groups/group-1" {
		t.Errorf("request = %s %s, want PATCH /groups/group-1", gotMethod, gotPath)
	}
	reqGroup, ok := gotBody["group"].(map[string]any)
	if !ok {
		t.Fatalf("body[\"group\"] = %v, want an object", gotBody["group"])
	}
	if reqGroup["name"] != "renamed" {
		t.Errorf("body group.name = %v, want renamed", reqGroup["name"])
	}
	if group.Name != "renamed" {
		t.Errorf("group.Name = %q, want renamed", group.Name)
	}
}

func TestGroupsService_Update_SyncedGroupForbidden(t *testing.T) {
	client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusForbidden, "Cannot update a synced Group"))

	_, err := client.Groups.Update(context.Background(), "group-1", &firezone.UpdateGroupRequest{Name: "renamed"})
	if !firezone.IsForbidden(err) {
		t.Fatalf("IsForbidden(err) = false, want true (err: %v)", err)
	}
}

func TestGroupsService_List_Filters(t *testing.T) {
	tests := []struct {
		name      string
		opts      *firezone.GroupListOptions
		wantQuery string
	}{
		{
			name:      "by name",
			opts:      &firezone.GroupListOptions{Name: "Engineering"},
			wantQuery: "name=Engineering",
		},
		{
			name:      "by entity_type",
			opts:      &firezone.GroupListOptions{EntityType: "org_unit"},
			wantQuery: "entity_type=org_unit",
		},
		{
			name:      "by a specific directory_id",
			opts:      &firezone.GroupListOptions{DirectoryID: firezone.String("dir-1")},
			wantQuery: "directory_id=dir-1",
		},
		{
			name:      "unsynced only, via a pointer to an empty string",
			opts:      &firezone.GroupListOptions{DirectoryID: firezone.String("")},
			wantQuery: "directory_id=",
		},
		{
			name:      "unset DirectoryID sends no filter at all",
			opts:      &firezone.GroupListOptions{},
			wantQuery: "",
		},
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

			_, err := client.Groups.List(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}
