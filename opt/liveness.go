package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type Liveness struct {
	LiveIn  map[*ir.BasicBlock]map[int]bool
	LiveOut map[*ir.BasicBlock]map[int]bool
	Def     map[*ir.BasicBlock]map[int]bool
	Use     map[*ir.BasicBlock]map[int]bool
}

// ComputeLiveness computes the liveness sets for each basic block in the function.
func ComputeLiveness(f *ir.Function) *Liveness {
	l := &Liveness{
		LiveIn:  make(map[*ir.BasicBlock]map[int]bool),
		LiveOut: make(map[*ir.BasicBlock]map[int]bool),
		Def:     make(map[*ir.BasicBlock]map[int]bool),
		Use:     make(map[*ir.BasicBlock]map[int]bool),
	}

	for _, b := range f.Blocks {
		l.LiveIn[b] = make(map[int]bool)
		l.LiveOut[b] = make(map[int]bool)
		l.Def[b] = make(map[int]bool)
		l.Use[b] = make(map[int]bool)

		// Compute Def and Use for the block.
		for _, instr := range b.Instructions {
			// Phis uses are evaluated at the predecessor edge, not inside the block.
			if _, isPhi := instr.(*ir.Phi); !isPhi {
				for _, op := range OperandsOf(instr) {
					if inst, ok := op.(ir.Instruction); ok {
						id := inst.GetID()
						if !l.Def[b][id] {
							l.Use[b][id] = true
						}
					} else if param, ok := op.(*ir.Parameter); ok {
						if !l.Def[b][param.ID] {
							l.Use[b][param.ID] = true
						}
					}
				}
			}

			// Definition
			l.Def[b][instr.GetID()] = true
		}

		if b.Terminator != nil {
			for _, op := range OperandsOf(b.Terminator) {
				if inst, ok := op.(ir.Instruction); ok {
					id := inst.GetID()
					if !l.Def[b][id] {
						l.Use[b][id] = true
					}
				} else if param, ok := op.(*ir.Parameter); ok {
					if !l.Def[b][param.ID] {
						l.Use[b][param.ID] = true
					}
				}
			}
		}
	}

	// Fixed-point iteration
	changed := true
	for changed {
		changed = false
		for i := len(f.Blocks) - 1; i >= 0; i-- {
			b := f.Blocks[i]

			// LiveOut[B] = Union(LiveIn[S]) for all S in Successors
			for _, succ := range b.Successors {
				for id := range l.LiveIn[succ] {
					if !l.LiveOut[b][id] {
						l.LiveOut[b][id] = true
						changed = true
					}
				}

				// Also add uses from Phis in successor that correspond to this predecessor edge.
				for _, instr := range succ.Instructions {
					if phi, isPhi := instr.(*ir.Phi); isPhi {
						for _, edge := range phi.Edges {
							if edge.Block == b {
								if inst, ok := edge.Value.(ir.Instruction); ok {
									id := inst.GetID()
									if !l.LiveOut[b][id] {
										l.LiveOut[b][id] = true
										changed = true
									}
								} else if param, ok := edge.Value.(*ir.Parameter); ok {
									if !l.LiveOut[b][param.ID] {
										l.LiveOut[b][param.ID] = true
										changed = true
									}
								}
							}
						}
					}
				}
			}

			// LiveIn[B] = Use[B] U (LiveOut[B] - Def[B])
			for id := range l.Use[b] {
				if !l.LiveIn[b][id] {
					l.LiveIn[b][id] = true
					changed = true
				}
			}
			for id := range l.LiveOut[b] {
				if !l.Def[b][id] && !l.LiveIn[b][id] {
					l.LiveIn[b][id] = true
					changed = true
				}
			}
		}
	}

	return l
}
