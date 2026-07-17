# Security Policy

## Scope

This project is a virtual memory simulator. It does not handle real credentials, user data, or network-facing services in production deployments. Security concerns are most relevant when running the HTTP server (`cmd/server`) on a network-accessible interface.

## Reporting a vulnerability

Do **not** open a public GitHub issue for security vulnerabilities.

Send a private report to **sanskar@noclick.com** with:

- A description of the vulnerability and its impact
- Steps to reproduce
- Any proof-of-concept code

You will receive an acknowledgement within 48 hours. Confirmed vulnerabilities will be patched and disclosed in the CHANGELOG under the next release.

## Known security posture

The codebase has been reviewed through a 70-point security and correctness audit (see CHANGELOG — Production Hardening). Key hardening points:

- All data races and TOCTOU bugs are eliminated (verified with `go test -race`)
- Atomic model fields carry `json:"-"` tags; serialization goes through explicit DTOs
- Lock ordering is enforced throughout (`pm.mu → parent.mu`, never reversed)
- Trace files are written with `0600` permissions
- Event channel backpressure is counted in metrics, never blocks callers
- `govulncheck` runs in CI on every push
