package firezone

import "context"

// Site is a Firezone Site - a logical grouping of Gateways and the
// Resources they expose.
type Site struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateSiteRequest is the request body for [SitesService.Create].
type CreateSiteRequest struct {
	Name string `json:"name"`
}

// UpdateSiteRequest is the request body for [SitesService.Update]. All
// fields are optional; omitted fields keep their current value.
type UpdateSiteRequest struct {
	Name string `json:"name,omitempty"`
}

// SitesService manages Sites, and, nested under them, Gateways.
type SitesService struct {
	client *Client
}

// Gateways returns a [GatewaysService] scoped to the Site identified by
// siteID, matching the API's own URL nesting
// (/sites/{site_id}/gateways/...).
func (s *SitesService) Gateways(siteID string) *GatewaysService {
	return &GatewaysService{client: s.client, siteID: siteID}
}

// Get fetches a single Site by ID.
func (s *SitesService) Get(ctx context.Context, id string) (*Site, error) {
	if err := checkID("site ID", id); err != nil {
		return nil, err
	}
	var site Site
	if err := s.client.do(ctx, "GET", buildPath("sites", id), nil, nil, &site); err != nil {
		return nil, err
	}
	return &site, nil
}

// SiteListOptions extends ListOptions with Sites-specific filters.
type SiteListOptions struct {
	ListOptions
	// Name filters to the Site with this exact name.
	Name string
}

// List returns a page of Sites. Pass nil for opts to use the API's
// default page size and no filters.
func (s *SitesService) List(ctx context.Context, opts *SiteListOptions) (*Page[Site], error) {
	if opts == nil {
		opts = &SiteListOptions{}
	}
	q := filterQuery(opts.ListOptions, [2]string{"name", opts.Name})
	return doList[Site](ctx, s.client, "GET", "sites", q)
}

// Create creates a new Site.
func (s *SitesService) Create(ctx context.Context, req *CreateSiteRequest) (*Site, error) {
	body, err := wrapBody("site", req)
	if err != nil {
		return nil, err
	}
	var site Site
	if err := s.client.do(ctx, "POST", "sites", nil, body, &site); err != nil {
		return nil, err
	}
	return &site, nil
}

// Update updates a Site.
func (s *SitesService) Update(ctx context.Context, id string, req *UpdateSiteRequest) (*Site, error) {
	if err := checkID("site ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("site", req)
	if err != nil {
		return nil, err
	}
	var site Site
	if err := s.client.do(ctx, "PATCH", buildPath("sites", id), nil, body, &site); err != nil {
		return nil, err
	}
	return &site, nil
}

// Delete deletes a Site.
func (s *SitesService) Delete(ctx context.Context, id string) error {
	if err := checkID("site ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", buildPath("sites", id), nil, nil, nil)
}
