package firezone

import (
	"context"
	"time"
)

// Group is a Firezone actor Group. Groups synced from an identity
// provider have a non-empty DirectoryID and are read-only via this API
// (writes return 403 Forbidden) - see [Group.IsSynced].
type Group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email,omitempty"`
	EntityType  string     `json:"entity_type,omitempty"`
	DirectoryID string     `json:"directory_id,omitempty"`
	IdpID       string     `json:"idp_id,omitempty"`
	SyncedAt    *time.Time `json:"synced_at,omitempty"`
	InsertedAt  time.Time  `json:"inserted_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// IsSynced reports whether the Group is managed by an identity
// provider sync, and therefore read-only via this API.
func (g *Group) IsSynced() bool {
	return g.DirectoryID != ""
}

// CreateGroupRequest is the request body for [GroupsService.Create].
type CreateGroupRequest struct {
	Name string `json:"name"`
}

// UpdateGroupRequest is the request body for [GroupsService.Update].
type UpdateGroupRequest struct {
	Name string `json:"name,omitempty"`
}

// GroupsService manages Groups, and, nested under them, memberships.
type GroupsService struct {
	client *Client
}

// Memberships returns a [MembershipsService] scoped to the Group
// identified by groupID.
func (s *GroupsService) Memberships(groupID string) *MembershipsService {
	return &MembershipsService{client: s.client, groupID: groupID}
}

// Get fetches a single Group by ID.
func (s *GroupsService) Get(ctx context.Context, id string) (*Group, error) {
	if err := checkID("Group ID", id); err != nil {
		return nil, err
	}
	var group Group
	if err := s.client.do(ctx, "GET", buildPath("groups", id), nil, nil, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// GroupListOptions extends ListOptions with Groups-specific filters.
type GroupListOptions struct {
	ListOptions
	// Name filters to Groups with this exact name.
	Name string
	// DirectoryID filters to Groups synced from this directory. A
	// pointer since the two meaningful "set" states aren't
	// distinguishable through a plain string's zero value: nil means no
	// filter, a pointer to "" filters to unsynced (native) Groups only,
	// and a pointer to a directory ID filters to that directory. Use
	// [String] to build the pointer inline, e.g. DirectoryID:
	// firezone.String("").
	DirectoryID *string
	// EntityType filters to Groups of this entity type ("group" or
	// "org_unit").
	EntityType string
}

// List returns a page of Groups. Pass nil for opts to use the API's
// default page size and no filters.
func (s *GroupsService) List(ctx context.Context, opts *GroupListOptions) (*Page[Group], error) {
	if opts == nil {
		opts = &GroupListOptions{}
	}
	q := filterQuery(opts.ListOptions,
		[2]string{"name", opts.Name},
		[2]string{"entity_type", opts.EntityType},
	)
	if opts.DirectoryID != nil {
		q.Set("directory_id", *opts.DirectoryID)
	}
	return doList[Group](ctx, s.client, "GET", "groups", q)
}

// Create creates a new (unsynced) Group.
func (s *GroupsService) Create(ctx context.Context, req *CreateGroupRequest) (*Group, error) {
	body, err := wrapBody("group", req)
	if err != nil {
		return nil, err
	}
	var group Group
	if err := s.client.do(ctx, "POST", "groups", nil, body, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// Update updates a Group. Returns 403 Forbidden if the Group is synced
// from an identity provider (see [Group.IsSynced]).
func (s *GroupsService) Update(ctx context.Context, id string, req *UpdateGroupRequest) (*Group, error) {
	if err := checkID("Group ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("group", req)
	if err != nil {
		return nil, err
	}
	var group Group
	if err := s.client.do(ctx, "PATCH", buildPath("groups", id), nil, body, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// Delete deletes a Group. Returns 403 Forbidden if the Group is synced
// from an identity provider (see [Group.IsSynced]).
func (s *GroupsService) Delete(ctx context.Context, id string) error {
	if err := checkID("Group ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", buildPath("groups", id), nil, nil, nil)
}
