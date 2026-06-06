# Walkthrough: IR Dead Function Elimination (DFE)

## Changes Made

1. **Created `opt/dfe.go`**:
    - Implemented `EliminateDeadFunctions(p *ir.Program) bool`.
    - Implemented reachability analysis starting from entry points: `main`, `prelude.init_0`, `main.init_0`, and `init_main`.
    - Traced explicit function calls (`ir.Call`) and function value usage (`ir.AddressOfFunc`) through the IR block structures.
    - Pruned any function in `p.Functions` that could not be reached from the entry points.

2. **Updated `opt/opt.go`**:
    - Added `EnableDFE` boolean to `opt.Config`.
    - Added a call to `EliminateDeadFunctions(p)` inside `OptimizeProgram(p *ir.Program, config Config)`, triggered immediately after per-function optimization passes (DBE, DCE, etc.) achieve a fixed point. This enables dead function elimination at the module scale, allowing dead code discovered by local optimizations to propagate globally.

3. **Updated `main.go`**:
    - Registered a new command-line flag `-no-dfe` to explicitly disable this optimization (default is `false`).
    - Handled the `NO_DFE` environment variable to support headless testing and bulk disabling.
    - Added `EnableDFE` to all four instantiations of `optConfig` across the compiler's backend phases (`IR`, `CBE`, `x86_64`, `m6809`).

## What Was Tested

- All system tests in `system_test.go` (`go test ./...`) for all three backend modes (`m6809`, `CBE`, and `x86_64`).
- Extensive manual testing and edge-case resolution involving name-mangling edge cases where the `CBE` backend was expecting the function to be labeled `main` instead of `main.main` resulting in undefined reference linker failures.

## Validation Results

- The `go test ./...` test suite passed cleanly across all 144 test configurations.
- The IR size effectively decreases for functions that were pruned (like unused prelude variants or unused application logic), ensuring smaller code generation footprints on restricted targets like `m6809`.
