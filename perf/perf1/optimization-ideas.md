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
  - Functions with `TrunkLevel > 0` are guaranteed to run at most once.
  - Inlining a trunk function is a **win for both time AND space**: the original function body can be eliminated completely by Dead Function Elimination (DFE), eliminating `jsr`/`rts` and stack frame overhead with zero net code growth.

---

### C. `LeafLevel` (Call Graph Depth)
* **Location**: Defined on `ir.Function.LeafLevel` ([`ir/ir.go`](file:///home/strick/github.com/strickyak/minigolf/ir/ir.go)).
* **Computation**: [`ir/builder.go`](file:///home/strick/github.com/strickyak/minigolf/ir/builder.go) via `Builder.AnnotateLeafLevels()`.
* **Mechanism**:
  - **Level 1 (Pure Leaf)**: Function makes 0 calls and no indirect calls.
  - **Level 2**: Calls only Level 1 leaf functions.
  - **Level $K$**: Calls at most Level $K-1$ functions.
* **Usage in Decisions**:
  - Inlining Level 1 leaves is safe and never increases caller call-tree depth.
  - Inlining a Level 1 leaf into a Level 2 caller can promote the caller to a Level 1 leaf, unlocking leaf-frame optimizations (`NoLeafOpt6809` in backend) which omit stack frame creation and register saves entirely.

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
