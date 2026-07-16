# Page Replacement Simulator + Copy-on-Write

A production-grade virtual memory simulator implementing 12 page replacement algorithms and Copy-on-Write (CoW) semantics. Includes a real-time web UI, REST API, WebSocket streaming, CLI, and a comprehensive test suite with race-detector coverage.

## Algorithms

| Algorithm | Description |
|-----------|-------------|
| **LRU** | Least Recently Used — evicts the page unused longest |
| **CLOCK** | Second-chance circular buffer with reference bits |
| **LFU** | Least Frequently Used — evicts the least-accessed page |
| **FIFO** | First-In First-Out — evicts the oldest resident page |
| **Optimal** | Bélády's theoretically optimal offline algorithm |
| **OPT+** | Lookahead-enhanced optimal with future-access hints |
| **ARC** | Adaptive Replacement Cache — self-tuning recency/frequency balance |
| **CAR** | Clock with Adaptive Replacement — ARC with CLOCK-style clocks |
| **WSClock** | Working Set Clock — combines working-set age with clock hand |
| **NRU** | Not Recently Used — 4-class eviction by reference/dirty bits |
| **PFF** | Page Fault Frequency — adjusts resident-set size dynamically |
| **Random** | Random victim selection — baseline for comparison |

## Features

### Memory Subsystem
- **Virtual memory** — per-process page tables with Present, Dirty, and Reference bits
- **Physical frames** — configurable pool with LRU TLB for fast address translation
- **4-level page table** — x86-64 style multi-level PT with huge-page (2 MB) support
- **NUMA-aware allocation** — per-node frame ranges with access-cost estimation
- **Memory compression** — transparent page compression on eviction, decompression on fault
- **Page clustering / prefetch** — sequential-pattern detection with look-ahead prefetch (per-process, race-safe)
- **Working set model** — sliding-window working-set tracking per process
- **Thrashing detection** — rolling fault-rate analysis with configurable threshold

### Copy-on-Write
- Fork-based CoW: child shares parent pages marked read-only
- Write triggers physical copy; reference counting ensures last writer takes ownership without copying
- Cross-process race safety: `HandleWrite` guards against spurious copies for unregistered processes

### Observability
- Structured JSON logging via `log/slog`
- `GET /api/metrics` — live counters including `dropped_events` (events lost due to channel backpressure)
- `GET /api/history` — rolling metric snapshots
- `WS /api/ws` — real-time WebSocket event stream (process lifecycle, page faults, CoW copies, evictions)

### CLI
- Single-scenario run, 12-algorithm head-to-head comparison, and Bélády-curve frame-count sweep
- `--output text|json|csv` for CI-friendly output

## Security & Reliability

This codebase has been hardened through a 70-point audit. Key fixes include:

- All data races and TOCTOU bugs eliminated (verified with `go test -race`)
- Atomic fields carry `json:"-"` tags; process/frame state is serialized through DTOs, not raw structs
- Compressed-page data preserved on OOM fault-in failure (`RestoreCompressed`)
- `CoW.HandleWrite` and `PageTable.Clone` are atomic under their respective mutexes
- Lock order enforced throughout (`pm.mu → parent.mu`, never reversed)
- Trace files written with `0600` permissions
- Event channel backpressure counted in metrics, never blocks callers

## Architecture

```
Page-Replacement-Simulator-CoW/
├── cmd/
│   ├── server/          # HTTP server + web UI
│   └── cli/             # Command-line interface
├── internal/
│   ├── memory/          # MemoryManager, FrameTable, TLB, PageTable, advanced features
│   ├── algorithms/      # 12 page replacement algorithm implementations
│   ├── cow/             # Copy-on-Write manager and reference counter
│   ├── process/         # ProcessManager: lifecycle, fork, memory dispatch
│   ├── simulator/       # Scenario runner, trace save/replay, comparison engine
│   └── monitor/         # System status aggregation
├── pkg/
│   ├── models/          # Core value types (Page, Frame, Process, Metrics, …)
│   └── api/             # HTTP handlers, WebSocket hub, routing
├── web/static/          # Vanilla JS + D3.js frontend
└── tests/
    ├── unit/            # ~25 focused unit test files
    ├── integration/     # 20 end-to-end tests with defer mm.Close()
    └── benchmark/       # 7 benchmarks, all race-detector clean
```

## Installation

```bash
git clone https://github.com/sanskarpan/page-replacement-cow.git
cd page-replacement-cow
go mod download

go build -o bin/server ./cmd/server
go build -o bin/cli    ./cmd/cli
```

Requires **Go 1.22+**.

## Usage

### Web server

```bash
./bin/server -port 8080 -frames 128 -tlb 16 -algorithm LRU
# open http://localhost:8080
```

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | HTTP listen port |
| `-frames` | `128` | Physical frame count |
| `-tlb` | `16` | TLB capacity |
| `-algorithm` | `LRU` | Starting algorithm |

### CLI

```bash
# Run a single scenario
./bin/cli -algorithm LRU -scenario mixed -frames 64

# Compare all 12 algorithms on the same workload
./bin/cli -compare -scenario mixed -frames 64 -output csv

# Bélády curve: sweep frame counts from 8 to 256
./bin/cli -frames-sweep -algorithm LRU -scenario mixed \
          -frame-min 8 -frame-max 256 -output json
```

### Available scenarios

`sequential` · `random` · `locality` · `looping` · `mixed` · `fork_cow` · `thrashing`

## API Reference

### System
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/status` | System status snapshot |
| `GET` | `/api/metrics` | Live counters (includes `dropped_events`) |
| `GET` | `/api/history?last=N` | Rolling metric history |
| `GET` | `/api/events?last=N` | Recent event log |
| `POST` | `/api/reset` | Reset all state |

### Processes
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/processes` | List all processes |
| `POST` | `/api/processes` | Create process `{name, priority, virtual_pages}` |
| `GET` | `/api/processes/{id}` | Process detail (DTO — atomic fields as numbers) |
| `DELETE` | `/api/processes/{id}` | Terminate process |
| `POST` | `/api/processes/{id}/fork` | Fork process (CoW) |
| `GET` | `/api/processes/{id}/pages` | Page table entries |
| `POST` | `/api/processes/{id}/hugepage` | Map a huge page |
| `GET` | `/api/processes/{id}/hugepages` | Huge page mappings |
| `GET` | `/api/processes/{id}/workingset` | Working set size and window |
| `GET` | `/api/processes/{id}/mpt` | Multi-level page table dump |

### Memory
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/memory/access` | `{process_id, virtual_page, write}` |
| `GET` | `/api/memory/frames` | Frame details |
| `POST` | `/api/memory/algorithm` | Switch algorithm live |

### Stats
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/tlb/stats` | TLB hit/miss counts |
| `GET` | `/api/cow/stats` | CoW copies, savings, ref counts |
| `GET` | `/api/numa/stats` | NUMA node info |
| `GET` | `/api/compression/stats` | Compression ratio, page counts |

### Simulation
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/simulation/scenarios` | Available scenario names |
| `POST` | `/api/simulation/run` | `{scenario, num_frames, tlb_size}` |
| `POST` | `/api/simulation/compare` | 12-algorithm head-to-head |
| `POST` | `/api/simulation/sweep` | Bélády curve across frame counts |
| `GET` | `/api/simulation/thrashing` | Live thrashing status |

### Features
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/feature/toggle` | Enable/disable NUMA, compression, clustering |

### WebSocket
```
WS /api/ws
```
Events: `initial_state`, `memory_access`, `page_fault`, `page_eviction`, `cow_copy`, `process_created`, `process_forked`, `process_removed`, `system_reset`, `huge_page_mapped`.

### Quick curl examples

```bash
BASE=http://localhost:8080/api

# Create a process
curl -sX POST $BASE/processes \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo","priority":1,"virtual_pages":1000}'

# Access a page (write)
curl -sX POST $BASE/memory/access \
  -H 'Content-Type: application/json' \
  -d '{"process_id":"P1","virtual_page":42,"write":true}'

# Switch to ARC
curl -sX POST $BASE/memory/algorithm \
  -H 'Content-Type: application/json' \
  -d '{"algorithm":"ARC"}'

# Enable compression
curl -sX POST $BASE/feature/toggle \
  -H 'Content-Type: application/json' \
  -d '{"feature":"compression","enabled":true}'
```

## Testing

```bash
# Full suite
go test ./...

# With race detector (recommended)
go test -race ./...

# Specific suites
go test -race ./tests/unit/...
go test -race ./tests/integration/...

# Benchmarks (includes CoW, TLB, and all algorithm benchmarks)
go test -bench=. -race ./tests/benchmark/
```

### Test structure

| Suite | Files | Focus |
|-------|-------|-------|
| `tests/unit/` | ~25 files | Algorithm correctness, race conditions, compression, CoW, prefetch, clustering |
| `tests/integration/` | 1 file, 20 tests | End-to-end: memory access, page replacement, CoW chains, TLB, all algorithms |
| `tests/benchmark/` | 1 file, 7 benchmarks | Memory access, LRU/CLOCK/LFU/FIFO throughput, TLB lookup, CoW fork |

All integration tests use `defer mm.Close()` to prevent goroutine leaks. All benchmarks carry error checks on `ForkProcess`.

## Performance

Measured on Apple M3 Pro with race detector enabled:

| Benchmark | Throughput |
|-----------|------------|
| Memory access (LRU, 128 frames) | ~144k ops/s |
| CLOCK algorithm | ~295k ops/s |
| TLB lookup (warm cache) | ~354k ops/s |
| CoW fork + 10 writes | ~623 forks/s |

## Contributing

1. Fork the repository and create a feature branch
2. Add tests — unit tests for new logic, integration tests for new end-to-end paths
3. Run `go test -race ./...` and confirm clean
4. Submit a pull request with a clear description of what changed and why
