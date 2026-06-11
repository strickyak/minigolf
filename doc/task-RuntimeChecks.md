# Bounds and Nil Checking

- `[x]` Add `-check-bounds` and `-check-nil` flags to `main.go`.
- `[x]` Read `CHECK_BOUNDS` and `CHECK_NIL` environment variables in `main.go`.
- `[x]` Add `CheckBounds` and `CheckNil` to `ir.Builder`.
- `[x]` Implement `emitBoundsCheck` and `emitNilCheck` in `ir/builder.go`.
- `[x]` Add nil checks for pointer dereferences, method receivers, and indirect function calls.
- `[x]` Add bounds checks for array and slice indexing.
  - `[x]` Account for `Chop` allowing `idx == len`.
- `[x]` Verify with tests.
