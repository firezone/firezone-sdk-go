package firezone

import "context"

// GroupMember is an Actor's minimal representation as returned by
// [MembershipsService.List].
type GroupMember struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	Type ActorType `json:"type"`
}

// MembershipsService manages a single Group's memberships. Obtain one
// via [GroupsService.Memberships].
type MembershipsService struct {
	client  *Client
	groupID string
}

func (s *MembershipsService) basePath() string {
	return buildPath("groups", s.groupID, "memberships")
}

// List returns a page of the Group's members.
func (s *MembershipsService) List(ctx context.Context, opts *ListOptions) (*Page[GroupMember], error) {
	if err := checkID("Group ID", s.groupID); err != nil {
		return nil, err
	}
	return doList[GroupMember](ctx, s.client, "GET", s.basePath(), listOptionsToQuery(opts))
}

// membershipEntry is the wire shape of one entry in a ReplaceAll request.
type membershipEntry struct {
	ActorID string `json:"actor_id"`
}

// membershipPatchBody is the wire shape of a Patch request body.
type membershipPatchBody struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// membershipActorIDs is the wire shape of a memberships write response.
type membershipActorIDs struct {
	ActorIDs []string `json:"actor_ids"`
}

// ReplaceAll replaces the Group's entire membership list with
// actorIDs, returning the resulting member actor IDs. Unlike [Patch],
// this is a full replace: any actor not in actorIDs is removed.
//
// Prefer [Patch] when multiple independent callers manage membership on
// the same Group - ReplaceAll from more than one caller will overwrite
// each other's changes.
//
// Members that are not changing keep their server-side membership rows,
// so replacing a list with itself is a no-op rather than a full
// rewrite. Repeating an ID in actorIDs is not an error; the list is
// deduplicated. The returned IDs are sorted, not echoed back in the
// order they were sent.
func (s *MembershipsService) ReplaceAll(ctx context.Context, actorIDs []string) ([]string, error) {
	if err := checkID("Group ID", s.groupID); err != nil {
		return nil, err
	}
	entries := make([]membershipEntry, len(actorIDs))
	for i, id := range actorIDs {
		entries[i] = membershipEntry{ActorID: id}
	}
	body, err := wrapBody("memberships", entries)
	if err != nil {
		return nil, err
	}
	var result membershipActorIDs
	if err := s.client.do(ctx, "PUT", s.basePath(), nil, body, &result); err != nil {
		return nil, err
	}
	return result.ActorIDs, nil
}

// Patch adds and removes members from the Group without disturbing any
// other membership, returning the resulting member actor IDs. This is
// the safe choice when more than one caller manages membership on the
// same Group independently.
//
// The operation is idempotent - adding an actor already in the Group
// and removing one that isn't are both no-ops - so a request that may
// already have been applied is safe to retry. Removals are applied
// before additions, so an ID passed in both add and remove ends up a
// member. Repeating an ID within either list is not an error; both are
// deduplicated. The returned IDs are sorted.
func (s *MembershipsService) Patch(ctx context.Context, add, remove []string) ([]string, error) {
	if err := checkID("Group ID", s.groupID); err != nil {
		return nil, err
	}
	body, err := wrapBody("memberships", membershipPatchBody{Add: add, Remove: remove})
	if err != nil {
		return nil, err
	}
	var result membershipActorIDs
	if err := s.client.do(ctx, "PATCH", s.basePath(), nil, body, &result); err != nil {
		return nil, err
	}
	return result.ActorIDs, nil
}
