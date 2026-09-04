package firezone_test

import (
	"context"
	"net/http"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

// TestAuthProvidersService_List_NameFilter checks all five services
// push the name filter to the API rather than scanning, and hit the
// right endpoint. The five are near-identical, so a copy-paste slip in
// one path would otherwise go unnoticed.
func TestAuthProvidersService_List_NameFilter(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*firezone.Client, *firezone.AuthProviderListOptions) error
		wantPath string
	}{
		{
			name: "email_otp",
			call: func(c *firezone.Client, o *firezone.AuthProviderListOptions) error {
				_, err := c.EmailOTPAuthProviders.List(context.Background(), o)
				return err
			},
			wantPath: "/email_otp_auth_providers",
		},
		{
			name: "oidc",
			call: func(c *firezone.Client, o *firezone.AuthProviderListOptions) error {
				_, err := c.OIDCAuthProviders.List(context.Background(), o)
				return err
			},
			wantPath: "/oidc_auth_providers",
		},
		{
			name: "google",
			call: func(c *firezone.Client, o *firezone.AuthProviderListOptions) error {
				_, err := c.GoogleAuthProviders.List(context.Background(), o)
				return err
			},
			wantPath: "/google_auth_providers",
		},
		{
			name: "entra",
			call: func(c *firezone.Client, o *firezone.AuthProviderListOptions) error {
				_, err := c.EntraAuthProviders.List(context.Background(), o)
				return err
			},
			wantPath: "/entra_auth_providers",
		},
		{
			name: "okta",
			call: func(c *firezone.Client, o *firezone.AuthProviderListOptions) error {
				_, err := c.OktaAuthProviders.List(context.Background(), o)
				return err
			},
			wantPath: "/okta_auth_providers",
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

			if err := tt.call(client, &firezone.AuthProviderListOptions{Name: "Corp SSO"}); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotQuery != "name=Corp+SSO" {
				t.Errorf("query = %q, want name=Corp+SSO", gotQuery)
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

// TestAuthProvidersService_Get_TypeSpecificFields covers the fields
// that differ per provider type, since the shared AuthProvider embed
// makes it easy to forget the ones that don't come from it.
func TestAuthProvidersService_Get_TypeSpecificFields(t *testing.T) {
	t.Run("okta", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{
				"id": "ap-1", "name": "Corp SSO", "issuer": "https://corp.okta.com",
				"is_disabled": false, "is_default": true,
				"client_id": "0oa1234", "okta_domain": "corp.okta.com",
				"inserted_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
			},
		}))

		p, err := client.OktaAuthProviders.Get(context.Background(), "ap-1")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		// From the embedded AuthProvider...
		if p.ID != "ap-1" || p.Name != "Corp SSO" {
			t.Errorf("provider = %+v, want ID=ap-1 Name=\"Corp SSO\"", p)
		}
		// ...and from the Okta-specific fields.
		if !p.IsDefault || p.ClientID != "0oa1234" || p.OktaDomain != "corp.okta.com" {
			t.Errorf("provider = %+v, want IsDefault=true ClientID=0oa1234 OktaDomain=corp.okta.com", p)
		}
	})

	t.Run("oidc", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{
				"id": "ap-2", "name": "Generic",
				"client_id":                 "abc",
				"discovery_document_uri":    "https://idp.example.com/.well-known/openid-configuration",
				"email_verification_method": "id_token",
				"inserted_at":               "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
			},
		}))

		p, err := client.OIDCAuthProviders.Get(context.Background(), "ap-2")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if p.DiscoveryDocumentURI == "" || p.EmailVerificationMethod != "id_token" {
			t.Errorf("provider = %+v, want discovery URI and EmailVerificationMethod=id_token", p)
		}
	})

	t.Run("entra", func(t *testing.T) {
		client := testutil.NewClient(t, testutil.JSONResponse(http.StatusOK, map[string]any{
			"data": map[string]any{
				"id": "ap-3", "name": "Entra", "email_claim": "upn",
				"inserted_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
			},
		}))

		p, err := client.EntraAuthProviders.Get(context.Background(), "ap-3")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if p.EmailClaim != "upn" {
			t.Errorf("EmailClaim = %q, want upn", p.EmailClaim)
		}
	})
}

func TestAuthProvidersService_Get_NotFound(t *testing.T) {
	client := testutil.NewClient(t, testutil.ProblemResponse(http.StatusNotFound, "not found"))

	_, err := client.GoogleAuthProviders.Get(context.Background(), "missing")
	if !firezone.IsNotFound(err) {
		t.Fatalf("IsNotFound(err) = false, want true (err: %v)", err)
	}
}
