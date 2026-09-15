package opt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

// DominatorTree represents dominance relations for an ir.Function CFG.
type DominatorTree struct {
	Function *ir.Function
	Dom      map[*ir.BasicBlock]map[*ir.BasicBlock]bool // Dom[b][d] == true iff d dominates b
	IDom     map[*ir.BasicBlock]*ir.BasicBlock          // Immediate dominator of b
}

// ComputeDominators computes the dominator sets and immediate dominator tree
// for each basic block in the given function.
func ComputeDominators(f *ir.Function) *DominatorTree {
	dt := &DominatorTree{
		Function: f,
		Dom:      make(map[*ir.BasicBlock]map[*ir.BasicBlock]bool),
		IDom:     make(map[*ir.BasicBlock]*ir.BasicBlock),
	}

	if len(f.Blocks) == 0 {
		return dt
	}

	entry := f.Blocks[0]

	// Initialize Dom sets.
	// Dom[entry] = {entry}
	// Dom[b] = AllBlocks for b != entry
	allBlocks := make(map[*ir.BasicBlock]bool, len(f.Blocks))
	for _, b := range f.Blocks {
		allBlocks[b] = true
	}

	for _, b := range f.Blocks {
		dt.Dom[b] = make(map[*ir.BasicBlock]bool)
		if b == entry {
			dt.Dom[b][entry] = true
		} else {
			for blk := range allBlocks {
				dt.Dom[b][blk] = true
			}
		}
	}

	// Iterative dataflow algorithm until fixed point:
	// Dom(b) = {b} U (Intersection over all p in Pred(b) of Dom(p))
	changed := true
	for changed {
		changed = false
		for _, b := range f.Blocks {
			if b == entry {
				continue
			}

			// Intersect predecessors' dominators
			var newDom map[*ir.BasicBlock]bool
			first := true
			for _, p := range b.Predecessors {
				// Only consider predecessors reachable from entry
				if len(dt.Dom[p]) == 0 {
					continue
				}
				if first {
					newDom = make(map[*ir.BasicBlock]bool)
					for d := range dt.Dom[p] {
						newDom[d] = true
					}
					first = false
				} else {
					for d := range newDom {
						if !dt.Dom[p][d] {
							delete(newDom, d)
						}
					}
				}
			}

			if newDom == nil {
				newDom = make(map[*ir.BasicBlock]bool)
			}
			newDom[b] = true // Each block dominates itself

			if len(newDom) != len(dt.Dom[b]) {
				changed = true
				dt.Dom[b] = newDom
			} else {
				for d := range newDom {
					if !dt.Dom[b][d] {
						changed = true
						dt.Dom[b] = newDom
						break
					}
				}
			}
		}
	}

	// Compute IDom (Immediate Dominators)
	// IDom(b) is the unique strict dominator d of b such that d does not strictly dominate any other strict dominator of b.
	for _, b := range f.Blocks {
		if b == entry {
			continue
		}

		var strictDoms []*ir.BasicBlock
		for d := range dt.Dom[b] {
			if d != b {
				strictDoms = append(strictDoms, d)
			}
		}

		// Sort by dominator set size descending; the immediate dominator dominates all other strict dominators
		for _, cand := range strictDoms {
			isImmediate := true
			for _, other := range strictDoms {
				if cand != other && dt.StrictlyDominates(cand, other) {
					// cand dominates other, so cand cannot be immediate (other is closer to b)
					isImmediate = false
					break
				}
			}
			if isImmediate {
				dt.IDom[b] = cand
				break
			}
		}
	}

	return dt
}

// Dominates returns true if a dominates b (every path from entry to b goes through a).
func (dt *DominatorTree) Dominates(a, b *ir.BasicBlock) bool {
	if a == nil || b == nil {
		return false
	}
	return dt.Dom[b] != nil && dt.Dom[b][a]
}

// StrictlyDominates returns true if a dominates b and a != b.
func (dt *DominatorTree) StrictlyDominates(a, b *ir.BasicBlock) bool {
	return a != b && dt.Dominates(a, b)
}

// ImmediateDominator returns the immediate dominator of b, or nil if none (e.g. entry block).
func (dt *DominatorTree) ImmediateDominator(b *ir.BasicBlock) *ir.BasicBlock {
	return dt.IDom[b]
}

// InductionVar represents a loop induction variable defined by a Phi at the loop header.
type InductionVar struct {
	Phi     *ir.Phi
	Initial ir.Value
	Step    int64
	Update  *ir.BinaryOp
	Type    ir.Type
}

// Loop represents a natural loop in the control flow graph.
type Loop struct {
	Header    *ir.BasicBlock
	Preheader *ir.BasicBlock // Pred of Header outside loop, if unique
	Latches   []*ir.BasicBlock
	Blocks    map[*ir.BasicBlock]bool
	Exits     []*ir.BasicBlock
	IndVars   []*InductionVar
	Invariants []ir.Value
	SubLoops  []*Loop
}

// Contains returns true if block b is part of this loop.
func (l *Loop) Contains(b *ir.BasicBlock) bool {
	return l.Blocks != nil && l.Blocks[b]
}

// IsInnermost returns true if this loop contains no subloops.
func (l *Loop) IsInnermost() bool {
	return len(l.SubLoops) == 0
}

// String returns a compact representation of the loop for debugging.
func (l *Loop) String() string {
	var blockIDs []int
	for b := range l.Blocks {
		blockIDs = append(blockIDs, b.ID)
	}
	sort.Ints(blockIDs)

	var bStrs []string
	for _, id := range blockIDs {
		bStrs = append(bStrs, fmt.Sprintf("b%d", id))
	}

	preStr := "none"
	if l.Preheader != nil {
		preStr = fmt.Sprintf("b%d", l.Preheader.ID)
	}

	return fmt.Sprintf("Loop(Header: b%d, Preheader: %s, Blocks: [%s], IVs: %d)",
		l.Header.ID, preStr, strings.Join(bStrs, ", "), len(l.IndVars))
}

// FindLoops identifies all natural loops in the function using the dominator tree.
func FindLoops(f *ir.Function) []*Loop {
	dt := ComputeDominators(f)
	var loops []*Loop
	headerLoops := make(map[*ir.BasicBlock]*Loop)

	// A back-edge is an edge L -> H where H dominates L.
	for _, b := range f.Blocks {
		for _, succ := range b.Successors {
			if dt.Dominates(succ, b) {
				header := succ
				latch := b

				loop, exists := headerLoops[header]
				if !exists {
					loop = &Loop{
						Header:  header,
						Latches: []*ir.BasicBlock{latch},
						Blocks:  make(map[*ir.BasicBlock]bool),
					}
					headerLoops[header] = loop
					loops = append(loops, loop)
				} else {
					loop.Latches = append(loop.Latches, latch)
				}

				// Add natural loop body:
				// Start from latch and traverse backwards via predecessors until reaching header.
				loop.Blocks[header] = true
				if !loop.Blocks[latch] {
					loop.Blocks[latch] = true
					worklist := []*ir.BasicBlock{latch}
					for len(worklist) > 0 {
						curr := worklist[len(worklist)-1]
						worklist = worklist[:len(worklist)-1]

						for _, pred := range curr.Predecessors {
							if !loop.Blocks[pred] {
								loop.Blocks[pred] = true
								worklist = append(worklist, pred)
							}
						}
					}
				}
			}
		}
	}

	// For each loop, determine Preheader, Exits, Induction Variables, and Invariants
	for _, loop := range loops {
		// Preheader: look at predecessors of Header outside the loop
		var outsidePreds []*ir.BasicBlock
		for _, pred := range loop.Header.Predecessors {
			if !loop.Blocks[pred] {
				outsidePreds = append(outsidePreds, pred)
			}
		}
		if len(outsidePreds) == 1 && len(outsidePreds[0].Successors) == 1 && outsidePreds[0].Successors[0] == loop.Header {
			loop.Preheader = outsidePreds[0]
		}

		// Exits: blocks in the loop that have successors outside the loop
		exitMap := make(map[*ir.BasicBlock]bool)
		for b := range loop.Blocks {
			for _, succ := range b.Successors {
				if !loop.Blocks[succ] {
					exitMap[b] = true
					break
				}
			}
		}
		for b := range exitMap {
			loop.Exits = append(loop.Exits, b)
		}

		// Discover Induction Variables
		loop.IndVars = FindInductionVars(loop)

		// Discover Loop-Invariant values
		loop.Invariants = FindLoopInvariants(loop)
	}

	// Determine nesting hierarchy: if loop A's blocks are a strict superset of loop B's blocks, B is a subloop of A
	for _, l1 := range loops {
		for _, l2 := range loops {
			if l1 == l2 {
				continue
			}
			if len(l1.Blocks) > len(l2.Blocks) {
				isSubset := true
				for b := range l2.Blocks {
					if !l1.Blocks[b] {
						isSubset = false
						break
					}
				}
				if isSubset {
					l1.SubLoops = append(l1.SubLoops, l2)
				}
			}
		}
	}

	return loops
}

// FindInductionVars identifies induction variables defined by Phis at the loop header.
func FindInductionVars(loop *Loop) []*InductionVar {
	var ivs []*InductionVar

	for _, instr := range loop.Header.Instructions {
		phi, isPhi := instr.(*ir.Phi)
		if !isPhi {
			continue
		}

		// A basic induction variable phi should have:
		// 1 edge coming from outside the loop (initial value)
		// 1 edge coming from a latch inside the loop (updated value)
		var initVal ir.Value
		var updateVal ir.Value

		for _, edge := range phi.Edges {
			if loop.Blocks[edge.Block] {
				updateVal = edge.Value
			} else {
				initVal = edge.Value
			}
		}

		if initVal == nil || updateVal == nil {
			continue
		}

		// Check if updateVal is a BinaryOp of the form:
		// phi + Const or phi - Const
		binOp, isBin := updateVal.(*ir.BinaryOp)
		if !isBin {
			continue
		}

		var step int64 = 0
		var matched bool

		switch binOp.Op {
		case "add":
			if binOp.Left == phi {
				if c, ok := asConstInt(binOp.Right); ok {
					step = c
					matched = true
				}
			} else if binOp.Right == phi {
				if c, ok := asConstInt(binOp.Left); ok {
					step = c
					matched = true
				}
			}
		case "sub":
			if binOp.Left == phi {
				if c, ok := asConstInt(binOp.Right); ok {
					step = -c
					matched = true
				}
			}
		}

		if matched {
			ivs = append(ivs, &InductionVar{
				Phi:     phi,
				Initial: initVal,
				Step:    step,
				Update:  binOp,
				Type:    phi.Type(),
			})
		}
	}

	return ivs
}

// FindLoopInvariants finds SSA values used in the loop that are defined outside the loop.
func FindLoopInvariants(loop *Loop) []ir.Value {
	invMap := make(map[ir.Value]bool)

	for b := range loop.Blocks {
		for _, instr := range b.Instructions {
			if _, isPhi := instr.(*ir.Phi); isPhi && b == loop.Header {
				// Don't count header phi inputs as loop invariants
				continue
			}

			for _, op := range OperandsOf(instr) {
				if isLoopInvariant(op, loop) {
					invMap[op] = true
				}
			}
		}

		if b.Terminator != nil {
			for _, op := range OperandsOf(b.Terminator) {
				if isLoopInvariant(op, loop) {
					invMap[op] = true
				}
			}
		}
	}

	var invariants []ir.Value
	for val := range invMap {
		invariants = append(invariants, val)
	}
	return invariants
}

func isLoopInvariant(val ir.Value, loop *Loop) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case *ir.ConstByte, *ir.ConstWord, *ir.Parameter, *ir.Global, *ir.AddressOfGlobal, *ir.AddressOfFunc:
		return true
	case ir.Instruction:
		// Check if defined in a block outside this loop
		for b := range loop.Blocks {
			for _, instr := range b.Instructions {
				if instr.GetID() == v.GetID() {
					return false // Defined inside loop
				}
			}
		}
		return true
	}
	return false
}

func asConstInt(val ir.Value) (int64, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case *ir.ConstByte:
		return int64(v.Val), true
	case *ir.ConstWord:
		return int64(v.Val), true
	}
	return 0, false
}
