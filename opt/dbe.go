package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type DBEPass struct{}

func (p *DBEPass) Name() string { return "DBE" }

func (p *DBEPass) Run(f *ir.Function) bool {
	changed := false

	// 1. Fold branches with constant conditions
	for _, b := range f.Blocks {
		if br, ok := b.Terminator.(*ir.Branch); ok {
			var takenBlock, untakenBlock *ir.BasicBlock

			if c, isConstB := br.Condition.(*ir.ConstByte); isConstB {
				if c.Val != 0 {
					takenBlock = br.TrueBlock
					untakenBlock = br.FalseBlock
				} else {
					takenBlock = br.FalseBlock
					untakenBlock = br.TrueBlock
				}
			} else if c, isConstW := br.Condition.(*ir.ConstWord); isConstW {
				if c.Val != 0 {
					takenBlock = br.TrueBlock
					untakenBlock = br.FalseBlock
				} else {
					takenBlock = br.FalseBlock
					untakenBlock = br.TrueBlock
				}
			}

			if takenBlock != nil {
				// We don't change the block if both true and false blocks are the SAME block anyway,
				// but wait, if True and False are the SAME block, it's already an unconditional branch conceptually,
				// though our IR models it as conditional. It's safer to always convert it to a jump.
				// However, if they are the same block, we only remove it ONCE from successors/predecessors!

				if takenBlock != untakenBlock {
					b.Terminator = &ir.Jump{
						BaseInstruction: ir.BaseInstruction{ID: br.ID, Typ: ir.TypeVoid, Comment: "DBE Jump"},
						Target:          takenBlock,
					}

					// Remove untaken block from successors
					newSuccs := make([]*ir.BasicBlock, 0, len(b.Successors))
					for _, s := range b.Successors {
						if s != untakenBlock {
							newSuccs = append(newSuccs, s)
						}
					}
					b.Successors = newSuccs

					// Remove this block from untaken block's predecessors
					newPreds := make([]*ir.BasicBlock, 0, len(untakenBlock.Predecessors))
					for _, p := range untakenBlock.Predecessors {
						if p != b {
							newPreds = append(newPreds, p)
						}
					}
					untakenBlock.Predecessors = newPreds

					// Remove phi edges in untaken block coming from this block
					removePhiEdgesFrom(untakenBlock, b)
					changed = true
				} else {
					// Both edges went to the exact same block! Just convert to jump.
					b.Terminator = &ir.Jump{
						BaseInstruction: ir.BaseInstruction{ID: br.ID, Typ: ir.TypeVoid, Comment: "DBE Jump Same"},
						Target:          takenBlock,
					}
					// Successors/Predecessors probably list it twice, so deduplicate if necessary,
					// or we just leave it alone since CFG doesn't break if an edge is merged.
					// Let's just fix successors/predecessors manually to be 1 count.
					b.Successors = []*ir.BasicBlock{takenBlock}

					newPreds := make([]*ir.BasicBlock, 0, len(takenBlock.Predecessors))
					found := false
					for _, p := range takenBlock.Predecessors {
						if p == b {
							if !found {
								newPreds = append(newPreds, p)
								found = true
							}
						} else {
							newPreds = append(newPreds, p)
						}
					}
					takenBlock.Predecessors = newPreds

					// We must also remove one of the duplicate Phi edges in takenBlock,
					// but it's simpler to just let PhiSimp handle it, or remove duplicates here.
					removeDuplicatePhiEdgesFrom(takenBlock, b)
					changed = true
				}
			}
		}
	}

	// 2. Remove unreachable blocks
	if len(f.Blocks) > 0 {
		reachable := make(map[*ir.BasicBlock]bool)
		var visit func(b *ir.BasicBlock)
		visit = func(b *ir.BasicBlock) {
			if reachable[b] {
				return
			}
			reachable[b] = true
			for _, s := range b.Successors {
				visit(s)
			}
		}
		visit(f.Blocks[0])

		newBlocks := make([]*ir.BasicBlock, 0, len(f.Blocks))
		for _, b := range f.Blocks {
			if reachable[b] {
				newBlocks = append(newBlocks, b)
			} else {
				// Block is unreachable!
				// Need to remove this block from its successors' predecessors
				for _, s := range b.Successors {
					newPreds := make([]*ir.BasicBlock, 0, len(s.Predecessors))
					for _, p := range s.Predecessors {
						if p != b {
							newPreds = append(newPreds, p)
						}
					}
					s.Predecessors = newPreds
					removePhiEdgesFrom(s, b)
				}
				changed = true
			}
		}
		f.Blocks = newBlocks
	}

	return changed
}

func removePhiEdgesFrom(target *ir.BasicBlock, pred *ir.BasicBlock) {
	for _, instr := range target.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			newEdges := make([]ir.PhiEdge, 0, len(phi.Edges))
			for _, edge := range phi.Edges {
				if edge.Block != pred {
					newEdges = append(newEdges, edge)
				}
			}
			phi.Edges = newEdges
		}
	}
}

func removeDuplicatePhiEdgesFrom(target *ir.BasicBlock, pred *ir.BasicBlock) {
	for _, instr := range target.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			newEdges := make([]ir.PhiEdge, 0, len(phi.Edges))
			found := false
			for _, edge := range phi.Edges {
				if edge.Block == pred {
					if !found {
						newEdges = append(newEdges, edge)
						found = true
					}
				} else {
					newEdges = append(newEdges, edge)
				}
			}
			phi.Edges = newEdges
		}
	}
}
