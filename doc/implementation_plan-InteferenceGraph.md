# Goal

Implement Interference Graph construction for functions in SSA form. Since strict SSA programs naturally produce chordal interference graphs, this lays the foundation for future linear-time optimal register allocation. When `--debug_opt` is passed, the compiler will compute and print these interference graphs.

> [!NOTE]
> This plan focuses purely on *constructing* the interference graphs and establishing the liveness analysis prerequisite. Using these graphs for the actual linear-time chordal coloring / register allocation will be a logical next phase.

## User Review Required

Please review the proposed placement of the new packages and logic. Since `minigolf` currently allocates registers locally per-block inside the backends (`x86_64` / `m6809`), we will expose this graph construction in `opt` (or `ir`) so it can be utilized globally by any backend later. Does this architectural alignment make sense?

## Proposed Changes

### `opt`

#### [NEW] [liveness.go](file:///home/strick/antig/opt/liveness.go)
Create a new file to calculate global liveness sets using dataflow analysis.
- Define `Liveness` struct holding `LiveIn`, `LiveOut`, `Def`, and `Use` mappings per block (tracking variable `ID`s).
- **Initialization**: Compute `Def` and `Use` sets for each basic block.
  - **Phi Node Handling**: A critical SSA constraint is that `Phi` operands from predecessor `P` are treated as "live-out" of block `P`, rather than "used" inside the block containing the `Phi`.
- **Fixed-Point Iteration**: Repeatedly propagate `LiveIn` to predecessors' `LiveOut` until sets stop changing.

#### [NEW] [interference.go](file:///home/strick/antig/opt/interference.go)
Create a new file to construct the actual graph.
- Define `InterferenceGraph` mapping each variable ID to a set of interfering variable IDs.
- **Graph Construction**:
  - For each Basic Block, initialize the `LiveSet` to the block's `LiveOut`.
  - Walk the block's instructions in reverse.
  - For an instruction defining variable `D`, add an interference edge between `D` and every variable currently in the `LiveSet`.
  - Remove `D` from `LiveSet` and insert the instruction's operands.

#### [MODIFY] [opt.go](file:///home/strick/antig/opt/opt.go)
- Add a helper function `OperandsOf(instr ir.Instruction) []ir.Value` (similar to the existing `replaceInInstruction`) to easily extract all used values from any instruction. This is required to populate the `Use` sets during liveness analysis.

### Main Package

#### [MODIFY] [main.go](file:///home/strick/antig/main.go)
- Wire up the debug logging for the new feature. 
- After the optimization passes complete, if `*debugOpt` is enabled, compute the interference graph for each function.
- Print the nodes and their corresponding edges in a readable format to stdout.

## Verification Plan

### Automated Tests
- Create a test `tests/test_interference.golf` with interesting control flow and variables.
- Run `go run main.go --debug_opt tests/test_interference.golf` and verify the output correctly prints the chordal interference graph without crashing.
- Verify `go test -v .` executes properly to ensure no breakages in existing compilation passes.
