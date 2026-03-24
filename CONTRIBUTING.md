# Contributing

## Getting Started

1. Fork the repository and create a branch from `main`.
2. Make focused changes with tests.
3. Run the local quality checks before opening a PR.

## Local Checks

```bash
go test ./...
go test -race ./pkg/... ./internal/ffi ./test/e2e ./test/interop
go vet ./...
bash ./scripts/check_docs_contract.sh
```

If you have `golangci-lint` installed locally, run:

```bash
golangci-lint run ./...
```

## Native/Shim Changes

When changing `shim/shim.h` or the FFI boundary:

1. Regenerate Go bindings with `go generate ./internal/ffi`
2. Rebuild or validate the shim artifacts for affected platforms
3. Update tests and docs for any contract change
4. Keep unsafe pointer usage isolated and documented

## Pull Request Guidelines

- Keep PRs focused and explain user-visible contract changes clearly.
- Prefer additive or compatibility-preserving changes unless a breaking cleanup is intentional.
- Update `README.md` when runtime behavior or support status changes.
- Do not silently add unsupported API surfaces; document them as unsupported until implemented.
