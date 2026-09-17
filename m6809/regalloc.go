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
// AllocateRegisters performs global SSA register allocation for function f.
// It assigns allocatable physical registers (U, Y) to high-frequency variables
// using interference graph coloring with register preferencing.
func (b *Backend) AllocateRegisters(f *ir.Function) map[int]string {
	if b.NoGlobalRegAlloc {
		return nil
	}

	// Step 1: Detect instructions that clobber physical registers (multi-byte copies)
	// emitCopy uses Y as a scratch register when copying values with size > 2.
	hasMultiByteCopy := false
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if b.safeTypeSize(instr.Type()) > 2 {
				hasMultiByteCopy = true
			}
			for _, op := range opt.OperandsOf(instr) {
				if b.safeTypeSize(op.Type()) > 2 {
					hasMultiByteCopy = true
				}
			}
		}
		if blk.Terminator != nil {
			for _, op := range opt.OperandsOf(blk.Terminator) {
				if b.safeTypeSize(op.Type()) > 2 {
					hasMultiByteCopy = true
				}
			}
		}
	}

	// Step 2: Determine available physical registers for this configuration
	availMask := AllocatableRegisters(b.globalsAtY, b.useFramePointer)
	var allocatableRegs []string

	// On M6809, X and D are expression evaluation scratch registers.
	// Dedicated global register allocation pool:
	// U is index register (available if !useFramePointer)
	// Y is index register (available if !globalsAtY and no multi-byte copy using Y)
	if availMask.Contains(RegU) {
		allocatableRegs = append(allocatableRegs, "u")
	}
	if availMask.Contains(RegY) && !hasMultiByteCopy {
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

	// Also identify any values whose fields are extracted (they must stay in stack slots for ExtractField)
	fieldExtracted := make(map[int]bool)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if e, ok := instr.(*ir.ExtractField); ok {
				if sInst, ok := e.Struct.(ir.Instruction); ok {
					fieldExtracted[sInst.GetID()] = true
				}
			}
		}
	}

	// Collect candidates: 16-bit values, not address-taken
	var candidates []candidate
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			id := instr.GetID()
			if b.addressTaken != nil && b.addressTaken[id] {
				continue
			}
			if b.localAddressTaken != nil && b.localAddressTaken[id] {
				continue
			}
			if fieldExtracted[id] {
				continue
			}

			// Do not allocate physical registers to constants, sizeof, or AddressOfGlobal
			switch instr.(type) {
			case *ir.ConstByte, *ir.ConstWord, *ir.Sizeof, *ir.AddressOfGlobal:
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
			if typ.IsAStruct() || typ.IsAnArray() || typ.IsASlice() || typ.Bits&(ir.TypeBitStruct|ir.TypeBitArray|ir.TypeBitSlice) != 0 || len(typ.FieldNamesAndTypes) > 0 || typ.ArrayLen > 0 {
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

	// Step 4: Build candidate interference subgraph
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

	// Priority-based greedy coloring: assign highest-scoring candidates first
	allocated := make(map[int]string)
	assignedColor := make(map[int]int)

	for _, cand := range candidates {
		usedColors := make(map[int]bool)
		for neighbor := range candIG.Edges[cand.id] {
			if col, ok := assignedColor[neighbor]; ok {
				usedColors[col] = true
			}
		}

		chosen := -1
		if prefs, ok := preferences[cand.id]; ok {
			for _, pref := range prefs {
				if pref < len(allocatableRegs) && !usedColors[pref] {
					chosen = pref
					break
				}
			}
		}
		if chosen == -1 {
			for c := 0; c < len(allocatableRegs); c++ {
				if !usedColors[c] {
					chosen = c
					break
				}
			}
		}

		if chosen >= 0 {
			assignedColor[cand.id] = chosen
			allocated[cand.id] = allocatableRegs[chosen]
		}
	}

	return allocated
}
