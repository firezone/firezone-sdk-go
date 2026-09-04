package firezone

import (
	"context"
	"time"
)

// ClientDevice is a Firezone Client - an enrolled end-user device.
//
// The API calls this a "Client", but [Client] is already this SDK's own
// API client type, so the device concept carries the Device suffix here.
//
// Client devices are not created through this API: a device registers
// itself when it first connects. They can be read, renamed, verified,
// and deleted, but never provisioned - so there is no
// CreateClientRequest.
type ClientDevice struct {
	ID           string `json:"id"`
	FirezoneID   string `json:"firezone_id"`
	ActorID      string `json:"actor_id"`
	Name         string `json:"name"`
	IPv4         string `json:"ipv4"`
	IPv6         string `json:"ipv6"`
	Online       bool   `json:"online"`
	PublicKey    string `json:"public_key,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
	DeviceSerial string `json:"device_serial,omitempty"`
	DeviceUUID   string `json:"device_uuid,omitempty"`

	// VerifiedAt is nil until an admin verifies the device. Policies can
	// require verification via the client_verified condition property.
	VerifiedAt *time.Time `json:"verified_at,omitempty"`

	// LastSeenAt is nil for a Client that has enrolled but never
	// connected.
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	LastSeenVersion  string     `json:"last_seen_version,omitempty"`
	LastSeenRemoteIP string     `json:"last_seen_remote_ip,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateClientRequest is the request body for [ClientsService.Update].
// Name is the only mutable field, and the API requires it, so it is
// always sent - omitting it on an empty value would produce a body the
// API rejects for a reason that doesn't name the field.
type UpdateClientRequest struct {
	Name string `json:"name"`
}

// ClientsService manages Clients.
type ClientsService struct {
	client *Client
}

// Get fetches a single Client by ID.
func (s *ClientsService) Get(ctx context.Context, id string) (*ClientDevice, error) {
	if err := checkID("Client ID", id); err != nil {
		return nil, err
	}
	var c ClientDevice
	if err := s.client.do(ctx, "GET", buildPath("clients", id), nil, nil, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ClientListOptions extends ListOptions with Clients-specific filters.
type ClientListOptions struct {
	ListOptions
	// Name filters to Clients with this exact name. Names are not
	// unique, so this can still match more than one.
	Name string
	// FirezoneID filters to Clients with this exact Firezone ID. Unique
	// per actor rather than per account, so this too can match more than
	// one - though in practice rarely does.
	FirezoneID string
}

// List returns a page of Clients. Pass nil for opts to use the API's
// default page size and no filters.
func (s *ClientsService) List(ctx context.Context, opts *ClientListOptions) (*Page[ClientDevice], error) {
	if opts == nil {
		opts = &ClientListOptions{}
	}
	q := filterQuery(opts.ListOptions,
		[2]string{"name", opts.Name},
		[2]string{"firezone_id", opts.FirezoneID},
	)
	return doList[ClientDevice](ctx, s.client, "GET", "clients", q)
}

// Update renames a Client.
func (s *ClientsService) Update(ctx context.Context, id string, req *UpdateClientRequest) (*ClientDevice, error) {
	if err := checkID("Client ID", id); err != nil {
		return nil, err
	}
	body, err := wrapBody("client", req)
	if err != nil {
		return nil, err
	}
	var c ClientDevice
	if err := s.client.do(ctx, "PATCH", buildPath("clients", id), nil, body, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Delete deletes a Client, unenrolling the device.
func (s *ClientsService) Delete(ctx context.Context, id string) error {
	if err := checkID("Client ID", id); err != nil {
		return err
	}
	return s.client.do(ctx, "DELETE", buildPath("clients", id), nil, nil, nil)
}

// Verify marks a Client as admin-verified, satisfying the
// client_verified Policy condition.
func (s *ClientsService) Verify(ctx context.Context, id string) (*ClientDevice, error) {
	if err := checkID("Client ID", id); err != nil {
		return nil, err
	}
	var c ClientDevice
	if err := s.client.do(ctx, "PUT", buildPath("clients", id, "verify"), nil, nil, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Unverify clears a Client's verification.
func (s *ClientsService) Unverify(ctx context.Context, id string) (*ClientDevice, error) {
	if err := checkID("Client ID", id); err != nil {
		return nil, err
	}
	var c ClientDevice
	if err := s.client.do(ctx, "PUT", buildPath("clients", id, "unverify"), nil, nil, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
