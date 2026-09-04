package firezone

import "context"

// ConditionProperty is the subject property a policy [Condition]
// evaluates.
type ConditionProperty string

// ConditionProperty values.
const (
	ConditionPropertyRemoteIPLocationRegion ConditionProperty = "remote_ip_location_region"
	ConditionPropertyRemoteIP               ConditionProperty = "remote_ip"
	ConditionPropertyAuthProviderID         ConditionProperty = "auth_provider_id"
	ConditionPropertyCurrentUTCDatetime     ConditionProperty = "current_utc_datetime"
	ConditionPropertyClientVerified         ConditionProperty = "client_verified"
)

// ConditionOperator is the comparison a policy [Condition] applies
// between the subject property and Values. Which operators are valid
// depends on Property - see the API's policy schema documentation.
type ConditionOperator string

// ConditionOperator values.
const (
	ConditionOperatorIsIn        ConditionOperator = "is_in"
	ConditionOperatorIsNotIn     ConditionOperator = "is_not_in"
	ConditionOperatorIsInCIDR    ConditionOperator = "is_in_cidr"
	ConditionOperatorIsNotInCIDR ConditionOperator = "is_not_in_cidr"

	// ConditionOperatorIsInDayOfWeekTimeRanges matches when the current
	// time falls inside one of the given weekly windows. Each value is a
	// "DAY/TIME_RANGES/TIMEZONE" string, where DAY is one of M T W R F S
	// U (Monday through Sunday), TIME_RANGES is a comma-separated list of
	// HH:MM-HH:MM ranges, and TIMEZONE is an IANA timezone name - for
	// example "M/09:00-17:00/America/New_York".
	//
	// All three segments are required; the API rejects a value with no
	// timezone. Days are specified one value per day, so a Monday-Friday
	// window is five values, not one.
	ConditionOperatorIsInDayOfWeekTimeRanges ConditionOperator = "is_in_day_of_week_time_ranges"

	ConditionOperatorIs ConditionOperator = "is"
)

// Condition restricts when a Policy grants access. All Conditions on a
// Policy must evaluate to true for access to be granted.
//
// Which Operators are valid, and how Values is interpreted, depends on
// Property:
//
//   - [ConditionPropertyRemoteIPLocationRegion] with is_in / is_not_in:
//     Values are ISO 3166-1 alpha-2 country codes, e.g. "US", "CA".
//   - [ConditionPropertyRemoteIP] with is_in_cidr / is_not_in_cidr:
//     Values are CIDR ranges, IPv4 or IPv6.
//   - [ConditionPropertyAuthProviderID] with is_in / is_not_in: Values
//     are authentication provider IDs (UUIDs).
//   - [ConditionPropertyCurrentUTCDatetime] with
//     is_in_day_of_week_time_ranges: each value is a
//     "DAY/TIME_RANGES/TIMEZONE" string - see
//     [ConditionOperatorIsInDayOfWeekTimeRanges].
//   - [ConditionPropertyClientVerified] with is: Values is a
//     single-element list holding "true" or "false".
type Condition struct {
	Property ConditionProperty `json:"property"`
	Operator ConditionOperator `json:"operator"`
	Values   []string          `json:"values"`
}

// Policy grants a Group access to a Resource, optionally restricted by
// Conditions.
type Policy struct {
	ID                    string      `json:"id"`
	GroupID               string      `json:"group_id"`
	ResourceID            string      `json:"resource_id"`
	Description           string      `json:"description"`
	FlowLogUploadsEnabled bool        `json:"flow_log_uploads_enabled"`
	IsDisabled            bool        `json:"is_disabled"`
	Conditions            []Condition `json:"conditions"`
}

// CreatePolicyRequest is the request body for [PoliciesService.Create].
type CreatePolicyRequest struct {
	GroupID               string      `json:"group_id"`
	ResourceID            string      `json:"resource_id"`
	Description           string      `json:"description,omitempty"`
	FlowLogUploadsEnabled *bool       `json:"flow_log_uploads_enabled,omitempty"`
	Conditions            []Condition `json:"conditions,omitempty"`
}

// UpdatePolicyRequest is the request body for [PoliciesService.Update].
// Every field is optional; omitted fields keep their current value.
//
// Setting IsDisabled to true stops the Policy granting access without
// deleting it.
type UpdatePolicyRequest struct {
	GroupID    string `json:"group_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	// Description is nullable, so it is typed [Null] - Clear[string]()
	// removes it, and a nil pointer leaves it alone.
	Description           *Null[string] `json:"description,omitempty"`
	FlowLogUploadsEnabled *bool         `json:"flow_log_uploads_enabled,omitempty"`
	IsDisabled            *bool         `json:"is_disabled,omitempty"`
	// Conditions replaces the Policy's conditions wholesale. nil leaves
	// them unchanged; a pointer to an empty slice removes all of them,
	// making the Policy grant access unconditionally.
	Conditions *[]Condition `json:"conditions,omitempty"`
}

// PoliciesService manages Policies.
type PoliciesService struct {
	client *Client
}

// Get fetches a single Policy by ID.
func (s *PoliciesService) Get(ctx context.Context, id string) (*Policy, error) {
	if err := checkID("Policy ID", id); err != nil {
		return nil, err
	}
	var policy Policy
	if err := s.client.do(ctx, "GET", buildPath("policies", id), nil, nil, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// PolicyListOptions extends ListOptions with Policies-specific filters.
type PolicyListOptions struct {
	ListOptions
	// GroupID filters to Policies granting this Group.
	GroupID string
	// ResourceID filters to Policies granting access to this Resource.
	ResourceID string
}

// List returns a page of Policies. Pass nil for opts to use the API's
// default page size and no filters.
func (s *PoliciesService) List(ctx context.Context, opts *PolicyListOptions) (*Page[Policy], error) {
	if opts == nil {
		opts = &PolicyListOptions{}
	}
	q := filterQuery(opts.ListOptions,
		[2]string{"group_id", opts.GroupID},
		[2]string{"resource_id", opts.ResourceID},
	)
	return doList[Policy](ctx, s.client, "GET", "policies", q)
}

// Create creates a new Policy.
func (s *PoliciesService) Create(ctx context.Context, req *CreatePolicyRequest) (*Policy, error) {
	body, err := wrapBody("policy", req)
	if err != nil {
		return nil, err
	}
	var policy Policy
	if err := s.client.do(ctx, "POST", "policies", nil, body, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// Update updates a Policy.
func (s *PoliciesService) Update(ctx context.Context, id string, req *UpdatePolicyRequest) (*Policy, error) {
	if err := checkID("Policy ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("policy", req)
	if err != nil {
		return nil, err
	}
	var policy Policy
	if err := s.client.do(ctx, "PATCH", buildPath("policies", id), nil, body, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// Delete deletes a Policy.
func (s *PoliciesService) Delete(ctx context.Context, id string) error {
	if err := checkID("Policy ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", buildPath("policies", id), nil, nil, nil)
}

// Disable disables a Policy, stopping it granting access without
// deleting it. Idempotent - disabling an already-disabled Policy is a
// no-op.
//
// This is a convenience wrapper over [PoliciesService.Update]; the API
// has no dedicated disable endpoint.
func (s *PoliciesService) Disable(ctx context.Context, id string) (*Policy, error) {
	if err := checkID("Policy ID", id); err != nil {
		return nil, err
	}
	disabled := true
	return s.Update(ctx, id, &UpdatePolicyRequest{IsDisabled: &disabled})
}

// Enable enables a disabled Policy. Idempotent - enabling an
// already-enabled Policy is a no-op.
//
// This is a convenience wrapper over [PoliciesService.Update]; the API
// has no dedicated enable endpoint.
func (s *PoliciesService) Enable(ctx context.Context, id string) (*Policy, error) {
	if err := checkID("Policy ID", id); err != nil {
		return nil, err
	}
	disabled := false
	return s.Update(ctx, id, &UpdatePolicyRequest{IsDisabled: &disabled})
}
