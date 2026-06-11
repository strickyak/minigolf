# Panic Infrastructure Implementation Plan

This plan details the implementation of the Panic Infrastructure proposed in `doc/panic.md`, incorporating the `setjmp`/`longjmp` mechanism and the per-defer block improvements discussed previously.

## User Review Required

> [!WARNING]
> **Prototype Mismatch**: The proposal specifies `func panic(*byte) panicked`. However, `prelude/prelude.go` currently defines `func panic(w word)` and uses it extensively with integer error codes (e.g., `panic(4001)`). 
> **Decision needed:** Should we update all `panic(word)` calls in `prelude.go` to pass strings (e.g., `panic("4001")`), or should we change the proposal to accept `word`? (This plan assumes we update `prelude.go` to use `*byte` strings).

> [!IMPORTANT]
> **M6809 Assembly**: M6809 does not link against a standard C library, so we will need to implement `setjmp` and `longjmp` natively in `prelude.excerpt.asm` (or equivalent `m6809` runtime assembly).

## Open Questions

1. **Compiler Flags**: The proposal mentions `--bounds_checks` and `--nil_checks`. Should we implement these flags and their corresponding IR emission in this project phase, or strictly build the panic/recover *infrastructure* first and add the automatic bounds/nil checks in a follow-up? (This plan focuses on building the infrastructure first).
2. **`panicked` Type**: Does the `panicked` type need to be exposed as a keyword in the lexer so user code can declare `func myFunc() panicked`, or is it strictly an internal compiler type for the `panic` builtin?

## Proposed Changes

---

### AST & Semantic Analysis

#### [MODIFY] `ast/ast.go` & `semantic/type.go`
- Introduce `TypePanicked` as a new bottom type (similar to `void`).
- If `panicked` is to be a user-accessible keyword, add it to the lexer/parser.
- Update type checking so that a function returning `panicked` is considered a valid terminal branch (like a return statement).

#### [MODIFY] `prelude/prelude.go`
- Change `func panic(w word)` to `func panic(msg *byte) panicked`.
- Update all internal `panic(...)` calls in `prelude.go` to pass string literals instead of words.
- Define `var _panic_ *byte` and `var _jmp_chain_ *byte` (as opaque pointers) globally in `prelude.go` so they can be accessed by the compiler and user code.

---

### IR Generation

#### [MODIFY] `ir/builder.go`
- **Defer Block Refactoring**: Instead of placing a single `setjmp` at the top of the function, we will inject a `setjmp` block dynamically when a `DeferStatement` is evaluated.
- When evaluating a `DeferStatement`:
  - Allocate a local `jmp_struct` on the stack.
  - Emit IR to link the `jmp_struct` into `_jmp_chain_`.
  - Emit an intrinsic `ir.SetJmp` instruction.
  - Branch based on the `setjmp` result:
    - **`0` (Normal execution):** Execute the body of the defer block. At the end of the block, emit IR to unlink `_jmp_chain_ = jumper.prev` and then execute the deferred action.
    - **`!= 0` (Panic occurred):** Emit IR to unlink `_jmp_chain_ = jumper.prev`, execute the deferred action, and then if `_panic_ != 0`, emit `ir.LongJmp(_jmp_chain_)` to propagate the panic upwards.
- **Return Statement Unlinking**: Ensure that `buildReturn` unlinks any active `jmp_struct`s from the `_jmp_chain_` for defers that haven't naturally exited their scope yet.
- **Panic Builtin**: When compiling a call to `panic()`, emit an `ir.Panic` instruction which sets `_panic_ = msg` and calls `longjmp(_jmp_chain_, 1)`.

---

### Compiler Backends

#### [MODIFY] `cbe/cbe.go`
- Emit C code for `setjmp` and `longjmp` intrinsics using `#include <setjmp.h>`.
- Define the `jmp_struct` C type globally.
- Implement emission for the new `ir.SetJmp` and `ir.LongJmp` instructions.

#### [MODIFY] `x86_64/backend.go`
- Implement `ir.SetJmp` by emitting a `call _setjmp` (relying on libc).
- Implement `ir.LongJmp` by emitting a `call _longjmp`.
- Allocate space for `jmp_buf` (200 bytes) on the stack for each active defer block.

#### [MODIFY] `m6809/backend.go` & `show/prelude.excerpt.asm`
- Write native M6809 assembly for `_setjmp` and `_longjmp`. `setjmp` will need to save the stack pointer `S`, frame pointer `U`, and PC.
- Implement `ir.SetJmp` and `ir.LongJmp` emission.
- Allocate space for `jmp_buf` (16 bytes) on the stack for each active defer block.

## Verification Plan

### Automated Tests
- Create `tests/test_panic.golf` to verify manual calls to `panic()` successfully propagate through multiple function layers and trigger deferred actions along the way.
- Create `tests/test_panic_recover.golf` (if recover is implemented) or ensure that an unhandled panic reaches `main` and cleanly aborts.
- Run `go test ./...` and `sh run9.sh` to ensure all backends (`cbe`, `x86_64`, `m6809`) correctly execute the panic jumps without corrupting the stack or infinite looping.
