# Phase Four Completion Report: SSA Register Allocation & IR Optimization

## 1. Overview & Completion

Phase Four of the optimizing compiler and M6809 backend is **complete**. Guided by modern SSA register allocation theory (decoupled spilling, chordal graph coloring, and parallel copy sequentialization) and whole-program IR optimization (dominator analysis, natural loop induction, LICM, and store-to-load forwarding), all five planned milestones have been implemented and verified.

The test suite achieves **100% PASS** across all three backends (CBE, AMD64, M6809) and across all 8 M6809 architectural variants (`default`, `globals-at-y`, `frame-pointer`, `pic`, and all combinations).

### Phase Four Architecture & Deliverables:
- **Milestone 4.1: Foundation, Liveness Analysis & Register Bitmasks (`m6809/reg.go`, `m6809/liveness.go`, `opt/liveness.go`)**:
  - Exact bitmask modeling of M6809 overlapping register classes (`RegA`, `RegB`, `RegD = RegA|RegB`, `RegX`, `RegY`, `RegU`, `RegS`, `RegCC`).
  - Variant-aware register availability filtering (`AllocatableRegisters`).
  - Dominator-aware SSA live interval and register pressure computation.
  - Guard: `-no-liveness6809` (`NO_LIVENESS6809`).
- **Milestone 4.2: Local Basic-Block Register Allocation (`m6809/backend.go`)**:
  - Intra-block tracking of values resident in accumulators `D` and `B`.
  - Immediate reuse and elimination of redundant stack loads (`ldd slot,s`) and dead stack stores.
  - Safe register-to-register address transfers (`tfr d,x`).
  - Guard: `-no-local-regalloc6809` (`NO_LOCAL_REGALLOC6809`).
- **Milestone 4.3: Natural Loop Analysis, LICM & Store-to-Load Forwarding (`opt/loop.go`, `opt/licm.go`, `opt/store_load.go`)**:
  - CFG Dominator analysis and natural loop discovery ($O(N)$ back-edge identification).
  - Loop Invariant Code Motion (LICM): Hoisting pure invariant computations to loop preheaders across all backends.
  - Intra-block store-to-load forwarding with strict operand-level escape analysis.
  - Guards: `-no-licm` (`NO_LICM`), `-no-store-load` (`NO_STORE_LOAD`).
- **Milestone 4.4: Conventional SSA ($\Phi$-Elimination) & Parallel Copy Resolution (`opt/parallel_copy.go`, `m6809/backend.go`)**:
  - Formal decomposition of simultaneous parallel copies at basic-block boundaries into sequential moves, swaps, and scratch saves/restores.
  - Direct synthesis of hardware `exg` instructions for 2-cycles (`exg d, x`, `exg a, b`, `exg u, y`).
  - Zero stack-frame overhead for acyclic copy chains and 2-cycles.
  - Guard: `-no-cssa-lowering6809` (`NO_CSSA_LOWERING6809`).
- **Milestone 4.5: Global SSA Chordal Coloring & Spilling (`opt/chordal.go`, `m6809/regalloc.go`, `m6809/backend.go`)**:
  - Maximum Cardinality Search (MCS) simplicial elimination ordering ($O(V + E)$).
  - Greedy chordal graph coloring with capacity constraints and register preferencing.
  - Decoupled spilling heuristic using Belady's furthest-next-use distance.
  - Dedicated leaf function allocation for index registers `U` and `Y`, prioritizing pointer variables and memory dereferences (`ldd ,u`, `std ,u`, `ldb ,u`).
  - Guard: `-no-global-regalloc6809` (`NO_GLOBAL_REGALLOC6809`).

---

## 2. Telemetry & Benchmark Results

### A. Milestone 4.4 $\rightarrow$ Milestone 4.5 Impact

Telemetry comparison between **Milestone 4.4** (`perf-2026-09-14-223500-milestone-4.4-*`) and **Milestone 4.5 Final** (`perf-2026-09-15-002500-milestone-4.5-*`):

| Configuration | Code Size (M4.4 $\rightarrow$ M4.5) | Run Cycles (M4.4 $\rightarrow$ M4.5) | Regressions |
|---|---|---|---|
| **Default Configuration** | 326,346 $\rightarrow$ 324,226 bytes (**-2,120 B, -0.65%**) | 29,906,126 $\rightarrow$ 29,742,806 cycles (**-163,320 cycles, -0.55%**) | **0** |
| **All 8 Variants Matrix** | 2,029,080 $\rightarrow$ 2,025,699 bytes (**-3,381 B, -0.17%**) | 113,350,462 $\rightarrow$ 112,798,520 cycles (**-551,942 cycles, -0.49%**) | **0** |

#### Top Benchmark Gains in Milestone 4.5:
- `test_arcfour`: **-43,396 cycles (-7.52%)**, **-134 bytes (-2.56%)**
- `jun26_whole-collatz`: **-42,189 cycles (-0.32%)**, **-253 bytes (-1.87%)**
- `test_regexp`: **-37,791 cycles (-0.45%)**, **-109 bytes (-0.72%)**
- `test_8queens`: **-9,752 cycles (-1.86%)**
- `forth_count`: **-7,553 cycles (-1.69%)**, **-108 bytes (-0.89%)**
- `test_big_mul`: **-2,780 cycles (-1.13%)**, **-106 bytes (-1.23%)**
- `lisp_2`: **-2,640 cycles (-1.14%)**

---

### B. Cumulative Phase Four Results (Phase 3 Baseline $\rightarrow$ Phase 4 Complete)

Telemetry comparison between **Phase Three Complete** (`perf-2026-09-14-190049-*` / `perf-2026-09-14-190145-*`) and **Phase Four Complete** (`perf-2026-09-15-002500-*`):

| Metric | Phase Three Baseline | Phase Four Complete | Absolute Delta | Percentage Delta |
|---|---|---|---|---|
| **Default Code Size** | 352,098 bytes | 324,226 bytes | -27,872 bytes | **-7.92%** |
| **Default Run Cycles** | 31,585,853 cycles | 29,742,806 cycles | -1,843,047 cycles | **-5.83%** |
| **All-Variants Code Size** | 2,227,206 bytes | 2,025,699 bytes | -201,507 bytes | **-9.05%** |
| **All-Variants Run Cycles** | 119,893,166 cycles | 112,798,520 cycles | -7,094,646 cycles | **-5.92%** |

#### Cumulative Top Cycle Count Reductions:
- `test_regexp`: 9,149,590 $\rightarrow$ 8,350,793 cycles (**-798,797 cycles, -8.73%**)
- `test_arcfour`: 601,897 $\rightarrow$ 533,714 cycles (**-68,183 cycles, -11.33%**)
- `test_8queens`: 570,374 $\rightarrow$ 515,003 cycles (**-55,371 cycles, -9.71%**)
- `test_primes`: 282,273 $\rightarrow$ 253,518 cycles (**-28,755 cycles, -10.19%**)
- `arcfour`: 438,627 $\rightarrow$ 411,954 cycles (**-26,673 cycles, -6.08%**)
- `forth_count`: 453,866 $\rightarrow$ 440,649 cycles (**-13,217 cycles, -2.91%**)
- `lisp_2`: 239,268 $\rightarrow$ 229,536 cycles (**-9,732 cycles, -4.07%**)
- `basic_count10`: 481,227 $\rightarrow$ 471,689 cycles (**-9,538 cycles, -1.98%**)
- `joy_2`: 572,395 $\rightarrow$ 564,635 cycles (**-7,760 cycles, -1.36%**)
- `joy_1`: 594,731 $\rightarrow$ 587,943 cycles (**-6,788 cycles, -1.14%**)

#### Cumulative Top Code Size Reductions:
- `test_regexp`: 17,276 $\rightarrow$ 14,964 bytes (**-2,312 bytes, -13.38%**)
- `forth_count`: 14,035 $\rightarrow$ 11,985 bytes (**-2,050 bytes, -14.61%**)
- `joy_1`: 18,003 $\rightarrow$ 16,282 bytes (**-1,721 bytes, -9.56%**)
- `test_sort`: 15,662 $\rightarrow$ 14,027 bytes (**-1,635 bytes, -10.44%**)
- `basic_count10`: 17,144 $\rightarrow$ 15,513 bytes (**-1,631 bytes, -9.51%**)
- `joy_2`: 16,591 $\rightarrow$ 15,007 bytes (**-1,584 bytes, -9.55%**)
- `test_smap`: 13,313 $\rightarrow$ 11,916 bytes (**-1,397 bytes, -10.49%**)
- `jun26_whole-collatz`: 14,575 $\rightarrow$ 13,264 bytes (**-1,311 bytes, -8.99%**)
- `test_append`: 9,715 $\rightarrow$ 8,536 bytes (**-1,179 bytes, -12.14%**)
- `test_buf`: 11,561 $\rightarrow$ 10,542 bytes (**-1,019 bytes, -8.81%**)

---

## 3. Key Architectural Insights for M6809 Global Register Allocation

1. **Non-Orthogonal Register File Specialization**:
   - The M6809 ALU instructions (`addd`, `subd`, `cmpd`) only accept immediate or memory effective addresses—never register operands.
   - Forcing short-lived integer variables into physical registers `U` and `Y` requires push/pop register staging (`tfr y,x; pshs x; cmpd ,s++`), introducing significant cycle overhead relative to direct stack slots (`cmpd 24,s`).
   - Conversely, `U` and `Y` are dedicated 16-bit **Index Registers**. Allocating pointer variables and array base addresses to `U` and `Y` enables direct dereferencing (`ldd ,u` / `std ,u`), eliminating scratch loads to `X` and yielding massive speedups (-7.5% on `test_arcfour`).

2. **Chordal Interference Graphs for Global Colorability**:
   - Maximum Cardinality Search (MCS) finds a perfect elimination ordering in linear time $O(V + E)$ without NP-hard backtracking.
   - Constraining candidates by capacity and preferencing registers by class ensures that color assignments always satisfy physical constraints without spilled spills.

3. **CSSA Parallel Copy Resolution with Hardware `EXG`**:
   - $\Phi$-nodes across control flow boundaries lower cleanly into parallel moves.
   - Decomposing transfer cycles into 2-cycle swaps emits single-instruction hardware exchanges (`exg d, x`, `exg u, y`), avoiding stack spilling entirely in leaf functions.

---

## 4. Verification & Quality Assurance

- **Unit Test Suites**: `go test ./opt/... ./m6809/...` passed 100%.
- **All Golf Tests**: `TestSystemAllGolfFiles` passed 100% across all 87 tests.
- **C Test Suite**: `TestAllCFiles` passed 100% across all translated C benchmarks.
- **8-Variant Matrix**: `ALL_VARIANTS=1 go test -run TestSystemAllVariants_m6809 .` passed 100% across all 8 flag configurations.
- **Triage Isolation**: All optimizations are individually guarded by flags (`-no-licm`, `-no-store-load`, `-no-cssa-lowering6809`, `-no-global-regalloc6809`) and corresponding environment variables.
