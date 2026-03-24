# Contributing

Thanks for helping improve `libgowebrtc`.

## Before You Start

1. Fork the repository and create a topic branch from `main`.
2. Keep changes focused so reviews stay fast and safe.
3. Open an issue first for large API changes, platform support changes, or behavior that could be breaking.
4. Follow the [Code of Conduct](CODE_OF_CONDUCT.md) in all project interactions.

## Local Checks

Run the same core checks that gate protected branches:

```bash
go test ./...
go test -race ./pkg/... ./internal/ffi ./test/e2e ./test/interop
go vet ./...
bash ./scripts/check_docs_contract.sh
```

If you have `golangci-lint` installed locally, also run:

```bash
golangci-lint run ./...
```

## Native And Shim Changes

When changing `shim/shim.h`, `internal/ffi`, or release-loading behavior:

1. Regenerate Go bindings with `go generate ./internal/ffi`.
2. Rebuild or validate the shim artifacts for affected platforms.
3. Update tests and docs for any runtime or contract change.
4. Keep unsafe pointer usage isolated and documented.

## Pull Request Guidelines

- Explain the problem and the chosen approach clearly.
- Include the commands you ran to verify the change.
- Update `README.md` or package docs when behavior, support status, or public API expectations change.
- Prefer additive and compatibility-preserving changes unless a breaking cleanup is intentional and documented.
- Do not silently add unsupported API surfaces; document them as unsupported until implemented.

## Reporting Bugs And Requesting Features

- Use the issue templates when possible so reproduction details do not get lost.
- For support questions, check [SUPPORT.md](SUPPORT.md) before opening an issue.
- For security reports, follow [SECURITY.md](SECURITY.md) and avoid public disclosure until a fix is available.
