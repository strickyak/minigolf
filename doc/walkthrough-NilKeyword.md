# Implementation of the `nil` Keyword

I have fully implemented the requested `nil` keyword support for `minigolf`.

### 1. Lexer & Parser
- Added `NIL` to the lexical map in `token/token.go` and defined the underlying value as `"nil"`.
- Defined `ast.NilLiteral` in `ast/ast.go`.
- Enabled the parser in `parser/parser.go` to correctly identify and parse `nil` tokens as an `ast.NilLiteral`.

### 2. IR & Type System
- Added the `TypeNil` abstraction in `ir/ir.go`.
- Modified the IR `builder.go` `eval()` function to map `ast.NilLiteral` directly to a generic `ZeroInit` of type `TypeNil`.

### 3. Semantics & Type Coercion
- Implemented intelligent coercion in `builder.go:coerceType()`. When the builder needs to coerce a `TypeNil` expression to a `targetType` (e.g. for `x = nil`, or when passing `nil` as a parameter to a function), it intercepts it. If `targetType` is a valid nullable type (pointers, function pointers, and `slice`), it transparently constructs and emits a `ZeroInit` struct typed exactly as `targetType`.
- Modified `builder.go:commonTypeOfValues()` so that during comparisons (`==` and `!=`), the `nil` operand seamlessly assumes the type of the opposing operand, enabling the comparison expression to coerce the `nil` and type-check smoothly.

### 4. Verification
- Created `demos/test_nil.golf` and `demos/test_nil2.golf` to verify `nil` handling in:
  - Direct pointer comparisons and variable assignment.
  - Initializations for `slice` and `func()`.
  - Function parameters receiving `nil`.
  - Function `return nil`.
- Compiled and verified these demo files against the 6809 backend.
- Executed the comprehensive compiler test suite (`go test ./...`), which ran for 64.8s and completed successfully, proving no existing logic or parsing flows were interrupted.
