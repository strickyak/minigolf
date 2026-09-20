package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// LICMPass implements Loop Invariant Code Motion.
// It detects computations within natural loops whose inputs are invariant,
// and hoists them to the loop preheader so they execute only once.
type LICMPass struct{}

func (p *LICMPass) Name() string {
	return "LICMPass"
}

func (p *LICMPass) Run(f *ir.Function) bool {
	changed := false
	nextBlockID, nextInstrID := getIDAllocators(f)

	maxRounds := len(f.Blocks)*4 + 20
	for round := 0; round < maxRounds; round++ {
		loops := FindLoops(f)
		if len(loops) == 0 {
			break
		}

		hoistedAny := false
		for _, loop := range loops {
			// Index all instructions defined within this loop for O(1) lookup
			loopInstrs := make(map[int]bool)
			for b := range loop.Blocks {
				for _, instr := range b.Instructions {
					loopInstrs[instr.GetID()] = true
				}
			}

			// Identify loop-invariant instructions
			invariantInstrs := make(map[int]bool)
			var hoistList []ir.Instruction

			// Fixed-point iteration to discover invariant instructions
			// whose inputs may be other invariant instructions.
			loopChanged := true
			for loopChanged {
				loopChanged = false

				for _, b := range f.Blocks {
					if !loop.Blocks[b] {
						continue
					}
					for _, instr := range b.Instructions {
						id := instr.GetID()
						if invariantInstrs[id] {
							continue
						}

						if p.canHoist(instr, loopInstrs, invariantInstrs) {
							invariantInstrs[id] = true
							hoistList = append(hoistList, instr)
							loopChanged = true
						}
					}
				}
			}

			if len(hoistList) == 0 {
				continue
			}

			// Ensure a dedicated preheader exists for hoisting.
			preheader := loop.Preheader
			if preheader == nil {
				preheader = p.insertPreheader(f, loop, nextBlockID, nextInstrID)
				loop.Preheader = preheader
			}

			// Hoist invariant instructions to Preheader before its terminator
			insertBeforeTerminator(preheader, hoistList)
			changed = true
			hoistedAny = true

			for _, b := range f.Blocks {
				if !loop.Blocks[b] {
					continue
				}
				var remaining []ir.Instruction
				for _, instr := range b.Instructions {
					if !invariantInstrs[instr.GetID()] {
						remaining = append(remaining, instr)
					}
				}
				b.Instructions = remaining
			}

			// Break out to re-run FindLoops with fresh CFG
			break
		}

		if !hoistedAny {
			break
		}
	}

	return changed
}

func insertBeforeTerminator(b *ir.BasicBlock, instrs []ir.Instruction) {
	if len(b.Instructions) > 0 {
		last := b.Instructions[len(b.Instructions)-1]
		if _, isTerm := last.(ir.Terminator); isTerm {
			term := last
			b.Instructions = append(b.Instructions[:len(b.Instructions)-1], append(instrs, term)...)
			return
		}
	}
	b.Instructions = append(b.Instructions, instrs...)
	if b.Terminator != nil {
		b.Instructions = append(b.Instructions, b.Terminator)
	}
}

func (p *LICMPass) insertPreheader(f *ir.Function, loop *Loop, nextBlockID func() int, nextInstrID func() int) *ir.BasicBlock {
	header := loop.Header

	// 1. Identify all predecessors of header that are OUTSIDE the loop.
	var outsidePreds []*ir.BasicBlock
	outsideSet := make(map[*ir.BasicBlock]bool)
	for _, pred := range header.Predecessors {
		if !loop.Blocks[pred] && !outsideSet[pred] {
			outsidePreds = append(outsidePreds, pred)
			outsideSet[pred] = true
		}
	}

	// 2. Create the dedicated preheader block ending with a jump to header.
	pre := &ir.BasicBlock{
		ID: nextBlockID(),
	}
	preJump := &ir.Jump{
		BaseInstruction: ir.BaseInstruction{
			ID:      nextInstrID(),
			Typ:     ir.TypeVoid,
			Comment: "LICM Preheader",
		},
		Target: header,
	}
	pre.Terminator = preJump
	pre.Instructions = []ir.Instruction{preJump}
	pre.Successors = []*ir.BasicBlock{header}

	if len(outsidePreds) == 0 {
		// Header was the function entry block (f.Blocks[0]).
		// The new preheader becomes the new entry block.
		pre.Predecessors = nil
		header.Predecessors = append(header.Predecessors, pre)

		// Insert pre at the start of f.Blocks
		f.Blocks = append([]*ir.BasicBlock{pre}, f.Blocks...)
		return pre
	}

	// 3. Connect outside predecessors to pre instead of header.
	pre.Predecessors = make([]*ir.BasicBlock, len(outsidePreds))
	copy(pre.Predecessors, outsidePreds)

	for _, pred := range outsidePreds {
		// Update pred's terminator
		switch term := pred.Terminator.(type) {
		case *ir.Jump:
			if term.Target == header {
				term.Target = pre
			}
		case *ir.Branch:
			if term.TrueBlock == header {
				term.TrueBlock = pre
			}
			if term.FalseBlock == header {
				term.FalseBlock = pre
			}
		}

		// Update pred's successors: replace header with pre
		for idx, succ := range pred.Successors {
			if succ == header {
				pred.Successors[idx] = pre
			}
		}
	}

	// 4. Update header's predecessors: remove all outsidePreds, add pre.
	var newHeaderPreds []*ir.BasicBlock
	for _, pred := range header.Predecessors {
		if !outsideSet[pred] {
			newHeaderPreds = append(newHeaderPreds, pred)
		}
	}
	newHeaderPreds = append(newHeaderPreds, pre)
	header.Predecessors = newHeaderPreds

	// 5. Update Phi nodes in header.
	for _, instr := range header.Instructions {
		phi, ok := instr.(*ir.Phi)
		if !ok {
			continue
		}

		var outsideEdges []ir.PhiEdge
		var inLoopEdges []ir.PhiEdge
		for _, edge := range phi.Edges {
			if outsideSet[edge.Block] {
				outsideEdges = append(outsideEdges, edge)
			} else {
				inLoopEdges = append(inLoopEdges, edge)
			}
		}

		if len(outsideEdges) == 0 {
			// No edges from outside predecessors
			continue
		} else if len(outsideEdges) == 1 {
			// Single incoming edge from outside: redirect directly from pre
			inLoopEdges = append(inLoopEdges, ir.PhiEdge{
				Block: pre,
				Value: outsideEdges[0].Value,
			})
			phi.Edges = inLoopEdges
		} else {
			// Multiple incoming edges from outside.
			// Check if all outside edges provide the same value.
			sameVal := true
			firstVal := outsideEdges[0].Value
			for _, edge := range outsideEdges[1:] {
				if edge.Value != firstVal {
					sameVal = false
					break
				}
			}

			var incomingVal ir.Value
			if sameVal {
				incomingVal = firstVal
			} else {
				// Insert a new Phi in pre (at the beginning, before the terminator)
				// to combine values from outside predecessors.
				prePhi := &ir.Phi{
					BaseInstruction: ir.BaseInstruction{
						ID:      nextInstrID(),
						Typ:     phi.Type(),
						Comment: "LICM Preheader Phi",
					},
					Edges: outsideEdges,
				}
				pre.Instructions = append([]ir.Instruction{prePhi}, pre.Instructions...)
				incomingVal = prePhi
			}

			inLoopEdges = append(inLoopEdges, ir.PhiEdge{
				Block: pre,
				Value: incomingVal,
			})
			phi.Edges = inLoopEdges
		}
	}

	// 6. Insert pre into f.Blocks immediately before header.
	var newBlocks []*ir.BasicBlock
	for _, b := range f.Blocks {
		if b == header {
			newBlocks = append(newBlocks, pre)
		}
		newBlocks = append(newBlocks, b)
	}
	f.Blocks = newBlocks

	return pre
}

func getIDAllocators(f *ir.Function) (func() int, func() int) {
	maxBlockID := 0
	maxInstrID := 0
	for _, b := range f.Blocks {
		if b.ID > maxBlockID {
			maxBlockID = b.ID
		}
		for _, instr := range b.Instructions {
			if instr.GetID() > maxInstrID {
				maxInstrID = instr.GetID()
			}
		}
		if b.Terminator != nil && b.Terminator.GetID() > maxInstrID {
			maxInstrID = b.Terminator.GetID()
		}
	}
	nextBlockID := func() int {
		maxBlockID++
		return maxBlockID
	}
	nextInstrID := func() int {
		maxInstrID++
		return maxInstrID
	}
	return nextBlockID, nextInstrID
}

func (p *LICMPass) canHoist(instr ir.Instruction, loopInstrs, invariantInstrs map[int]bool) bool {
	// Cannot hoist instructions with side effects or control flow
	switch i := instr.(type) {
	case *ir.BinaryOp:
		if i.Op == "div" || i.Op == "mod" {
			// Division by zero: only hoist if divisor is a non-zero constant
			if c, ok := asConstInt(i.Right); !ok || c == 0 {
				return false
			}
		}
	case *ir.Compare, *ir.Cast, *ir.Sizeof, *ir.ConstByte, *ir.ConstWord:
		// Safe arithmetic / constant operations
	case *ir.AddressOfGlobal, *ir.AddressOfFunc:
		// Safe symbol addresses
	default:
		// Do not hoist memory loads, stores, calls, phis, etc.
		return false
	}

	// Check that all operands are loop-invariant:
	// either constants, defined outside the loop, or in invariantInstrs
	for _, op := range OperandsOf(instr) {
		if !p.isOperandInvariant(op, loopInstrs, invariantInstrs) {
			return false
		}
	}

	return true
}

func (p *LICMPass) isOperandInvariant(op ir.Value, loopInstrs, invariantInstrs map[int]bool) bool {
	if op == nil {
		return false
	}
	switch v := op.(type) {
	case *ir.ConstByte, *ir.ConstWord, *ir.StringLiteral, *ir.Parameter, *ir.Global, *ir.AddressOfGlobal, *ir.AddressOfFunc:
		return true
	case ir.Instruction:
		if invariantInstrs[v.GetID()] {
			return true
		}
		if loopInstrs[v.GetID()] {
			return false // Defined inside loop and not invariant
		}
		return true // Defined outside the loop
	}
	return false
}
