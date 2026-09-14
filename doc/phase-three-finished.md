# Phase Three Completion Report: Optimizations, Inlining & Telemetry

## 1. Overview & Completion
Phase Three of the optimizing M6809 code generator is **complete**. All features have been implemented, all tests pass across all targets and compilation variants, and benchmark telemetry demonstrates substantial gains in both code density and runtime execution speed.

Key optimizations delivered in Phase Three:
- **Whole-Program Function Inlining (`opt/inline.go`)**: Inlining of tiny leaf functions (`peekb`, `pokeb`, `peekw`, `pokew`, `vpeekb`, `vpeekw`) and single-callsite functions in `-fwhole-program` style, while strictly respecting safety boundaries (skipping `defer`, destructors, `setjmp`/`longjmp`, and escaped local addresses).
- **Branch Layout & Condition Inversion (`m6809/backend.go` & `m6809/peephole.go`)**: Inverting branch conditions to fall through directly to the sequentially next basic block; eliminating jumps over jumps and redundant unconditional branches.
- **Fused Compare + Branch (`m6809/backend.go`)**: Fusing single-use comparison operations directly into conditional branch instructions (`cmpd`/`cmpb`/`tstb` directly followed by branch, bypassing boolean materialization and stack stores).
- **Leaf Function Stack Frame Omission (`m6809/backend.go`)**: Omitting hardware frame pointer setup/teardown (`pshs u` / `puls u,pc`) for leaf functions requiring no local stack or parameter frames.
- **Granular Triage Flags (`main.go` & `m6809/backend.go`)**: CLI flags and environment variables for every individual pass (`-no-inline`, `-no-inline-tiny`, `-no-inline-single-call`, `-no-branch-layout6809`, `-no-fused-compares6809`, `-no-leaf-opt6809`).

---

## 2. Telemetry & Benchmark Results

Telemetry comparison between **Phase Two** (`perf-2026-09-14-130828-phase-two-complete` / `perf-2026-09-14-130940-phase-two-variants`) and **Phase Three** (`perf-2026-09-14-190049-phase-three-complete` / `perf-2026-09-14-190145-phase-three-variants`):

| Metric | Phase Two Baseline | Phase Three Final | Absolute Delta | Percentage Delta |
|---|---|---|---|---|
| **Default Code Size** | 443,493 bytes | 359,687 bytes | -83,806 bytes | **-18.90%** |
| **Default Run Cycles** | 48,653,411 cycles | 30,752,725 cycles | -17,900,686 cycles | **-36.79%** |
| **All-Variants Code Size** | 2,710,645 bytes | 2,227,206 bytes | -483,439 bytes | **-17.83%** |
| **All-Variants Run Cycles** | 224,426,372 cycles | 119,893,166 cycles | -104,533,206 cycles | **-46.58%** |

### Top Execution Cycle Reductions:
- `a3`: 3,021,266 $\rightarrow$ 384,171 cycles (**-87.28%**)
- `joy_2`: 2,699,061 $\rightarrow$ 572,395 cycles (**-78.79%**)
- `joy_1`: 1,621,227 $\rightarrow$ 594,731 cycles (**-63.32%**)
- `test_buf`: 1,202,285 $\rightarrow$ 456,980 cycles (**-61.99%**)
- `basic_count10`: 1,219,426 $\rightarrow$ 481,227 cycles (**-60.54%**)
- `test_sort`: 944,313 $\rightarrow$ 392,020 cycles (**-58.49%**)
- `test_smap`: 1,286,656 $\rightarrow$ 659,310 cycles (**-48.76%**)
- `test_sort_strings`: 1,732,243 $\rightarrow$ 1,074,455 cycles (**-37.97%**)
- `jun26_whole-collatz`: 15,251,969 $\rightarrow$ 13,157,508 cycles (**-13.73%**)
- `test_regexp`: 10,288,929 $\rightarrow$ 9,149,590 cycles (**-11.07%**)

### Top Code Size Reductions:
- `basic_count10`: 21,382 $\rightarrow$ 17,144 bytes (**-19.82%**)
- `test_append`: 11,713 $\rightarrow$ 9,715 bytes (**-17.06%**)
- `lisp_2`: 12,414 $\rightarrow$ 10,369 bytes (**-16.47%**)
- `test_buf`: 13,723 $\rightarrow$ 11,561 bytes (**-15.75%**)
- `test_sort`: 18,526 $\rightarrow$ 15,662 bytes (**-15.46%**)
- `test_smap`: 15,468 $\rightarrow$ 13,313 bytes (**-13.93%**)
- `forth_count`: 16,305 $\rightarrow$ 14,035 bytes (**-13.92%**)
- `joy_2`: 19,194 $\rightarrow$ 16,591 bytes (**-13.56%**)
- `jun26_whole-collatz`: 16,773 $\rightarrow$ 14,575 bytes (**-13.10%**)
- `joy_1`: 20,688 $\rightarrow$ 18,003 bytes (**-12.98%**)

---

## 3. Key Findings & Bug Fixes

### 1. Unsigned Zero Comparisons and M6809 `TST` Semantics
- **Bug**: In `test_big_mul.golf` (`big.Div2`), a loop counter `i > 0` was emitted as `tstb` followed by `bhi` (inverted to `lbls`). In Motorola 6809, `TST` updates N and Z and clears V, but **does not affect the Carry flag (C)**.
- **Impact**: Any unsigned branch testing the carry bit (`bhi`, `bls`, `blo`, `bhs`) after a `tstb` was evaluating stale carry flags from preceding arithmetic operations (`subb` or `cmpb`), causing loops to exit prematurely when C happened to be 1.
- **Fix**: For unsigned values compared against zero, $x > 0 \iff x \neq 0$ (`bne` / `lbne`) and $x \le 0 \iff x == 0$ (`beq` / `lbeq`). Both `emitCompare` and `emitTerminator` now properly map unsigned zero comparisons to Z-flag conditional branches.

### 2. Fused Compare Cast Unwrapping
- **Bug**: When checking if a condition was an equality test against zero (`cmp2: innerCmp != 0`), `emitFunc` unwrapped intermediate `Cast` instructions and marked both the casts and `innerCmp` as fused. However, `emitTerminator` did not unwrap casts when resolving `cmp2.Left`, failing to identify `innerCmp`.
- **Impact**: `emitTerminator` attempted to load the result of the cast from its local stack slot, but that cast instruction was skipped by `emitInstr`, leading to reading uninitialized memory.
- **Fix**: Updated `emitTerminator` to unwrap casts consistently with `emitFunc`.

### 3. Stack Slot Allocation for Memory-Addressed Constants
- **Bug**: An optimization pass naively skipped allocating stack slots for `ConstByte`, `ConstWord`, and `Sizeof`.
- **Impact**: When instructions like `AddressOfLocal`, struct insertions, or memory copies invoked `getAddrStr()`, `localAddr()` panicked because no slot was registered.
- **Fix**: Preserved stack slots for instructions that require addressable storage.

---

## 4. Verification & Quality Assurance
- **Full Test Suite (`go test .`)**: Passes 100% across CBE, AMD64, and M6809 targets.
- **8-Variant Matrix (`ALL_VARIANTS=1 go test -run TestSystemAllVariants_m6809`)**: Passes 100% across all 8 compiler flag configurations (`globals-at-y`, `frame-pointer`, `pic`, and all combinations).
- **Bug Triage Flags**: Verified that all passes can be selectively toggled on/off without compiler failure.
