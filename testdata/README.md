# testdata

## `openapi.json`

A verbatim copy of Firezone's published OpenAPI document, vendored so
the spec checks in `spec_test.go` run in CI without needing a monorepo
checkout alongside this one.

- **Source:** `elixir/priv/static/openapi.json` in the
  [`firezone/firezone`](https://github.com/firezone/firezone) monorepo
- **API version:** 1.0.0 (OpenAPI 3.0.0)

Refresh it against a local monorepo checkout with:

```bash
FIREZONE_MONOREPO=/path/to/firezone mise run spec-update
```

Copy it verbatim - it is already pretty-printed with sorted keys
upstream, so an unmodified copy produces readable diffs when a field
changes. Reformatting it would bury the real change in noise.

Refreshing the spec is a deliberate act: the diff is the list of API
changes this SDK has not accounted for yet, and it belongs in the same
pull request as the code that responds to it.
