# Security Policy

## Reporting A Vulnerability

Please do not open public issues for suspected security vulnerabilities.

Send a private report with:

- A description of the issue and impacted package or API
- Reproduction steps or a proof of concept
- Any known mitigations or workarounds
- Your preferred contact details for follow-up

If GitHub private vulnerability reporting is enabled for this repository, use it.
Otherwise contact the maintainer directly and treat the report as confidential
until a fix or mitigation is ready.

## Scope

Security-sensitive areas in this repository include:

- Shared-library loading and auto-download paths in `internal/ffi`
- Checksum validation and artifact release flow
- Native shim boundary code under `shim/`
- Data-channel, capture, and callback crossing points between Go and C++

## Expectations

- We aim to acknowledge reports promptly and keep reporters informed.
- Fix timelines depend on severity and reproducibility.
- Coordinated disclosure is preferred once a fix is available.
