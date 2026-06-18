# Phase 6: Overlap global allocation for Trunk/Leaf functions at the same level

The primary goal is to allocate local variables and parameters globally for functions at Trunk Levels > 0, to avoid stack allocation and overlapping memory when it's safe (functions at the same level do not execute recursively or concurrently).

## Proposed Changes

### `m6809/backend.go`
- **Pre-scan for frame sizes**: Add a pass before generating the final assembly to calculate the `stackSize` required for every function.
- **Compute Level Maximums**: For each `TrunkLevel > 0`, find the maximum frame size required by any function at that level.
- **Global Allocation**: Emit a `.equ` or data section declaration for each `TrunkLevel > 0` with a name like `v__level_N_frame` and size equal to the calculated maximum.
- **Update `getSlot` and `paramSlots`**: If `b.f.TrunkLevel > 0`, map variables to positive offsets from `0` to `max_size`, instead of negative offsets from the frame pointer.
- **Update `memAccess`**:
  - If `b.f.TrunkLevel > 0`, return an absolute memory access string: `v__level_N_frame + offset`.
  - For PIC mode, return `v__level_N_frame+offset,pcr`.
- **Function Entry/Exit**: Do not emit `leas -%d,s` (stack reservation) or frame pointer setup for functions with `TrunkLevel > 0`.

# Phase 7: Register-Tracking for Incoming Parameters

Parameters passed in registers (like `X` and `B`/`D`) are currently stored to their stack/global slots immediately upon function entry.

## Proposed Changes

### `m6809/backend.go`
- We will track which registers hold incoming parameters. Since `ir.Parameter` instances don't have unique `int` IDs like instructions, we can assign them pseudo-IDs (e.g., negative numbers based on their position index).
- Initialize `b.activeRegs` at the start of the function entry block with these parameter pseudo-IDs.
- Modify `emitLoad` and `resolveVal` to check if a parameter's pseudo-ID is currently in a register, avoiding unnecessary loads from memory.
- If the register needs to be spilled, it will be spilled to the parameter's designated slot (global or stack).

# Phase 8: Stack Frame Elision for Zero-Byte Functions

For functions that remain on the stack (`TrunkLevel == 0`), if their required `stackSize` is exactly 0, we can omit stack frame setup entirely.

## Proposed Changes

### `m6809/backend.go`
- **Skip `leas`**: In `emitFunc`, skip `leas -N,s` and `leas N,s` if `N == 0`.
- **Skip Frame Pointer**: If `useFramePointer` is enabled but the frame size is 0 and the function makes no calls that require a frame pointer, skip `pshs u` and `tfr s,u` to save cycles.

## Verification Plan

### Automated Tests
- Run `go test -v ./...` to ensure no existing tests break across any backend (CBE, X86_64, M6809).
- Compile sample GOLF programs with M6809 and inspect the output assembly to verify:
  - Variables use `v__level_X_frame+offset` instead of `,s` or `,u`.
  - Parameter stores are elided.
  - Frame setup is omitted for zero-byte leaf functions.

### Open Questions for the User
- For `globalsAtY` mode, should the shared Trunk/Leaf memory blocks be placed within the same global structure (using Y-relative offsets), or should we continue to allocate them with absolute addresses? I plan to put them in the Y-relative global space if `globalsAtY` is enabled to keep all RAM together.

### Phase 6: Overlap Global Allocation for Trunk/Leaf Functions (M6809)

**Goal:** Reduce stack usage and potentially improve speed by allocating local variables and parameters of non-reentrant functions (Trunks and Leaves) into shared global memory blocks instead of the hardware stack. Since functions at the same TrunkLevel (or LeafLevel) are mutually exclusive in the call graph, their local frames can be overlaid in the same memory locations.

#### Proposed Changes to `m6809/backend.go`:

1.  **Memory Model Tracking:**
    *   Introduce `maxTrunkSize map[int]int` and `maxLeafSize map[int]int` in `Backend` to track the maximum frame size needed for each TrunkLevel and LeafLevel.
    *   During `emitFunc`, if `f.TrunkLevel > 0`, allocate slots sequentially from `0` up to `frameSize`. Update `maxTrunkSize[f.TrunkLevel]`.
    *   If `f.TrunkLevel == 0` and `f.LeafLevel > 0`, do the same for `maxLeafSize[f.LeafLevel]`.
    *   If neither (or if it's a reentrant/recursive function), allocate on the hardware stack as before (negative offsets from S or U).

2.  **Modifying `memAccess`, `getAddrStr`, and `getSlot`:**
    *   `getSlot` will return a positive offset (from the start of the shared block) if the function qualifies for Trunk/Leaf allocation. Wait, to distinguish from stack offsets (which are negative), we can use a parallel map `globalSlots map[int]int` or simply check `b.f.TrunkLevel` inside `memAccess`.
    *   `memAccess(offset)` needs to check if the current function is using a global frame. If so, it returns an access string based on the shared memory block instead of `S` or `U`.
        *   If `globalsAtY` is false: `g_trunk_%d+%d` or `g_leaf_%d+%d`.
        *   If `globalsAtY` is true: `%d,y` (where the offset is `levelYOffsets[level] + local_offset`).

3.  **Global Data Emission:**
    *   In `Generate()`, after calculating the maximum sizes for all levels, emit the required data sections.
    *   If `globalsAtY` is false, emit `g_trunk_%d` and `g_leaf_%d` labels using `equ` with the appropriate sizes.
    *   If `globalsAtY` is true, assign offsets for each level sequentially within the `Y` block and populate `levelYOffsets`.

4.  **Parameter Passing & Registers:**
    *   Parameters are currently passed on the stack (except the first few which are in X/B).
    *   If a function uses global allocation, the *caller* still pushes arguments to the stack during the call? Wait!
    *   Currently, arguments are pushed to the stack before the `jsr`. Then the callee (`emitFunc`) either reads them from the stack and stores them into its own stack frame, or leaves them on the caller's stack.
    *   Let's check how `emitFunc` handles parameters: It currently iterates through `f.Parameters` and moves them from the stack (using `stackArgOffset`) to its local slots (`b.paramSlots[p.Name]`).
    *   So the calling convention doesn't need to change immediately; the callee will just copy from the stack arguments into its *global* local slots instead of its *stack* local slots.
    *   (Eventually, we might want to pass arguments directly to the global slots to save the push/pop, but for Phase 6, just allocating the function's own frame in global memory is a great first step).

5.  **Prologue / Epilogue (`emitFunc`):**
    *   If a function uses global allocation, `b.stackSize` for hardware stack allocation will be 0 (excluding anything specifically requiring the stack).
    *   We don't need `leas -N,s` and `leas N,s` for the frame size! This saves instructions.

#### Verification:
*   Run the M6809 backend tests to ensure tests still pass.
*   Inspect the generated assembly to verify that variables are mapped to `g_trunk_X` / `g_leaf_X` rather than offsets from `S`.

> [!WARNING]
> PIC Mode Address resolution: In PIC mode, we use `,pcr` relative addressing for global variables. For `v__level_N_frame + offset`, `v__level_N_frame+offset,pcr` is correct for M6809 assembly. Does this syntax work smoothly with your assembler?

> [!IMPORTANT]
> For Phase 7, parameters are accessed via `b.paramSlots[p.Name]`. To track them in `activeRegs`, they need integer IDs. Is it acceptable to assign pseudo-IDs (e.g., `-1`, `-2`) to parameters for register tracking?
