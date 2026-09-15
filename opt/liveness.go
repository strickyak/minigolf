package opt

import (
	"sort"

	"github.com/strickyak/minigolf/ir"
)

// LiveInterval represents the linear lifetime of an SSA value [Start, End).
type LiveInterval struct {
	ID    int
	Val   ir.Value
	Start int // Program point of definition (0 for parameters)
	End   int // Program point of last use (or extended for loop live-outs)
}

type Liveness struct {
	LiveIn      map[*ir.BasicBlock]map[int]bool
	LiveOut     map[*ir.BasicBlock]map[int]bool
	Def         map[*ir.BasicBlock]map[int]bool
	Use         map[*ir.BasicBlock]map[int]bool
	Intervals   map[int]*LiveInterval
	InstrPos    map[int]int
	TotalPoints int
}

// ComputeLiveness computes the liveness sets and linear live intervals
// for each basic block and SSA value in the function.
func ComputeLiveness(f *ir.Function) *Liveness {
	l := &Liveness{
		LiveIn:    make(map[*ir.BasicBlock]map[int]bool),
		LiveOut:   make(map[*ir.BasicBlock]map[int]bool),
		Def:       make(map[*ir.BasicBlock]map[int]bool),
		Use:       make(map[*ir.BasicBlock]map[int]bool),
		Intervals: make(map[int]*LiveInterval),
		InstrPos:  make(map[int]int),
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

	// Assign linear positions and compute LiveIntervals
	pos := 0
	for _, param := range f.Parameters {
		l.Intervals[param.ID] = &LiveInterval{
			ID:    param.ID,
			Val:   param,
			Start: 0,
			End:   0,
		}
	}

	updateUse := func(op ir.Value, curPos int) {
		var id int = -1
		if inst, ok := op.(ir.Instruction); ok {
			id = inst.GetID()
		} else if param, ok := op.(*ir.Parameter); ok {
			id = param.ID
		}
		if id >= 0 {
			if inv, exists := l.Intervals[id]; exists {
				if curPos > inv.End {
					inv.End = curPos
				}
			}
		}
	}

	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			id := instr.GetID()
			l.InstrPos[id] = pos
			if _, exists := l.Intervals[id]; !exists {
				l.Intervals[id] = &LiveInterval{
					ID:    id,
					Val:   instr,
					Start: pos,
					End:   pos,
				}
			}
			for _, op := range OperandsOf(instr) {
				updateUse(op, pos)
			}
			pos += 2
		}

		if b.Terminator != nil {
			if termInst, ok := b.Terminator.(ir.Instruction); ok {
				l.InstrPos[termInst.GetID()] = pos
			}
			for _, op := range OperandsOf(b.Terminator) {
				updateUse(op, pos)
			}
			pos += 2
		}
	}
	l.TotalPoints = pos

	// Extend intervals for values in LiveOut of blocks (e.g. loops)
	for _, b := range f.Blocks {
		blockEndPos := pos
		if len(b.Instructions) > 0 {
			lastInstr := b.Instructions[len(b.Instructions)-1]
			blockEndPos = l.InstrPos[lastInstr.GetID()]
		}
		for id := range l.LiveOut[b] {
			if inv, exists := l.Intervals[id]; exists {
				if blockEndPos > inv.End {
					inv.End = blockEndPos
				}
			}
		}
	}

	return l
}

// LiveAt returns the list of value IDs whose live intervals contain position pos.
// The interval is [Start, End), meaning it is live at its definition (Start),
// but freed at the point of its last use (End).
func (l *Liveness) LiveAt(pos int) []int {
	var live []int
	for id, interval := range l.Intervals {
		if pos >= interval.Start && pos < interval.End {
			live = append(live, id)
		}
	}
	sort.Ints(live)
	return live
}

// PressureAt calculates the number of simultaneously active variables at a given program point.
func (l *Liveness) PressureAt(pos int) int {
	return len(l.LiveAt(pos))
}

// MaxPressure computes the peak register pressure across the entire function.
func (l *Liveness) MaxPressure() int {
	maxP := 0
	for pos := 0; pos <= l.TotalPoints; pos += 2 {
		p := l.PressureAt(pos)
		if p > maxP {
			maxP = p
		}
	}
	return maxP
}

// Interferes returns true if two values have overlapping lifetimes.
func (l *Liveness) Interferes(id1, id2 int) bool {
	int1, ok1 := l.Intervals[id1]
	int2, ok2 := l.Intervals[id2]
	if !ok1 || !ok2 {
		return false
	}
	return int1.Start < int2.End && int2.Start < int1.End
}
