# Implement `defer func() { ... }()` Block Syntax

The goal is to implement a new deferred execution syntax in Minigolf that looks like `defer func() { ... }()`. This will execute an arbitrary block of code at the end of the function, acting as syntactic sugar around a new type of `DeferredAction` instead of actually creating and invoking a lambda expression.

## User Review Required

> [!IMPORTANT]
> Because this is syntactic sugar for a block executed during the return sequence, statements that alter control flow such as `return`, `break`, and `continue` cannot safely be used inside this deferred block. If they were allowed, a `return` might recursively trigger the deferred actions again, leading to compiler panics or infinite loops during IR generation.
> **I plan to explicitly forbid `return`, `break`, and `continue` statements inside `defer func() { ... }() ` blocks during the Semantic Analysis phase.**

## Proposed Changes

### AST (`ast/ast.go`)
Expand the `DeferStatement` node to hold a block, and the `DeferredAction` to execute it:
#### [MODIFY] [ast.go](file:///home/strick/antig/ast/ast.go)
- Add a `Block *BlockStatement` field to `DeferStatement`. If it's a block-based defer, `Call` will be `nil` and `Block` will be set.

### Parser (`parser/parser.go`)
Modify the `defer` parsing logic to look for the exact token sequence `func`, `()`, `{`.
#### [MODIFY] [parser.go](file:///home/strick/antig/parser/parser.go)
- Update `parseDeferStatement()` to check if the next tokens are `token.FUNC`, `token.LPAREN`, `token.RPAREN`, and `token.LBRACE`.
- If matched, consume those tokens, parse the block with `parseBlockStatement()`, and consume the trailing `()` syntax to complete the `defer func() { ... }()` mimicry.

### Semantic Analyzer (`semantic/semantic.go`)
Add validation and scope handling for deferred blocks.
#### [MODIFY] [semantic.go](file:///home/strick/antig/semantic/semantic.go)
- Add `inDeferBlock int` to the `Analyzer` struct to track if we are analyzing a deferred block.
- When evaluating `*ast.DeferStatement`, if `s.Block != nil`, increment `a.inDeferBlock`, call `a.analyzeBlock(s.Block, true)` to create a new local scope, and then decrement.
- Update `defineLocalSymbol()` to report an error if `a.inDeferBlock > 0` and the type being initialized `isDestructible`. This satisfies the requirement to forbid creating destructibles inside deferred blocks.
- Update `analyzeStatement()` to report an error if it encounters a `ReturnStatement`, `BreakStatement`, or `ContinueStatement` while `a.inDeferBlock > 0`.

### IR Builder (`ir/builder.go`)
Emit and execute the deferred block at the end of the function.
#### [MODIFY] [builder.go](file:///home/strick/antig/ir/builder.go)
- Update `DeferredAction` to include a `Block *ast.BlockStatement` field.
- In `buildDefer()`, if `s.Block != nil`, append a `DeferredAction` containing the block and return early.
- In `buildReturn()`, during the reverse-iteration of `b.deferredActions`, if `action.Block != nil`, evaluate the block by calling `b.buildBlock(action.Block)`. Because local variables are tracked by their unique semantic names, the variables referenced in the block will seamlessly map to their correct stack slots.

## Verification Plan

### Automated Tests
- Create a new test file `tests/test_defer_block.golf` to verify that `defer func() { ... }()` successfully modifies variables declared outside the block and correctly prints values at function exit.
- Run `go test ./...` to ensure all existing compilation passes correctly and the new feature executes on `x86_64`, `CBE`, and `M6809` targets.
