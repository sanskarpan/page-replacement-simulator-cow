# Bug Tracker — Deep Audit Findings

All 15 bugs found in the July 2026 codebase audit. Each entry includes severity, location, root cause, reproduction steps, and fix summary.

---

## BUG-01 — TOCTOU Race in `cow.HandleWrite`

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Concurrency / Data Race |
| **File** | `internal/cow/cow.go:87–107` |
| **Status** | Fixed ✅ |

**Description:** `HandleWrite` reads `sharedPage.RefCount` under `sharedPage.mu.RLock`, releases the lock at line 89, then calls `decrementRefCount` later. Two concurrent writers both see refCount=2 and both create CoW copies. The original shared frame is permanently leaked.

**Reproduction:** Two goroutines concurrently write to the same shared page. Without synchronization, both observe refCount=2 before either decrements it.

**Fix:** Acquire `cow.mu.Lock()` (write lock) for the entire read-check-decrement sequence, eliminating the window between check and decrement.

---

## BUG-02 — `mm.pageTables` Read Without Lock in TLB-Hit Write Path

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Concurrency / Data Race |
| **File** | `internal/memory/manager.go:204` |
| **Status** | Fixed ✅ |

**Description:** After `mm.mu.RUnlock()` at line 185, the TLB-hit path reads `mm.pageTables[processID]` at line 204 with no lock held. Concurrent `CreateProcess` or `RemoveProcess` can mutate `mm.pageTables` simultaneously.

**Reproduction:** Concurrent write-access on a TLB-cached page while another goroutine terminates the process.

**Fix:** Move CoW processing inside the `mm.mu.Lock()` section that guards map access.

---

## BUG-03 — `mm.recentAccesses` Written Without Lock in `trackAccess`

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Concurrency / Data Race |
| **File** | `internal/memory/manager.go:293` |
| **Status** | Fixed ✅ |

**Description:** `trackAccess` writes to `mm.recentAccesses[processID]` but is called from the unlocked fast path (after `mm.mu.RUnlock()`). Concurrent accesses race on the slice.

**Reproduction:** Enable clustering and call `AccessMemory` concurrently from two goroutines.

**Fix:** Call `trackAccess` while holding `mm.mu.RLock()`.

---

## BUG-04 — `pageTable.Entries` Written Without Lock in `handleCoW`

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Concurrency / Data Race |
| **File** | `internal/memory/manager.go:431` |
| **Status** | Fixed ✅ |

**Description:** `handleCoW` is invoked from the TLB-hit path with no `mm.mu` lock held. It writes `pageTable.Entries[page.ID] = newPage` while other goroutines concurrently read the same map.

**Reproduction:** Fork a process so pages are shared, then have two goroutines write to shared pages concurrently.

**Fix:** Move the TLB-hit CoW path inside the `mm.mu.Lock()` section.

---

## BUG-05 — PFF `computeFaultRate` Writes `p.faultTimes` Under RLock

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Concurrency / Data Race |
| **File** | `internal/algorithms/pff.go:47–51, 163–171` |
| **Status** | Fixed ✅ |

**Description:** `GetFaultRate()` and `GetStats()` acquire `p.mu.RLock()` then call `computeFaultRate()`, which assigns `p.faultTimes = valid` — a write under a read lock. Multiple concurrent callers race on the slice header.

**Reproduction:** Call `GetFaultRate()` and `OnPageFault()` concurrently from multiple goroutines.

**Fix:** Upgrade `GetFaultRate` and `GetStats` to use `p.mu.Lock()`.

---

## BUG-06 — `handleCoW` Doesn't Call `OnPageFault` on New Frame

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Logic / Algorithm Corruption |
| **File** | `internal/memory/manager.go:~408` |
| **Status** | Fixed ✅ |

**Description:** After allocating a new frame for a CoW copy, `handleCoW` never calls `mm.algorithm.OnPageFault(newFrame)`. For ARC/CAR this means the frame is invisible to the algorithm's T1/T2 lists. Subsequent eviction creates spurious B1 ghost entries and corrupts the adaptive `p` parameter.

**Reproduction:** Use ARC algorithm, fork a process, trigger CoW writes, then force eviction pressure.

**Fix:** Add `mm.algorithm.OnPageFault(newFrame)` after the new CoW frame is allocated.

---

## BUG-07 — `tryPrefetch` Doesn't Call `OnPageFault` on Prefetch Frames

| Field | Value |
|-------|-------|
| **Severity** | MEDIUM |
| **Category** | Logic / Algorithm Corruption |
| **File** | `internal/memory/manager.go:~331` |
| **Status** | Fixed ✅ (was already correct after prior fix, verified) |

**Description:** `tryPrefetch` allocates frames but the `OnPageFault` call was missing in the original code path. For ARC/CAR, prefetch frames would be untracked.

**Reproduction:** Enable clustering with ARC, trigger sequential access pattern, observe missing ARC tracking.

**Fix:** Ensure `mm.algorithm.OnPageFault(frame)` is called after each prefetch frame allocation.

---

## BUG-08 — `atomicEvictAndAlloc` Swallows All Errors

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Error Handling |
| **File** | `internal/memory/manager.go:374–391` |
| **Status** | Fixed ✅ |

**Description:** `atomicEvictAndAlloc` returns `void`. When `SelectVictim` fails (e.g., all frames pinned), the function silently returns. The page stays with frame=-1 but `handlePageFault` returns nil (success). The caller believes the fault was handled.

**Reproduction:** Pin all frames, then access a new page. Returns nil but the page has no physical frame.

**Fix:** Return an error from `atomicEvictAndAlloc` and propagate it through `handlePageFault`.

---

## BUG-09 — `emitEvent` Spawns Unbounded Goroutines

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Resource Leak / Performance |
| **File** | `internal/memory/manager.go:613–617` |
| **Status** | Fixed ✅ |

**Description:** `emitEvent` does `go mm.eventCallback(event, data)` unconditionally on every call. Under load (e.g., 1000 memory accesses/second), this spawns thousands of goroutines, exhausting scheduler resources.

**Reproduction:** Set an event callback and run high-throughput memory accesses. `runtime.NumGoroutine()` grows unboundedly.

**Fix:** Replace with a buffered channel (capacity 256) and a single background consumer goroutine.

---

## BUG-10 — `ClearClusters` Ignores `processID` and Clears All Clusters

| Field | Value |
|-------|-------|
| **Severity** | MEDIUM |
| **Category** | Logic |
| **File** | `internal/memory/advanced.go:167–171` |
| **Status** | Fixed ✅ |

**Description:** `ClearClusters(processID)` always does `pcm.clusters = make(map[uint64]*models.PageCluster)`, wiping all processes' cluster data. Terminating one process destroys prefetch tracking for all surviving processes.

**Reproduction:** Create two processes with sequential access patterns. Terminate one. The other's prefetch pages vanish.

**Fix:** Add `ProcessID` field to `PageCluster`, filter `ClearClusters` to only delete entries matching the given process.

---

## BUG-11 — WSClock `scheduleWriteback` Is a No-Op

| Field | Value |
|-------|-------|
| **Severity** | MEDIUM |
| **Category** | Logic |
| **File** | `internal/algorithms/wsclock.go:99–101` |
| **Status** | Fixed ✅ |

**Description:** `scheduleWriteback` only calls `frame.ClearReferenceBit()`. It never clears `frame.Modified`. Dirty frames are considered "written back" but remain dirty forever, causing them to be repeatedly skipped during eviction.

**Reproduction:** Mark a frame dirty, run WSClock eviction — the dirty frame is never evicted, even after 2× full sweeps.

**Fix:** Perform writeback inline (set `frame.Modified.Store(false)`) instead of spawning a goroutine that only clears the reference bit.

---

## BUG-12 — TLB Insert After CoW Uses Synthetic PageID Instead of Virtual Page

| Field | Value |
|-------|-------|
| **Severity** | MEDIUM |
| **Category** | Logic / Performance |
| **File** | `internal/memory/manager.go:434` |
| **Status** | Fixed ✅ |

**Description:** After a CoW copy, `mm.tlb.Insert(processID, newPageID, newFrame.ID)` inserts a TLB entry for the synthetic CoW ID (≥1,000,000) instead of the original virtual page. Subsequent accesses to the original virtual page always miss the TLB.

**Reproduction:** Fork a process, trigger CoW on page 5, then re-access page 5. TLB reports 0 hits.

**Fix:** Change to `mm.tlb.Insert(processID, page.ID, newFrame.ID)` to prime the TLB for the original virtual address.

---

## BUG-13 — `AllocateFrameOnNode` Writes Under RLock and Returns nil

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Concurrency / Logic |
| **File** | `internal/memory/advanced.go:84–99` |
| **Status** | Fixed ✅ |

**Description:** `AllocateFrameOnNode` acquires `nm.mu.RLock()` but then writes `node.LocalFrames--` — a data race. Additionally, it always returns `nil, nil` on success, so any caller dereferencing the frame panics.

**Reproduction:** Call `AllocateFrameOnNode` concurrently from multiple goroutines with race detector enabled.

**Fix:** Upgrade to `nm.mu.Lock()` and construct and return a real `*models.Frame`.

---

## BUG-14 — Compression Never Executes (Simulated Ratio Always Exceeds Threshold)

| Field | Value |
|-------|-------|
| **Severity** | MEDIUM |
| **Category** | Logic |
| **File** | `internal/memory/advanced.go:207–210` |
| **Status** | Fixed ✅ |

**Description:** `CompressPage` computes `compressedSize = data*3/4` (ratio = 0.75). With default `minRatio=0.7`, the guard `0.75 > 0.7` is always true and the function returns nil. Compression is never performed regardless of data compressibility.

**Reproduction:** Call `CompressPage` with any data. `GetStats().PagesCompressed` is always 0.

**Fix:** Change simulated ratio to `len(data)/2` (0.5) and set default `minRatio=0.8` so the ratio 0.5 < 0.8 passes.

---

## BUG-15 — `evictPage` O(n) Linear Scan Instead of O(1) Lookup

| Field | Value |
|-------|-------|
| **Severity** | LOW |
| **Category** | Performance |
| **File** | `internal/memory/manager.go:468–473` |
| **Status** | Fixed ✅ |

**Description:** `evictPage` calls `pageTable.GetAllPages()` which allocates a full slice and iterates every page to find the one matching `pageID`. `pageTable.GetPage(pageID)` is O(1) and already defined.

**Reproduction:** Benchmark eviction with large page tables (1000+ pages). Eviction time scales linearly with table size.

**Fix:** Replace `for _, page := range pageTable.GetAllPages()` with `pageTable.GetPage(pageID)`.

---

## Summary

| ID | Severity | Category | File | Fixed |
|----|----------|----------|------|-------|
| BUG-01 | CRITICAL | Race | cow/cow.go | ✅ |
| BUG-02 | CRITICAL | Race | memory/manager.go | ✅ |
| BUG-03 | HIGH | Race | memory/manager.go | ✅ |
| BUG-04 | CRITICAL | Race | memory/manager.go | ✅ |
| BUG-05 | HIGH | Race | algorithms/pff.go | ✅ |
| BUG-06 | HIGH | Logic | memory/manager.go | ✅ |
| BUG-07 | MEDIUM | Logic | memory/manager.go | ✅ |
| BUG-08 | HIGH | Error | memory/manager.go | ✅ |
| BUG-09 | HIGH | Leak | memory/manager.go | ✅ |
| BUG-10 | MEDIUM | Logic | memory/advanced.go | ✅ |
| BUG-11 | MEDIUM | Logic | algorithms/wsclock.go | ✅ |
| BUG-12 | MEDIUM | Logic | memory/manager.go | ✅ |
| BUG-13 | HIGH | Race+Logic | memory/advanced.go | ✅ |
| BUG-14 | MEDIUM | Logic | memory/advanced.go | ✅ |
| BUG-15 | LOW | Perf | memory/manager.go | ✅ |

---

## Round-2 Audit — 7 new bugs fixed (July 2026)

| ID | Severity | Category | File | Status |
|----|----------|----------|------|--------|
| BUG-16 | CRITICAL | Concurrency | cow/cow.go | ✅ |
| BUG-17 | CRITICAL | Data Corruption | memory/manager.go | ✅ |
| BUG-18 | HIGH | Data Race | memory/manager.go | ✅ |
| BUG-19 | CRITICAL | Panic | api/server.go | ✅ |
| BUG-20 | CRITICAL | Panic | api/websocket.go | ✅ |
| BUG-21 | MEDIUM | Goroutine Leak | api/websocket.go | ✅ |
| BUG-22 | HIGH | Logic | algorithms/car.go | ✅ |

---

## BUG-16 — HandleWrite TOCTOU: unlock before `unsharePageInternal` re-lock

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Concurrency / TOCTOU |
| **File** | `internal/cow/cow.go:97–100` |
| **Status** | Fixed ✅ |

**Description:** After BUG-01's fix, `HandleWrite` still released `cow.mu` before calling `unsharePageInternal`, which immediately re-acquired it. Between the unlock and re-lock, a concurrent `ForkProcess` could add a new sharer; the process that decided it was the "last reference" would then write directly to the shared frame, corrupting the new sharer's view.

**Fix:** Inline the unshare logic directly under the already-held `cow.mu.Lock()`, eliminating the unlock-relock window entirely.

---

## BUG-17 — Compressed path missing CoW check for writes to shared pages

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Data Corruption |
| **File** | `internal/memory/manager.go:342–357` |
| **Status** | Fixed ✅ |

**Description:** When `compressionEnabled` is true and a page is decompressed (fault handler enters the compressed branch), the function returned `nil` unconditionally without checking `write && page.IsShared()`. A write to a shared compressed page bypassed `handleCoW` entirely, letting the writer modify the shared frame and silently corrupting other processes' view. Also, the NUMA node assignment was missing from the `AllocateFrame`-succeeds branch inside the compressed path.

**Fix:** Added `if write && page.IsShared()` CoW dispatch and NUMA node stamp before the `return nil` in the compressed branch.

---

## BUG-18 — Data race on `mm.eventCallback` in `eventWorker`

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Data Race |
| **File** | `internal/memory/manager.go:625–649` |
| **Status** | Fixed ✅ |

**Description:** `eventWorker` (goroutine, no lock) and `emitEvent` (called under various states of `mm.mu`) both read `mm.eventCallback` without synchronization. `SetEventCallback` wrote it under `mm.mu.Lock()`. The Go race detector flags this because `eventWorker` never acquires `mm.mu`.

**Fix:** Added `eventCallbackMu sync.RWMutex` — a dedicated lock whose only job is protecting `mm.eventCallback`. `SetEventCallback` uses `eventCallbackMu.Lock()`. Both `emitEvent` and `eventWorker` read under `eventCallbackMu.RLock()`. Lock order `mm.mu → eventCallbackMu` is safe since `eventCallbackMu` is never held when `mm.mu` is acquired.

---

## BUG-19 — WebSocket double-close panic in `handleBroadcast`

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Panic / Concurrency |
| **File** | `pkg/api/server.go:67–85` |
| **Status** | Fixed ✅ |

**Description:** `handleBroadcast` closed `client.send` under `clientsMu.RLock()` (line 72) and only deleted the client from the map later under `clientsMu.Lock()`. Between those two operations, `UnregisterClient` could acquire `clientsMu.Lock()`, find the client still in the map, and close the already-closed channel → `close of closed channel` panic.

**Fix:** Removed the close from the RLock section. Dead clients are now only closed+deleted inside the WLock section with an existence check, mirroring `UnregisterClient`.

---

## BUG-20 — WebSocket initial-state goroutine sends to closed channel

| Field | Value |
|-------|-------|
| **Severity** | CRITICAL |
| **Category** | Panic |
| **File** | `pkg/api/websocket.go:50–56` |
| **Status** | Fixed ✅ |

**Description:** `HandleWebSocket` spawns a goroutine that sleeps 100 ms then sends to `client.send`. If the client disconnects within that window, `UnregisterClient` closes `client.send`. The goroutine wakes up and sends to the closed channel → panic.

**Fix:** The goroutine now uses two nested `select` statements: one to wait out the timer (or abort on `client.done` closure), and one to send (or abort if `client.done` closed first).

---

## BUG-21 — Unlimited `sendPeriodicMetrics` goroutines per WebSocket client

| Field | Value |
|-------|-------|
| **Severity** | MEDIUM |
| **Category** | Goroutine Leak |
| **File** | `pkg/api/websocket.go:144–146` |
| **Status** | Fixed ✅ |

**Description:** Every `subscribe_metrics` WebSocket message spawned a new `sendPeriodicMetrics` goroutine with no deduplication. A replaying or misbehaving client could accumulate unbounded goroutines, all writing to the same `client.send` channel.

**Fix:** Added `metricsStarted int32` atomic field to `Client`. `handleMessage` uses `atomic.CompareAndSwapInt32` to ensure only the first `subscribe_metrics` message per connection spawns the goroutine.

---

## BUG-22 — CAR `SelectVictim` loop bound shrinks as T1 entries are promoted

| Field | Value |
|-------|-------|
| **Severity** | HIGH |
| **Category** | Logic / Algorithm |
| **File** | `internal/algorithms/car.go:56–78` |
| **Status** | Fixed ✅ |

**Description:** The T1 scan loop used `len(c.t1)*2` as the bound, but Go re-evaluates this expression on every iteration. As entries with `refBit=true` are promoted to T2 (via `moveT1ToT2` → `removeT1`), `len(c.t1)` decreases, shrinking the effective bound mid-scan. The scan exits early, leaving valid clean eviction candidates in T1 permanently skipped. The same bug existed in the T2 scan.

**Fix:** Captured `maxT1Iters := len(c.t1) * 2` (and `maxT2Iters`) before each loop so the bound is fixed for the full scan. Added `if len(c.t1) == 0 { break }` guard inside the loop to handle the case where all entries are promoted.
