# Contributing

Thanks for your interest in improving the Firezone Go SDK.

## Getting set up

Tool versions come from `.tool-versions`, so local runs and CI can't
drift apart. With [mise](https://mise.jdx.dev) installed:

```bash
mise install
mise run check     # everything CI runs, in one shot
```

The SDK has no third-party dependencies — standard library only. Please
keep it that way; a dependency in a client library becomes a dependency
for everyone who imports it.

## Before opening a pull request

```bash
mise run check
```

That runs formatting, `go vet` (including the `integration` and `spec`
build tags), `golangci-lint`, unit tests with `-race`, the OpenAPI spec
checks, `govulncheck`, and a build. Individual tasks are listed by
`mise tasks`.

Two more that CI doesn't run for you:

```bash
mise run test-floor       # build + test on the oldest supported Go
mise run test-acceptance  # against a real portal - see the README
```

Run the acceptance tests after changing anything that touches the wire.
They are the only tests that can disagree with the SDK about how the
server behaves, and they have caught real bugs that every other check
passed.

## Conventions worth knowing

**Every exported identifier needs a doc comment.** `golangci-lint`
enforces it. This package is meant to be read through `go doc` and
pkg.go.dev.

**JSON struct tags are checked against the OpenAPI spec.** A tag the
server doesn't recognise fails silently at runtime — a response field
decodes as a zero value, and a request field is dropped by the server
without an error. `mise run spec-check` compares the tags against the
spec vendored at `testdata/openapi.json`. Run it whenever you add or
change a field.

**Nullable fields have rules in both directions.** On a merge-patch
update request, a field the spec marks nullable must be `*Null[T]` so it
can be cleared as well as set, and an array field must be `*[]T` so an
empty list can be sent. On a read model, a nullable non-string field
must be a pointer, because `0` and `false` are legitimate values that a
null would be indistinguishable from. Both rules are enforced by the
spec tests.

**Request paths go through `buildPath`, never string concatenation**,
and caller-supplied IDs go through `checkID` first.

**The `go` directive in `go.mod` is the oldest Go this SDK supports**,
not the version we build with. Keep it as low as the code allows —
raising it is a hard requirement on every consumer.

## Updating the vendored OpenAPI spec

```bash
FIREZONE_MONOREPO=/path/to/firezone mise run spec-update
```

The resulting diff is the list of API changes this SDK hasn't accounted
for yet, so it belongs in the same pull request as the code that
responds to it — never as a standalone commit to make CI green.

## Cutting a release

1. Refresh the vendored spec (`mise run spec-update`) and deal with any
   diff. Shipping against a stale spec means shipping unverified tags.
2. Update `Version` in `firezone.go`. It is sent in the `User-Agent`, so
   it needs to match the tag.
3. Move the `Unreleased` section of `CHANGELOG.md` into a new version
   section and update the comparison links at the bottom.
4. `mise run check`, `mise run test-floor`, and `mise run test-acceptance`.
   The acceptance run is not optional for a release: it is the only
   check that can disagree with the SDK about how the server behaves.
5. Tag `vX.Y.Z` and push the tag. pkg.go.dev picks it up from there;
   there are no artifacts to build.

## Versioning

Semantic versioning applies to the exported API. Retyping an existing
exported field or changing what a call does is breaking, even when it is
a bug fix.

**This project is at 0.x.** The exported API is not frozen yet: a
breaking change bumps the minor version (0.1.0 → 0.2.0) and is called
out in the changelog. That is deliberate. The SDK is verified against a
live portal, but it has not yet been through enough sustained real use.

Once the library has seen enough real use and any adjustments have been made
we will move to 1.0.0
