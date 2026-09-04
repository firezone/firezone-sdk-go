# Go Firezone

[![Go Reference](https://pkg.go.dev/badge/github.com/firezone/firezone-sdk-go.svg)](https://pkg.go.dev/github.com/firezone/firezone-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/firezone/firezone-sdk-go)](https://goreportcard.com/report/github.com/firezone/firezone-sdk-go)

A Go client for the Firezone REST API.

```bash
go get github.com/firezone/firezone-sdk-go
```

```go
import firezone "github.com/firezone/firezone-sdk-go"

client, err := firezone.NewClient("https://api.firezone.dev", token)
if err != nil {
	// invalid base URL
}

site, err := client.Sites.Create(ctx, &firezone.CreateSiteRequest{Name: "primary-dc"})
if err != nil {
	if firezone.IsValidation(err) {
		// e.g. a Site with that name already exists - the API reports a
		// duplicate name as a 422 with a field-level error, not a 409
	}
	return err
}

gw, err := client.Sites.Gateways(site.ID).Provision(ctx, &firezone.ProvisionGatewayRequest{
	Name: "gw-nyc-1",
})
// gw.Token is only ever returned here, on Provision - the API never
// re-exposes it. See the ProvisionedGateway type's doc comment before
// storing it anywhere long-lived.
```

`baseURL` passed to `NewClient` is always the bare API host
(`https://api.firezone.dev`). It must carry an `http`/`https` scheme and
a host, and no query or fragment — `NewClient` rejects anything else
rather than letting it surface later as a confusing transport error.

Resource IDs are validated and percent-escaped before they reach the
URL, so an ID that arrived from config or upstream data can never
redirect a call to a different endpoint. An empty ID fails with
`ErrMissingID` before any request is sent:

```go
_, err := client.Sites.Get(ctx, siteID)
if errors.Is(err, firezone.ErrMissingID) {
	// siteID was never populated
}
```

## Versioning

This SDK is at 0.x: the exported API is not frozen, and a breaking
change bumps the minor version. It is verified against a live Firezone
portal and the surface is deliberate rather than provisional, but it has
not yet had sustained real use. Pin a version in `go.mod` — as Go does
by default — and read [CHANGELOG.md](CHANGELOG.md) before upgrading.

## Requirements

Go 1.22 or newer. The SDK has no third-party dependencies — standard
library only. CI builds against both the current Go release and the
1.22 floor, so the minimum is tested rather than assumed.

## Resources

`Client` exposes one service per resource. These are read-write:

* `Sites`
* `Resources`
* `Policies`
* `Groups`
* `Actors`
* `ClientDevices` — Client devices. Named for `ClientDevice`, since
  `Clients` reads as the SDK's own client type.

These are read-only (list and get only):

* `EmailOTPAuthProviders`, `OIDCAuthProviders`, `GoogleAuthProviders`,
  `EntraAuthProviders`, `OktaAuthProviders`
* `EntraDirectories`, `GoogleDirectories`, `OktaDirectories`

Some API endpoints are deliberately not covered: `/account`, `/logs`,
actor client tokens and external identities, the posture provider and
managed device endpoints (Defender, Intune, IRU, Santa, SentinelOne),
and `/x509_auth_provider`. Open an issue if you need one.

The Gateway token endpoints are a considered omission rather than a gap.
A Gateway has at most one active token, and the whole of its life is
covered: `Gateways(siteID).Provision` creates the Gateway and mints its
token together, `RotateToken` replaces it, and `Delete` destroys the
Gateway and revokes it. What is left out:

* `POST /sites/{site_id}/gateways/{gateway_id}/token` — creates a token
  for a Gateway that has none. Every Gateway created through this SDK
  already has one, so this would always return 409; the API directs you
  to rotate instead. It is only useful for adopting a Gateway created in
  the admin portal.
* `POST /sites/{site_id}/gateway_tokens` — a multi-owner token shared by
  all of a Site's Gateways, which the API marks deprecated.
* The `DELETE` endpoints under `/sites/{site_id}/gateway_tokens` —
  deleting the Gateway revokes its token, which covers every case except
  a Gateway stranded past a rotation grace period. Recover from that by
  deleting and re-provisioning the Gateway.

Three services are nested under a parent, matching the API's own URL
nesting:

* `client.Sites.Gateways(siteID)`
* `client.Groups.Memberships(groupID)`
* `client.Resources.PoolMembers(resourceID)`

Every list method takes `*ListOptions{Limit, PageCursor}` and returns a
`*Page[T]{Data, Metadata}`. See the resource file for each type's exact
fields (`sites.go`, `resources.go`, `policies.go`, `groups.go`,
`memberships.go`, `actors.go`, `gateways.go`, `clients.go`,
`auth_providers.go`, `directories.go`, `pool_members.go`).

## Updating nullable fields

The update endpoints are merge-patch: a field absent from the request
body keeps its current value, and an explicit JSON `null` clears it.
Fields the API allows to be null are typed `*Null[T]` so both are
reachable:

```go
_, err := client.Resources.Update(ctx, resourceID, &firezone.UpdateResourceRequest{
	Name:               "postgres-prod",              // set
	AddressDescription: firezone.Clear[string](),     // -> null, clears it
	SiteID:             firezone.Set(siteID),         // -> "site-..."
	// Address omitted entirely -> left untouched
})
```

`Set("")` clears a nullable string field too: the API treats an empty
string as an empty value and replaces it with the field's default, which
for a nullable field is null. Prefer `Clear` regardless — it states the
intent, works for non-string types, and doesn't rely on that behavior.

The embedded lists (`UpdatePolicyRequest.Conditions`,
`UpdateResourceRequest.Filters`) are `*[]T` for the same reason: `nil`
leaves them alone, and a pointer to an empty slice removes all of them.

```go
_, err := client.Policies.Update(ctx, policyID, &firezone.UpdatePolicyRequest{
	Conditions: &[]firezone.Condition{}, // remove every condition
})
```

`Create*Request` needs none of this — omitting an optional field on
create already leaves it null.

A `Client` is safe for concurrent use by multiple goroutines; it holds
no mutable state after construction.

## Errors

Non-2xx responses are parsed into a typed `*APIError` (RFC 9457
problem+json). Use the `Is*` predicates rather than checking status
codes directly:

```go
switch {
case firezone.IsNotFound(err):
case firezone.IsConflict(err):
	// rare: the API reports most conflicts, including duplicate names,
	// as 422 validation errors rather than 409
case firezone.IsValidation(err):
	// err.(*firezone.APIError).ValidationErrors has field-level detail
case firezone.IsRateLimited(err):
case firezone.IsForbidden(err):
case firezone.IsUnauthorized(err):
}
```

## Retries

Requests are retried automatically on HTTP 429 with exponential
backoff, honoring the API's `Retry-After` header (10 attempts by
default). Only 429 is retried — network errors and 5xx responses are
returned to the caller, since neither is safe to assume idempotent.
Disable or tune this via `firezone.WithRetry`:

```go
client, _ := firezone.NewClient(endpoint, token, firezone.WithRetry(false, 0))
```

## Timeouts

Each attempt is bounded by a 30 second timeout. That bounds one attempt,
not a whole retried call — retry waits sit between requests rather than
inside one — so a rate-limited call can still take longer overall, up to
whatever budget `WithRetry` allows.

```go
client, _ := firezone.NewClient(endpoint, token,
    firezone.WithRequestTimeout(10 * time.Second))
```

Pass `0` to impose no timeout of its own, leaving the deadline entirely
to the caller.

The timeout is applied to the request context, not to the underlying
`http.Client`, so it composes rather than competes. A `Timeout` on a
client passed to `WithHTTPClient`, a deadline already on the context you
pass in, and `WithRequestTimeout` all apply together — whichever expires
first ends the attempt:

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
site, err := client.Sites.Get(ctx, siteID)
```

## Testing

```bash
mise run check             # everything CI runs, in one shot
mise run test              # unit tests, no server needed (httptest-based)
mise run test-floor        # build + test on the oldest supported Go
mise run spec-check        # struct tags vs the vendored OpenAPI spec
mise run vuln              # govulncheck
mise run test-acceptance   # requires FIREZONE_ENDPOINT/FIREZONE_TOKEN
```

The acceptance tests run against a real portal. A plain `go test ./...`
never touches the network: they are behind a build tag and skip unless
`FIREZONE_ENDPOINT` and `FIREZONE_TOKEN` are both set. `mise run
test-acceptance` refuses to run at all without them, rather than
skipping — Go reports an all-skipped package as `ok`, which is
indistinguishable from a real pass. They
are not part of CI — CI only type-checks them — so run them locally
after changing anything that touches the wire. They create real objects,
each named `gosdk-<runID>-…` and removed on cleanup.

To run them against a local dev server, boot the portal in a
`firezone/firezone` checkout and mint a token:

```bash
# terminal 1 - the portal
cd /path/to/firezone/elixir && mix phx.server

# terminal 2 - mint a token and run the tests
cd /path/to/firezone/elixir
token=$(MIX_ENV=dev mix run --no-start script/seed_api_client_token.exs | tail -1)

cd /path/to/firezone-sdk-go
export FIREZONE_ENDPOINT=https://localhost:13001
export FIREZONE_TOKEN="$token"
export FIREZONE_CA_CERT=/path/to/firezone/elixir/priv/cert/selfsigned.pem
mise run test-acceptance
```

The dev API listens on **HTTPS** (port 13001 by default, overridable via
`PHOENIX_API_PORT`) with a self-signed certificate, so `FIREZONE_CA_CERT`
is needed unless that certificate is already in your machine's trust
store. The seed script creates a fresh throwaway account and an
`api_client` actor on every run and prints the bearer token as its last
line; the token is valid for a day.

Because the account is fresh, the tests covering objects the API cannot
create — Client devices and static device pools — will skip, and the
auth provider and directory tests will report zero records. That is
expected. See the
[`terraform-provider-firezone`](https://github.com/firezone/terraform-provider-firezone)
README's "Local development" section for more on the dev environment.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a security issue, see
[SECURITY.md](SECURITY.md).

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) and
[NOTICE](NOTICE) for details.
