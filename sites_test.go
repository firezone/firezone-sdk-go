package firezone_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

func decodeJSONBody(t *testing.T, r *http.Request, out any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decoding request body: %v", err)
	}
}

func jsonEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

func TestSitesService_Get(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		var gotPath, gotMethod string
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotMethod = r.Method
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "site-1", "name": "primary-dc"},
			})(w, r)
		}))

		site, err := client.Sites.Get(context.Background(), "site-1")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}

		if gotMethod != http.MethodGet {
			t.Errorf("method = %q, want GET", gotMethod)
		}
		if gotPath != "/sites/site-1" {
			t.Errorf("path = %q, want /sites/site-1", gotPath)
		}
		if site.ID != "site-1" || site.Name != "primary-dc" {
			t.Errorf("site = %+v, want {ID: site-1, Name: primary-dc}", site)
		}
	})

	t.Run("not found", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "The requested resource could not be found."))

		_, err := client.Sites.Get(context.Background(), "missing")
		if !firezone.IsNotFound(err) {
			t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusUnauthorized, "Authentication credentials were missing or invalid."))

		_, err := client.Sites.Get(context.Background(), "site-1")
		if !firezone.IsUnauthorized(err) {
			t.Fatalf("IsUnauthorized(err) = false, want true (err: %v)", err)
		}
	})
}

func TestSitesService_List(t *testing.T) {
	t.Run("happy path with pagination metadata", func(t *testing.T) {
		var gotQuery string
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data": []map[string]any{
					{"id": "site-1", "name": "primary-dc"},
					{"id": "site-2", "name": "secondary-dc"},
				},
				"metadata": map[string]any{
					"count": 2, "limit": 50, "next_page": "", "prev_page": "",
				},
			})(w, r)
		}))

		page, err := client.Sites.List(context.Background(), &firezone.SiteListOptions{
			ListOptions: firezone.ListOptions{Limit: 50},
		})
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}

		if gotQuery != "limit=50" {
			t.Errorf("query = %q, want limit=50", gotQuery)
		}
		if len(page.Data) != 2 {
			t.Fatalf("len(page.Data) = %d, want 2", len(page.Data))
		}
		if page.Metadata.Count != 2 {
			t.Errorf("page.Metadata.Count = %d, want 2", page.Metadata.Count)
		}
	})

	t.Run("filters by name", func(t *testing.T) {
		var gotQuery string
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			testutil.JSONResponse(http.StatusOK, map[string]any{
				"data":     []map[string]any{{"id": "site-1", "name": "primary-dc"}},
				"metadata": map[string]any{"count": 1, "limit": 50, "next_page": "", "prev_page": ""},
			})(w, r)
		}))

		_, err := client.Sites.List(context.Background(), &firezone.SiteListOptions{Name: "primary-dc"})
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if gotQuery != "name=primary-dc" {
			t.Errorf("query = %q, want name=primary-dc", gotQuery)
		}
	})

	t.Run("invalid limit is a bad request", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusBadRequest, "limit must be an integer"))

		_, err := client.Sites.List(context.Background(), nil)
		var apiErr *firezone.APIError
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("err = %v, want *APIError with StatusCode 400", err)
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.RateLimitedResponse(2))

		_, err := client.Sites.List(context.Background(), nil)
		if !firezone.IsRateLimited(err) {
			t.Fatalf("IsRateLimited(err) = false, want true (err: %v)", err)
		}
		var apiErr *firezone.APIError
		errors.As(err, &apiErr)
		if apiErr.RetryAfter.Seconds() != 2 {
			t.Errorf("RetryAfter = %v, want 2s", apiErr.RetryAfter)
		}
	})
}

func TestSitesService_Create(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			decodeJSONBody(t, r, &gotBody)
			testutil.JSONResponse(http.StatusCreated, map[string]any{
				"data": map[string]any{"id": "site-1", "name": "primary-dc"},
			})(w, r)
		}))

		site, err := client.Sites.Create(context.Background(), &firezone.CreateSiteRequest{Name: "primary-dc"})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		if gotMethod != http.MethodPost || gotPath != "/sites" {
			t.Errorf("request = %s %s, want POST /sites", gotMethod, gotPath)
		}
		wantBody := map[string]any{"site": map[string]any{"name": "primary-dc"}}
		if !jsonEqual(gotBody, wantBody) {
			t.Errorf("body = %v, want %v", gotBody, wantBody)
		}
		if site.Name != "primary-dc" {
			t.Errorf("site.Name = %q, want primary-dc", site.Name)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ValidationErrorResponse(map[string][]string{
			"name": {"can't be blank"},
		}))

		_, err := client.Sites.Create(context.Background(), &firezone.CreateSiteRequest{})
		if !firezone.IsValidation(err) {
			t.Fatalf("IsValidation(err) = false, want true (err: %v)", err)
		}
		var apiErr *firezone.APIError
		errors.As(err, &apiErr)
		if len(apiErr.ValidationErrors["name"]) == 0 {
			t.Errorf("ValidationErrors[name] is empty, want at least one message")
		}
	})

	t.Run("billing limit reached is forbidden", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusForbidden, "Sites limit reached"))

		_, err := client.Sites.Create(context.Background(), &firezone.CreateSiteRequest{Name: "one-too-many"})
		if !firezone.IsForbidden(err) {
			t.Fatalf("IsForbidden(err) = false, want true (err: %v)", err)
		}
	})
}

func TestSitesService_Update(t *testing.T) {
	var gotMethod, gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "site-1", "name": "renamed"},
		})(w, r)
	}))

	site, err := client.Sites.Update(context.Background(), "site-1", &firezone.UpdateSiteRequest{Name: "renamed"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/sites/site-1" {
		t.Errorf("request = %s %s, want PATCH /sites/site-1", gotMethod, gotPath)
	}
	if site.Name != "renamed" {
		t.Errorf("site.Name = %q, want renamed", site.Name)
	}

	t.Run("conflict", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusConflict, "already exists"))
		_, err := client.Sites.Update(context.Background(), "site-1", &firezone.UpdateSiteRequest{Name: "dup"})
		if !firezone.IsConflict(err) {
			t.Fatalf("IsConflict(err) = false, want true (err: %v)", err)
		}
	})
}

func TestSitesService_Delete(t *testing.T) {
	var gotMethod, gotPath string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{"id": "site-1", "name": "primary-dc"},
		})(w, r)
	}))

	if err := client.Sites.Delete(context.Background(), "site-1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/sites/site-1" {
		t.Errorf("request = %s %s, want DELETE /sites/site-1", gotMethod, gotPath)
	}

	t.Run("not found", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "not found"))
		err := client.Sites.Delete(context.Background(), "missing")
		if !firezone.IsNotFound(err) {
			t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
		}
	})
}
