# Popularity Heuristic Task List

- `[x]` Add `Popularity int` field to `FuncStatement` in `ast/ast.go`.
- `[x]` Update `MarkTrunkFunctions` in `ast/trunk.go` to calculate `Popularity` using loop depth factors.
- `[x]` Modify `main.go` to print `Popularity` alongside `TrunkLevel` under `-debug_opt`.
- `[x]` Verify changes by running non-m6809 tests and manual check.
