# Phase Four Design & Implementation Plan: SSA Register Allocation for M6809

## 1. Executive Summary & Architecture Context

The Motorola 6809 is an asymmetric, non-orthogonal 8-bit/16-bit microprocessor with a very compact register set:
- **Accumulators**:
  - `A` (8-bit high byte)
  - `B` (8-bit low byte)
  - `D` (16-bit composite accumulator: `A:B`)
- **Index / Pointer Registers**:
  - `X` (16-bit index register)
  - `Y` (16-bit index register — reserved when `-globals-at-y` is active)
  - `U` (16-bit user stack / index register — reserved as hardware frame pointer when `-frame-pointer` is active)
- **Hardware Stack**:
  - `S` (16-bit system stack pointer, permanently reserved for call frames)
- **Condition Codes**:
  - `CC` (8-bit status flags: `E F H I N Z V C`)

### The Challenge
Standard compiler register allocation algorithms (like LLVM's greedy allocator or GCC's IRA) assume a large, orthogonal register file (e.g., 16 or 32 interchangeable 64-bit registers). On the M6809:
1. Registers are strictly specialized: pointer indexing cannot use `D`, while 16-bit multiplication and division cannot use `X` or `Y` directly without register transfers.
2. Accumulators overlap: Allocating `D` conflicts with both `A` and `B`.
3. Registers are constrained by compilation variants: Under `-globals-at-y -frame-pointer`, only `X` and `D` are freely allocatable! Under default mode, `X`, `Y`, `U`, and `D` are all available.
4. "Interpolation points" (runtime helpers `__mul16`, `__div16`, `__memset0`, OS9 traps, hypercalls) require specific fixed registers and clobber scratch registers.

### The Strategy: Incremental SSA Register Allocation
As outlined in `doc/SSA-Register-Allocation-for-M6809.pdf`, modern SSA-based register allocation decouples spilling from coloring, leveraging the **chordal graph property** of SSA interference graphs to color in linear time without backtracking.

Rather than implementing a monolithic allocator in a single high-risk step, we decompose Phase Four into **5 discrete, independently verifiable milestones**. Every milestone will be guarded by a dedicated command-line flag and environment variable, with automated testing against the full test suite and the 8-variant compilation matrix.

```mermaid
flowchart TD
    M1["Milestone 4.1: Foundation & Liveness Analysis<br/>(RegMask, Bitmask Conflict, SSA Live Intervals)"] --> M2["Milestone 4.2: Local / Basic-Block Allocation<br/>(Track & Pin D/B/X within Basic Blocks)"]
    M2 --> M3["Milestone 4.3: Loop Allocation & Pinning<br/>(Pin Induction Variables & Pointers in Loops to X/Y/U)"]
    M3 --> M4["Milestone 4.4: Conventional SSA (CSSA) & Phi Lowering<br/>(Parallel Copy Resolution, EXG/TFR Synthesis)"]
    M4 --> M5["Milestone 4.5: Global SSA Chordal Coloring & Coalescing<br/>(Decoupled Spilling, MCS Greedy Coloring, Coalescing)"]
```

---

## 2. Milestone Breakdown

### Milestone 4.1: Foundation, Register Bitmasks & Liveness Analysis
**Objective**: Build the data structures and mathematical analysis passes needed for register allocation, without changing emitted assembly yet.

1. **Register Representation (`m6809/reg.go`)**:
   - Bitmask architecture to model register overlapping and classes:
     ```go
     type RegMask uint16

     const (
         RegNone RegMask = 0
         RegA    RegMask = 1 << 0 // High byte accumulator
         RegB    RegMask = 1 << 1 // Low byte accumulator
         RegD    RegMask = RegA | RegB // 16-bit composite accumulator
         RegX    RegMask = 1 << 2 // Index X
         RegY    RegMask = 1 << 3 // Index Y
         RegU    RegMask = 1 << 4 // User stack / Index U
         RegS    RegMask = 1 << 5 // System stack (reserved)
         RegCC   RegMask = 1 << 6 // Condition codes
     )
     ```
   - Register Class definitions:
     - `ClassAcc8`: `{RegA, RegB}`
     - `ClassAcc16`: `{RegD}`
     - `ClassIndex`: `{RegX, RegY, RegU}` (filtered by `-globals-at-y` and `-frame-pointer`)
     - `ClassAllAllocatable`: Dynamic bitmask based on active variant.
2. **Variant-Aware Availability Filter**:
   - `AllocatableRegisters(globalsAtY, framePointer bool) RegMask`
     - If `globalsAtY`: clear `RegY`.
     - If `framePointer`: clear `RegU`.
3. **SSA Liveness Analysis (`m6809/liveness.go`)**:
   - Compute `Def`, `Use`, `LiveIn`, `LiveOut` for every basic block.
   - Compute linear `LiveInterval` for each SSA value: `[StartInstrID, EndInstrID]`.
   - Compute **Register Pressure** at each instruction:
     - Count simultaneous live 8-bit and 16-bit values.
4. **Guard Flag**:
   - `-no-liveness6809` (env `NO_LIVENESS6809`).
5. **Testing & Verification**:
   - Unit tests verifying live intervals, loop liveness extensions, and interference computation across complex control flow graphs.

---

### Milestone 4.2: Local Basic-Block Register Allocation & Accumulator Reuse
**Objective**: Eliminate redundant stores and immediate reloads within straight-line basic blocks.

1. **Local Value Tracking**:
   - Within each basic block, track which SSA values are currently resident in physical registers (`D`, `B`, `A`, `X`, `Y`).
   - If an SSA instruction computes a value into `D` or `B`, and its uses are strictly local within the block:
     - Check if any subsequent instruction before the last use clobbers `D`/`B`.
     - If not clobbered, subsequent uses read directly from `D`/`B` without reloading from the stack slot (`ldd slot,s`).
2. **Dead Stack Store Elimination for Short-Lived Locals**:
   - If an SSA value's lifetime is entirely contained within the accumulator and does not cross a call or clobbering instruction, omit `std slot,s`!
3. **Index Register Pinning for Sequential Access**:
   - When traversing arrays or structs, keep the base pointer in `X` or `Y` rather than re-computing `leax slot,s; ldd ,x` repeatedly.
4. **Clobber Handling at Interpolation Points**:
   - Invalidate all caller-saved physical registers upon `jsr`, `__mul16`, `__div16`, or runtime panic/hypercall.
5. **Guard Flag**:
   - `-no-local-regalloc6809` (env `NO_LOCAL_REGALLOC6809`).
6. **Testing & Verification**:
   - Full test suite across all 8 variants.
   - Benchmark code size and cycle count drops on straight-line benchmarks (`alphabet`, `count_up`, `test_struct`).

---

### Milestone 4.3: Loop Induction Variable & Pointer Pinning
**Objective**: Keep high-frequency loop counters and array pointers in dedicated index registers throughout loop execution.

1. **Loop Analysis & Identification**:
   - Detect natural loops and innermost hot loops from CFG dominator trees.
   - Identify loop-carried dependencies (SSA $\Phi$-nodes at loop headers) and loop-invariant pointers.
2. **Candidate Selection & Preferencing**:
   - **Pointer candidates**: Array cursors, slice bases, struct pointers $\rightarrow$ Prefer `X`, `Y`, or `U`.
   - **Counter candidates**: 8-bit counters $\rightarrow$ Prefer `B` or `A`; 16-bit counters $\rightarrow$ Prefer `X` or `D`.
3. **Loop Register Allocation**:
   - Allocate physical registers across the loop body.
   - Emit loop pre-header: Load initial value from parameter or stack into the assigned register.
   - Emit loop exit: Store final value to stack slot if live after loop.
   - Replace loop body loads and stores with register operations (e.g. `leax 1,x`, `incb`, `ldb ,x+`, `stb ,x+`).
4. **Guard Flag**:
   - `-no-loop-regalloc6809` (env `NO_LOOP_REGALLOC6809`).
5. **Testing & Verification**:
   - Run tests on prime generation (`test_primes.golf`), sorting (`test_sort.golf`), buffer manipulation (`test_buf.golf`), and big integer arithmetic (`test_big_mul.golf`).
   - Measure cycle count reductions (targeting >30% speedup on loop-heavy tests).

---

### Milestone 4.4: Conventional SSA ($\Phi$-Elimination) & Parallel Copy Resolution
**Objective**: Correctly lower SSA $\Phi$-nodes across arbitrary CFG edges into physical register moves without lost updates or register swap cycles.

1. **The Parallel Copy Problem in M6809**:
   - At block boundaries, $\Phi$-functions represent simultaneous parallel assignments:
     $$\begin{pmatrix} r_1 \\ r_2 \end{pmatrix} \leftarrow \begin{pmatrix} r_2 \\ r_1 \end{pmatrix}$$
   - Naive sequential assignment (`tfr r2, r1` then `tfr r1, r2`) overwrites $r_1$'s old value.
2. **Parallel Copy Sequentializer**:
   - Implement the standard graph-cycle decomposition algorithm (Hack et al. / Sreedhar):
     - Identify non-cyclic moves: emit `tfr src, dst`.
     - Identify 2-cycles: emit **hardware `exg r1, r2`**!
       - On M6809, `exg` operates on *any* pair of registers: `exg a,b`, `exg d,x`, `exg x,y`, `exg x,u`, `exg d,u`! This is an enormous architectural advantage over x86/ARM (which lack generic register exchange).
     - For $N$-cycles ($N \ge 3$): use a scratch register or push one register to stack (`pshs`), sequentialize remaining transfers, and pop to target (`puls`).
3. **CSSA (Conventional SSA) Coalescing**:
   - Group $\Phi$-related SSA variables into congruence classes to minimize parallel copies.
4. **Guard Flag**:
   - `-no-cssa-lowering6809` (env `NO_CSSA_LOWERING6809`).
5. **Testing & Verification**:
   - Stress-test diamond CFGs, break/continue statements in nested loops, and mutual recursion phi patterns.

---

### Milestone 4.5: Global SSA Chordal Coloring, Spilling & Coalescing
**Objective**: Full decoupled global register allocation for the entire function.

1. **Phase A: Spilling (Lowering Register Pressure to $\le K$)**:
   - Compute maximum register pressure per register class across all program points.
   - If pressure exceeds available registers $K$:
     - Use Linear Scan / Belady's furthest-next-use heuristic to select virtual registers to spill to stack slots.
     - Insert spill stores immediately after definitions and reloads immediately before uses.
2. **Phase B: Maximum Cardinality Search (MCS) Chordal Coloring**:
   - Build Register Interference Graph (RIG) for non-spilled SSA values.
   - Since SSA interference graphs are chordal, order nodes via MCS in $O(V + E)$ time.
   - Greedily color nodes according to MCS order with **Register Preferencing**:
     - Values used in pointer dereferences $\rightarrow$ Bias to `ClassIndex` (`X, Y, U`).
     - Values used in arithmetic / shifts $\rightarrow$ Bias to `ClassAcc` (`D, B, A`).
     - Respect bitmask aliasing (`RegD` locks both `RegA` and `RegB`).
3. **Phase C: Conservative Coalescing (Chaitin-Briggs / George-Appel)**:
   - Coalesce copy instructions between non-interfering virtual registers to eliminate redundant `tfr` and `exg` instructions.
4. **Sub-register Handling**:
   - Lower `GetHighByte` and `GetLowByte` to zero-cost references if 16-bit value is allocated to `D`.
5. **Guard Flag**:
   - `-no-global-regalloc6809` (env `NO_GLOBAL_REGALLOC6809`).
6. **Testing & Verification**:
   - Full test suite, 8-variant test matrix, and Phase Four final telemetry comparison.

---

## 3. Risk Mitigation & Quality Invariants

| Risk | Mitigation Strategy |
|---|---|
| **Register exhaustion in dense code** | Decoupled spilling guarantees pressure $\le K$ before coloring begins. Fallback to existing stack slot addressing if all physical registers are occupied. |
| **Clobbered registers across runtime calls** | Explicitly model `ClobberMask` for every helper (`__mul16`, `__div16`, `__memset0`, `printf`, hypercalls). Spill caller-saved registers before calls. |
| **Variant breakage (`-globals-at-y`, `-frame-pointer`)** | Dynamic bitmask `AllocatableRegisters` strictly excludes `Y` and/or `U` according to active flags. |
| **Lost update on $\Phi$-nodes** | Formal parallel copy sequentializer with hardware `exg` cycle resolution in Milestone 4.4. |
| **Regressions & Debuggability** | Every milestone has an explicit `-no-...` flag and environment variable. Automated git commits on `work-hatvan-0` after each milestone verification. |

---

## 4. Proposed Milestone Schedule

| Milestone | Scope & Deliverables | Guard Flag & Env Var | Expected Impact |
|---|---|---|---|
| **4.1: Foundation & Liveness** | Register bitmasks, variant availability filters, CFG live interval computation (`[start, end]`), and instruction register pressure tracking. | `-no-liveness6809`<br/>`NO_LIVENESS6809` | Zero assembly changes; verified mathematical foundation. |
| **4.2: Local Block Allocation** | Straight-line register tracking; reuse values in `D`/`B`/`X`; eliminate redundant `std`/`ldd` stack traffic within basic blocks. | `-no-local-regalloc6809`<br/>`NO_LOCAL_REGALLOC6809` | Noticeable drop in stack loads/stores on straight-line code. |
| **4.3: Loop Register Pinning** | Identify loop-carried variables and cursors; pin loop counters and array pointers into `X`, `Y`, `U`, or `B`; pre-header loads and post-loop stores. | `-no-loop-regalloc6809`<br/>`NO_LOOP_REGALLOC6809` | Major speedup (>25–40%) on loop-heavy benchmarks. |
| **4.4: CSSA & Phi Lowering** | Parallel copy resolution across block boundaries; cyclic transfer decomposition using hardware `exg` and `tfr`. | `-no-cssa-lowering6809`<br/>`NO_CSSA_LOWERING6809` | Robust, lost-update-free inter-block register passing. |
| **4.5: Global Chordal Coloring** | Decoupled linear-scan spilling to stack (pressure $\le K$); Maximum Cardinality Search (MCS) greedy coloring with class preferencing; Chaitin-Briggs copy coalescing. | `-no-global-regalloc6809`<br/>`NO_GLOBAL_REGALLOC6809` | Full end-to-end SSA register allocation for M6809. |

- [ ] **Milestone 4.1**: Liveness analysis, `RegMask` bitmask architecture, interference graph & pressure computation.
- [ ] **Milestone 4.2**: Local basic-block allocation (accumulator reuse & dead stack store elimination).
- [ ] **Milestone 4.3**: Loop induction variable & pointer pinning (`X`, `Y`, `U`).
- [ ] **Milestone 4.4**: Conventional SSA ($\Phi$-elimination) & parallel copy resolution with `exg`.
- [ ] **Milestone 4.5**: Global chordal graph coloring, decoupled spilling, and conservative coalescing.
- [ ] **Phase 4 Telemetry & Final Verification**: Comprehensive benchmark telemetry vs. Phase Three baseline.
