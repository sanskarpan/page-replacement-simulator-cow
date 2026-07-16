# Changelog

## [Unreleased]

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
