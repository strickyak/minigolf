# Branch Folding / Empty Block Elimination Optimization

The optimization you described is typically called **Branch Folding**, **Empty Block Elimination**, or a simple form of **Jump Threading**. Its goal is to remove redundant intermediate blocks that serve no purpose other than forwarding control flow.

Here is the plan to implement this optimization.

## User Review Required
Please review the plan below. I will proceed with execution once approved.

## Open Questions

> [!NOTE]
> 1. **CFG Integrity**: Does Minigolf perfectly maintain `Predecessors` and `Successors` across all optimization passes? If not, it is safer for the pass to scan all `Terminator`s and `Phi` nodes directly to retarget them, rather than strictly trusting the `Predecessors` list. (The plan below assumes the safer route of scanning/patching).
> 2. **Branch Degeneration**: If a conditional `Branch` has its True and False paths folded into the exact same target block `T`, it functionally degenerates into a `Jump`. Should this pass automatically downgrade such `Branch` instructions into unconditional `Jump`s? I recommend yes.

## Proposed Changes

### `opt/branch_fold.go`
#### [NEW] [branch_fold.go](file:///home/strick/antig/opt/branch_fold.go)
- Create `BranchFoldPass` implementing the `Pass` interface.
- Iterate over all blocks `E` in `f.Blocks` (skipping the entry block).
- Check if `E` is empty (`len(E.Instructions) == 0`) and its `Terminator` is `*ir.Jump`.
- Let `T` be the target of `E`. Avoid self-loops (`T == E`).
- Find all blocks `P` that jump/branch to `E`.
  - Update `P`'s terminator to point to `T` instead.
  - If `T` has `Phi` nodes with incoming edges from `E`, update those edges to originate from `P`.
  - If `P` was an `ir.Branch` and both true/false targets now point to `T`, downgrade it to an `ir.Jump`.
- Safely remove block `E` from `f.Blocks`.

---

### `opt/opt.go`
#### [MODIFY] [opt.go](file:///home/strick/antig/opt/opt.go)
- Add `EnableBranchFold bool` to the `Config` struct.
- Append `&BranchFoldPass{}` to the list of passes when `EnableBranchFold` is true.

---

### `main.go`
#### [MODIFY] [main.go](file:///home/strick/antig/main.go)
- Define a new command-line boolean flag: `noBranchFold = flag.Bool("no-branchfold", false, "Disable Branch Folding optimization")`.
- Map it to `EnableBranchFold: !*noBranchFold` when building the `opt.Config`.

---

### `system_test.go`
#### [MODIFY] [system_test.go](file:///home/strick/antig/system_test.go)
- Add a check for the `NO_BRANCHFOLD` environment variable.
- If present, append `-no-branchfold` to the compiler invocation arguments.

## Verification Plan

### Automated Tests
- Run `go test ./...` and verify that the full test suite remains stable.
- Verify that `tests/joy_1.golf` compiles successfully without runtime changes.

### Manual Verification
- Inspect the generated `.asm` and `.s` dumps for `tests/joy_1.golf` with and without the optimization to visually confirm that the single-instruction trampoline blocks (`bne ... lbra ...`) are eliminated.
