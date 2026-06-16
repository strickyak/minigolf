# Implementation Plan: Adding `nil` keyword

The user wants a `nil` keyword that automatically converts to the Zero Value for pointer types, function types, and `slice[T]`. This must work for assignment, function calls, function returns, and comparisons (`==` and `!=`).

## User Review Required
Please review the proposed approach for handling `nil` conversion.

## Open Questions
None. The approach seems solid and maps perfectly to how minigolf currently generates struct and variable initializations (using `ZeroInit`).

## Proposed Changes

### Lexer
#### [MODIFY] token/token.go
- Add `NIL = "NIL"` to the `TokenType` constants.
- Add `"nil": NIL` to the `keywords` map so it is lexed as a keyword.

### AST & Parser
#### [MODIFY] ast/ast.go
- Create a new `NilLiteral` struct implementing `Expression`.

#### [MODIFY] parser/parser.go
- Register a prefix parser for `token.NIL` to call a new `parseNil()` method.
- `parseNil()` will return a `&ast.NilLiteral{}`.

### Semantic Analysis
#### [MODIFY] semantic/semantic.go
- In `analyzeExpression()`, add a case for `*ast.NilLiteral` and return `UnknownType` (or a dedicated pseudo-type). This ensures that semantic passes like `isDestructible` naturally ignore it.

### IR Generation
#### [MODIFY] ir/ir.go
- Add `TypeNil = Type{Name: "nil", Expr: &ast.Identifier{Value: "nil"}}`.

#### [MODIFY] ir/builder.go
- Remove the old hack in `eval()` that checked if `e.Value == "nil"` for identifiers.
- Add `case *ast.NilLiteral:` in `eval()` that returns `ExprResult` with `Typ: TypeNil` and a placeholder `ZeroInit{Typ: TypeNil}`.
- Update `commonTypeOfValues()` to return the other side's type if one of the operands is `TypeNil`. If both are `TypeNil`, return `TypeWord`.
- Update `coerceType()` to catch `TypeNil`. When coercing `TypeNil` to a target type (like a pointer, func, or slice), it will generate and return a `b.addInstr(&ZeroInit{BaseInstruction: BaseInstruction{Typ: targetType}}, val)`.
- In `InfixExpression` parsing (inside `buildExpr()`), explicitly call `left = b.coerceType(left, typ)` and `right = b.coerceType(right, typ)` just before the `switch e.Operator` block. This guarantees that `==` and `!=` correctly receive a properly sized `ZeroInit` struct for slices (allowing `prelude.memeq` to compare them correctly), or a `0` word for pointers.

## Verification Plan
1. Create a `demos/test_nil.golf` file that exercises:
   - `x := nil` (pointer and slice assignments).
   - `func_call(nil)`
   - `if slice == nil` and `if ptr != nil`
2. Compile and run `go run main.go -m M6809 demos/test_nil.golf`.
3. Ensure no backend panics about 0-sized values and proper output is produced.
