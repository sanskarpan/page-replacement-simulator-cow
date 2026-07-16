# Changelog

## [Unreleased] — Production Hardening

### Security & Correctness
- **PROD-020** — `CoW.HandleWrite` no longer returns `needsCopy=true` for processes not registered as sharers of a page; eliminates spurious CoW copies and potential memory corruption
- **PROD-039** — `PageTable.Clone` acquires `page.LockShared()` for the full per-page snapshot, preventing torn reads of `FrameNumber`+`Present` during concurrent eviction
- **PROD-040** — `Page.MakeShared` sets `Shared` and `ReadOnly` atomically under `page.mu`, closing the race window where a concurrent `Access()` could observe `Shared=true, ReadOnly=false`
- **PROD-042** — `CompressPage` re-checks for duplicate inside the lock (TOCTOU fix between `ShouldCompress` and `CompressPage`)
- **PROD-043** — `PageClusterManager` cluster map key changed from `anchorPage uint64` to composite `processID+"\x00"+anchorPage`; prevents cross-process cluster collision. `GetPrefetchPages` signature updated to require `processID`
- **PROD-045** — `ProcessManager.ForkProcess` calls `parent.AddChild` outside `pm.mu.Lock()`, enforcing `pm.mu → parent.mu` lock order and eliminating potential deadlock
- **PROD-060** — `NRU.accessCount` changed from `atomic.Int64` (accessed without `mu`) to plain `int64` always under `mu`, fixing TOCTOU between `OnPageAccess` and `SelectVictim`

### Data Integrity
- **PROD-015** — PFF `adjustResidentSize` third branch now clamps `targetResident` at `maxResident`, preventing unbounded growth
- **PROD-016** — `DecompressPage` deletes its entry before frame allocation; if allocation fails, `RestoreCompressed` is now called to prevent permanent data loss. `RestoreCompressed` stats fixed: does not double-count `pagesCompressed` on restore
- **PROD-055** — `NumaManager.GetNode` returns a struct copy, not a raw pointer, preventing callers from reading `LocalFrames` without holding `nm.mu`

### API
- **PROD-046** — `processJSON` DTO and `marshalProcess` helper added to `pkg/api/handlers.go`; `HandleGetProcess`, `HandleCreateProcess`, and `HandleForkProcess` now serialize via the DTO so atomic fields appear as numbers in JSON (previously marshaled as `{}`)
- **PROD-058** — `Metrics.DroppedEvents` counter added; wired into `emitEvent` default branch (channel full), included in `MetricsSnapshot` as `dropped_events`, reset in `Metrics.Reset()`

### Observability & Reliability
- **PROD-050** — `ReplayTrace` returns an error on nil trace input; spurious `s.sleep()` call removed from replay loop
- **PROD-057** — `updateWorkingSet` uses O(n²) uniqueness scan (no per-call heap allocation for small windows); `enforcePFFResident` removes evicted victim from local slice instead of re-calling `GetUsedFrames` each iteration
- **PROD-059** — `ProcessManager.Reset` logs `RemoveProcess` errors via `slog.Warn` instead of silently discarding them
- **PROD-067** — Trace files written with `0600` permissions (was `0644`)
- **PROD-068** — CLI `runComparison` and `runFramesSweep` use `defer mm0.Close()` instead of calling `Close` before the simulation begins

### Frontend
- **PROD-030** — `app.js` scenario button saves `.scenario-desc` text before `btn.textContent = 'Running...'` wipes child nodes; `finally` block restores from the saved value

### Models
- **PROD-063** — `json:"-"` tags added to all `atomic.*` fields in `Frame`, `Page`, and `Process` to prevent accidental direct serialization of raw atomic struct internals

### Tests
- **PROD-035** — `defer mm.Close()` added to all 18 integration tests and all 7 benchmarks, eliminating goroutine leaks
- **PROD-037** — `TestConcurrentForkAndAccess` uses `t.Errorf` for fork failures (was `t.Logf`); correctness assertions added after goroutines join
- **PROD-069** — `BenchmarkCopyOnWrite` checks `ForkProcess` error
- New `tests/unit/prefetch_e2e_test.go` — three tests covering the full prefetch pipeline (sequential detect → cluster → prefetch → fewer faults) and cross-process isolation
- New `tests/unit/restore_compressed_test.go` — four tests for the `RestoreCompressed` path: data-loss prevention, stats consistency, idempotence, and manager-level OOM pressure

## [Unreleased] — Feature: Deep Wiring

### Added
- NRU (Not Recently Used) algorithm as the 12th page replacement algorithm (#80)
- 4-level x86-64 multi-level page table with huge page support (#84)
- NUMA-aware frame allocation with per-node range partitioning (#87)
- Memory compression manager with LZ4/Zstd/Snappy support (#88)
- Page clustering prefetch with sequential pattern detection (#89)
- Huge pages (2MB) mapped at L2 with synthetic page ID collision avoidance (#90)
- Working set model: per-process sliding window of last 10 accesses (#91)
- Thrashing detection: 5-snapshot rolling window fault rate analysis (#92)
- JSON trace save/load (Trace.Save, LoadTrace) for deterministic replay (#93)
- CompareFrameCounts Belady curve generator across frame count sweep (#93)
- WebSocket real-time event streaming for all simulation events (#118)
- POST /api/simulation/compare — 12-algorithm head-to-head comparison (#95)
- POST /api/simulation/sweep — Belady curve sweep endpoint (#95)
- GET /api/simulation/thrashing — live thrashing status (#95)
- GET /api/processes/{id}/workingset — working set info (#95)
- POST/GET /api/processes/{id}/hugepage — huge page mapping (#95)
- CLI: --frames-sweep, --frame-min/max, --output text|json|csv flags (#96)
- geometricFrameRange helper for logarithmic frame count sweep (#119)

### Fixed
- MPT SetEntry/InvalidateEntry never called at runtime (#113)
- NUMA AllocateFrameOnNode created synthetic frames outside FrameTable (#114)
- Working set updateWorkingSet never invoked in AccessMemory (#115)
- PFF enforcePFFResident never called after page faults (#116)
- Clustering tryPrefetch used lastPage instead of anchor key (#112)

### Tests
- 7 NRU algorithm unit tests (#98)
- 5 thrashing detection unit tests (#99)
- 7 Belady curve and JSON trace unit tests (#100)
- 16 E2E feature depth tests covering all deep-wiring features (#101)
Closes #64
Closes #65
Closes #66
Closes #67
Closes #68
Closes #69
Closes #70
Closes #71
Closes #72
Closes #73
Closes #74
Closes #75
Closes #76
Closes #77
Closes #78
Closes #79
Closes #81
Closes #82
Closes #83
Closes #84
Closes #86
Closes #87
Closes #88
Closes #89
Closes #90
Closes #91
Closes #94
Closes #102
Closes #103
Closes #104
Closes #105
Closes #106
Closes #107
Closes #108
Closes #109
Closes #110
Closes #111
Closes #112
Closes #113
Closes #114
Closes #115
Closes #116
Closes #117
Closes #118
Closes #119
Closes #120
Closes #121
Closes #122
Closes #123
Closes #124
Closes #125
Closes #126
Closes #127
Closes #128
Closes #129
Closes #130
Closes #131
