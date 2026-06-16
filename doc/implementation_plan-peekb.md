# M6809 Backend Optimization Implementation Plan

We will optimize the compiler to produce close to optimal Motorola 6809 machine code for simple memory-access functions like [prelude.peekb](file:///home/strick/antig/prelude/prelude.go#L16). The goal is to reduce the current 17-instruction output for `peekb` to the optimal 2 instructions:
```assembly
f_prelude.peekb:
	ldb ,x
	rts
```

We propose a detailed 5-step plan where each step can be developed, tested, and verified independently.

## User Review Required

No critical breaking changes are expected. Each phase is fully self-contained and verifiable via the existing compiler test suites.

## Open Questions

None.

## Proposed Changes

### Phase 1: AST Escape Analysis Pre-Pass
- **Component**: [ir/builder.go](file:///home/strick/antig/ir/builder.go)
- **Goal**: Implement a pass to identify which local variables and parameters have their address taken in the AST.
- **Details**:
  - Before building the IR for a function, traverse its AST to locate references where the variable has its address taken (e.g. `&x` or generic pointer coercion).
  - Populate a set of escaping variable names `addressTakenVars map[string]bool`.
  - Verify that compiler tests pass cleanly with this tracking in place.

---

### Phase 2: Promote Non-Escaping Variables to SSA Values
- **Component**: [ir/builder.go](file:///home/strick/antig/ir/builder.go)
- **Goal**: Stop generating `AddressOfLocal` and `LoadPtr`/`StorePtr` for non-escaping variables and parameters.
- **Details**:
  - When evaluating an identifier in `eval` (around line 2147), if the identifier is a local variable/parameter that does NOT escape:
    - Return it as a pure SSA value (`IsLValue: false`, `Value: b.readVariable(name)`).
  - When building an assignment to a non-escaping local variable, directly update its value using `b.writeVariable(...)` instead of generating a pointer store.
  - Verify that all compiler tests pass for all backends (CBE, x86_64, and M6809).

---

### Phase 3: No-Op Cast optimization
- **Component**: [m6809/backend.go](file:///home/strick/antig/m6809/backend.go)
- **Goal**: Elide code generation for zero-cost type casts.
- **Details**:
  - `word_to_ptr` is a no-op on M6809 since both words and pointers are 16-bit.
  - Update [m6809/backend.go](file:///home/strick/antig/m6809/backend.go) to bypass emitting code for `word_to_ptr` casts and directly forward the operand's location/register.
  - Verify all compiler tests pass successfully.

---

### Phase 4: Register-Tracking for Incoming Parameters
- **Component**: [m6809/backend.go](file:///home/strick/antig/m6809/backend.go)
- **Goal**: Prevent immediate spilling of registers for parameters at the function start.
- **Details**:
  - Currently, parameters passed in `X` and `B` are immediately written to the stack.
  - Update `emitFunc` so that if the parameter is not address-taken, the compiler registers it in `b.valInReg` as active in register `X` or `B`, avoiding immediate spilling to stack slots.
  - Spill it only if the register is claimed by other instructions.
  - Verify all compiler tests pass successfully.

---

### Phase 5: Stack Frame Elision for Zero-Byte Functions
- **Component**: [m6809/backend.go](file:///home/strick/antig/m6809/backend.go)
- **Goal**: Clean up function prologues/epilogues for simple leaf/trunk functions.
- **Details**:
  - If a leaf function has a stack frame size of 0 bytes, make sure we do not emit any register saving/restoring or stack adjustments, allowing a direct return.
  - Verify that `peekb` compiles to exactly two instructions (`ldb ,x \n rts`).
  - Run the entire test suite `go test -count=1 ./...` to ensure no regressions.

---

## Verification Plan

### Automated Tests
- Run `go test -count=1 -run "CBE|x86_64"` after each phase to verify that intermediate changes are correct.
- Run `go test -count=1 ./...` at the end of the plan to ensure full correctness across all backends.

### Manual Verification
- Compile `tests/test_logical.golf` or other simple test files with `-m M6809 -debug_opt` to inspect the generated assembly code and verify the reduction of instruction counts.
