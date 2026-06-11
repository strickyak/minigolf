# Panic Infrastructure Task List

### Panic Type and Builtin (AST/IR/Semantic)
- `[x]` Add `TypePanicked` to IR types (`ir.go`).
- `[x]` Register `panicked` as a builtin type in `semantic.go`.
- `[x]` Register `panic` as a builtin function in `semantic.go`.
- `[x]` Add `Panic`, `SetJmp`, `LongJmp`, `UnlinkJmp` and `PropagatePanic` as new IR instructions to handle the panicking control flow.
- `[x]` Handle the `panic` built-in correctly during IR construction (`builder.go`).

### `setjmp` / `longjmp` Infrastructure
- `[x]` Implement translation logic for `SetJmp`, `LongJmp`, `UnlinkJmp`, `PropagatePanic`, and `Panic` instructions in the CBE backend (`cbe.go`).
- `[x]` Embed C struct `jmp_struct` definitions into CBE generated source.
- `[x]` Generate `_panic_` and `_jmp_chain_` globals in `prelude.go`.
- `[x]` Wrap deferred action blocks with proper `setjmp` setup and teardown logic (`builder.go`), and propagate panic using `_propagate_panic_`.
- `[x]` Implement translation logic for `x86_64` backend with matching `setjmp` and `longjmp` hooks to libc (`backend.go`).
- `[x]` Implement `setjmp`/`longjmp` in `cbe/cbe.go`
- `[x]` Implement `setjmp`/`longjmp` in `x86_64/backend.go`
- `[x]` Implement `setjmp`/`longjmp` in `m6809/backend.go`.

### Refactoring `panic` Calls
- `[x]` Update the signature of `panic` in `prelude.go` to accept strings (`func panic(msg *byte) panicked`), or rather, remove its function body from `prelude.go` entirely.
- `[x]` Identify and update all `panic()` usages across the codebase to pass uppercase string literals instead of numbers (`prelude.go`).

### Testing
- `[x]` Create unit test `.panic.golf` specifically testing the `CBE` `panic()` + `defer` functionality.
- `[x]` Confirm no existing `.golf` tests are broken. `tests/test_panic.golf`.
- `[x]` Verify CBE backend executes panics and defer blocks correctly.
