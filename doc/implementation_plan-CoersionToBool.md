# Formalizing `bool` Coercion in IR

Currently, `bool` is simply treated as a structural alias for `byte`. This means coercion from types like `word`, `int`, pointers, or functions into `bool` effectively uses simple value truncation (via a `trunc` IR operation) into a 1-byte value. For values like `0x0100`, truncation to `byte` yields `0x00`, mistakenly resolving as `bool(0)` (false) despite the value being non-zero.

This plan defines the formal representation and coercion paths for `bool` within the compiler's Intermediate Representation (IR), ensuring all scalar, pointer, function, and slice types adhere to correct zero-value truthiness.

## Proposed Changes

### 1. Distinct `TypeBool` Representation
- **`ir/ir.go`**: Introduce `TypeBool` (`Type{Name: "bool", Expr: &ast.Identifier{Value: "bool"}}`) to decouple boolean logic from the underlying `TypeByte`.
- **`ir/builder.go`**: Update `astToIRType` to return `TypeBool` when parsing `"bool"` identifiers. Update `getTypeSize` to specify a size of `1` for `TypeBool`.

### 2. Formal Coercion via `Compare`
- **`ir/builder.go:coerceType()`**: Introduce a formal coercion branch for when `targetType.Equals(TypeBool)`. If the source expression is not already `TypeBool`:
  - **For Pointers/Funcs/Words/Ints/Bytes**: Emit a new `Compare` operation (`neq`) between the expression and a generated `ZeroInit` of the expression's original type. This evaluates directly to `TypeBool`.
  - **For Slices**: A "zero" slice evaluates to false. We will automatically emit a call to `prelude.memeq` against a `ZeroInit` struct to detect if the slice structure is completely empty, matching the `s != nil` comparison behavior.

### 3. Conditional Branch Enforcement
- Update the condition evaluators in `buildIfStatement`, `buildForStatement`, and the binary infix logic for `&&` / `||` to enforce `val = b.coerceType(val, TypeBool)`. By the time these expressions reach an `ir.Branch` instruction, they are guaranteed to be of `TypeBool` (which contains either exactly 1 or 0). 

### 4. Backend Adjustments
- **`m6809/backend.go`**: Update `getTypeSizeByType` to report a size of `1` for `TypeBool`.
- **`cbe/cbe.go`**: Expand `mapType` parsing to smoothly map `bool` to C-compliant `byte` or `uint8_t` values.

## Verification Plan

### Automated Tests
- Run `go test ./...` to verify the compiler and code generator backends adapt cleanly to the new `TypeBool`.

### Manual Verification
- We will construct `demos/test_bool.golf` directly assigning variables like `0x0100` and testing against `bool` values through `if` blocks and explicit coercion, confirming `bool(1)` mapping correctly executes against the `M6809` backend.

> [!IMPORTANT]
> Because slice coercion to boolean requires comparing the slice struct natively, the logic relies on `prelude.memeq`. This strictly defines an uninitialized or completely zeroed slice memory block as `bool(0)`. Is this acceptable for your "C-like" memory perspective for slices?
