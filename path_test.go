package firezone_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

// capturePath records the raw request target the client sent. It reads
// r.RequestURI rather than r.URL.Path because net/http decodes the
// latter - and the whole point of these tests is what stayed encoded.
func capturePath(t *testing.T, response any, call func(c *firezone.Client) error) string {
	t.Helper()

	var target string
	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target = r.RequestURI
		testutil.JSONResponse(http.StatusOK, map[string]any{"data": response})(w, r)
	}))

	if err := call(client); err != nil {
		t.Fatalf("request returned error: %v", err)
	}
	return target
}

func TestRequestPath_EscapesIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "a plain ID passes through", id: "site-1", want: "/sites/site-1"},
		{
			// Unescaped, this would address GET /actors/evil - a
			// different resource entirely.
			name: "traversal stays inside one segment",
			id:   "../actors/evil",
			want: "/sites/..%2Factors%2Fevil",
		},
		{name: "a slash is escaped", id: "a/b", want: "/sites/a%2Fb"},
		{name: "a query separator is escaped", id: "a?limit=100", want: "/sites/a%3Flimit=100"},
		{name: "a fragment separator is escaped", id: "a#f", want: "/sites/a%23f"},
		{name: "a space is escaped", id: "a b", want: "/sites/a%20b"},
		{name: "a non-ASCII ID is escaped", id: "sité", want: "/sites/sit%C3%A9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capturePath(t, map[string]any{"id": tt.id}, func(c *firezone.Client) error {
				_, err := c.Sites.Get(context.Background(), tt.id)
				return err
			})
			if got != tt.want {
				t.Errorf("request target = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequestPath_NestedAndNormalPathsAreUnchanged(t *testing.T) {
	// List endpoints decode into a slice, single-resource endpoints into
	// an object, so each case supplies the envelope payload its call
	// expects.
	tests := []struct {
		name     string
		response any
		call     func(c *firezone.Client) error
		want     string
	}{
		{
			name:     "nested gateway",
			response: map[string]any{"id": "gw-1"},
			call: func(c *firezone.Client) error {
				_, err := c.Sites.Gateways("site-1").Get(context.Background(), "gw-1")
				return err
			},
			want: "/sites/site-1/gateways/gw-1",
		},
		{
			name:     "gateway token rotation",
			response: map[string]any{"id": "tok-1", "token": "secret"},
			call: func(c *firezone.Client) error {
				_, err := c.Sites.Gateways("site-1").RotateToken(context.Background(), "gw-1")
				return err
			},
			want: "/sites/site-1/gateways/gw-1/token/rotate",
		},
		{
			name:     "group memberships",
			response: []any{},
			call: func(c *firezone.Client) error {
				_, err := c.Groups.Memberships("group-1").List(context.Background(), nil)
				return err
			},
			want: "/groups/group-1/memberships",
		},
		{
			name:     "resource pool members",
			response: []any{},
			call: func(c *firezone.Client) error {
				_, err := c.Resources.PoolMembers("res-1").List(context.Background(), nil)
				return err
			},
			want: "/resources/res-1/pool_members",
		},
		{
			name:     "client verify",
			response: map[string]any{"id": "client-1"},
			call: func(c *firezone.Client) error {
				_, err := c.ClientDevices.Verify(context.Background(), "client-1")
				return err
			},
			want: "/clients/client-1/verify",
		},
		{
			name:     "a query is still appended",
			response: []any{},
			call: func(c *firezone.Client) error {
				_, err := c.Sites.List(context.Background(), &firezone.SiteListOptions{
					ListOptions: firezone.ListOptions{Limit: 10},
				})
				return err
			},
			want: "/sites?limit=10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capturePath(t, tt.response, tt.call)
			if got != tt.want {
				t.Errorf("request target = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEmptyID_MakesNoRequest covers the failure an empty ID used to
// cause: "sites/" + "" collapses to the collection path, so Get("")
// became a list and Delete("") aimed at the collection endpoint.
func TestEmptyID_MakesNoRequest(t *testing.T) {
	tests := []struct {
		name string
		call func(c *firezone.Client) error
	}{
		{"Sites.Get", func(c *firezone.Client) error { _, err := c.Sites.Get(context.Background(), ""); return err }},
		{"Sites.Delete", func(c *firezone.Client) error { return c.Sites.Delete(context.Background(), "") }},
		{"Resources.Get", func(c *firezone.Client) error {
			_, err := c.Resources.Get(context.Background(), "")
			return err
		}},
		{"Policies.Delete", func(c *firezone.Client) error { return c.Policies.Delete(context.Background(), "") }},
		{"Actors.Disable", func(c *firezone.Client) error { _, err := c.Actors.Disable(context.Background(), ""); return err }},
		{"ClientDevices.Verify", func(c *firezone.Client) error {
			_, err := c.ClientDevices.Verify(context.Background(), "")
			return err
		}},
		{"Groups.Get", func(c *firezone.Client) error { _, err := c.Groups.Get(context.Background(), ""); return err }},
		{"OktaDirectories.Get", func(c *firezone.Client) error {
			_, err := c.OktaDirectories.Get(context.Background(), "")
			return err
		}},
		{"Gateways.Get with an empty gateway ID", func(c *firezone.Client) error {
			_, err := c.Sites.Gateways("site-1").Get(context.Background(), "")
			return err
		}},
		{"Gateways.Get with an empty site ID", func(c *firezone.Client) error {
			_, err := c.Sites.Gateways("").Get(context.Background(), "gw-1")
			return err
		}},
		{"Gateways.List with an empty site ID", func(c *firezone.Client) error {
			_, err := c.Sites.Gateways("").List(context.Background(), nil)
			return err
		}},
		{"Memberships.List with an empty group ID", func(c *firezone.Client) error {
			_, err := c.Groups.Memberships("").List(context.Background(), nil)
			return err
		}},
		{"Memberships.ReplaceAll with an empty group ID", func(c *firezone.Client) error {
			_, err := c.Groups.Memberships("").ReplaceAll(context.Background(), []string{"actor-1"})
			return err
		}},
		{"PoolMembers.List with an empty resource ID", func(c *firezone.Client) error {
			_, err := c.Resources.PoolMembers("").List(context.Background(), nil)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				testutil.JSONResponse(http.StatusOK, map[string]any{"data": map[string]any{}})(w, r)
			}))

			err := tt.call(client)
			if !errors.Is(err, firezone.ErrMissingID) {
				t.Errorf("error = %v, want one matching ErrMissingID", err)
			}
			if called {
				t.Error("a request was sent; an empty ID must fail before reaching the network")
			}
		})
	}
}

func TestDotID_MakesNoRequest(t *testing.T) {
	// url.PathEscape leaves "." and ".." alone, so escaping alone can't
	// contain them - a normalizing proxy would resolve them into a
	// different path. They're rejected outright instead.
	for _, id := range []string{".", ".."} {
		t.Run(id, func(t *testing.T) {
			var called bool
			client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				testutil.JSONResponse(http.StatusOK, map[string]any{"data": map[string]any{}})(w, r)
			}))

			_, err := client.Sites.Get(context.Background(), id)
			if err == nil {
				t.Fatal("Get returned no error, want a rejection")
			}
			if !strings.Contains(err.Error(), "not a valid path segment") {
				t.Errorf("error = %v, want it to name the invalid segment", err)
			}
			if called {
				t.Error("a request was sent; a dot segment must fail before reaching the network")
			}
		})
	}
}

// TestBaseURLPathPrefixPreserved guards the note in the package doc:
// the API is unversioned today, but if a path prefix ever comes back,
// resolvePath is the one place that has to keep working.
func TestBaseURLPathPrefixPreserved(t *testing.T) {
	var target string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target = r.RequestURI
		testutil.JSONResponse(http.StatusOK, map[string]any{"data": map[string]any{"id": "site-1"}})(w, r)
	}))
	defer server.Close()

	for _, base := range []string{server.URL + "/api/v1", server.URL + "/api/v1/"} {
		client, err := firezone.NewClient(base, "test-token", firezone.WithRetry(false, 0))
		if err != nil {
			t.Fatalf("NewClient(%q) returned error: %v", base, err)
		}
		if _, err := client.Sites.Get(context.Background(), "site-1"); err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if want := "/api/v1/sites/site-1"; target != want {
			t.Errorf("base %q: request target = %q, want %q", base, target, want)
		}
	}
}

func TestNewClient_RejectsUnusableBaseURLs(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{
			// url.Parse reads this as a relative path with no scheme and
			// no host, and returns no error.
			name: "no scheme", baseURL: "api.firezone.dev",
			wantErr: "scheme must be http or https",
		},
		{name: "empty", baseURL: "", wantErr: "scheme must be http or https"},
		{name: "unsupported scheme", baseURL: "ftp://api.firezone.dev", wantErr: "scheme must be http or https"},
		{name: "scheme but no host", baseURL: "https://", wantErr: "missing host"},
		{
			name: "query string", baseURL: "https://api.firezone.dev?limit=10",
			wantErr: "must not include a query string",
		},
		{name: "fragment", baseURL: "https://api.firezone.dev#x", wantErr: "must not include a fragment"},
		{name: "unparseable", baseURL: "https://api.firezone.dev/%zz", wantErr: "invalid base URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := firezone.NewClient(tt.baseURL, "token")
			if err == nil {
				t.Fatalf("NewClient(%q) returned no error, want one", tt.baseURL)
			}
			if client != nil {
				t.Error("NewClient returned a non-nil client alongside an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewClient_AcceptsValidBaseURLs(t *testing.T) {
	for _, base := range []string{
		"https://api.firezone.dev",
		"https://api.firezone.dev/",
		"http://localhost:13001",
		"https://api.firezone.dev/api/v1",
	} {
		if _, err := firezone.NewClient(base, "token"); err != nil {
			t.Errorf("NewClient(%q) returned error: %v", base, err)
		}
	}
}
