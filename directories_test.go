package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func TestEntraDirectoriesService_Get(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		var gotPath string
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "dir-1", "name": "Entra", "tenant_id": "tenant-1"},
			})(w, r)
		}))

		dir, err := client.EntraDirectories.Get(context.Background(), "dir-1")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if gotPath != "/entra_directories/dir-1" {
			t.Errorf("path = %q, want /entra_directories/dir-1", gotPath)
		}
		if dir.ID != "dir-1" || dir.Name != "Entra" || dir.TenantID != "tenant-1" {
			t.Errorf("dir = %+v, want {ID: dir-1, Name: Entra, TenantID: tenant-1}", dir)
		}
	})

	t.Run("not found", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "The requested resource could not be found."))

		_, err := client.EntraDirectories.Get(context.Background(), "missing")
		if !firezone.IsNotFound(err) {
			t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
		}
	})
}

func TestEntraDirectoriesService_List(t *testing.T) {
	var gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data":     []map[string]any{{"id": "dir-1", "name": "Entra"}},
			"metadata": map[string]any{"count": 1, "limit": 50, "next_page": "", "prev_page": ""},
		})(w, r)
	}))

	page, err := client.EntraDirectories.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if gotPath != "/entra_directories" {
		t.Errorf("path = %q, want /entra_directories", gotPath)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "dir-1" {
		t.Errorf("page.Data = %+v, want one directory with ID dir-1", page.Data)
	}
}

func TestGoogleDirectoriesService_Get(t *testing.T) {
	var gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "dir-2", "name": "Google", "domain": "example.com"},
		})(w, r)
	}))

	dir, err := client.GoogleDirectories.Get(context.Background(), "dir-2")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if gotPath != "/google_directories/dir-2" {
		t.Errorf("path = %q, want /google_directories/dir-2", gotPath)
	}
	if dir.ID != "dir-2" || dir.Name != "Google" || dir.Domain != "example.com" {
		t.Errorf("dir = %+v, want {ID: dir-2, Name: Google, Domain: example.com}", dir)
	}
}

func TestOktaDirectoriesService_Get(t *testing.T) {
	var gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "dir-3", "name": "Okta", "okta_domain": "example.okta.com"},
		})(w, r)
	}))

	dir, err := client.OktaDirectories.Get(context.Background(), "dir-3")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if gotPath != "/okta_directories/dir-3" {
		t.Errorf("path = %q, want /okta_directories/dir-3", gotPath)
	}
	if dir.ID != "dir-3" || dir.Name != "Okta" || dir.OktaDomain != "example.okta.com" {
		t.Errorf("dir = %+v, want {ID: dir-3, Name: Okta, OktaDomain: example.okta.com}", dir)
	}
}

// TestDirectoriesService_List_NameFilter checks all three directory
// services push the name filter to the API rather than scanning. These
// endpoints gained a ?name= filter precisely so a data source read costs
// one request instead of one per page of the account's directories.
func TestDirectoriesService_List_NameFilter(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*firezone.Client, *firezone.DirectoryListOptions) error
		wantPath string
	}{
		{
			name: "entra",
			call: func(c *firezone.Client, o *firezone.DirectoryListOptions) error {
				_, err := c.EntraDirectories.List(context.Background(), o)
				return err
			},
			wantPath: "/entra_directories",
		},
		{
			name: "google",
			call: func(c *firezone.Client, o *firezone.DirectoryListOptions) error {
				_, err := c.GoogleDirectories.List(context.Background(), o)
				return err
			},
			wantPath: "/google_directories",
		},
		{
			name: "okta",
			call: func(c *firezone.Client, o *firezone.DirectoryListOptions) error {
				_, err := c.OktaDirectories.List(context.Background(), o)
				return err
			},
			wantPath: "/okta_directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath, gotQuery string
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				testutil.JSONResponse(http.StatusOK, map[string]any{
					"data":     []map[string]any{},
					"metadata": map[string]any{"count": 0, "limit": 50},
				})(w, r)
			}))

			opts := &firezone.DirectoryListOptions{Name: "Corp Directory"}
			if err := tt.call(client, opts); err != nil {
				t.Fatalf("List returned error: %v", err)
			}

			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotQuery != "name=Corp+Directory" {
				t.Errorf("query = %q, want name=Corp+Directory", gotQuery)
			}
		})

		t.Run(tt.name+" nil opts", func(t *testing.T) {
			var gotQuery string
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				testutil.JSONResponse(http.StatusOK, map[string]any{
					"data":     []map[string]any{},
					"metadata": map[string]any{"count": 0, "limit": 50},
				})(w, r)
			}))

			// nil must stay valid and must not send an empty name, which
			// the API would read as "match the empty string".
			if err := tt.call(client, nil); err != nil {
				t.Fatalf("List(nil) returned error: %v", err)
			}
			if gotQuery != "" {
				t.Errorf("query = %q, want empty", gotQuery)
			}
		})
	}
}
