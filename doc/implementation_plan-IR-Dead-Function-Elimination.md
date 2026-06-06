# IR Dead Function Elimination

## Goal
Implement a sophisticated, iterated Dead Function Elimination (DFE) pass on the Intermediate Representation (IR). 

You are entirely correct: we want the early AST filtering (`semantic.go`) to remain as a fast initial prune, but we *also* need a robust IR-level DFE. 

Right now, the AST filtering successfully eliminates `main.small` when evaluated purely on `-D` constant overrides (thanks to the recent fix in `resolve.go` ensuring `constExprs` gets updated). However, as you noted, we need more sophisticated IR filtering. If a function becomes unreachable *after* complex IR optimizations (like deep constant folding, CSE, or IR branch folding), the AST filter will miss it because the AST filter only performs basic evaluation. 

The C Backend (CBE) generates code directly from the `ir.Program`. Because our current `opt/dce.go` only eliminates dead *instructions* within a single function and not entire *functions*, any function that becomes dead during IR optimization still gets emitted by CBE. 

## User Review Required
Please review the plan to implement Dead Function Elimination on the IR.

> [!IMPORTANT]
> This pass will operate across the entire `ir.Program` and accurately trace function reachability, dropping dead functions before any backend (CBE, m6809, x86_64) emits them.

## Proposed Changes

### 1. IR Dead Function Elimination (`opt/dfe.go`)
[NEW] `opt/dfe.go`
Create a new optimization pass `DFEPass` that operates on the entire program.
- `func EliminateDeadFunctions(prog *ir.Program) bool`
- **Reachability Analysis**:
  - Start with a `reachable` set containing the entry points: `main.main` and any initialization functions (e.g. `prelude.init_0`, `main.init_0`, etc.).
  - Iterate through all instructions in all currently `reachable` functions.
  - If a `*ir.Call` or `*ir.IndirectCall` is found, add the target function to the `reachable` set.
  - Recursively trace the call graph until the `reachable` set stops growing.
- **Pruning**:
  - Rebuild the `prog.Functions` map/slice to only include functions present in the `reachable` set.
  - Return `true` if any functions were removed.

### 2. Integration (`main.go` and `opt/opt.go`)
[MODIFY] `opt/opt.go`
- Update the optimization pipeline. Because `EliminateDeadFunctions` operates on the whole `ir.Program` rather than a single `ir.Function`, we will run it *after* the per-function optimization loops reach a fixed point. (Alternatively, we can wrap the entire process in a master loop if we want inter-procedural optimizations later).

[MODIFY] `main.go`
- Add a new command-line flag `-no-dfe` to optionally disable IR Dead Function Elimination.
- Add `EnableDFE` to `opt.Config`.

## Verification Plan
### Automated Tests
- Run `go test ./... -count=1` to ensure no regressions.
- Compile a test file where a branch is only eliminated during IR optimization (e.g. a condition relying on complex arithmetic that the AST folder doesn't catch). Verify that the dead function inside that branch is correctly omitted from the generated IR and from the final CBE output.
