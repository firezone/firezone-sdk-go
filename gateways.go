package firezone

import (
	"context"
	"time"
)

// Gateway is a Firezone Gateway - a host that exposes a Site's
// Resources to Clients.
//
// IPv4 and IPv6 are the Gateway's tunnel addresses, allocated when the
// Gateway is created rather than on first connect, so both are always
// populated - including on a Gateway that has never connected. See
// LastSeenRemoteIP on the API's own responses for the public address.
type Gateway struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IPv4   string `json:"ipv4"`
	IPv6   string `json:"ipv6"`
	Online bool   `json:"online"`

	// GatewayTokenID is the token this Gateway last connected with.
	// Empty until the Gateway connects for the first time.
	GatewayTokenID string `json:"gateway_token_id,omitempty"`

	// RotatedAt is when the token named by GatewayTokenID was rotated
	// out, and is nil in the normal case. A non-nil value means a
	// replacement token has been minted and this Gateway has not picked
	// it up yet - see [Gateway.RotationPending].
	RotatedAt *time.Time `json:"rotated_at,omitempty"`
}

// RotationPending reports whether a replacement token has been minted
// that this Gateway has not yet connected with.
//
// While it is true the current token stays valid only until the Gateway
// connects with the replacement or the API's rotation grace period
// elapses from [Gateway.RotatedAt], whichever comes first. A Gateway
// left in this state past the grace period is stranded: the API deletes
// the previous token once the replacement is confirmed, so rolling a
// host's configuration back to it will not work.
func (g *Gateway) RotationPending() bool {
	return g.RotatedAt != nil
}

// ProvisionedGateway is a newly provisioned Gateway along with its
// one-time token secret, returned only from [GatewaysService.Provision].
// The API never re-exposes the token after creation - see
// [GatewaysService.Get], which returns a plain [Gateway] with no token
// field at all, so the type system itself prevents a caller from
// expecting a token after a refresh.
type ProvisionedGateway struct {
	Gateway
	// Token is the one-time Gateway token secret. Store it securely -
	// it cannot be retrieved again.
	Token string `json:"token"`
}

// ProvisionGatewayRequest is the request body for
// [GatewaysService.Provision]. Name is optional; the API generates a
// random name when omitted.
type ProvisionGatewayRequest struct {
	Name string `json:"name,omitempty"`
}

// RotatedGatewayToken is the replacement token minted by
// [GatewaysService.RotateToken]. Like [ProvisionedGateway.Token], the
// secret is shown exactly once.
type RotatedGatewayToken struct {
	// ID is the new token's ID, which becomes the Gateway's
	// GatewayTokenID once it connects with this token.
	ID string `json:"id"`
	// Token is the replacement secret. Store it securely - it cannot be
	// retrieved again.
	Token string `json:"token"`
}

// UpdateGatewayRequest is the request body for [GatewaysService.Update].
// Name is the only mutable field - a Gateway's Site is permanent.
type UpdateGatewayRequest struct {
	Name string `json:"name"`
}

// GatewaysService manages the Gateways belonging to a single Site.
// Obtain one via [SitesService.Gateways].
//
// # Token lifecycle
//
// A Gateway has at most one active token, and this service covers the
// whole of that token's life: [GatewaysService.Provision] creates the
// Gateway and mints its token together, [GatewaysService.RotateToken]
// replaces it, and [GatewaysService.Delete] destroys the Gateway and
// revokes it.
//
// Two API endpoints are deliberately left out of that set:
//
//   - POST /sites/{site_id}/gateways/{gateway_id}/token creates a token
//     for a Gateway that has none. Every Gateway this SDK creates goes
//     through Provision, which already mints one, so calling it would
//     always fail with 409 Conflict - the API allows only one active
//     token per Gateway and directs callers to rotate instead. The
//     endpoint is only useful for a Gateway created elsewhere, such as
//     in the admin portal.
//   - POST /sites/{site_id}/gateway_tokens creates a multi-owner token
//     shared by all of a Site's Gateways. The API marks it deprecated in
//     favour of the per-Gateway endpoint above.
//
// The DELETE counterparts under /sites/{site_id}/gateway_tokens are
// absent for the same reason: a token this SDK can create belongs to a
// Gateway, and deleting that Gateway revokes it. The one case this
// leaves uncovered is a Gateway stranded past a rotation grace period
// (see [Gateway.RotationPending]), which is recovered by deleting and
// re-provisioning the Gateway rather than by deleting its token.
//
// If you need to adopt Gateways created outside this SDK, the token
// endpoints above are the gap to fill - adding them is additive, and
// [IsConflict] is already here for the 409 the first one returns.
type GatewaysService struct {
	client *Client
	siteID string
}

func (s *GatewaysService) basePath(segments ...string) string {
	return buildPath(append([]string{"sites", s.siteID, "gateways"}, segments...)...)
}

// Get fetches a single Gateway by ID.
func (s *GatewaysService) Get(ctx context.Context, id string) (*Gateway, error) {
	if err := checkID("site ID", s.siteID); err != nil {
		return nil, err
	}
	if err := checkID("Gateway ID", id); err != nil {
		return nil, err
	}
	var gateway Gateway
	if err := s.client.do(ctx, "GET", s.basePath(id), nil, nil, &gateway); err != nil {
		return nil, err
	}
	return &gateway, nil
}

// GatewayListOptions extends ListOptions with Gateways-specific
// filters. There's no SiteID filter here - the Site is already fixed
// by which [GatewaysService] you called List on (see
// [SitesService.Gateways]).
type GatewayListOptions struct {
	ListOptions
	// Name filters to the Gateway with this exact name.
	Name string
	// IPv4 filters to the Gateway with this exact IPv4 address.
	IPv4 string
	// IPv6 filters to the Gateway with this exact IPv6 address.
	IPv6 string
}

// List returns a page of the Site's Gateways. Pass nil for opts to use
// the API's default page size and no filters.
func (s *GatewaysService) List(ctx context.Context, opts *GatewayListOptions) (*Page[Gateway], error) {
	if err := checkID("site ID", s.siteID); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &GatewayListOptions{}
	}
	q := filterQuery(opts.ListOptions,
		[2]string{"name", opts.Name},
		[2]string{"ipv4", opts.IPv4},
		[2]string{"ipv6", opts.IPv6},
	)
	return doList[Gateway](ctx, s.client, "GET", s.basePath(), q)
}

// Provision creates a new Gateway and mints its single-owner token in
// one call. The returned Token is shown once - store it securely.
func (s *GatewaysService) Provision(ctx context.Context, req *ProvisionGatewayRequest) (*ProvisionedGateway, error) {
	if err := checkID("site ID", s.siteID); err != nil {
		return nil, err
	}
	body, err := wrapBody("gateway", req)
	if err != nil {
		return nil, err
	}
	var provisioned ProvisionedGateway
	if err := s.client.do(ctx, "POST", s.basePath(), nil, body, &provisioned); err != nil {
		return nil, err
	}
	return &provisioned, nil
}

// Update renames a Gateway.
func (s *GatewaysService) Update(ctx context.Context, id string, req *UpdateGatewayRequest) (*Gateway, error) {
	if err := checkID("site ID", s.siteID); err != nil {
		return nil, err
	}
	if err := checkID("Gateway ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("gateway", req)
	if err != nil {
		return nil, err
	}
	var gateway Gateway
	if err := s.client.do(ctx, "PATCH", s.basePath(id), nil, body, &gateway); err != nil {
		return nil, err
	}
	return &gateway, nil
}

// RotateToken mints a replacement single-owner token for the Gateway,
// returning the new secret once.
//
// The Gateway's current token is not invalidated immediately: it keeps
// working until the Gateway first connects with the replacement or the
// API's grace period elapses, whichever comes first. That window is the
// point - it exists so the replacement can be delivered to the Gateway
// host without downtime.
//
// Two consequences worth planning for:
//
//   - Deliver the replacement and restart the Gateway before the grace
//     period expires, or the Gateway is stranded. Poll
//     [Gateway.RotationPending] to confirm pickup.
//   - Once pickup is confirmed the previous token is deleted, so rolling
//     a host's configuration back to it will not work.
//
// Rotating again before the Gateway picks up a pending replacement
// replaces only that pending token; the in-use one keeps its original
// deadline.
func (s *GatewaysService) RotateToken(ctx context.Context, id string) (*RotatedGatewayToken, error) {
	if err := checkID("site ID", s.siteID); err != nil {
		return nil, err
	}
	if err := checkID("Gateway ID", id); err != nil {
		return nil, err
	}
	var rotated RotatedGatewayToken
	if err := s.client.do(ctx, "POST", s.basePath(id, "token", "rotate"), nil, nil, &rotated); err != nil {
		return nil, err
	}
	return &rotated, nil
}

// Delete deletes a Gateway, revoking its token. This is the only way
// this SDK revokes a token: see the [GatewaysService] doc comment for
// why the API's standalone token-deletion endpoints are not wrapped.
func (s *GatewaysService) Delete(ctx context.Context, id string) error {
	if err := checkID("site ID", s.siteID); err != nil {
		return err
	}
	if err := checkID("Gateway ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", s.basePath(id), nil, nil, nil)
}
