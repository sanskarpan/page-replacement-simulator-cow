# FIXES

## F-01: TLB key collision (I-01)
- **Files**: `internal/memory/tlb.go`
- **Change**: `string(rune(virtualPage))` → `strconv.FormatUint(virtualPage, 10)`
- **Validation**: All tests pass, race detector clean
- **Impact**: TLB now correctly handles all page numbers

## F-02: Unprotected map read (I-02)
- **Files**: `internal/memory/manager.go:154-157`
- **Change**: Added `mm.mu.RLock()`/`mm.mu.RUnlock()` around process map read
- **Validation**: Concurrent access stress test passes
- **Impact**: Eliminates concurrent map read/write panic

## F-03: TLB Lookup TOCTOU race (I-03)
- **Files**: `internal/memory/tlb.go:44-61`
- **Change**: Single write lock for entire Lookup when entry exists, instead of RLock→unlock→Lock→unlock
- **Validation**: All tests pass, race detector clean
- **Impact**: Atomic lookup-update under lock

## F-04: Map write under RLock (I-04)
- **Files**: `pkg/api/server.go:56-66`
- **Change**: Collect dead clients under RLock, then delete under write Lock after releasing RLock
- **Validation**: All tests pass, race detector clean
- **Impact**: Eliminates concurrent map write panic

## F-05: PageTable UpdateFrame RLock→Lock (I-05)
- **Files**: `internal/memory/page_table.go:167`
- **Change**: `pt.mu.RLock()` → `pt.mu.Lock()`
- **Validation**: All tests pass

## F-06: CoW double counting (I-06)
- **Files**: `internal/cow/cow.go:102`
- **Change**: Removed `cow.copiesCreated.Add(1)` from HandleWrite
- **Validation**: CoW test still shows correct behavior
- **Impact**: CoW copy count is now accurate

## F-07: Reference counter race (I-07)
- **Files**: `internal/cow/reference_counter.go:38-58`
- **Change**: Single write lock for entire Decrement instead of RLock→Unlock→Lock
- **Validation**: All tests pass, race detector clean

## F-08: Wrong RNG source (I-09)
- **Files**: `internal/simulator/simulator.go:90`
- **Change**: `rand.Float64()` → `s.rng.Float64()`
- **Validation**: Compiler confirms correctness

## F-09: Intn(0) panic guard (I-10)
- **Files**: `internal/simulator/simulator.go:57-60`
- **Change**: Added `if workingSetSize == 0 { return error }` at function entry
- **Validation**: All tests pass

## F-10: WebSocket goroutine leak (I-11)
- **Files**: `pkg/api/websocket.go:20-24,36-39,86-90,157-171`
- **Change**: Added `done chan struct{}` to Client, closed in writePump defer, checked in sendPeriodicMetrics select loop
- **Validation**: Build passes

## F-11: Monitor goroutine leak (I-12)
- **Files**: `pkg/api/server.go:14-20,44-49,54-58`
- **Change**: Stored stop channel in Server struct, added Shutdown() method
- **Validation**: Build passes

## F-12: Graceful shutdown (I-13)
- **Files**: `cmd/server/main.go:1-11,57-70`
- **Change**: Added context import, `httpServer.Close()` → `httpServer.Shutdown(ctx)` with 10s timeout, call server.Shutdown()
- **Validation**: Build passes

## F-13: Frontend XSS (I-14)
- **Files**: `web/static/js/app.js:330-362`
- **Change**: Replaced inline onclick with data-pid attributes + addEventListener. Added escapeHtml() utility function.
- **Validation**: No syntax errors

## F-14: Input validation (I-15)
- **Files**: `pkg/api/handlers.go:52-62`
- **Change**: Added validation for name (required), priority (0-10), virtual_pages (1-100000)
- **Validation**: All tests pass
