package firezone_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestMembershipsService_ReplaceAll(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"actor_ids": []string{"actor-1", "actor-2"}},
		})(w, r)
	}))

	ids, err := client.Groups.Memberships("group-1").ReplaceAll(context.Background(), []string{"actor-1", "actor-2"})
	if err != nil {
		t.Fatalf("ReplaceAll returned error: %v", err)
	}

	if gotMethod != http.MethodPut || gotPath != "/groups/group-1/memberships" {
		t.Errorf("request = %s %s, want PUT /groups/group-1/memberships", gotMethod, gotPath)
	}
	entries, ok := gotBody["memberships"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("body[memberships] = %v, want a 2-element list", gotBody["memberships"])
	}
	if len(ids) != 2 {
		t.Errorf("len(ids) = %d, want 2", len(ids))
	}
}

func TestMembershipsService_Patch(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"actor_ids": []string{"actor-1"}},
		})(w, r)
	}))

	ids, err := client.Groups.Memberships("group-1").Patch(context.Background(), []string{"actor-1"}, []string{"actor-2"})
	if err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}

	if gotMethod != http.MethodPatch || gotPath != "/groups/group-1/memberships" {
		t.Errorf("request = %s %s, want PATCH /groups/group-1/memberships", gotMethod, gotPath)
	}
	membershipBody, ok := gotBody["memberships"].(map[string]any)
	if !ok {
		t.Fatalf("body[memberships] = %v, want an object", gotBody["memberships"])
	}
	if add, _ := membershipBody["add"].([]any); len(add) != 1 {
		t.Errorf("body.memberships.add = %v, want one entry", membershipBody["add"])
	}
	if remove, _ := membershipBody["remove"].([]any); len(remove) != 1 {
		t.Errorf("body.memberships.remove = %v, want one entry", membershipBody["remove"])
	}
	if len(ids) != 1 {
		t.Errorf("len(ids) = %d, want 1", len(ids))
	}
}
