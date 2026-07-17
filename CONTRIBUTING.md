# Contributing

## Getting started

```bash
git clone https://github.com/sanskarpan/page-replacement-simulator-cow.git
cd page-replacement-simulator-cow
go mod download
```

## Before you submit

Run the full test suite with the race detector:

```bash
go test -race ./...
```

Run static analysis:

```bash
go vet ./...
```

All tests must pass and `go vet` must be clean before opening a PR.

## Adding a page replacement algorithm

1. Create `internal/algorithms/<name>.go` implementing the `PageReplacementAlgorithm` interface (see `internal/algorithms/algorithm.go`).
2. Register the algorithm constant in `algorithm.go` and wire it into the `NewAlgorithm` factory.
3. Add unit tests in `tests/unit/<name>_test.go` covering at minimum: basic eviction, empty pool, and any algorithm-specific invariants.
4. Add a benchmark in `tests/benchmark/performance_test.go`.

## Pull requests

- One logical change per PR.
- Include tests for new behavior; do not reduce overall coverage below 70% of `internal/` and `pkg/`.
- Write a clear PR description explaining *why*, not just *what*.
- Squash fixup commits before merging.

## Reporting bugs

Use the [Bug Report](.github/ISSUE_TEMPLATE/bug_report.md) issue template.

## Proposing features

Open a [Feature Request](.github/ISSUE_TEMPLATE/feature_request.md) issue first to discuss before implementing.
