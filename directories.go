package firezone

import (
	"context"
	"net/url"
	"time"
)

// EntraDirectory is a Microsoft Entra directory connection. Directories
// are read-only via this API - they're managed through the Firezone
// dashboard's identity provider setup, not created or updated here.
type EntraDirectory struct {
	ID             string     `json:"id"`
	AccountID      string     `json:"account_id"`
	Name           string     `json:"name"`
	TenantID       string     `json:"tenant_id"`
	IsDisabled     bool       `json:"is_disabled"`
	DisabledReason string     `json:"disabled_reason,omitempty"`
	SyncedAt       *time.Time `json:"synced_at,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	ErroredAt      *time.Time `json:"errored_at,omitempty"`
	EmailField     string     `json:"email_field"`
	SyncAllGroups  bool       `json:"sync_all_groups"`
	InsertedAt     time.Time  `json:"inserted_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// GoogleDirectory is a Google Workspace directory connection. Directories
// are read-only via this API - they're managed through the Firezone
// dashboard's identity provider setup, not created or updated here.
type GoogleDirectory struct {
	ID                 string     `json:"id"`
	AccountID          string     `json:"account_id"`
	Name               string     `json:"name"`
	Domain             string     `json:"domain"`
	ImpersonationEmail string     `json:"impersonation_email"`
	IsDisabled         bool       `json:"is_disabled"`
	DisabledReason     string     `json:"disabled_reason,omitempty"`
	SyncedAt           *time.Time `json:"synced_at,omitempty"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	ErroredAt          *time.Time `json:"errored_at,omitempty"`
	GroupSyncMode      string     `json:"group_sync_mode"`
	OrgUnitSyncEnabled bool       `json:"orgunit_sync_enabled"`
	InsertedAt         time.Time  `json:"inserted_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// OktaDirectory is an Okta directory connection. Directories are
// read-only via this API - they're managed through the Firezone
// dashboard's identity provider setup, not created or updated here.
type OktaDirectory struct {
	ID             string     `json:"id"`
	AccountID      string     `json:"account_id"`
	Name           string     `json:"name"`
	ClientID       string     `json:"client_id"`
	Kid            string     `json:"kid"`
	OktaDomain     string     `json:"okta_domain"`
	IsDisabled     bool       `json:"is_disabled"`
	DisabledReason string     `json:"disabled_reason,omitempty"`
	SyncedAt       *time.Time `json:"synced_at,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	ErroredAt      *time.Time `json:"errored_at,omitempty"`
	InsertedAt     time.Time  `json:"inserted_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// EntraDirectoriesService reads Entra directory connections. Read-only:
// there is no Create, Update, or Delete - see [EntraDirectory].
type EntraDirectoriesService struct {
	client *Client
}

// Get fetches a single Entra directory by ID.
func (s *EntraDirectoriesService) Get(ctx context.Context, id string) (*EntraDirectory, error) {
	if err := checkID("directory ID", id); err != nil {
		return nil, err
	}
	var d EntraDirectory
	if err := s.client.do(ctx, "GET", buildPath("entra_directories", id), nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// DirectoryListOptions extends ListOptions with the filters every
// directory list endpoint accepts. Shared across the three providers,
// which expose the same filter surface.
type DirectoryListOptions struct {
	ListOptions
	// Name filters to directories with this exact name, as shown in the
	// dashboard's identity provider settings.
	Name string
}

// List returns a page of Entra directories. Pass nil for opts to use
// the API's default page size and no filters.
func (s *EntraDirectoriesService) List(ctx context.Context, opts *DirectoryListOptions) (*Page[EntraDirectory], error) {
	return doList[EntraDirectory](ctx, s.client, "GET", "entra_directories", directoryQuery(opts))
}

// GoogleDirectoriesService reads Google Workspace directory connections.
// Read-only: there is no Create, Update, or Delete - see [GoogleDirectory].
type GoogleDirectoriesService struct {
	client *Client
}

// Get fetches a single Google Workspace directory by ID.
func (s *GoogleDirectoriesService) Get(ctx context.Context, id string) (*GoogleDirectory, error) {
	if err := checkID("directory ID", id); err != nil {
		return nil, err
	}
	var d GoogleDirectory
	if err := s.client.do(ctx, "GET", buildPath("google_directories", id), nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns a page of Google Workspace directories. Pass nil for
// opts to use the API's default page size.
func (s *GoogleDirectoriesService) List(ctx context.Context, opts *DirectoryListOptions) (*Page[GoogleDirectory], error) {
	return doList[GoogleDirectory](ctx, s.client, "GET", "google_directories", directoryQuery(opts))
}

// OktaDirectoriesService reads Okta directory connections. Read-only:
// there is no Create, Update, or Delete - see [OktaDirectory].
type OktaDirectoriesService struct {
	client *Client
}

// Get fetches a single Okta directory by ID.
func (s *OktaDirectoriesService) Get(ctx context.Context, id string) (*OktaDirectory, error) {
	if err := checkID("directory ID", id); err != nil {
		return nil, err
	}
	var d OktaDirectory
	if err := s.client.do(ctx, "GET", buildPath("okta_directories", id), nil, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns a page of Okta directories. Pass nil for opts to use the
// API's default page size.
func (s *OktaDirectoriesService) List(ctx context.Context, opts *DirectoryListOptions) (*Page[OktaDirectory], error) {
	return doList[OktaDirectory](ctx, s.client, "GET", "okta_directories", directoryQuery(opts))
}

// directoryQuery builds the query string for a directory list request,
// tolerating a nil opts the way every List method's contract promises.
func directoryQuery(opts *DirectoryListOptions) url.Values {
	if opts == nil {
		opts = &DirectoryListOptions{}
	}
	return filterQuery(opts.ListOptions, [2]string{"name", opts.Name})
}
