package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestClientsService_Get(t *testing.T) {
	var gotMethod, gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{
				"id":           "client-1",
				"firezone_id":  "fz-abc123",
				"actor_id":     "actor-1",
				"name":         "jane-laptop",
				"ipv4":         "100.64.0.2",
				"ipv6":         "fd00:2021:1111::2",
				"online":       true,
				"verified_at":  "2024-01-15T10:30:00Z",
				"last_seen_at": "2024-01-16T08:00:00Z",
				"created_at":   "2024-01-01T00:00:00Z",
				"updated_at":   "2024-01-16T08:00:00Z",
			},
		})(w, r)
	}))

	device, err := client.ClientDevices.Get(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/clients/client-1" {
		t.Errorf("request = %s %s, want GET /clients/client-1", gotMethod, gotPath)
	}
	if device.Name != "jane-laptop" {
		t.Errorf("Name = %q, want jane-laptop", device.Name)
	}
	if device.FirezoneID != "fz-abc123" {
		t.Errorf("FirezoneID = %q, want fz-abc123", device.FirezoneID)
	}
	if !device.Online {
		t.Error("Online = false, want true")
	}
	if device.VerifiedAt == nil {
		t.Error("VerifiedAt = nil, want a timestamp")
	}
}

func TestClientsService_Get_Unverified(t *testing.T) {
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{
				"id": "client-1", "name": "new-laptop", "online": false,
				"created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
			},
		})(w, r)
	}))

	device, err := client.ClientDevices.Get(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	// An unverified device that has never connected omits both fields.
	// They're pointers so "never verified" stays distinguishable from
	// the zero time rather than collapsing into it.
	if device.VerifiedAt != nil {
		t.Errorf("VerifiedAt = %v, want nil", device.VerifiedAt)
	}
	if device.LastSeenAt != nil {
		t.Errorf("LastSeenAt = %v, want nil", device.LastSeenAt)
	}
}

func TestClientsService_List(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": []map[string]any{
				{"id": "client-1", "name": "jane-laptop"},
				{"id": "client-2", "name": "field-tablet-02"},
			},
			"metadata": map[string]any{"count": 2, "limit": 100},
		})(w, r)
	}))

	page, err := client.ClientDevices.List(context.Background(),
		&firezone.ClientListOptions{ListOptions: firezone.ListOptions{Limit: 100}})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if gotMethod != http.MethodGet || gotPath != "/clients" {
		t.Errorf("request = %s %s, want GET /clients", gotMethod, gotPath)
	}
	if gotQuery != "limit=100" {
		t.Errorf("query = %q, want limit=100", gotQuery)
	}
	if len(page.Data) != 2 {
		t.Fatalf("len(page.Data) = %d, want 2", len(page.Data))
	}
}

func TestClientsService_Update(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		decodeJSONBody(t, r, &gotBody)
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "client-1", "name": "renamed"},
		})(w, r)
	}))

	if _, err := client.ClientDevices.Update(context.Background(), "client-1",
		&firezone.UpdateClientRequest{Name: "renamed"}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if gotMethod != http.MethodPatch || gotPath != "/clients/client-1" {
		t.Errorf("request = %s %s, want PATCH /clients/client-1", gotMethod, gotPath)
	}
	reqClient, ok := gotBody["client"].(map[string]any)
	if !ok || reqClient["name"] != "renamed" {
		t.Errorf("body[client] = %v, want {name: renamed}", gotBody["client"])
	}
}

func TestClientsService_VerifyUnverify(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*firezone.Client) (*firezone.ClientDevice, error)
		wantPath string
	}{
		{
			name: "verify",
			call: func(c *firezone.Client) (*firezone.ClientDevice, error) {
				return c.ClientDevices.Verify(context.Background(), "client-1")
			},
			wantPath: "/clients/client-1/verify",
		},
		{
			name: "unverify",
			call: func(c *firezone.Client) (*firezone.ClientDevice, error) {
				return c.ClientDevices.Unverify(context.Background(), "client-1")
			},
			wantPath: "/clients/client-1/unverify",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				testutil.JSONResponse(http.StatusOK, map[string]any{
					"data": map[string]any{"id": "client-1", "name": "jane-laptop"},
				})(w, r)
			}))

			if _, err := tt.call(client); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			if gotMethod != http.MethodPut || gotPath != tt.wantPath {
				t.Errorf("request = %s %s, want PUT %s", gotMethod, gotPath, tt.wantPath)
			}
		})
	}
}

func TestClientsService_Delete(t *testing.T) {
	var gotMethod, gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))

	if err := client.ClientDevices.Delete(context.Background(), "client-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/clients/client-1" {
		t.Errorf("request = %s %s, want DELETE /clients/client-1", gotMethod, gotPath)
	}
}

func TestClientsService_Get_NotFound(t *testing.T) {
	client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "not found"))

	_, err := client.ClientDevices.Get(context.Background(), "missing")
	if !firezone.IsNotFound(err) {
		t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
	}
}

func TestClientsService_List_Filters(t *testing.T) {
	tests := []struct {
		name      string
		opts      *firezone.ClientListOptions
		wantQuery string
	}{
		{
			name:      "by name",
			opts:      &firezone.ClientListOptions{Name: "jane-laptop"},
			wantQuery: "name=jane-laptop",
		},
		{
			name:      "by firezone_id",
			opts:      &firezone.ClientListOptions{FirezoneID: "fz-abc123"},
			wantQuery: "firezone_id=fz-abc123",
		},
		{
			name:      "both",
			opts:      &firezone.ClientListOptions{Name: "jane-laptop", FirezoneID: "fz-abc123"},
			wantQuery: "firezone_id=fz-abc123&name=jane-laptop",
		},
		{
			// nil opts must not send empty filter params, which the API
			// would treat as "match the empty string".
			name:      "no filters",
			opts:      nil,
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
					"metadata": map[string]any{"count": 0, "limit": 50},
				})(w, r)
			}))

			if _, err := client.ClientDevices.List(context.Background(), tt.opts); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}
