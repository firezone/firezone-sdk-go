# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Because this is a client library, "breaking" means a change that stops
existing code compiling or changes what a call does. A new exported
field or method is additive, thus not breaking.

While this project is at 0.x the exported API is not yet frozen:
breaking changes bump the minor version and are listed under
`### Changed`, with a note on what to do about them. From 1.0.0 onward
they will require a new major version.

## [Unreleased]

## [0.1.0] - 2026-09-04

First public release. The API is complete and verified against a live
Firezone portal, but ships as 0.x while it gets real use. See the
versioning note above for what that means for compatibility.

This module supersedes `github.com/firezone/firezone-go`, which was
published to the module proxy by mistake and has been retracted. There
is no code difference to migrate: change the import path to
`github.com/firezone/firezone-sdk-go` and the package name stays
`firezone`.

### Added

- Read-write services for Sites, Resources, Policies, Groups, Actors and
  Client devices, with Gateways nested under Sites, memberships nested
  under Groups, and pool members nested under Resources.
- Read-only services for the five auth provider types and the three
  directory connection types.
- Cursor pagination on every list method (`ListOptions` → `Page[T]`).
- Typed errors: `APIError` carrying RFC 9457 problem details, with
  `IsNotFound`, `IsValidation`, `IsRateLimited`, `IsForbidden`,
  `IsUnauthorized` and `IsConflict` predicates. `ErrMissingID` and
  `ErrNilRequest` cover the two ways a call can be malformed before it
  is sent.
- Automatic retry with exponential backoff and jitter on HTTP 429,
  honouring `Retry-After`. Configurable via `WithRetry` and
  `WithRetryMaxWait`.
- A 30 second bound on each HTTP attempt, configurable via
  `WithRequestTimeout` and disabled by passing `0`.
- `Null[T]` with `Set` and `Clear`, so nullable fields on merge-patch
  update requests can be cleared as well as set.
- `Version`, sent as part of the default `User-Agent`.

### Notes

- `Update` methods send `PATCH`. The API routes `PATCH` and `PUT` to the
  same handler, but a partial update is what `PATCH` means, and sending
  the matching verb insures against the two ever diverging.
  `Memberships.ReplaceAll` and `PoolMembers.ReplaceAll` send `PUT`,
  where the API genuinely distinguishes them, as do `Verify` and
  `Unverify`.
- Some endpoints are deliberately not wrapped; the README lists which,
  and the `GatewaysService` doc comment explains the Gateway token ones
  in particular.
- `Option` is `func(*Client) error`, so an option validates its own
  input and `NewClient` reports the failure at construction, stopping at
  the first error.
- The request timeout is applied to the request context rather than to
  the `http.Client`, so it composes with a client passed to
  `WithHTTPClient` and with a deadline already on the caller's context
  instead of overriding either. Whichever expires first ends the
  attempt, and the bound covers one attempt rather than a whole retried
  call.
- The SDK does not use `http.DefaultClient`. That value is
  process-global, so sharing it would mean SDK transport changes leaking
  into the rest of the binary and vice versa.

[Unreleased]: https://github.com/firezone/firezone-sdk-go/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/firezone/firezone-sdk-go/releases/tag/v0.1.0
