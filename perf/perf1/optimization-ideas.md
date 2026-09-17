# MiniGolf Optimization Ideas, Signals, and Roadmap

This document catalogs optimization ideas, available compiler signals/heuristics, and concrete implementation plans for improving MiniGolf's generated code size and execution cycle efficiency on the Motorola 6809 (and other targets).

---

## 1. Available Signals & Heuristics

MiniGolf gathers several static analysis metrics across the AST and IR before code generation. These signals provide the decision inputs for inlining, loop unrolling, and register allocation.

### A. `Popularity` (Execution Frequency Estimate)
* **Location**: Defined on `ast.FuncStatement.Popularity` ([`ast/ast.go`](file:///home/strick/github.com/strickyak/minigolf/ast/ast.go)) and copied to `ir.Function.Popularity` ([`ir/ir.go`](file:///home/strick/github.com/strickyak/minigolf/ir/ir.go)).
* **Computation**: [`ast/trunk.go`](file:///home/strick/github.com/strickyak/minigolf/ast/trunk.go) via `Program.MarkTrunkFunctions()`.
* **Mechanism**:
  - `main` starts with `Popularity = 1`.
  - Every call site is weighted by loop nesting depth:
    $$\text{weight} = 1 \ll (2 \times \text{loopDepth}) = 4^{\text{loopDepth}}$$
    - Depth 0 (straight-line / outside loops): $\text{weight} = 1$
    - Depth 1 (inside 1 loop): $\text{weight} = 4$
    - Depth 2 (nested loop): $\text{weight} = 16$
  - Propagated through the call graph in topological order:
    $$\text{callee.Popularity} += \text{caller.Popularity} \times \text{weight}$$
* **Usage in Decisions**:
  - **High Popularity ($\ge 16$)**: The function is executed in inner loops or frequent call paths. Aggressive inlining and unrolling are justified here even if code size grows.
  - **Low Popularity ($1$ or $2$)**: Cold code (initialization, error aborts). Code size should be prioritized over execution speed; do not inline unless it saves space.

---

### B. `TrunkLevel` (Single Execution / Entry Spine)
* **Location**: Defined on `ast.FuncStatement.TrunkLevel` and `ir.Function.TrunkLevel`.
* **Computation**: [`ast/trunk.go`](file:///home/strick/github.com/strickyak/minigolf/ast/trunk.go).
* **Mechanism**:
  - Level 1 is `main.main`.
  - Level $N$ is assigned if a function is called from **exactly one call site**, which is in a Level $N-1$ function, **outside of any loop**, with no dynamic address taken.
* **Usage in Decisions**:
  - **Inlining**: Functions with `TrunkLevel > 0` are guaranteed to run at most once. Inlining a trunk function is a **win for both time AND space**: the original function body can be eliminated completely by Dead Function Elimination (DFE), eliminating `jsr`/`rts` and stack frame overhead with zero net code growth.
  - **Static Frame Sharing**: **No two functions with `TrunkLevel == N` (for $N > 0$) can ever exist on the call stack simultaneously.** They are non-reentrant with respect to each other, meaning their activation records (parameters, local variables, temporaries) can share a single statically allocated global memory block rather than consuming stack frames.

---

### C. `LeafLevel` (Call Graph Depth)
* **Location**: Defined on `ir.Function.LeafLevel` ([`ir/ir.go`](file:///home/strick/github.com/strickyak/minigolf/ir/ir.go)).
* **Computation**: [`ir/builder.go`](file:///home/strick/github.com/strickyak/minigolf/ir/builder.go) via `Builder.AnnotateLeafLevels()`.
* **Mechanism**:
  - **Level 1 (Pure Leaf)**: Function makes 0 calls and no indirect calls.
  - **Level 2**: Calls only Level 1 leaf functions.
  - **Level $K$**: Calls at most Level $K-1$ functions.
* **Usage in Decisions**:
  - **Inlining**: Inlining Level 1 leaves is safe and never increases caller call-tree depth. Inlining a Level 1 leaf into a Level 2 caller can promote the caller to a Level 1 leaf, unlocking leaf-frame optimizations (`NoLeafOpt6809` in backend) which omit stack frame creation and register saves entirely.
  - **Static Frame Sharing**: **No two functions with `LeafLevel == N` (for $N > 0$) can ever exist on the call stack simultaneously** (a level $N$ function only calls levels $< N$, so level $N$ functions are strictly non-ancestral to each other). Their activation records can safely overlay the same global/Direct Page memory block.

---

### D. Function Size & Instruction Count
* **Location**: Calculated dynamically during optimization passes (e.g. `isTinyFunction` in [`opt/inline.go`](file:///home/strick/github.com/strickyak/minigolf/opt/inline.go)).
* **Metrics**: Instruction count, basic block count, presence of nested calls.
* **Tunable**: `opts.MaxTinyInstructions` (default 8, configurable via `-inline-max-tiny`).

---

## 2. Optimization Areas & Concrete Tasks

### Area 1: Inlining Policies Tuned by Heuristics

- [ ] **Popularity-Weighted Inlining Budget**:
  Instead of a binary "tiny" check, evaluate an inlining cost-benefit equation:
  $$\text{Score} = \frac{\text{Popularity} \times \text{CallOverheadSavings}}{\text{FunctionInstructionCount}}$$
  Inline candidates with Score exceeding a tunable threshold `InlinePopularityThreshold`.
- [ ] **Trunk Function Auto-Inlining**:
  Functions with `TrunkLevel > 0` and single callers should always be inlined prior to dead function elimination.
- [ ] **Leaf Promotion Inlining**:
  Target functions that call only one or two tiny leaf functions to convert the caller into a leaf function, eliminating frame pointer setup.

---

### Area 2: Loop Optimizations

- [ ] **Loop Induction Variable Pinning**:
  In benchmarks like `02_count_loop`, `07_sieve`, and `08_bubble_sort`, the loop counter (`i`) is spilled and reloaded from the stack each iteration (`ldd 2,s; addd #1; std 2,s`).
  * *Concrete change*: If a loop induction variable fits in `X` or `Y`, hold it in the register across the entire loop body and emit `leax 1,x` or `leay 1,y` instead of stack memory accesses.
- [ ] **Loop Unrolling Pass (`opt/unroll.go`)**:
  - Use `MaxLoopUnrollCount` and `MaxLoopUnrollInstructions` from `opt.Config`.
  - For small constant trip counts (e.g. `count <= 4` or `8`), duplicate loop bodies straight-line to remove branch and decrement overhead.
- [ ] **Strength Reduction for Multiplies in Loops**:
  Extend `StrengthReductionPass` to convert array index multiplications `i * sizeof(elem)` into induction pointer stepping (`p += sizeof(elem)`).

---

### Area 3: 6809 Backend Code Generation & Addressing Modes

- [ ] **Redundant Spill/Reload Elimination**:
  Detect and eliminate back-to-back operations:
  ```asm
  ; Redundant:
  std 2,s
  ldd 2,s   ; <-- Remove: value already in D
  ```
- [ ] **Direct Stack Addressing**:
  Replace two-instruction indirect patterns:
  ```asm
  ; Current:
  leax 4,s
  ldd ,x
  ; Optimized:
  ldd 4,s
  ```
- [ ] **Hardware Autoincrement / Autodecrement Addressing**:
  For pointer traversals (e.g. `*p++` in `05_array_sum.c` and `06_string_ops.c`):
  ```asm
  ; Current (explicit pointer update):
  ldx ptr
  ldb ,x
  leax 1,x
  stx ptr
  ; Optimized:
  ldb ,x+
  ```
- [ ] **Dense Switch Jump Tables**:
  In `10_switch_case.c`, MiniGolf currently emits an $O(N)$ if-else comparison chain.
  * *Concrete change*: When case values form a dense integer range ($[\text{min}, \text{max}]$ with density $> 0.6$), emit a jump table:
    ```asm
    subd #min_val
    aslb
    rola
    ldx #jump_table
    jmp [d,x]
    ```

---

### Area 4: Backend Helper Inlining

- [x] **Tunable 16-bit Multiply Inlining (`InlineMul16`)**:
  Flag `-inline-mul16` / `INLINE_MUL16`. Emits 3 hardware `mul` instructions straight-line instead of `jsr __mul16`, saving 25+ cycles.
- [x] **Tunable 16-bit Div/Mod Inlining (`InlineDivMod16`)**:
  Flag `-inline-divmod16` / `INLINE_DIVMOD16`. Emits 16-step division loop inline, saving `jsr`/`lbsr`/`rts` wrapper overhead (~35 cycles).
- [x] **Memory & Shift Unroll Thresholds**:
  - `MemcpyUnrollThreshold` (default 4 bytes)
  - `MemsetUnrollThreshold` (default 2 bytes)
  - `ShiftUnrollThreshold` (default 4 bits)
- [ ] **Automatic Single-Call Helper Inlining**:
  If `__mul16` or `__div16` is used at only 1 site in the entire program, inline it automatically so the subroutine body and symbol do not need to be emitted into the final binary.

---

### Area 5: Non-Reentrant Static Frame Overlays & Direct Page Allocation

#### The Core Insight: Call-Stack Mutual Exclusion
* **TrunkLevel Property**: No two functions with `TrunkLevel == N` ($N > 0$) can ever exist on the call stack at the same time. Since every trunk function has exactly one call site in a level $N-1$ caller outside loops, they are non-reentrant and strictly mutually exclusive.
* **LeafLevel Property**: No two functions with `LeafLevel == N` ($N > 0$) can ever exist on the call stack at the same time, because a level $N$ function only calls levels $< N$. No level $N$ function can ever be an ancestor of another level $N$ function.

#### Statically Allocated Frame Overlays
Because functions at the same level have non-overlapping lifetimes:
1. **Shared Activation Blocks**: Instead of dynamically carving frames out of the hardware stack (`leas -N,s`), all functions at level $N$ can share a single statically allocated global memory block:
   $$\text{BlockSize}(\text{Level } N) = \max_{f \in \text{Level } N}(\text{FrameSize}(f))$$
2. **Elimination of Frame Setup & Teardown**:
   - Omits `leas -N,s` on entry (4–5 cycles saved).
   - Omits `leas N,s` on exit (4–5 cycles saved).
   - Entirely removes stack-depth growth for non-recursive portions of the program.

#### Direct Page (DP) Addressing Speedup (M6809)
When static frame overlays are placed in the 6809 **Direct Page** (`$00`–`$FF` or a page selected via the `DP` register):
* **Direct Addressing (`<offset`)**:
  - Instruction size: **2 bytes** (opcode + 8-bit address).
  - Cycle count: **4 cycles** (e.g. `ldd <dp_var`, `std <dp_var`).
* **Indexed Stack Addressing (`n,s` or `n,u`)**:
  - Instruction size: **3 to 4 bytes** (opcode + post-byte + offset).
  - Cycle count: **5 to 6 cycles** (5 cycles for 5-bit offset, 6 cycles for 8-bit offset).
* **Net Advantage**: **1 to 2 cycles faster and 1 to 2 bytes smaller on every local variable read and write!**
* **Direct Parameter Passing**: Callers can store arguments directly into the callee's fixed parameter addresses (`std <callee_arg0`), eliminating `pshs` and subsequent caller stack cleanup (`leas 2,s`).

#### Deployment Scenarios & Tradeoffs
* **User-Space Programs / Games / CoCo Applications**: Global RAM is readily available, and programs are typically single-threaded and non-reentrant. Static frame overlaying yields dramatic speedups and size reductions.
* **Device Drivers / Interrupt Handlers / ROM Libraries**: Re-entrancy and stack independence may be required, or global RAM may be severely constrained.
* **Actionable Next Steps**:
  - [ ] Add compiler flag `-static-frames` (env `STATIC_FRAMES`) to enable static frame overlaying.
  - [ ] Add compiler flag `-direct-page-frames` (env `DIRECT_PAGE_FRAMES`) to locate level-1 leaf or trunk frames in the Direct Page.
  - [ ] Compute `maxTrunkSize[level]` and `maxLeafSize[level]` and allocate overlay symbols (`v__trunk_level_N_frame`, `v__leaf_level_N_frame`) in the data section.
  - [ ] Update M6809 backend `getSlot()` to generate direct/global addressing for functions qualifying for static overlay.

