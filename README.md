# Page Replacement Simulator + Copy-on-Write

[![CI](https://github.com/sanskarpan/page-replacement-simulator-cow/actions/workflows/ci.yml/badge.svg)](https://github.com/sanskarpan/page-replacement-simulator-cow/actions/workflows/ci.yml)
[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/dl/)

A virtual memory simulator implementing 12 page replacement algorithms and Copy-on-Write (CoW) semantics. Includes a real-time web UI, REST API, WebSocket streaming, CLI, and a comprehensive test suite with race-detector coverage.

## Table of contents

- [Algorithms](#algorithms)
- [Features](#features)
- [Architecture](#architecture)
- [Installation](#installation)
- [Usage](#usage)
- [API Reference](#api-reference)
- [Testing](#testing)
- [Performance](#performance)
- [Contributing](#contributing)

## Algorithms

| Algorithm | Strategy |
|-----------|----------|
| **LRU** | Evicts the page unused longest |
| **CLOCK** | Second-chance circular buffer with reference bits |
| **LFU** | Evicts the least-accessed page |
| **FIFO** | Evicts the oldest resident page |
| **Optimal** | Bélády's theoretically optimal offline algorithm |
| **OPT+** | Lookahead-enhanced optimal with future-access hints |
| **ARC** | Adaptive Replacement Cache — self-tuning recency/frequency balance |
| **CAR** | Clock with Adaptive Replacement — ARC with CLOCK-style clocks |
| **WSClock** | Working Set Clock — combines working-set age with clock hand |
| **NRU** | Not Recently Used — 4-class eviction by reference/dirty bits |
| **PFF** | Page Fault Frequency — adjusts resident-set size dynamically |
| **Random** | Random victim selection — baseline for comparison |

## Features

### Memory subsystem
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
- Write triggers a physical copy; reference counting lets the last writer take ownership without copying
- `HandleWrite` guards against spurious copies for processes not registered as page sharers

### Observability
- Structured JSON logging via `log/slog`
- `GET /api/metrics` — live counters including `dropped_events` (events lost to channel backpressure)
- `GET /api/history` — rolling metric snapshots
- `WS /api/ws` — real-time WebSocket event stream (page faults, CoW copies, evictions, process lifecycle)

### CLI
- Single-scenario run, 12-algorithm head-to-head comparison, and Bélády-curve frame-count sweep
- `--output text|json|csv` for pipeline-friendly output

## Architecture

```
page-replacement-simulator-cow/
├── cmd/
│   ├── server/          # HTTP server + web UI entry point
│   └── cli/             # Command-line interface entry point
├── internal/
│   ├── memory/          # MemoryManager, FrameTable, TLB, PageTable, compression, clustering
│   ├── algorithms/      # 12 page replacement algorithm implementations
│   ├── cow/             # Copy-on-Write manager and reference counter
│   ├── process/         # ProcessManager: lifecycle, fork, memory dispatch
│   ├── simulator/       # Scenario runner, trace save/replay, comparison engine
│   └── monitor/         # System status aggregation
├── pkg/
│   ├── models/          # Core types (Page, Frame, Process, Metrics, …)
│   └── api/             # HTTP handlers, WebSocket hub, routing
├── web/static/          # Vanilla JS + D3.js frontend
└── tests/
    ├── unit/            # ~25 focused unit test files
    ├── integration/     # 20 end-to-end tests
    └── benchmark/       # 7 benchmarks
```

## Installation

Requires **Go 1.25+**.

```bash
git clone https://github.com/sanskarpan/page-replacement-simulator-cow.git
cd page-replacement-simulator-cow
go mod download

go build -o bin/server ./cmd/server
go build -o bin/cli    ./cmd/cli
```

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

**Available scenarios:** `sequential` · `random` · `locality` · `looping` · `mixed` · `fork_cow` · `thrashing`

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
| `POST` | `/api/processes` | Create process — `{name, priority, virtual_pages}` |
| `GET` | `/api/processes/{id}` | Process detail (atomic fields serialized as numbers) |
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
| `POST` | `/api/memory/access` | Access a page — `{process_id, virtual_page, write}` |
| `GET` | `/api/memory/frames` | Frame table details |
| `POST` | `/api/memory/algorithm` | Switch replacement algorithm live |

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

### Feature toggles
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/feature/toggle` | Enable/disable NUMA, compression, clustering — `{feature, enabled}` |

### WebSocket

```
WS /api/ws
```

Events: `initial_state`, `memory_access`, `page_fault`, `page_eviction`, `cow_copy`, `process_created`, `process_forked`, `process_removed`, `system_reset`, `huge_page_mapped`.

### Quick examples

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

# Benchmarks
go test -bench=. -benchmem ./tests/benchmark/
```

| Suite | Coverage |
|-------|----------|
| `tests/unit/` | ~25 files — algorithm correctness, compression, CoW, prefetch, clustering |
| `tests/integration/` | 20 end-to-end tests — memory access, page replacement, CoW chains, TLB, all algorithms |
| `tests/benchmark/` | 7 benchmarks — LRU/CLOCK/LFU/FIFO throughput, TLB lookup, CoW fork |

## Performance

Measured on Apple M3 Pro with race detector enabled:

| Benchmark | Throughput |
|-----------|------------|
| Memory access (LRU, 128 frames) | ~144k ops/s |
| CLOCK algorithm | ~295k ops/s |
| TLB lookup (warm cache) | ~354k ops/s |
| CoW fork + 10 writes | ~623 forks/s |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions, coding guidelines, and the PR process.

To report a security vulnerability, see [SECURITY.md](SECURITY.md).
