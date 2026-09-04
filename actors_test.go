package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestActorsService_DisableEnable(t *testing.T) {
	t.Run("disable", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "actor-1", "name": "svc", "type": "service_account", "is_disabled": true},
			})(w, r)
		}))

		actor, err := client.Actors.Disable(context.Background(), "actor-1")
		if err != nil {
			t.Fatalf("Disable returned error: %v", err)
		}
		if gotMethod != http.MethodPatch || gotPath != "/actors/actor-1" {
			t.Errorf("request = %s %s, want PATCH /actors/actor-1", gotMethod, gotPath)
		}

		reqActor, ok := gotBody["actor"].(map[string]any)
		if !ok {
			t.Fatalf("body[\"actor\"] = %v, want an object", gotBody["actor"])
		}
		if reqActor["is_disabled"] != true {
			t.Errorf("body actor.is_disabled = %v, want true", reqActor["is_disabled"])
		}
		// Disable must send nothing but is_disabled - a zero-valued
		// Name or Type would otherwise overwrite the Actor's real ones.
		if len(reqActor) != 1 {
			t.Errorf("body actor = %v, want only is_disabled", reqActor)
		}
		if !actor.IsDisabled {
			t.Error("actor.IsDisabled = false, want true")
		}
	})

	t.Run("enable", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "actor-1", "name": "svc", "type": "service_account", "is_disabled": false},
			})(w, r)
		}))

		actor, err := client.Actors.Enable(context.Background(), "actor-1")
		if err != nil {
			t.Fatalf("Enable returned error: %v", err)
		}
		if gotMethod != http.MethodPatch || gotPath != "/actors/actor-1" {
			t.Errorf("request = %s %s, want PATCH /actors/actor-1", gotMethod, gotPath)
		}

		reqActor, ok := gotBody["actor"].(map[string]any)
		if !ok {
			t.Fatalf("body[\"actor\"] = %v, want an object", gotBody["actor"])
		}
		// false is not the same as omitted: is_disabled is a *bool
		// precisely so enabling doesn't drop out of the JSON body.
		if reqActor["is_disabled"] != false {
			t.Errorf("body actor.is_disabled = %v, want false", reqActor["is_disabled"])
		}
		if actor.IsDisabled {
			t.Error("actor.IsDisabled = true, want false")
		}
	})

	t.Run("cannot disable self", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusForbidden, "You cannot disable the Actor used to make this request"))
		_, err := client.Actors.Disable(context.Background(), "self")
		if !firezone.IsForbidden(err) {
			t.Fatalf("IsForbidden(err) = false, want true (err: %v)", err)
		}
	})
}

func TestActorsService_Create(t *testing.T) {
	t.Run("service account limit reached", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusForbidden, "Service accounts limit reached"))

		_, err := client.Actors.Create(context.Background(), &firezone.CreateActorRequest{
			Name: "svc", Type: firezone.ActorTypeServiceAccount,
		})
		if !firezone.IsForbidden(err) {
			t.Fatalf("IsForbidden(err) = false, want true (err: %v)", err)
		}
	})
}

func TestActorsService_List_Filters(t *testing.T) {
	tests := []struct {
		name      string
		opts      *firezone.ActorListOptions
		wantQuery string
	}{
		{name: "by name", opts: &firezone.ActorListOptions{Name: "alice"}, wantQuery: "name=alice"},
		{
			name:      "by email",
			opts:      &firezone.ActorListOptions{Email: "alice@example.com"},
			wantQuery: "email=alice%40example.com",
		},
		{
			name:      "by type",
			opts:      &firezone.ActorListOptions{Type: firezone.ActorTypeServiceAccount},
			wantQuery: "type=service_account",
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

			_, err := client.Actors.List(context.Background(), tt.opts)
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}
