# ISSUES

## Summary

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| I-01 | Critical | TLB key collision renders cache ineffective | FIXED |
| I-02 | High | Unprotected concurrent map read in manager.go | FIXED |
| I-03 | High | TOCTOU race in TLB Lookup | FIXED |
| I-04 | High | Map write under RLock in server handleBroadcast | FIXED |
| I-05 | Medium | PageTable.UpdateFrame uses RLock for mutation | FIXED |
| I-06 | Medium | CoW copies double-counted | FIXED |
| I-07 | Medium | Reference counter Decrement race condition | FIXED |
| I-08 | Medium | Optimal algorithm never seeded with future accesses | OPEN |
| I-09 | Medium | Wrong RNG source in SimulateLoopingAccess | FIXED |
| I-10 | Medium | Intn(0) panic in SimulateLocalityAccess | FIXED |
| I-11 | Medium | WebSocket sendPeriodicMetrics goroutine leak | FIXED |
| I-12 | Medium | Monitor periodic capture goroutine never stopped | FIXED |
| I-13 | Medium | Non-graceful server shutdown (Close vs Shutdown) | FIXED |
| I-14 | Medium | Frontend inline onclick handlers (XSS risk) | FIXED |
| I-15 | Medium | Missing input validation on process creation endpoint | FIXED |
| I-16 | Medium | Event channel broadcast to closed subscriber | NO-REPRO |
| I-17 | Low | Misleading metric name AvgEvictionTimeNs | OPEN |
| I-18 | Low | GetMetrics has side effects (updates state on read) | OPEN |
| I-19 | Low | Static file path relative to working directory | OPEN |
| I-20 | Low | No auth/CSRF on API endpoints | OPEN |
| I-21 | Low | CORS allows all origins | OPEN |

---

## I-01 | Critical | TLB key collision renders cache ineffective

- **File**: `internal/memory/tlb.go:40`
- **Root cause**: `makeKey()` uses `string(rune(virtualPage))` which converts uint64 to a Unicode character. Large page numbers collide to `\uFFFD`.
- **Impact**: TLB effectively useless for any page number. All lookups hit the wrong entry or miss.
- **Fix**: Changed to `strconv.FormatUint(virtualPage, 10)` for proper decimal key generation.

## I-02 | High | Unprotected concurrent map read

- **File**: `internal/memory/manager.go:154`
- **Root cause**: `mm.processes[processID]` read without holding `mm.mu.RLock()`. Concurrent `CreateProcess`/`RemoveProcess`/`ForkProcess` race with this read.
- **Impact**: Fatal concurrent map read/write panic under concurrent access.
- **Fix**: Wrapped the read with `mm.mu.RLock()`/`mm.mu.RUnlock()`.

## I-03 | High | TOCTOU race in TLB Lookup

- **File**: `internal/memory/tlb.go:44-62`
- **Root cause**: Lookup releases RLock before acquiring write lock to update LastAccess. Entry could be invalidated by concurrent goroutine.
- **Impact**: Nil dereference or writing to stale entry.
- **Fix**: Hold write lock for the entire lookup-modify block (simplified to single Lock/Unlock).

## I-04 | High | Map write under RLock

- **File**: `pkg/api/server.go:56-66`
- **Root cause**: `delete(s.clients, client)` called while holding `s.clientsMu.RLock()`.
- **Impact**: Fatal concurrent map write panic.
- **Fix**: Collect dead clients under RLock, then delete under write Lock after releasing RLock.

## I-05 | Medium | RLock for mutation in UpdateFrame

- **File**: `internal/memory/page_table.go:167`
- **Root cause**: `UpdateFrame` calls `page.SetFrame()` which mutates state, but acquired only RLock.
- **Impact**: Data race on page frame number.
- **Fix**: Changed to `pt.mu.Lock()`.

## I-06 | Medium | CoW copies double-counted

- **File**: `internal/cow/cow.go:102,324`
- **Root cause**: `copiesCreated` incremented in both `HandleWrite` (line 102) and `CopyPage` (line 324). Each copy-on-write triggers both calls.
- **Impact**: CoW copy metric is 2x actual value.
- **Fix**: Removed increment from `HandleWrite` (CopyPage is the definitive copy point).

## I-07 | Medium | Reference counter Decrement race

- **File**: `internal/cow/reference_counter.go:38-58`
- **Root cause**: Decrement releases RLock, then calls counter.Add(-1). Concurrent Increment can insert new entry, and the delete check races.
- **Impact**: Reference count may underflow, totalRefs may drift.
- **Fix**: Changed to single Lock/Unlock for entire Decrement operation.

## I-08 | Medium | Optimal algorithm never seeded

- **File**: `internal/algorithms/optimal.go`
- **Root cause**: `SetFutureAccesses()` must be called before use, but is never called anywhere. Future access map is always empty.
- **Impact**: Optimal algorithm always returns sentinel value 1,000,000, effectively becoming arbitrary-victim (= first non-pinned frame found).
- **Fix**: Not fixed (requires architectural change to feed access traces). Documented limitation.

## I-09 | Medium | Wrong RNG source in SimulateLoopingAccess

- **File**: `internal/simulator/simulator.go:90`
- **Root cause**: Uses `rand.Float64()` (global math/rand) instead of `s.rng.Float64()` (simulator's seeded RNG).
- **Impact**: Non-reproducible random behavior, potential concurrency issues with global RNG.
- **Fix**: Changed to `s.rng.Float64()`.

## I-10 | Medium | Intn(0) panic

- **File**: `internal/simulator/simulator.go:71`
- **Root cause**: If `workingSetSize == 0`, `s.rng.Intn(0)` panics.
- **Impact**: Runtime panic.
- **Fix**: Added validation at function entry.

## I-11 | Medium | sendPeriodicMetrics goroutine leak

- **File**: `pkg/api/websocket.go:157-171`
- **Root cause**: Goroutine has no exit condition. Runs forever sending to potentially-closed channel after client disconnects.
- **Impact**: Goroutine leak + panic on closed channel send.
- **Fix**: Added `done` channel to Client struct, checked in select loop.

## I-12 | Medium | Monitor periodic capture never stopped

- **File**: `internal/monitor/monitor.go:283-301`
- **Root cause**: `StartPeriodicCapture` returns stop channel but caller (`server.go:48`) discards it.
- **Impact**: Ticker goroutine leaks on shutdown.
- **Fix**: Stored stop channel in Server, added `Shutdown()` method that closes it.

## I-13 | Medium | Non-graceful server shutdown

- **File**: `cmd/server/main.go:63`
- **Root cause**: `httpServer.Close()` immediately closes listener without draining active connections.
- **Impact**: In-flight requests abruptly terminated.
- **Fix**: Changed to `httpServer.Shutdown(ctx)` with 10s timeout.

## I-14 | Medium | Frontend XSS via inline onclick

- **File**: `web/static/js/app.js:346-347`
- **Root cause**: Inline onclick attributes with string interpolation inject user-controlled data into HTML.
- **Impact**: XSS if process IDs contain special characters.
- **Fix**: Replaced inline onclick with addEventListener + data attributes. Added `escapeHtml()` utility.

## I-15 | Medium | Missing input validation

- **File**: `pkg/api/handlers.go:52-55`
- **Root cause**: No validation on priority range, virtual_pages range, or empty name.
- **Impact**: Invalid or malicious input propagates to business logic.
- **Fix**: Added validation with clear error messages.

## I-16 | Medium | Event channel broadcast to closed subscriber

- **File**: `internal/monitor/monitor.go:80-88`
- **Root cause**: Alleged race between close(ch) in Unsubscribe and send in handleEvent.
- **Analysis**: Both hold `eventSubsMu` during close and send. No actual race - mutex serializes them.
- **Status**: NO-REPRO - verified locking is correct.

## I-17 | Low | Misleading metric name

- **File**: `internal/memory/manager.go:273`
- **Root cause**: `AvgEvictionTimeNs` stores latest eviction time, not average.
- **Impact**: Confusing diagnostics.
- **Fix**: Open - rename or compute EMA.

## I-18 | Low | GetMetrics side effect

- **File**: `internal/memory/manager.go:453-454`
- **Root cause**: Read-only getter `GetMetrics()` calls `UpdateFrameStats`/`UpdatePageStats` inside, mutating state.
- **Impact**: Principle of least surprise violated. Metrics drift on every read.
- **Fix**: Open - would require restructuring.

## I-19 | Low | Static path relative to CWD

- **File**: `cmd/server/main.go:48`
- **Root cause**: `http.Dir("./web/static")` depends on working directory.
- **Impact**: Server fails if started from different directory.
- **Fix**: Open - use `embed` or absolute path.

## I-20 | Low | No auth/CSRF

- **Impact**: All API endpoints publicly accessible.
- **Fix**: Open - acceptable for local dev tool, not production.
- **Severity**: Low for this tool.

## I-21 | Low | CORS allows all origins

- **Impact**: Cross-origin access unrestricted.
- **Fix**: Open - acceptable for local dev.
