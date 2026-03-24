# Versioning And Releases

`libgowebrtc` now uses two separate release tracks:

- `vX.Y.Z`: Go module and API releases
- `shim-vX.Y.Z`: prebuilt native shim asset releases

Keeping those tracks separate lets us version the Go surface independently from
the downloadable binary assets.

## Module Releases

Module releases use standard semantic version tags such as `v0.1.0`.

Recommended bump rules:

- `patch`: bug fixes, dependency updates, docs-only corrections, or other
  backwards-compatible maintenance changes
- `minor`: backwards-compatible new APIs, new features, new supported runtime
  capabilities, or breaking API cleanup while the project is still pre-`v1`
- `major`: breaking API changes once the module has reached `v1.0.0`

While the module is still in `v0`, treat `minor` bumps as the place to signal
meaningful API shifts.

Examples:

```bash
# Preview the next patch release
./scripts/release-module.sh patch --dry-run

# Create and push the next minor release tag from origin/main
./scripts/release-module.sh minor --push

# Create a specific tag
./scripts/release-module.sh v0.1.0 --push
```

Pushing a module tag triggers the `Release Module` GitHub Actions workflow,
which:

1. Checks out the tagged commit
2. Runs the docs contract check
3. Runs the repository test suite
4. Publishes a GitHub release with generated notes if verification passes

## Shim Releases

Shim releases continue to use the existing `shim-vX.Y.Z` tags and release
artifacts.

Examples:

```bash
./scripts/release.sh 0.4.5 --release-dir release/shim-v0.4.5
```

Shim releases remain binary-asset releases. They are validated by the
`Validate Shim Release` workflow after publication.

## First Module Release

There are no stable module tags yet. The recommended first public module
release is `v0.1.0`.
