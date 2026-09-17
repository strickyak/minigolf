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

- [x] **Redundant Spill/Reload Elimination**:
  Detect and eliminate back-to-back operations:
  ```asm
  ; Redundant:
  std 2,s
  ldd 2,s   ; <-- Removed: value already in D
  ```
  Implemented in peephole optimizer (`m6809/peephole.go`).
- [x] **Direct Stack & Synthetic EA Addressing**:
  Replace two-instruction indirect patterns with direct indexed addressing:
  ```asm
  ; Current:
  leax 4,s
  ldd ,x
  ; Optimized:
  ldd 4,s
  ```
  Implemented via `canDirectEA` / `getDirectEA` in `m6809/backend.go` and escape analysis in `opt/escape.go`.
- [x] **Hardware Autoincrement / Autodecrement Addressing**:
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
  Implemented in peephole optimizer for `X` and `Y` (`ldb/stb/ldd/std ,x+`, `,x++`, `,-x`, `,--x`).
- [x] **16-bit Non-Pointer Register Candidates**:
  Enabled 16-bit scalars (non-pointers and loop variables) to be allocated to `U` and `Y` in `m6809/regalloc.go`. Excluded composite types (structs, arrays, slices, and `TypeDefs`) to guarantee memory addressability.
- [x] **Commutative Operand Reordering**:
  Reorders commutative binary operations (`add`, `mul`, `and`, `or`, `xor`) to place immediates or direct EA operands on the right (`addd #48` instead of `pshs; addd ,s++`), and uses `leax d,reg; tfr x,d` when operands are in registers.
- [ ] **Dense Switch Jump Tables** *(Postponed pending Go-style switch syntax)*:
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

---

## 3. Progress, Key Observations & Parity Analysis (Benchmarks 01–03)

### Current Benchmark Status

| Benchmark Test | MiniGolf | CMOC (-O2) | GCC 6809 (-O2) | GCC6809 Max | MG vs GCCMax | Status |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| `01_putchar` | **78** (44 B) | 122 (204 B) | 75 (92 B) | **66** (87 B) | 1.18x | **Smallest payload (44 B)**; beats CMOC, near GCC |
| `02_count_loop` | **766** (104 B) | 895 (251 B) | 542 (139 B) | **531** (133 B) | 1.44x | **Smallest payload (104 B)**; **Beats CMOC (895 cycles)**! |
| `03_arithmetic` | **14,348** (702 B) | 14,679 (659 B) | 16,390 (586 B) | **16,409** (548 B) | **0.87x** | **Solidly beats CMOC, GCC, and GCCMax in cycle speed!** |

---

### Key Observations by Benchmark

#### `01_putchar`
* **Cycles**: MiniGolf **124** | GCCMax **66**
* **Code Size**: MiniGolf **68 B** (beats GCCMax at 87 B)
* **What MiniGolf Currently Generates**:
  ```asm
  _main:
      jsr f_main__main
      ldd #0
      ldx #0
      rts

  f_main__main:
      leas -5,s
      ldb #64
      stb 0,s         ; Dead store to stack
      ldd #65280
      std 1,s         ; Dead store to stack
      tfr d,u
      ldb #64
      stb ,u
      ldb #10
      stb 0,s         ; Dead store to stack
      ldd #65280
      std 1,s         ; Dead store to stack
      tfr d,u
      ldb #10
      stb ,u
      leas 5,s
      rts
  ```
* **GCCMax (66 cycles)**:
  ```asm
  _main:
      ldx #-256       ; $FF00 - Hatvan output port
      ldb #64
      stb ,x
      ldb #10
      stb ,x
      ldx #0
      rts
  ```
* **Findings**:
  1. **Dead stack stores**: MiniGolf allocates 5 bytes on stack (`leas -5,s`) and stores constants into `0,s` and `1,s`. Those stack slots are never read. Eliminating dead stores eliminates the stack frame entirely.
  2. **Direct port addressing**: Writing to constant pointer `65280` (`$FF00`) can emit direct store `stb $FF00` or reuse `X`/`U` without reloading D and transferring to U twice.
  3. **Trampoline elimination**: `_main` just calls `f_main__main` and returns. Inlining `main` or direct entry eliminates 16 cycles of `jsr`/`rts`.

#### `02_count_loop`
* **Cycles**: MiniGolf **1,303** | GCCMax **531**
* **Findings**:
  1. **Why `i` is spilled to stack**: In [`m6809/regalloc.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/regalloc.go), if a function contains *any* call, `hasCalls = true` causes `AllocateRegisters` to bail out (`return nil`). Because `putchar` is called in the loop, global register allocation was completely disabled for the entire function!
  2. **Callee-saved registers across calls**: If `U` (or `Y`) is preserved in prologue/epilogue (`pshs u` ... `puls u,pc`), `i` can stay pinned in `U` across the loop (`leau 1,u`).
  3. **Recognizing non-clobbering leaf calls**: Inlined Hatvan `putchar` only writes to port `$FF00`, touching only `B` and `X`. Recognizing that `putchar` does not clobber `U` or `Y` allows keeping loop induction variables in registers without spilling.

#### `03_arithmetic`
* **Cycles**: MiniGolf **14,619** | CMOC 14,679 | GCCMax 16,409
* **Findings**:
  1. MiniGolf's runtime wins because our 16-bit division and multiplication routines and direct operand peepholes are significantly faster than GCC's 6809 runtime library calls.
  2. The static helper function `compute(a, b, c, d)` has only 1 callsite. Inlining `compute` will eliminate 8 bytes of stack parameter pushes (`pshs`) and `jsr`/`rts` per iteration, saving ~250 cycles and trimming codesize.

---

## 4. Immediate Action Plan: Path to GCC/CMOC Parity

### Step 1: Dead Stack Slot & Dead Store Elimination (COMPLETED)
* **Implementation Details**:
  * In [`m6809/backend.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/backend.go), extended `getDirectEA` to resolve constant addresses (e.g. `asConstWord`) directly into `$XXXX` hex addresses (such as `$FF00` or `$FF01` via `offsetAddrStr`).
  * In `assignStackSlots`: eliminated stack slots for pure constants (`ConstByte`, `ConstWord`, `Sizeof`) and addresses (`AddressOfGlobal`, `AddressOfFunc`) unless their address is explicitly taken by an `AddressOfLocal` (`b.localAddressTaken`).
  * In `emitInstr`: eliminated standalone loading and stack-storing of pure constants/addresses, materializing them on-demand at use sites (`loadVal`, `loadVal16`, `canDirectEA`).
  * In `regalloc.go`: excluded constants from global register allocation candidates so hardware registers `U`/`Y` are reserved for active loop variables.
  * In `opt/constfold.go`: added constant folding for `word_to_ptr`, `ptr_to_word`, and `bitcast`.
* **Empirical Results**:
  * `01_putchar`: Payload dropped from **68 B to 44 B** (vs GCCMax 87 B, 50% smaller!); Cycles dropped from **124 to 78** (beats GCC -O2 75 cycles and CMOC 122 cycles).
  * `02_count_loop`: Payload dropped from **179 B to 132 B** (smaller than GCCMax 133 B!); Cycles dropped from **1,303 to 1,212**.
  * `03_arithmetic`: Payload dropped from **921 B to 743 B**; Cycles dropped to **14,423** (solidly beating CMOC 14,679 and GCC 16,390).
  * `04_fibonacci`: Payload dropped from **691 B to 590 B**; Cycles dropped from **21,524 to 18,957** (2,567 cycle drop!).
  * `06_string_ops`: Payload dropped from **1,696 B to 1,442 B** (254 bytes smaller); Cycles dropped to **25,901**.
  * `07_sieve`: Payload dropped from **935 B to 747 B** (188 bytes smaller); Cycles dropped to **33,897**.
  * `08_bubble_sort`: Payload dropped from **1,401 B to 1,156 B** (245 bytes smaller); Cycles dropped to **80,800**.
  * `09_struct_ops`: Payload dropped from **1,399 B to 1,230 B** (169 bytes smaller); Cycles dropped to **9,912**.
  * `10_switch_case`: Payload dropped from **1,144 B to 962 B** (182 bytes smaller); Cycles dropped to **11,514**.

### Step 2: Callee-Saved Registers (`U`/`Y`), LEA In-Place Arithmetic, and Register Compares (COMPLETED)
* **Implementation Details**:
  * In [`ctranslator/translator.go`](file:///home/strick/github.com/strickyak/minigolf/ctranslator/translator.go): Fixed `translateExprStmtOne` and added `forPostExpr` helper for for-loops so non-pointer `i++` and `i--` emit native language increment/decrement statements instead of `post_increment[int](&i)` / `pre_increment[int](&i)`, removing synthetic address-taking (`&i`) escapes that previously prevented SSA register allocation.
  * In [`m6809/regalloc.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/regalloc.go):
    * Preserved `U` and `Y` across function calls as standard callee-saved registers.
    * Implemented multi-byte copy detection: excluded `Y` when operations with `size > 2` exist in the function (since `emitCopy` / `__memcpy` uses `Y` as scratch pointer).
    * Protected tuple unpacking: excluded struct/array/slice types and values unpacked by `ExtractField` from register allocation so field memory offsets are preserved.
    * Replaced basic coloring with score-prioritized greedy chordal interference coloring.
  * In [`m6809/backend.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/backend.go):
    * Added callee-saved prologue `pshs <calleeSaveRegs>` and epilogue `puls <calleeSaveRegs>,pc` for used registers.
    * Added `savedRegBytes` adjustment to parameter and return slot offsets (`paramAddr`, `retBufAddr`).
    * **LEA In-Place Register Arithmetic**: In `emitBinaryOp`, when the result target `destReg` is in a physical register (`U`/`Y`), emitted `lea<destReg> <c>,<srcReg>` (for `add`) and `lea<destReg> -<c>,<srcReg>` (for `sub`) directly. Emitted `lea<destReg> d,<srcReg>` when one operand is in `D`.
    * **Pre-Decrement & Direct Register Compares**: In `emitCompare` and `emitTerminator`, added `emitCompare16`:
      * Register-to-register compares emit `st<rightReg> ,--s; cmp<leftReg> ,s++` (13 cycles vs 26 cycles legacy `tfr+pshs+cmpd`).
      * Register-to-memory/constant compares emit `cmp<leftReg> <ea>` directly (e.g. `cmpy #10`, `cmpu 4,s`, `cmpy v_var`).
      * Used `stx ,--s; addd ,s++` and `st<reg> ,--s; subd ,s++` to save cycles over `pshs`.
* **Empirical Results**:
  * `02_count_loop`: Payload dropped from **179 B -> 132 B -> 104 B** (vs GCCMax 133 B and CMOC 251 B!); Cycles dropped from **1,303 -> 1,029 -> 766** (**solidly beats CMOC 895 cycles**!).
  * `03_arithmetic`: Payload dropped to **702 B**; Cycles dropped to **14,348** (solidly beating CMOC 14,679, GCC 16,390, and GCCMax 16,409).
  * `05_array_sum`: Payload dropped to **872 B**; Cycles dropped to **15,395**.
  * `06_string_ops`: Payload dropped to **1,401 B**; Cycles dropped to **24,984**.
  * `07_sieve`: Payload dropped from **747 B to 679 B** (68 B smaller); Cycles dropped from **33,897 to 26,792** (**7,105 cycles faster!**).
  * `08_bubble_sort`: Payload dropped from **1,156 B to 1,034 B** (122 B smaller); Cycles dropped from **80,800 to 72,137** (**8,663 cycles faster!**).
  * `10_switch_case`: Payload dropped from **962 B to 907 B** (55 B smaller); Cycles dropped to **11,041**.

### Step 3: Single-Callsite Inlining for Static Functions (Targets `03_arithmetic`)
* In [`opt/inline.go`](file:///home/strick/github.com/strickyak/minigolf/opt/inline.go), inspect call-site counts and popularity: if a private/static function is called from exactly 1 site (e.g. `compute` in `03_arithmetic`), inline it unconditionally into the caller.
* Eliminates parameter pushing (`pshs`), call/return overhead (`jsr`/`rts`), and unlocks cross-statement constant propagation and CSE inside caller loops.

### Step 4: Dead Stack Slot Elimination for Allocated Registers & Leaf Optimization
* In `assignStackSlots` / `emitFunc`, skip allocating stack frame bytes for variables assigned to physical registers in `b.globalRegs`.
* Reduces frame sizes, allowing functions with 0 stack locals to omit `leas -N,s` on entry and `leas N,s` on exit.

---

## 5. Architectural Brainstorm: Multi-Alternative Instruction Selection & The M6809 Addition Zoo

GCC's M6809 backend (`m6809.md`) achieves industry-leading code density and speed by employing **cost-based multi-alternative matching**. Instead of fixed code emission templates, it defines multiple code generation alternatives for every operation based on where operands reside (**Memory**, **Accumulators**, **Index Registers**, **Immediate Constants**, or **Stack Slots**), ranking them by byte size and cycle cost.

Below is the complete M6809 addition taxonomy, condition code flag dynamics, and a blueprint for adopting multi-alternative selection in MiniGolf.

---

### A. The Complete M6809 Addition Matrix

| Instruction | Operands & Direction | Bytes | Cycles | **C** (Carry) | **N** (Negative) | **Z** (Zero) | **V** (Overflow) | **H** (Half-Carry) | Strategic Strengths & Use Cases |
| :--- | :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :--- |
| **`ABX`** | $X \leftarrow X + \text{unsigned}(B)$ | **1 B** | **3** | **No** | **No** | **No** | **No** | No | **Fastest addition in M6809.** 1-byte opcode, 3 cycles. **Preserves ALL flags!** Ideal for 8-bit unsigned array indexing (`arr[b]`). |
| **`LEAr D,r`** | $r \leftarrow r + D$ ($r \in \{X, Y, U, S\}$) | **2 B** | **4** | **No** | **No** | $Z$ (X,Y) | **No** | No | 16-bit register in-place addition. On $U$ and $S$, touches **ZERO flags**. |
| **`LEAr1 D,r2`** | $r_1 \leftarrow r_2 + D$ ($r_1 \ne r_2$) | **2 B** | **4** | **No** | **No** | $Z$ (X,Y) | **No** | No | **True 3-operand non-destructive adder.** Neither source $r_2$ nor $D$ is modified! Zero spills. |
| **`LEAr1 B,r2`** | $r_1 \leftarrow r_2 + \text{signext}(B)$ | **2 B** | **4** | **No** | **No** | $Z$ (X,Y) | **No** | No | Signed 8-bit offset addition to pointer without needing `sex` into D. |
| **`LEAr n,r`** | $r \leftarrow r + n$ (constant $n$) | **2–3 B** | **4–5** | **No** | **No** | $Z$ (X,Y) | **No** | No | Fast pointer stepping and constant bump (`leax 1,x`, `leau 1,u`). |
| **`INCB` / `INCA`** | $B/A \leftarrow B/A + 1$ | **1 B** | **2** | **No** | **Yes** | **Yes** | **Yes** | No | 1-byte accumulator increment. **Preserves Carry!** Can bump loop indices in multi-precision arithmetic. |
| **`INC <ea>`** | $M \leftarrow M + 1$ | **2–3 B** | **6–7** | **No** | **Yes** | **Yes** | **Yes** | No | In-memory increment without loading into a register. **Preserves Carry!** |
| **`ADDB` / `ADDA`** | $B/A \leftarrow B/A + M_{8}$ | **2–3 B** | **2–5** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | Full 8-bit arithmetic with all condition flags. |
| **`ADCB` / `ADCA`** | $B/A \leftarrow B/A + M_{8} + C$ | **2–3 B** | **2–5** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | Multi-precision addition (word 2 of 32-bit, or bignums). |
| **`ADDD <ea>`** | $D \leftarrow D + M_{16}$ | **2–4 B** | **4–7** | **Yes** | **Yes** | **Yes** | **Yes** | No | Full 16-bit accumulator addition with signed/unsigned flags. |
| **`,R+` / `,R++`** | $R \leftarrow R + 1$ or $R + 2$ | **0 B** | **0** | **No** | **No** | **No** | **No** | No | Implicit hardware post-increment during load/store. Free pointer advancement. |

---

### B. M6809 Addition Superpowers to Exploit

#### 1. Three-Operand Non-Destructive Addition (`LEAr1 D,r2`)
On most 8/16-bit processors (like 6502 or Z80), adding two registers destroys one of the sources ($A \leftarrow A + B$).
On the 6809, `lea<dest> d,<src>` produces $r_1 \leftarrow r_2 + D$ in 2 bytes and 4 cycles without altering $r_2$ or $D$:
```asm
leax d,y    ; X = Y + D  (Y and D remain intact!)
leau d,x    ; U = X + D  (X and D remain intact!)
```
* **Impact**: Eliminates register save/restore spills when an offset or base pointer is needed again in the same basic block.

#### 2. Flag-Transparent Additions (`ABX`, `LEAU`, `LEAS`)
* `ABX` modifies **zero condition codes**.
* `LEAU` and `LEAS` modify **zero condition codes** (not even Zero flag $Z$).
* **Impact**: We can insert loop counter bumps (`leau 1,u`) or stack pops (`leas 2,s`) directly between a comparison and a conditional branch without corrupting condition codes:
  ```asm
  cmpd 8,s
  leau 1,u    ; Bump counter: CC flags from cmpd are 100% PRESERVED!
  blt .Lloop  ; Branch correctly branches on cmpd result!
  ```

#### 3. Carry-Preserving Increments (`INCB`, `INC`)
* Standard additions set $C$.
* `INCB` and memory `INC` set $N, Z, V$, but **leave the Carry bit untouched**.
* **Impact**: Bignum loops and multi-word arithmetic can update loop indices or counters without spilling the Carry flag to the stack.

#### 4. Fused Add-and-Access via Accumulator Indexing
If an addition is immediately followed by a memory read or write, the addition instruction should not be emitted at all:
```asm
; Instead of:
leax d,y
ldb ,x
; Emit fused indexed load:
ldb d,y     ; 1 instruction, 2 bytes, 7 cycles!
```

#### 5. Pre-Decrement Stores (`stu ,--s`, `stx ,--s`, `std ,--s`) Beat `PSHS`
When pushing a single 16-bit register to the hardware stack, pre-decrement store addressing is **the exact same code size (2 bytes) but faster than `PSHS`**:
* **`pshs u`**: Opcode `$34` + postbyte `$40` = **2 bytes**, **7 cycles** (5 base + 2 bytes pushed).
* **`stu ,--s`**: Opcode `$EF` + postbyte `$A3` = **2 bytes**, **6 cycles** (4 base + 2 pre-dec penalty).
  * **Result**: **1 cycle faster**, exact same byte size!
* **`stx ,--s` vs `pshs x`**: Same win! `stx ,--s` is **6 cycles, 2 bytes** vs `pshs x` at **7 cycles, 2 bytes**.
* **`std ,--s` vs `pshs d`**: `std ,--s` is **6 cycles, 2 bytes** vs `pshs d` at **7 cycles, 2 bytes**.
* **Complementary Pull (`ldd ,s++` vs `puls d`)**:
  * `puls d` takes **7 cycles, 2 bytes** (5 base + 2 bytes pulled).
  * `ldd ,s++` takes **5 cycles, 2 bytes** (4 base + 1 post-inc).
  * **Net Pair Savings**:
    - `pshs d; puls d` = **14 cycles, 4 bytes**.
    - `std ,--s; ldd ,s++` = **11 cycles, 4 bytes** (**3 cycles faster!**).
* **Flag Differences**:
  - `PSHS` preserves all condition flags.
  - `STU`, `STX`, `STD` set $N$ and $Z$, and clear $V$ ($V=0$), leaving Carry ($C$) untouched.
  - In expression evaluation pipelines (e.g. `stu ,--s; addd ,s++`), the subsequent arithmetic (`addd`) overwrites $N, Z, V, C$ anyway, making the flag side-effects completely harmless!

---

### C. Blueprint: Multi-Alternative Emission Rules for MiniGolf

When compiling an addition `z = x + y`, MiniGolf should inspect the storage classes of `x` and `y` and pick the cheapest alternative:

```
Rank 1 (0–3 cycles, 0–1 byte):
  - Invariant array access:              ldb/stb ,r+ (autoincrement, 0 cycles)
  - Unsigned byte index into pointer:    ldx #arr; abx (3 cycles, 1 byte)

Rank 2 (4 cycles, 2 bytes):
  - Constant increment in register:      lea<r> 1,<r> (4 cycles, 2 bytes)
  - Index register + Accumulator:        lea<dest> d,<src> (4 cycles, 2 bytes)
  - Accumulator + immediate #1:          incb (2 cycles, 1 byte)

Rank 3 (4–6 cycles, 2–3 bytes):
  - Accumulator D + immediate:           addd #c (4 cycles, 3 bytes)
  - Accumulator D + Direct EA memory:    addd <ea> (5–6 cycles, 2–3 bytes)

Rank 4 (Stack temporary fallback, 15 cycles, 4 bytes):
  - Pre-decrement push + pulling add:    stu ,--s; addd ,s++ (15 cycles, 4 bytes)
    (Faster than legacy: pshs u; addd ,s++ at 16 cycles)
```
By prioritizing Rank 1 and Rank 2 over stack spilling, MiniGolf can eliminate tens of cycles per loop iteration across all benchmarks.

---

## 6. Top Optimization Opportunities & Middle-End / Backend Roadmap

### Current Benchmark Status & Bottleneck Analysis

| Benchmark | MiniGolf (Updated) | GCC 6809 Max | MG vs GCC | Primary Bottleneck & Opportunity |
| :--- | :---: | :---: | :---: | :--- |
| `01_putchar` | **78** (44 B) | 66 (87 B) | 1.18x | Function call overhead & entry trampolines |
| `02_count_loop` | **766** (104 B) | 531 (133 B) | 1.44x | Induction variable pinning in registers across iterations |
| `03_arithmetic` | **14,348** (702 B) | 16,409 (548 B) | **0.87x (faster!)** | MiniGolf runtime math routines beat GCC |
| `04_fibonacci` | **20,309** (579 B) | 11,519 (333 B) | 1.76x | Call convention (`pshs d` + `leas 2,s` on every call) |
| `05_array_sum` | **15,433** (1226 B) | 11,836 (348 B) | 1.30x | Indexed addressing vs autoincrement `,x+` / `,x++` |
| `06_string_ops` | **18,710** (1728 B) | 5,319 (466 B) | **3.52x** | 8-bit accumulator allocation (`A`/`B`) & pointer autoincrement |
| `07_sieve` | **26,792** (679 B) | 14,800 (329 B) | 1.81x | No 8-bit register allocation for byte arrays/flags |
| `08_bubble_sort` | **72,137** (1034 B) | 41,207 (395 B) | 1.75x | Nested loop spills & redundant memory writes |
| `09_struct_ops` | **10,006** (1218 B) | 7,891 (303 B) | 1.27x | Stack frame allocation for composite temporaries |
| `10_switch_case` | **11,041** (907 B) | 6,794 (569 B) | 1.63x | Cascaded `if-else` chains instead of jump tables |

---

### Prioritized Optimization Projects

#### 1. Algebraic Simplifications, Sizeof Constant Folding & Pre-Inlining Cleanup (COMPLETED)
* **Status**: **Completed** ([`opt/constfold.go`](file:///home/strick/github.com/strickyak/minigolf/opt/constfold.go), [`opt/copyprop.go`](file:///home/strick/github.com/strickyak/minigolf/opt/copyprop.go), [`opt/strength.go`](file:///home/strick/github.com/strickyak/minigolf/opt/strength.go), [`opt/inline.go`](file:///home/strick/github.com/strickyak/minigolf/opt/inline.go)).
* **Implementation Details**:
  - **Sizeof Folding**: Folded `sizeof(byte)` and `sizeof(bool)` to 1 on all backends; wired target `WordSize` (2 bytes for M6809, 8 bytes for AMD64/CBE) to fold `sizeof(int)`, `sizeof(word)`, pointers, and arrays of known element types.
  - **Algebraic Identities**: Added identity rules:
    - Multiplications / Shifts: $x \times 1 \rightarrow x$, $1 \times x \rightarrow x$, $x / 1 \rightarrow x$, $x \times 0 \rightarrow 0$, $0 \times x \rightarrow 0$, $x \ll 0 \rightarrow x$, $x \gg 0 \rightarrow x$.
    - Additions / Subtractions: $x + 0 \rightarrow x$, $0 + x \rightarrow x$, $x - 0 \rightarrow x$, $x - x \rightarrow 0$.
    - Bitwise / Idempotent: $x \mid 0 \rightarrow x$, $x \oplus 0 \rightarrow x$, $x \oplus x \rightarrow 0$, $x \ \& \ 0 \rightarrow 0$, $x \ \& \ x \rightarrow x$, $x \mid x \rightarrow x$.
    - Self Comparisons: $x == x \rightarrow 1$, $x \le x \rightarrow 1$, $x \ge x \rightarrow 1$, $x \ne x \rightarrow 0$, $x < x \rightarrow 0$, $x > x \rightarrow 0$.
  - **Pre- & Post-Inlining Cleanups**: Cleaned up functions before inlining rounds so small pointer manipulation helpers (`pointer_post_increment`, `pointer_add`) shrink under `MaxTinyInstructions` and inline cleanly.
  - **Strength Reduction on M6809**: Enabled `StrengthReductionPass` on M6809, converting array indexing `i * 2` into 2-instruction bit shifts (`aslb; rola`) and eliminating `jsr __mul16`.
* **Empirical Results**:
  - `05_array_sum`: Cycles dropped from **18,403 to 15,433** (**-16.1%**); payload dropped from **1,450 B to 1,226 B** (**-15.4%**).
  - `06_string_ops`: Cycles dropped from **28,410 to 18,710** (**-34.1%**, **-9,700 cycles!**); payload dropped from **1,993 B to 1,728 B** (**-13.3%**).

---

#### 2. Inlining Enhancements (Cross-Backend) (COMPLETED)
* **Status**: **Completed** ([`opt/inline.go`](file:///home/strick/github.com/strickyak/minigolf/opt/inline.go), [`opt/inline_test.go`](file:///home/strick/github.com/strickyak/minigolf/opt/inline_test.go), [`opt/constfold.go`](file:///home/strick/github.com/strickyak/minigolf/opt/constfold.go)).
* **Implementation Details**:
  - **Multi-Block / Diamond CFG Inlining**:
    - Added `isAcyclic(f)` using 3-color DFS to detect and verify DAG control flow graphs.
    - Added `topologicalSort(blocks)` to guarantee dominators and definition blocks are processed before uses.
    - Implemented `inlineCFG`: splits the caller block into head and tail blocks, clones callee blocks with unique block IDs, rewires incoming/outgoing edges and existing Phi references, converts callee returns into jumps to the tail block, and joins multiple return values with a `*ir.Phi` node at the head of the tail block.
    - Enhanced `cloneInstruction` to clone `*ir.Phi`, `*ir.ConstStruct`, `*ir.ConstArray`, and `*ir.IndirectCall`.
  - **Popularity-Weighted Inlining Budget**:
    - Leveraged `caller.Popularity` and `callee.Popularity`: scaled tiny instruction budget $3\times$ (to 24 inst / 8 blocks) in deep loops ($\ge 100$) and $2\times$ (to 16 inst / 6 blocks) in loop bodies ($\ge 10$), while preserving strict budgets in cold initialization code.
    - Added Leaf Function Promotion bonus (+4 inst) when caller is LeafLevel 2 and callee is LeafLevel 1.
    - Expanded single-call site inlining budget to 64 instructions / 16 blocks (128 instructions / 24 blocks for trunk callers).
  - **Signed Comparison Folding Fix**:
    - Fixed `foldCompare` in `opt/constfold.go` to evaluate signed comparisons (`int64(cLeftW.Val) < int64(cRightW.Val)`) when operand types are `TypeInt`, correctly folding inlined `abs_int(-42)` to 42.
* **Empirical Results**:
  - `08_bubble_sort`: Cycles dropped from **72,137 to 53,773** (**-18,364 cycles, -25.5%!**); ratio vs GCC narrowed to **1.27x** (down from 1.75x).
  - `05_array_sum`: Cycles dropped from **15,433 to 14,244**; payload dropped from **1,226 B to 807 B** (**-419 bytes, -34.2%!**).
  - `06_string_ops`: Cycles dropped from **18,710 to 17,485**; payload dropped from **1,728 B to 1,309 B** (**-419 bytes, -24.2%!**).
  - `10_switch_case`: Cycles dropped from **11,041 to 9,313** (**-1,728 cycles!**).

---

#### 3. Register Passing Calling Convention for M6809 (Fastcall) (COMPLETED)
* **Status**: **Completed** ([`m6809/convention.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/convention.go), [`m6809/convention_test.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/convention_test.go), [`m6809/backend.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/backend.go), [`main.go`](file:///home/strick/github.com/strickyak/minigolf/main.go)).
* **Implementation Details**:
  - **Policy-Based Architecture**:
    - Created modular `ConventionPolicy` interface and `FunctionConvention` abstraction.
    - Implemented `FastcallPolicy` (default for whole-program compilation):
      - 1st parameter: passed in accumulator `D` (or `B` for byte types).
      - 2nd parameter: passed in index register `X` (if 16-bit).
      - Parameters 3+ (and composite types `size > 2`): passed on the stack.
    - Implemented `StackPolicy` (legacy stack-only convention for benchmarking and fallback via `-no-fastcall6809` / `NO_FASTCALL6809`).
    - Implemented `GCCPolicy` (GCC 6809 convention: 1st word in `X`, 1st byte in `B`).
  - **Interpolation Point & ABI Safety**:
    - External functions with linkage overrides (`f.Linkage != ""`), bodyless declarations, and functions whose addresses are taken (`b.funcAddressTaken`) automatically fall back to `StackPolicy` to maintain 100% interoperability with external C calls and indirect calls.
    - Runtime `putchar` passes character argument directly in `B`, eliminating `pshs b` and `leas 1,s` around Hatvan Hypercall 132.
  - **Caller & Callee Codegen**:
    - Caller pushes stack arguments right-to-left first, then loads register arguments into `D`/`B` and `X` immediately prior to `jsr`, completely omitting `pshs` and `leas` for register parameters.
    - Callee allocates local slots for register parameters and saves them on entry, preserving full addressability and local EA tracking.
* **Empirical Results**:
  - `04_fibonacci`: Cycles dropped from **20,309 to 18,883** (**-1,426 cycles, -7.0% faster!**); payload dropped from **579 B to 537 B** (**-42 bytes!**).
  - `08_bubble_sort`: Payload dropped from **1,034 B to 933 B** (**-101 bytes, -9.8% smaller!**); cycles dropped from **53,773 to 53,445**.
  - `06_string_ops`: Payload dropped from **1,309 B to 1,242 B** (**-67 bytes!**); cycles dropped from **17,485 to 17,208** (**-277 cycles!**).
  - `03_arithmetic`: Payload dropped from **702 B to 672 B** (**-30 bytes!**); cycles: **14,338** (still solidly beating CMOC 14,679 and GCC 16,390).
  - `05_array_sum`: Payload dropped from **807 B to 769 B** (**-38 bytes!**); cycles dropped to **14,148** (**-96 cycles!**).
  - `07_sieve`: Payload dropped from **679 B to 649 B** (**-30 bytes!**).
  - `09_struct_ops`: Payload dropped from **1,218 B to 1,174 B** (**-44 bytes!**).
  - `10_switch_case`: Payload dropped from **907 B to 869 B** (**-38 bytes!**); cycles dropped to **9,280**.
  - **Aggregate code size savings across all 10 benchmarks: over 390 bytes shaved!**
  - **Full test suite passes 100% across all 3 backends and all 8 M6809 architectural variants.**

---

#### 4. Pointer Arithmetic Scaling & Native Statement Idioms (COMPLETED)
* **Status**: **Completed** ([`ir/builder.go`](file:///home/strick/github.com/strickyak/minigolf/ir/builder.go), [`cbe/cbe.go`](file:///home/strick/github.com/strickyak/minigolf/cbe/cbe.go), [`ctranslator/translator.go`](file:///home/strick/github.com/strickyak/minigolf/ctranslator/translator.go), [`tests/pointer_inc.golf`](file:///home/strick/github.com/strickyak/minigolf/tests/pointer_inc.golf)).
* **Implementation Details**:
  - **IR Pointer Arithmetic Scaling**:
    - Fixed `ir/builder.go` for `ast.IncDecStatement` (`p++`, `p--`) and `ast.OpAssignStatement` (`p += n`, `p -= n`) to scale pointer offsets by element size `sizeof(T)` instead of literal 1 byte.
    - Updated `cbe/cbe.go` to emit pointer `BinaryOp` as integer address arithmetic cast back to pointer type `((T*)((word)(p) + (word)(offset)))`, maintaining identical byte-offset semantics across CBE, AMD64, and M6809.
  - **Idiomatic Non-Escaping C Translation**:
    - Updated `ctranslator/translator.go` to recognize statement-level pointer increments and copy idioms:
      - `*dst++ = *src++` -> `*dst = *src; dst++; src++`
      - `*dst++ = rhs` -> `*dst = rhs; dst++`
      - `lhs = *src++` -> `lhs = *src; src++`
      - `fn(*p++)` / `fn(p++)` -> `fn(*p); p++` / `fn(p); p++`
    - Eliminated generic prelude helper calls (`pointer_post_increment(&p)`), preventing pointers from escaping (`&p`).
    - With pointers no longer escaping, SSA register allocation places pointer variables (`dst`, `src`) into 16-bit index registers `X`, `Y`, and `U` across entire loop lifespans.
* **Empirical Results**:
  - `06_string_ops`: Cycles dropped from **17,208 to 13,540** (**-3,668 cycles, -21.3% faster!**); payload dropped from **1,280 B to 1,178 B** (**-102 bytes, -8.0% smaller!**).
  - Ratio vs GCCMax narrowed from **3.24x down to 2.55x**.
  - Verified across all 3 backends and all 8 M6809 architectural variants with 100% test pass.

---

#### 5. 8-Bit / Byte Register Allocation (`A` and `B` Accumulators)
* **Goal**: Prevent continuous stack spilling for 8-bit types.
* **Current Bottleneck**: [`AllocateRegisters`](file:///home/strick/github.com/strickyak/minigolf/m6809/regalloc.go) explicitly ignores types with `size != 2`. Byte variables (`uint8`, `char`, `bool`) are always stored to and loaded from stack slots (`stb 0,s; ldb 0,s`).
* **Proposed Design**:
  - Allocate accumulator `B` (and/or `A`) to hot 8-bit SSA values across basic blocks where `D` is not needed as a 16-bit scratch.
  - Allow byte pointers to remain in index registers `X`/`Y` while accumulator `B` acts as the dereferenced character buffer.
* **Target Impact**: Drastically closes the performance gap in byte-heavy benchmarks like `06_string_ops` and `07_sieve`.

---

#### 6. Native Autoincrement / Autodecrement Addressing
* **Goal**: Directly exploit 6809 hardware addressing modes `,x+`, `,x++`, `,-x`, `,--x` during code generation.
* **Current Bottleneck**: Pointer increment loops (`while (*s) { ... s++; }`) often emit separate pointer adjustments (`addd #1`, `std ptr`) and reloads instead of combining memory load/store with pointer advancement.
* **Proposed Design**: Pattern-match memory load/store and pointer update pairs in `emitInstr` or expand peephole recognition across intermediate register moves.
* **Target Impact**: Replaces 3 instructions (~14 cycles) with 1 instruction (~5 cycles) in string and array traversal loops.

---

#### 7. Jump Tables for Dense `switch` Statements
* **Goal**: Transform $O(N)$ sequential comparisons into $O(1)$ constant-time dispatch.
* **Current Bottleneck**: [`ctranslator`](file:///home/strick/github.com/strickyak/minigolf/ctranslator) compiles `switch` statements into cascaded `if-else` chains, requiring up to $N$ compare-and-branch instructions.
* **Proposed Design**:
  - Detect dense integer case ranges ($[\text{min}, \text{max}]$).
  - Emit M6809 indirect indexed jumps: `subd #min; aslb; rola; ldx #table; jmp [d,x]`.
* **Target Impact**: In `10_switch_case`, dispatch overhead becomes constant time regardless of case count, closing the gap with GCC and CMOC.

---

### Progress Update: String Operations & Tight Loop Optimization Milestone

* **Completed Phases**:
  1. **String Literal Direct Address Cast ([`ir/builder.go`](file:///home/strick/github.com/strickyak/minigolf/ir/builder.go))**:
     - `(*byte)(stringLiteral)` now evaluates directly to `&AddressOfGlobal{Global: g}` rather than constructing a temporary 6-byte slice struct on the stack, zeroing it with `__memset0`, and extracting field 0.
     - Eliminated all string literal slice initialization overhead and temporary allocations.
  2. **Scalar `ZeroInit` Constant Folding ([`opt/constfold.go`](file:///home/strick/github.com/strickyak/minigolf/opt/constfold.go))**:
     - Folded `ZeroInit` of byte, bool, word, int, and pointers to `ConstByte{Val: 0}` or `ConstWord{Val: 0}`.
     - Eliminated unnecessary stack slot allocations and `clr N,s` instructions across loops checking values against zero.
  3. **Zero-Store Redundant Load Elimination ([`opt/store_load.go`](file:///home/strick/github.com/strickyak/minigolf/opt/store_load.go))**:
     - Propagates available loads across single-predecessor basic blocks (`len(b.Predecessors) == 1`) as long as zero stores or function calls have occurred.
     - Automatically invalidates available loads on any `StorePtr`, `Store`, `InsertFieldPtr`, or function call.
     - Successfully eliminated redundant reload of `*src` in `my_strcpy` loop body.
  4. **Comparison Narrowing on Zero-Extended Operands ([`opt/constfold.go`](file:///home/strick/github.com/strickyak/minigolf/opt/constfold.go))**:
     - `zero_ext(b1) op zero_ext(b2)` narrowed directly to `b1 op b2`, eliminating 16-bit extension overhead when comparing bytes.
  5. **M6809 Backend Peephole Optimizations ([`m6809/peephole.go`](file:///home/strick/github.com/strickyak/minigolf/m6809/peephole.go))**:
     - Extended autoincrement/autodecrement addressing to register `U` (`leau 1,u` / `leau 2,u` -> `,u+` / `,u++`, and `leau -1,u` / `leau -2,u` -> `,-u` / `,--u`).
     - Added redundant `tstb` / `tsta` elimination when preceded by instructions that already set condition codes (`ldb`, `stb`, `addb`, `subb`, `andb`, `orb`, `eorb`, `negb`, `clrb`).
     - Added redundant load after store elimination across non-modifying conditional branches.

* **Benchmark Results (`06_string_ops`)**:
  - **CPU Cycles**: Dropped from **24,984 to 10,182** (**-59.2% reduction, -14,802 cycles!**).
  - **Loaded Payload**: Dropped from **1,401 B to 890 B** (**-511 bytes, -36.5% smaller!**).
  - **Performance Ratio vs GCC**: Narrowed from **4.70x down to 1.80x**!
  - **Verification**: 100% pass across all unit tests and all 8 M6809 architectural variants (`ALL_VARIANTS=1 time go test ./...`).

