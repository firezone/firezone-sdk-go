package firezone

import (
	"context"
	"time"
)

// ActorType is the type of a Firezone Actor. api_client is
// intentionally not offered as a constant here: it cannot be created,
// updated, or otherwise managed via this API (the API returns 422 if
// you try), since api_client actors are how API tokens themselves are
// issued.
type ActorType string

// ActorType values.
const (
	ActorTypeAccountUser      ActorType = "account_user"
	ActorTypeAccountAdminUser ActorType = "account_admin_user"
	ActorTypeServiceAccount   ActorType = "service_account"
)

// Actor is a Firezone Actor - a user, admin, or service account.
type Actor struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Type                 ActorType  `json:"type"`
	Email                string     `json:"email,omitempty"`
	AllowEmailOTPSignIn  bool       `json:"allow_email_otp_sign_in"`
	IsDisabled           bool       `json:"is_disabled"`
	LastSeenAt           *time.Time `json:"last_seen_at,omitempty"`
	CreatedByDirectoryID string     `json:"created_by_directory_id,omitempty"`
	InsertedAt           time.Time  `json:"inserted_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// IsSynced reports whether the Actor was created by an identity
// provider directory sync, and so is owned by the IdP rather than by
// this API. It mirrors [Group.IsSynced].
//
// Unlike a synced Group, a synced Actor is not wholly read-only here -
// but its existence and identity are the directory's to decide, so
// tooling that adopts an existing account should treat it as
// discovered rather than managed.
func (a *Actor) IsSynced() bool {
	return a.CreatedByDirectoryID != ""
}

// CreateActorRequest is the request body for [ActorsService.Create].
type CreateActorRequest struct {
	Name                string    `json:"name"`
	Type                ActorType `json:"type"`
	Email               string    `json:"email,omitempty"`
	AllowEmailOTPSignIn *bool     `json:"allow_email_otp_sign_in,omitempty"`
}

// UpdateActorRequest is the request body for [ActorsService.Update].
// Every field is optional; omitted fields keep their current value.
//
// Changing Email to a different address signs the Actor out and
// unlinks their identity providers - see the API's actor update
// documentation for details.
//
// Setting IsDisabled to true immediately revokes the Actor's active
// Client tokens and portal sessions. The API returns 403 Forbidden if
// it names the Actor behind the calling token - an Actor cannot
// disable itself.
type UpdateActorRequest struct {
	Name string    `json:"name,omitempty"`
	Type ActorType `json:"type,omitempty"`
	// Email is nullable, so it is typed [Null] - Clear[string]() removes
	// the Actor's email, and a nil pointer leaves it alone.
	Email               *Null[string] `json:"email,omitempty"`
	AllowEmailOTPSignIn *bool         `json:"allow_email_otp_sign_in,omitempty"`
	IsDisabled          *bool         `json:"is_disabled,omitempty"`
}

// ActorsService manages Actors.
type ActorsService struct {
	client *Client
}

// Get fetches a single Actor by ID.
func (s *ActorsService) Get(ctx context.Context, id string) (*Actor, error) {
	if err := checkID("Actor ID", id); err != nil {
		return nil, err
	}
	var actor Actor
	if err := s.client.do(ctx, "GET", buildPath("actors", id), nil, nil, &actor); err != nil {
		return nil, err
	}
	return &actor, nil
}

// ActorListOptions extends ListOptions with Actors-specific filters.
type ActorListOptions struct {
	ListOptions
	// Name filters to Actors with this exact name.
	Name string
	// Email filters to Actors with this exact email.
	Email string
	// Type filters to Actors of this type. Unlike [CreateActorRequest]
	// and [UpdateActorRequest], "api_client" is a valid filter value
	// here - it just can't be created or updated via the API.
	Type ActorType
}

// List returns a page of Actors. Pass nil for opts to use the API's
// default page size and no filters.
func (s *ActorsService) List(ctx context.Context, opts *ActorListOptions) (*Page[Actor], error) {
	if opts == nil {
		opts = &ActorListOptions{}
	}
	q := filterQuery(opts.ListOptions,
		[2]string{"name", opts.Name},
		[2]string{"email", opts.Email},
		[2]string{"type", string(opts.Type)},
	)
	return doList[Actor](ctx, s.client, "GET", "actors", q)
}

// Create creates a new Actor.
func (s *ActorsService) Create(ctx context.Context, req *CreateActorRequest) (*Actor, error) {
	body, err := wrapBody("actor", req)
	if err != nil {
		return nil, err
	}
	var actor Actor
	if err := s.client.do(ctx, "POST", "actors", nil, body, &actor); err != nil {
		return nil, err
	}
	return &actor, nil
}

// Update updates an Actor.
func (s *ActorsService) Update(ctx context.Context, id string, req *UpdateActorRequest) (*Actor, error) {
	if err := checkID("Actor ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("actor", req)
	if err != nil {
		return nil, err
	}
	var actor Actor
	if err := s.client.do(ctx, "PATCH", buildPath("actors", id), nil, body, &actor); err != nil {
		return nil, err
	}
	return &actor, nil
}

// Delete deletes an Actor.
func (s *ActorsService) Delete(ctx context.Context, id string) error {
	if err := checkID("Actor ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", buildPath("actors", id), nil, nil, nil)
}

// Disable disables an Actor, immediately revoking all of its active
// Client tokens and portal sessions. Returns 403 Forbidden if id is the
// authenticated actor itself.
//
// This is a convenience wrapper over [ActorsService.Update]; the API
// has no dedicated disable endpoint.
func (s *ActorsService) Disable(ctx context.Context, id string) (*Actor, error) {
	if err := checkID("Actor ID", id); err != nil {
		return nil, err
	}
	disabled := true
	return s.Update(ctx, id, &UpdateActorRequest{IsDisabled: &disabled})
}

// Enable enables a disabled Actor. Idempotent - enabling an
// already-enabled Actor is a no-op.
//
// This is a convenience wrapper over [ActorsService.Update]; the API
// has no dedicated enable endpoint.
func (s *ActorsService) Enable(ctx context.Context, id string) (*Actor, error) {
	if err := checkID("Actor ID", id); err != nil {
		return nil, err
	}
	disabled := false
	return s.Update(ctx, id, &UpdateActorRequest{IsDisabled: &disabled})
}
