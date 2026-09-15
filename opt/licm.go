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
	loops := FindLoops(f)
	if len(loops) == 0 {
		return false
	}

	changed := false
	for _, loop := range loops {
		if loop.Preheader == nil {
			// Hoisting requires a unique dedicated preheader block.
			continue
		}

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

		// Hoist invariant instructions to Preheader in strict dependency order
		loop.Preheader.Instructions = append(loop.Preheader.Instructions, hoistList...)
		changed = true

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
	}

	return changed
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
