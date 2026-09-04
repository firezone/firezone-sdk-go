package firezone

import (
	"context"
	"net/url"
	"time"
)

// AuthProvider is the state every authentication provider type shares.
// Each concrete type embeds it and adds its own fields.
//
// Auth providers are read-only through this API: they are configured in
// the Firezone dashboard, which owns the OAuth/OIDC secrets involved.
// The Policy condition property "auth_provider_id" takes these IDs.
type AuthProvider struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Issuer    string `json:"issuer"`
	Context   string `json:"context"`

	// ClientSessionLifetimeSecs and PortalSessionLifetimeSecs are how
	// long a sign-in through this provider stays valid on a Client
	// device and in the admin portal respectively.
	//
	// Both are nil when the provider sets no override and Firezone's own
	// defaults apply, which is the usual case - the columns have no
	// database default, so a provider that has never had them configured
	// stores null. They are pointers for that reason: a plain int would
	// decode null to 0, which reads as "sessions expire immediately"
	// rather than "not configured".
	//
	// The spec marks these nullable only on the Entra provider, but the
	// underlying schema is identical for all five, so treat every one of
	// them as nullable.
	ClientSessionLifetimeSecs *int `json:"client_session_lifetime_secs,omitempty"`
	PortalSessionLifetimeSecs *int `json:"portal_session_lifetime_secs,omitempty"`

	IsDisabled bool `json:"is_disabled"`

	InsertedAt time.Time `json:"inserted_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EmailOTPAuthProvider signs users in with a one-time passcode emailed
// to them. Unlike the other types it has no is_default flag - it is a
// fallback rather than something a sign-in page defaults to.
type EmailOTPAuthProvider struct {
	AuthProvider
}

// OIDCAuthProvider is a generic OpenID Connect provider.
type OIDCAuthProvider struct {
	AuthProvider
	IsDefault               bool   `json:"is_default"`
	ClientID                string `json:"client_id"`
	DiscoveryDocumentURI    string `json:"discovery_document_uri"`
	EmailVerificationMethod string `json:"email_verification_method"`
}

// GoogleAuthProvider is a Google Workspace sign-in provider.
type GoogleAuthProvider struct {
	AuthProvider
	IsDefault bool `json:"is_default"`
}

// EntraAuthProvider is a Microsoft Entra sign-in provider.
type EntraAuthProvider struct {
	AuthProvider
	IsDefault  bool   `json:"is_default"`
	EmailClaim string `json:"email_claim"`
}

// OktaAuthProvider is an Okta sign-in provider.
type OktaAuthProvider struct {
	AuthProvider
	IsDefault  bool   `json:"is_default"`
	ClientID   string `json:"client_id"`
	OktaDomain string `json:"okta_domain"`
}

// AuthProviderListOptions extends ListOptions with the filters every
// auth provider list endpoint accepts. Shared across the five types,
// which expose the same filter surface.
type AuthProviderListOptions struct {
	ListOptions
	// Name filters to providers with this exact name, as shown in the
	// dashboard's authentication settings.
	Name string
}

// EmailOTPAuthProvidersService reads Email OTP auth providers.
// Read-only - see [AuthProvider].
type EmailOTPAuthProvidersService struct {
	client *Client
}

// Get fetches a single Email OTP auth provider by ID.
func (s *EmailOTPAuthProvidersService) Get(ctx context.Context, id string) (*EmailOTPAuthProvider, error) {
	if err := checkID("auth provider ID", id); err != nil {
		return nil, err
	}
	var p EmailOTPAuthProvider
	if err := s.client.do(ctx, "GET", buildPath("email_otp_auth_providers", id), nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns a page of Email OTP auth providers. Pass nil for opts to
// use the API's default page size and no filters.
func (s *EmailOTPAuthProvidersService) List(ctx context.Context, opts *AuthProviderListOptions) (*Page[EmailOTPAuthProvider], error) {
	return doList[EmailOTPAuthProvider](ctx, s.client, "GET", "email_otp_auth_providers", authProviderQuery(opts))
}

// OIDCAuthProvidersService reads generic OIDC auth providers.
// Read-only - see [AuthProvider].
type OIDCAuthProvidersService struct {
	client *Client
}

// Get fetches a single OIDC auth provider by ID.
func (s *OIDCAuthProvidersService) Get(ctx context.Context, id string) (*OIDCAuthProvider, error) {
	if err := checkID("auth provider ID", id); err != nil {
		return nil, err
	}
	var p OIDCAuthProvider
	if err := s.client.do(ctx, "GET", buildPath("oidc_auth_providers", id), nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns a page of OIDC auth providers. Pass nil for opts to use
// the API's default page size and no filters.
func (s *OIDCAuthProvidersService) List(ctx context.Context, opts *AuthProviderListOptions) (*Page[OIDCAuthProvider], error) {
	return doList[OIDCAuthProvider](ctx, s.client, "GET", "oidc_auth_providers", authProviderQuery(opts))
}

// GoogleAuthProvidersService reads Google Workspace auth providers.
// Read-only - see [AuthProvider].
type GoogleAuthProvidersService struct {
	client *Client
}

// Get fetches a single Google auth provider by ID.
func (s *GoogleAuthProvidersService) Get(ctx context.Context, id string) (*GoogleAuthProvider, error) {
	if err := checkID("auth provider ID", id); err != nil {
		return nil, err
	}
	var p GoogleAuthProvider
	if err := s.client.do(ctx, "GET", buildPath("google_auth_providers", id), nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns a page of Google auth providers. Pass nil for opts to use
// the API's default page size and no filters.
func (s *GoogleAuthProvidersService) List(ctx context.Context, opts *AuthProviderListOptions) (*Page[GoogleAuthProvider], error) {
	return doList[GoogleAuthProvider](ctx, s.client, "GET", "google_auth_providers", authProviderQuery(opts))
}

// EntraAuthProvidersService reads Microsoft Entra auth providers.
// Read-only - see [AuthProvider].
type EntraAuthProvidersService struct {
	client *Client
}

// Get fetches a single Entra auth provider by ID.
func (s *EntraAuthProvidersService) Get(ctx context.Context, id string) (*EntraAuthProvider, error) {
	if err := checkID("auth provider ID", id); err != nil {
		return nil, err
	}
	var p EntraAuthProvider
	if err := s.client.do(ctx, "GET", buildPath("entra_auth_providers", id), nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns a page of Entra auth providers. Pass nil for opts to use
// the API's default page size and no filters.
func (s *EntraAuthProvidersService) List(ctx context.Context, opts *AuthProviderListOptions) (*Page[EntraAuthProvider], error) {
	return doList[EntraAuthProvider](ctx, s.client, "GET", "entra_auth_providers", authProviderQuery(opts))
}

// OktaAuthProvidersService reads Okta auth providers.
// Read-only - see [AuthProvider].
type OktaAuthProvidersService struct {
	client *Client
}

// Get fetches a single Okta auth provider by ID.
func (s *OktaAuthProvidersService) Get(ctx context.Context, id string) (*OktaAuthProvider, error) {
	if err := checkID("auth provider ID", id); err != nil {
		return nil, err
	}
	var p OktaAuthProvider
	if err := s.client.do(ctx, "GET", buildPath("okta_auth_providers", id), nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns a page of Okta auth providers. Pass nil for opts to use
// the API's default page size and no filters.
func (s *OktaAuthProvidersService) List(ctx context.Context, opts *AuthProviderListOptions) (*Page[OktaAuthProvider], error) {
	return doList[OktaAuthProvider](ctx, s.client, "GET", "okta_auth_providers", authProviderQuery(opts))
}

// authProviderQuery builds the query string for an auth provider list
// request, tolerating a nil opts the way every List method's contract
// promises.
func authProviderQuery(opts *AuthProviderListOptions) url.Values {
	if opts == nil {
		opts = &AuthProviderListOptions{}
	}
	return filterQuery(opts.ListOptions, [2]string{"name", opts.Name})
}
