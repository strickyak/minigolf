# TRUNK Function Analysis Implementation Plan

The user wants to identify and mark "trunk" functions. A trunk function is one that is guaranteed to execute at most once per program run.
- **Level 1**: `main.main` (by convention).
- **Level N**: A function called from exactly one call site, where that call site is within a Level N-1 trunk function, and the call site is NOT inside a `for` loop.
- Functions that are referenced dynamically (used as a value rather than directly called) cannot be trunk functions.

We will write an AST method to analyze the call graph and assign `TrunkLevel` to all `FuncStatement` nodes.

## Proposed Changes

### 1. Update AST definitions (`ast/ast.go`)
I will add the `TrunkLevel int` field to `FuncStatement`:
```go
type FuncStatement struct {
    Token token.Token
    // ... existing fields ...
    TrunkLevel int // 0 = non-trunk, >= 1 = trunk level
}
```

### 2. Semantic Resolver Helper (`semantic/semantic.go`)
I will add an exported helper method to `Analyzer` so the AST traversal can accurately map expressions (like `ast.Identifier` and `ast.SelectorExpression`) to their target `ast.FuncStatement`. This will leverage `Semantic`'s existing name mangling logic.
```go
func (a *Analyzer) ResolveFunc(expr ast.Expression) *ast.FuncStatement { ... }
```

### 3. Create AST Pass (`ast/trunk.go`)
I will write `func (p *Program) MarkTrunkFunctions(resolver func(ast.Expression) *FuncStatement)`
The algorithm will:
1. Initialize a tracking map `map[*FuncStatement]*trunkInfo` for all surviving functions.
2. Traverse the AST nodes recursively, tracking `currentFunc *ast.FuncStatement` and `loopDepth int`.
3. At `ast.CallExpression`:
   - Resolve the target function. If found, add `currentFunc` as a caller. If `loopDepth > 0`, flag the target with `loopCall = true`.
4. At other `ast.Expression` uses:
   - If the expression resolves to a function but is NOT the immediate `Function` target of a `CallExpression`, flag the target function with `dynamic = true`.
5. Iteratively propagate levels starting from `main` (Level 1). If a function is called exactly once by a Level `N-1` function, is not dynamic, and not called inside a loop, it gets Level `N`.
6. Terminate when no more levels are updated.

### 4. Wire the Pass in Compiler (`main.go`)
I will invoke the new pass right after `analyzer.Analyze(program)` (which internally trims dead functions) and before the Builder step.
```go
	analyzer := semantic.New(resolver)
	analyzer.Analyze(program)
    // ... existing error checking ...

    // Mark Trunk Functions via AST traversal
    program.MarkTrunkFunctions(analyzer.ResolveFunc)
```

## Verification Plan
1. Compile existing tests and demos.
2. The logic correctly handles loops (`ast.ForStatement`, `ast.For3Statement`, `ast.ForRangeStatement`).
3. Methods resolving dynamically through variables or parameters correctly poison the function (`dynamic = true`), preventing it from being incorrectly marked as trunk.
4. If requested, I can add a compiler debug flag to print trunk levels.
