package firezone

import "context"

// ResourceType is the type of network object a Resource represents.
type ResourceType string

// Resource types. "internet" also exists but is API-read-only - the API
// returns 403 if you try to create or update one - so it's deliberately
// not offered as a constant here.
const (
	ResourceTypeCIDR ResourceType = "cidr"
	ResourceTypeIP   ResourceType = "ip"
	ResourceTypeDNS  ResourceType = "dns"

	// ResourceTypeStaticDevicePool is currently readable but not
	// creatable: the API rejects any request that changes a Resource's
	// type to it, on both create and update, with a 422. Create device
	// pools in the admin portal instead.
	//
	// The constant stays because existing pools are still returned by
	// Get and List, still filterable via [ResourceListOptions.Type], and
	// still updatable and deletable - only the transition into this type
	// is refused. Note that restating an existing pool's own type on an
	// update is not a transition and is accepted.
	ResourceTypeStaticDevicePool ResourceType = "static_device_pool"
)

// IPStack constrains which IP families a Resource is reachable over.
type IPStack string

// IPStack values.
const (
	IPStackIPv4Only IPStack = "ipv4_only"
	IPStackIPv6Only IPStack = "ipv6_only"
	IPStackDual     IPStack = "dual"
)

// FilterProtocol is the transport protocol a [Filter] applies to.
type FilterProtocol string

// FilterProtocol values.
const (
	FilterProtocolTCP  FilterProtocol = "tcp"
	FilterProtocolUDP  FilterProtocol = "udp"
	FilterProtocolICMP FilterProtocol = "icmp"
)

// Filter restricts the protocols and ports a Resource exposes.
type Filter struct {
	Protocol FilterProtocol `json:"protocol"`
	// Ports are port numbers or ranges (e.g. "80" or "8000 - 9000").
	// Not applicable to FilterProtocolICMP.
	Ports []string `json:"ports,omitempty"`
}

// Resource is a Firezone Resource - a network object (CIDR, IP, DNS
// name, or static device pool) that Policies grant access to.
type Resource struct {
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	Address            string       `json:"address"`
	AddressDescription string       `json:"address_description"`
	Type               ResourceType `json:"type"`
	IPStack            IPStack      `json:"ip_stack,omitempty"`
	SiteID             string       `json:"site_id,omitempty"`
	Filters            []Filter     `json:"filters"`
}

// CreateResourceRequest is the request body for [ResourcesService.Create].
type CreateResourceRequest struct {
	Name               string       `json:"name"`
	Type               ResourceType `json:"type"`
	Address            string       `json:"address,omitempty"`
	AddressDescription string       `json:"address_description,omitempty"`
	IPStack            IPStack      `json:"ip_stack,omitempty"`
	SiteID             string       `json:"site_id,omitempty"`
	Filters            []Filter     `json:"filters,omitempty"`
}

// UpdateResourceRequest is the request body for [ResourcesService.Update].
// All fields are optional; omitted fields keep their current value.
//
// The nullable fields are typed [Null] so they can be cleared as well as
// set - see that type for the three states. Filters is a pointer to a
// slice for the same reason: a nil pointer leaves the Resource's filters
// alone, while a pointer to an empty slice removes all of them.
type UpdateResourceRequest struct {
	Name    string        `json:"name,omitempty"`
	Type    ResourceType  `json:"type,omitempty"`
	Address *Null[string] `json:"address,omitempty"`
	// AddressDescription is free-form text describing the address.
	// Clear[string]() removes it. Set("") removes it too - the API
	// replaces an empty string with the field's default rather than
	// storing it - but Clear states the intent.
	AddressDescription *Null[string]  `json:"address_description,omitempty"`
	IPStack            *Null[IPStack] `json:"ip_stack,omitempty"`
	// SiteID moves the Resource to another Site. Clearing it detaches
	// the Resource from its Site, which the API only permits for device
	// pool Resources.
	SiteID *Null[string] `json:"site_id,omitempty"`
	// Filters replaces the Resource's filters wholesale. nil leaves them
	// unchanged; a pointer to an empty slice removes all of them.
	Filters *[]Filter `json:"filters,omitempty"`
}

// ResourcesService manages Resources, and, nested under them, static
// device pool membership.
type ResourcesService struct {
	client *Client
}

// PoolMembers returns a [PoolMembersService] scoped to the
// static_device_pool Resource identified by resourceID. Calling it for
// any other Resource type is allowed, but every request that service
// makes will fail with 400.
func (s *ResourcesService) PoolMembers(resourceID string) *PoolMembersService {
	return &PoolMembersService{client: s.client, resourceID: resourceID}
}

// Get fetches a single Resource by ID.
func (s *ResourcesService) Get(ctx context.Context, id string) (*Resource, error) {
	if err := checkID("Resource ID", id); err != nil {
		return nil, err
	}
	var resource Resource
	if err := s.client.do(ctx, "GET", buildPath("resources", id), nil, nil, &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}

// ResourceListOptions extends ListOptions with Resources-specific
// filters.
type ResourceListOptions struct {
	ListOptions
	// Name filters to Resources with this exact name.
	Name string
	// Type filters to Resources of this type.
	Type ResourceType
	// SiteID filters to Resources connected to this Site.
	SiteID string
	// Address filters to Resources with this exact address.
	Address string
	// IPStack filters to Resources with this exact ip_stack.
	IPStack IPStack
}

// List returns a page of Resources. Pass nil for opts to use the API's
// default page size and no filters.
func (s *ResourcesService) List(ctx context.Context, opts *ResourceListOptions) (*Page[Resource], error) {
	if opts == nil {
		opts = &ResourceListOptions{}
	}
	q := filterQuery(opts.ListOptions,
		[2]string{"name", opts.Name},
		[2]string{"type", string(opts.Type)},
		[2]string{"site_id", opts.SiteID},
		[2]string{"address", opts.Address},
		[2]string{"ip_stack", string(opts.IPStack)},
	)
	return doList[Resource](ctx, s.client, "GET", "resources", q)
}

// Create creates a new Resource.
//
// Two types cannot be created: "internet" (403 Forbidden) and
// [ResourceTypeStaticDevicePool] (422) - create device pools in the
// admin portal instead.
func (s *ResourcesService) Create(ctx context.Context, req *CreateResourceRequest) (*Resource, error) {
	body, err := wrapBody("resource", req)
	if err != nil {
		return nil, err
	}
	var resource Resource
	if err := s.client.do(ctx, "POST", "resources", nil, body, &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}

// Update updates a Resource.
//
// Changing a Resource's type to [ResourceTypeStaticDevicePool] is
// refused with a 422, the same as creating one. Restating an existing
// pool's own type is not a change and is accepted, so a caller that
// echoes the whole Resource back on update still works.
func (s *ResourcesService) Update(ctx context.Context, id string, req *UpdateResourceRequest) (*Resource, error) {
	if err := checkID("Resource ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("resource", req)
	if err != nil {
		return nil, err
	}
	var resource Resource
	if err := s.client.do(ctx, "PATCH", buildPath("resources", id), nil, body, &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}

// Delete deletes a Resource.
func (s *ResourcesService) Delete(ctx context.Context, id string) error {
	if err := checkID("Resource ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", buildPath("resources", id), nil, nil, nil)
}
