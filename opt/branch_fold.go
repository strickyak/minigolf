package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// BranchFoldPass (Empty Block Elimination / Jump Threading)
// This pass eliminates basic blocks that contain only an unconditional jump
// by redirecting incoming branches to the jump's target block directly.
type BranchFoldPass struct{}

func (p *BranchFoldPass) Name() string {
	return "BranchFoldPass"
}

func (p *BranchFoldPass) Run(f *ir.Function) bool {
	changed := false

	if len(f.Blocks) == 0 {
		return false
	}

	for _, E := range f.Blocks {
		// Do not eliminate the entry block
		if E == f.Blocks[0] {
			continue
		}

		// The block must be completely empty of instructions
		if len(E.Instructions) > 0 {
			continue
		}

		// The terminator must be an unconditional jump
		jump, isJump := E.Terminator.(*ir.Jump)
		if !isJump {
			continue
		}

		T := jump.Target

		// Do not fold self-loops
		if T == E {
			continue
		}

		// Check if target has Phi nodes. If so, we must be careful not to
		// create duplicate incoming edges from a predecessor.
		hasPhi := false
		for _, instr := range T.Instructions {
			if _, ok := instr.(*ir.Phi); ok {
				hasPhi = true
				break
			}
		}

		// Copy predecessors since we're going to mutate the list
		preds := make([]*ir.BasicBlock, len(E.Predecessors))
		copy(preds, E.Predecessors)

		for _, P := range preds {
			if P == E {
				continue // Ignore self-references just in case
			}

			// Safety check: if T has phi nodes, and P already has an edge to T
			// (i.e. P is already a predecessor of T), doing P->T instead of P->E->T
			// would create multiple edges from P to T. Minigolf Phi nodes only expect
			// one edge per predecessor. It is safer to skip folding this specific edge.
			if hasPhi {
				alreadyGoesToT := false
				for _, s := range P.Successors {
					if s == T {
						alreadyGoesToT = true
						break
					}
				}
				if alreadyGoesToT {
					continue
				}
			}

			// Patch P's terminator to point to T instead of E
			patched := false
			switch term := P.Terminator.(type) {
			case *ir.Jump:
				if term.Target == E {
					term.Target = T
					patched = true
				}
			case *ir.Branch:
				if term.TrueBlock == E {
					term.TrueBlock = T
					patched = true
				}
				if term.FalseBlock == E {
					term.FalseBlock = T
					patched = true
				}
			}

			if patched {
				// 1. Update P's Successors: replace E with T
				newSuccs := make([]*ir.BasicBlock, 0, len(P.Successors))
				for _, s := range P.Successors {
					if s == E {
						newSuccs = append(newSuccs, T)
					} else {
						newSuccs = append(newSuccs, s)
					}
				}
				P.Successors = newSuccs

				// 2. Update E's Predecessors: remove P
				newPredsE := make([]*ir.BasicBlock, 0, len(E.Predecessors))
				for _, p := range E.Predecessors {
					if p != P {
						newPredsE = append(newPredsE, p)
					}
				}
				E.Predecessors = newPredsE

				// 3. Update T's Predecessors: add P
				T.Predecessors = append(T.Predecessors, P)

				// 4. Update T's Phi nodes: replace edge from E with edge from P
				for _, instr := range T.Instructions {
					if phi, ok := instr.(*ir.Phi); ok {
						for idx, edge := range phi.Edges {
							if edge.Block == E {
								phi.Edges[idx].Block = P
							}
						}
					}
				}

				changed = true
			}
		}
	}

	return changed
}
