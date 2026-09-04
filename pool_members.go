package firezone

import (
	"context"
	"time"
)

// PoolMember is a Client's minimal representation as returned by
// [PoolMembersService.List].
//
// Pool members are Client devices, not Actors - a static device pool
// grants access to specific machines, so this is not the device-shaped
// equivalent of [GroupMember] despite the similar surface.
type PoolMember struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// PoolMembersService manages the membership of a single
// static_device_pool Resource. Obtain one via
// [ResourcesService.PoolMembers].
//
// Every method returns 400 if the Resource is not a static_device_pool;
// no other Resource type has members.
type PoolMembersService struct {
	client     *Client
	resourceID string
}

func (s *PoolMembersService) basePath() string {
	return buildPath("resources", s.resourceID, "pool_members")
}

// List returns a page of the pool's member Clients.
func (s *PoolMembersService) List(ctx context.Context, opts *ListOptions) (*Page[PoolMember], error) {
	if err := checkID("Resource ID", s.resourceID); err != nil {
		return nil, err
	}
	return doList[PoolMember](ctx, s.client, "GET", s.basePath(), listOptionsToQuery(opts))
}

// poolMemberEntry is the wire shape of one entry in a ReplaceAll request.
type poolMemberEntry struct {
	DeviceID string `json:"device_id"`
}

// poolMemberPatchBody is the wire shape of a Patch request body.
type poolMemberPatchBody struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// poolMemberDeviceIDs is the wire shape of a pool members write response.
type poolMemberDeviceIDs struct {
	DeviceIDs []string `json:"device_ids"`
}

// ReplaceAll replaces the pool's entire membership with deviceIDs,
// returning the resulting member Client IDs. Any Client not in
// deviceIDs is removed from the pool. Passing an empty slice clears it.
//
// Prefer [PoolMembersService.Patch] when multiple independent callers
// manage membership on the same pool - ReplaceAll from more than one
// caller will overwrite each other's changes.
//
// Returns 422 if any ID is not a Client in the account: a Gateway ID, a
// Client from another account, and a nonexistent ID all fail the same
// way.
func (s *PoolMembersService) ReplaceAll(ctx context.Context, deviceIDs []string) ([]string, error) {
	if err := checkID("Resource ID", s.resourceID); err != nil {
		return nil, err
	}
	entries := make([]poolMemberEntry, len(deviceIDs))
	for i, id := range deviceIDs {
		entries[i] = poolMemberEntry{DeviceID: id}
	}
	body, err := wrapBody("pool_members", entries)
	if err != nil {
		return nil, err
	}
	var result poolMemberDeviceIDs
	if err := s.client.do(ctx, "PUT", s.basePath(), nil, body, &result); err != nil {
		return nil, err
	}
	return result.DeviceIDs, nil
}

// Patch adds and removes Clients without disturbing any other member of
// the pool, returning the resulting member Client IDs. This is the safe
// choice when more than one caller manages membership on the same pool
// independently.
//
// Both operations are idempotent: adding a Client already in the pool
// and removing one that isn't are both no-ops. remove is applied before
// add, so an ID in both slices ends up in the pool.
func (s *PoolMembersService) Patch(ctx context.Context, add, remove []string) ([]string, error) {
	if err := checkID("Resource ID", s.resourceID); err != nil {
		return nil, err
	}
	body, err := wrapBody("pool_members", poolMemberPatchBody{Add: add, Remove: remove})
	if err != nil {
		return nil, err
	}
	var result poolMemberDeviceIDs
	if err := s.client.do(ctx, "PATCH", s.basePath(), nil, body, &result); err != nil {
		return nil, err
	}
	return result.DeviceIDs, nil
}
