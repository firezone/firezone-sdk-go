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

After you refresh the spec, regenerate the posture field constants:

```bash
mise run generate
```

A refresh can add fields that a Policy posture accepts. The constants in
`posture_fields_gen.go` come from this file. CI runs `mise run
generate-check` and fails if the committed constants do not match the
spec. Commit the regenerated file in the same pull request.
