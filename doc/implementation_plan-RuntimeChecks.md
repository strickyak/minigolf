# Implement bounds and nil pointer checking

This plan details the addition of two command-line flags, `-check-bounds` and `-check-nil`, to selectively enable runtime checks for slice/array bounds and nil pointer dereferences.

## Proposed Changes

### Add command line flags

#### [MODIFY] [main.go](file:///home/strick/antig/main.go)
- Add the `-check-bounds` and `-check-nil` command-line flags.
- Pass the values of these flags to the `ir.Builder` struct after `NewBuilder()` is invoked in the compilation pipelines.

### Implement runtime checks in the IR builder

#### [MODIFY] [ir/builder.go](file:///home/strick/antig/ir/builder.go)
- Add `CheckBounds` and `CheckNil` boolean fields to the `Builder` struct.
- Introduce helper methods to generate the check logic:
  - `emitBoundsCheck(idx Value, limit Value, token ast.Node)`: Generates an `if (idx >= limit)` block that emits a `panic("INDEX OUT OF BOUNDS")`.
  - `emitNilCheck(ptr Value, token ast.Node)`: Generates an `if (ptr == nil)` block that emits a `panic("NIL POINTER DEREFERENCE")`.
- In `buildExpr`, evaluate `*ast.IndexExpression`:
  - For array indexing, parse the array's length and inject `emitBoundsCheck`.
  - For slice indexing, fetch the `Len` field from the slice struct and inject `emitBoundsCheck`.
- In `buildExpr`, evaluate `*ast.PrefixExpression` (dereference):
  - Inject `emitNilCheck` before resolving the memory address.
- In `buildExpr`, evaluate `*ast.SelectorExpression`:
  - If the receiver is a pointer and we're resolving a struct field, inject `emitNilCheck`.
- In `buildExpr`, evaluate `*ast.CallExpression`:
  - If it is a method invocation with a pointer receiver, inject `emitNilCheck`.
  - If it is an indirect function call via a function reference, inject `emitNilCheck`.

## Verification Plan

### Automated Tests
- I will run the standard test suite `go test ./...` with and without `-check-bounds` and `-check-nil` to ensure there are no regressions.
- I will construct tests that purposefully perform out-of-bounds indexing and nil pointer dereferences and ensure they panic as expected.
