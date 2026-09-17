package m6809

import (
	"sort"

	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/opt"
)

// candidate represents an SSA value being considered for physical register allocation.
type candidate struct {
	id    int
	score int
	isPtr bool
}

func (b *Backend) safeTypeSize(t ir.Type) int {
	if t.Equals(ir.TypeVoid) || t.Equals(ir.TypeUnknown) || (t.Name == "" && !t.IsAPointer() && !t.IsAnArray() && !t.IsAStruct()) {
		return 0
	}
	defer func() {
		_ = recover()
	}()
	return b.getTypeSizeByType(t)
}

// AllocateRegisters performs global SSA register allocation for function f.
// It assigns allocatable physical registers (U, Y) to high-frequency variables
// using chordal graph coloring with register preferencing.
func (b *Backend) AllocateRegisters(f *ir.Function) map[int]string {
	if b.NoGlobalRegAlloc {
		return nil
	}

	// Step 1: Detect instructions that clobber physical registers (calls, multi-byte copies)
	clobberPoints := make(map[int]bool)
	hasCalls := false
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			switch instr.(type) {
			case *ir.Call, *ir.IndirectCall, *ir.BuiltinCall:
				clobberPoints[instr.GetID()] = true
				hasCalls = true
			}
			sz := b.safeTypeSize(instr.Type())
			if sz > 2 {
				clobberPoints[instr.GetID()] = true
				hasCalls = true
			}
		}
	}

	if hasCalls {
		return nil
	}

	// Step 2: Determine available physical registers for this configuration
	availMask := AllocatableRegisters(b.globalsAtY, b.useFramePointer)
	var allocatableRegs []string

	// On M6809, X and D are expression evaluation scratch registers.
	// Dedicated global register allocation pool:
	// U is index register (available if !useFramePointer)
	// Y is index register (available if !globalsAtY)
	if availMask.Contains(RegU) {
		allocatableRegs = append(allocatableRegs, "u")
	}
	if availMask.Contains(RegY) {
		allocatableRegs = append(allocatableRegs, "y")
	}

	if len(allocatableRegs) == 0 {
		return nil
	}

	// Step 3: Identify eligible SSA values and score them by loop depth
	loops := opt.FindLoops(f)
	loopBlockDepth := make(map[*ir.BasicBlock]int)
	for _, l := range loops {
		for blk := range l.Blocks {
			loopBlockDepth[blk]++
		}
	}

	scores := make(map[int]int)
	isPointer := make(map[int]bool)

	for _, blk := range f.Blocks {
		depth := loopBlockDepth[blk]
		weight := 1
		for d := 0; d < depth; d++ {
			weight *= 10
		}

		for _, instr := range blk.Instructions {
			id := instr.GetID()

			// Check pointer usage
			switch inst := instr.(type) {
			case *ir.LoadPtr:
				if ptrInst, ok := inst.Ptr.(ir.Instruction); ok {
					isPointer[ptrInst.GetID()] = true
					scores[ptrInst.GetID()] += 5 * weight
				}
			case *ir.StorePtr:
				if ptrInst, ok := inst.Ptr.(ir.Instruction); ok {
					isPointer[ptrInst.GetID()] = true
					scores[ptrInst.GetID()] += 5 * weight
				}
			case *ir.AddressOfElement, *ir.ExtractFieldPtr:
				isPointer[id] = true
			}

			// Add weight for each use
			for _, op := range opt.OperandsOf(instr) {
				if opInst, ok := op.(ir.Instruction); ok {
					scores[opInst.GetID()] += 2 * weight
				}
			}
		}

		if blk.Terminator != nil {
			for _, op := range opt.OperandsOf(blk.Terminator) {
				if opInst, ok := op.(ir.Instruction); ok {
					scores[opInst.GetID()] += 2 * weight
				}
			}
		}
	}

	useCounts := make(map[int]int)
	isPhi := make(map[int]bool)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if _, ok := instr.(*ir.Phi); ok {
				isPhi[instr.GetID()] = true
			}
			for _, op := range opt.OperandsOf(instr) {
				if opInst, ok := op.(ir.Instruction); ok {
					useCounts[opInst.GetID()]++
				}
			}
		}
		if blk.Terminator != nil {
			for _, op := range opt.OperandsOf(blk.Terminator) {
				if opInst, ok := op.(ir.Instruction); ok {
					useCounts[opInst.GetID()]++
				}
			}
		}
	}

	// Collect candidates: 16-bit values, not address-taken, not crossing clobbers
	var candidates []candidate
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			id := instr.GetID()
			if b.addressTaken != nil && b.addressTaken[id] {
				continue
			}

			// Do not allocate physical registers to constants or sizeof
			switch instr.(type) {
			case *ir.ConstByte, *ir.ConstWord, *ir.Sizeof:
				continue
			}

			// Do not allocate physical registers to non-escaping AddressOfLocal
			// (they are synthetic addresses and will not be materialized).
			if aol, ok := instr.(*ir.AddressOfLocal); ok && b.escapeRes.EscapingAOL != nil && !b.escapeRes.EscapingAOL[aol.GetID()] {
				continue
			}

			typ := instr.Type()
			if b.program != nil {
				if _, isTypeDef := b.program.TypeDefs[typ.Name]; isTypeDef {
					continue
				}
			}
			if typ.Bits&(ir.TypeBitStruct|ir.TypeBitArray|ir.TypeBitSlice) != 0 || len(typ.FieldNamesAndTypes) > 0 || typ.ArrayLen > 0 {
				continue
			}

			sz := b.safeTypeSize(typ)
			if sz == 2 && scores[id] >= 4 {
				candidates = append(candidates, candidate{
					id:    id,
					score: scores[id],
					isPtr: isPointer[id],
				})
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort candidates by score descending (pointers first, then highest loop count)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].isPtr != candidates[j].isPtr {
			return candidates[i].isPtr
		}
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].id < candidates[j].id
	})

	// Step 4: Build candidate interference subgraph and color via MCS chordal coloring
	candidateSet := make(map[int]bool)
	for _, cand := range candidates {
		candidateSet[cand.id] = true
	}

	ig := opt.ComputeInterferenceGraph(f)
	candIG := opt.NewInterferenceGraph()
	for _, cand := range candidates {
		if candIG.Edges[cand.id] == nil {
			candIG.Edges[cand.id] = make(map[int]bool)
		}
		for neighbor := range ig.Edges[cand.id] {
			if candidateSet[neighbor] {
				candIG.AddEdge(cand.id, neighbor)
			}
		}
	}

	// Register preferencing:
	// Pointers prefer index register 0 ("u"), non-pointers prefer register 1 ("y") if available
	preferences := make(map[int][]int)
	for _, cand := range candidates {
		if cand.isPtr {
			preferences[cand.id] = []int{0, 1}
		} else {
			if len(allocatableRegs) > 1 {
				preferences[cand.id] = []int{1, 0}
			} else {
				preferences[cand.id] = []int{0}
			}
		}
	}

	coloring := opt.ColorChordalGraphWithPreferences(candIG, len(allocatableRegs), preferences)
	allocated := make(map[int]string)

	for _, cand := range candidates {
		if color, ok := coloring.Colors[cand.id]; ok && color >= 0 && color < len(allocatableRegs) {
			allocated[cand.id] = allocatableRegs[color]
		}
	}

	return allocated
}
