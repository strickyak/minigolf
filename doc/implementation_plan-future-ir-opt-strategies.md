# Future IR Optimizations Strategy

This document outlines implementation plans for six independent Intermediate Representation (IR) optimizations. They are designed so that any individual optimization can be built and merged independently of the others.

## User Review Required
Please review the six proposals below. Let me know which one(s) you would like to prioritize or tackle next.

---

### 1. Function Inlining
**Goal:** Eliminate function call overhead and expose smaller function bodies to intraprocedural optimizations (like CSE and Constant Folding) by merging them into the caller.

**Approach:**
1. Create `opt/inline.go`.
2. **Analysis:** Scan the program for `call` instructions to direct, known functions. Check if the target function is eligible for inlining (e.g., `len(Instructions) < Threshold`, no recursive calls, no complex control flow if restricted).
3. **Cloning:** When inlining `caller` -> `callee`:
   - Clone the `callee`'s Basic Blocks and Instructions, assigning new unique IDs.
   - Replace references to `callee` parameters with the actual arguments passed in the `call`.
4. **Wiring:** Split the caller's Basic Block at the `call` instruction. Wire the caller block to the entry of the cloned `callee`. Wire the `callee`'s `ret` instructions to a new continuation block in the caller, using a `Phi` node to gather the return value.
5. Register in `opt.go` with an `-no-inline` flag.

---

### 2. Mem2Reg (Memory to Register Promotion)
**Goal:** Convert local variables (currently heavily relying on stack allocation, `addrof_local`, `load_ptr`, `store_ptr`) into pure SSA virtual registers, completely bypassing memory access.

**Approach:**
1. Create `opt/mem2reg.go`.
2. **Escape Analysis:** Identify all local variables (`addrof_local`) that never "escape" (i.e., their pointer is never passed as a function argument, returned, or stored into another pointer).
3. **Dominance Frontiers:** Compute the Dominator Tree and Dominance Frontiers for the function's CFG.
4. **Phi Insertion:** For each non-escaping local variable, find all blocks where it is stored (`store_ptr`). Insert `Phi` nodes for the variable at the dominance frontiers of those blocks.
5. **Renaming:** Walk the dominator tree to replace `load_ptr` with the most recently stored SSA value, and replace `store_ptr` with a new SSA virtual register definition.
6. Delete the original `addrof_local` instructions.

---

### 3. Loop Invariant Code Motion (LICM)
**Goal:** Identify calculations or memory reads that do not change during a loop's execution and move them outside the loop to execute only once.

**Approach:**
1. Create `opt/licm.go`.
2. **Loop Detection:** Perform a depth-first search on the CFG to find back-edges (edges where the target dominates the source). The target is the "loop header."
3. **Pre-header Insertion:** Ensure each loop has a dedicated "pre-header" block immediately preceding the loop header. If one doesn't exist, split the incoming edges to create it.
4. **Invariant Detection:** Scan instructions inside the loop. An instruction is loop-invariant if:
   - All its operands are constants, OR
   - All its operands are defined outside the loop, OR
   - All its operands are already marked as loop-invariant.
5. **Hoisting:** If an invariant instruction is safe to move (has no side effects, and isn't an aliased pointer load that might be written to in the loop), move it to the end of the pre-header block.

---

### 4. Store-to-Load Forwarding (Local Alias Analysis)
**Goal:** A lightweight alternative/stepping-stone to Mem2Reg. Eliminates redundant memory reads by remembering recent writes within the same basic block.

**Approach:**
1. Create `opt/store_load.go`.
2. Iterate through each Basic Block linearly.
3. Maintain a map: `memoryMap[pointer_value] = stored_value`.
4. When encountering a `store_ptr(ptr, val)`, update the map `memoryMap[ptr] = val`.
   - *Safety Check:* If we encounter a `call` or an indirect `store_ptr` to an unknown pointer, we must conservatively clear the map to prevent aliasing bugs (or only track `addrof_local` pointers that don't escape).
5. When encountering a `load_ptr(ptr)`, check if `ptr` is in the map. If so, replace the load with the known `val` and delete the `load_ptr` instruction.

---

### 5. Unnecessary Parameter Elimination
**Goal:** Remove function parameters that are either never used, or are always passed the exact same constant/global value from every call site.

**Approach:**
1. Create `opt/param_elim.go`.
2. **Analysis Phase:**
   - Scan the entire program to find all direct `call` instructions.
   - Build a mapping of `Function -> []CallSites`.
   - If a function's address is taken (used as a value), mark it ineligible.
3. **Identification:** For each eligible function, analyze its parameters.
   - If a parameter is never referenced in the body, it can be dropped.
   - If all call sites pass the exact same value for the parameter (e.g., `const_word 5` or `addrof_global "foo"`), it can be dropped.
4. **Mutation:**
   - Change the function signature to remove the parameter.
   - If the parameter was constant, insert a new constant instruction inside the function body and replace all uses of the parameter with it.
   - Update all `call` instructions to omit the removed argument.

---

### 6. More Liberal Stack Slot Sharing
**Goal:** Relax the type-equality restriction during stack allocation so variables of different types can share the same memory slot if they have disjoint lifetimes, packing the stack frame tighter.

**Approach:**
1. Modify the existing `StackAllocPass` in `opt/stack_alloc.go`.
2. Currently, the pass checks if types are strictly identical (`p.Vars[id].Type.Equals(p.Vars[other].Type)`).
3. Change the logic to group variables by their byte `Size()` rather than their strict `Type`. 
   - A `word` (size 2), `int` (size 2), and pointer (size 2) can all share the same 2-byte slot.
   - A `slice` (size 6) could potentially share with another `slice` of a different type, or an array of size 6.
4. Update the backend physical allocator to reserve the maximum required size for the shared alias group. M6809 backend already relies on `Type.Size()`, so as long as the slot's physical footprint fits the largest variable in the sharing group, physical safety is guaranteed.
