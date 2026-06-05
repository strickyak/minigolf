package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// StrengthReductionPass replaces expensive operations (like multiplication and division)
// with cheaper equivalent operations (like bit-shifts) when operands are powers of two.
type StrengthReductionPass struct{}

func (p *StrengthReductionPass) Name() string {
	return "StrengthReductionPass"
}

func (p *StrengthReductionPass) Run(f *ir.Function) bool {
	changed := false

	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			if i, ok := instr.(*ir.BinaryOp); ok {
				if p.reduce(i) {
					changed = true
				}
			}
		}
	}

	return changed
}

func (p *StrengthReductionPass) reduce(i *ir.BinaryOp) bool {
	cLeftW, isLeftConst := i.Left.(*ir.ConstWord)
	cRightW, isRightConst := i.Right.(*ir.ConstWord)

	// Check x OP C
	if isRightConst {
		ok, log2 := p.isPowerOf2(cRightW.Val)
		if ok {
			switch i.Op {
			case "mul":
				// x * 2^N -> x << N
				i.Op = "shl"
				i.Right = &ir.ConstWord{BaseInstruction: ir.BaseInstruction{Typ: i.Right.Type()}, Val: log2}
				return true
			case "div":
				// x / 2^N -> x >> N
				i.Op = "shr"
				i.Right = &ir.ConstWord{BaseInstruction: ir.BaseInstruction{Typ: i.Right.Type()}, Val: log2}
				return true
			case "mod":
				// x % 2^N -> x & (2^N - 1)
				i.Op = "and"
				i.Right = &ir.ConstWord{BaseInstruction: ir.BaseInstruction{Typ: i.Right.Type()}, Val: cRightW.Val - 1}
				return true
			}
		}
	}

	// Check C OP x
	if isLeftConst {
		ok, log2 := p.isPowerOf2(cLeftW.Val)
		if ok {
			switch i.Op {
			case "mul":
				// 2^N * x -> x << N
				i.Op = "shl"
				// Swap Left and Right
				i.Left = i.Right
				i.Right = &ir.ConstWord{BaseInstruction: ir.BaseInstruction{Typ: i.Left.Type()}, Val: log2}
				return true
			}
		}
	}

	return false
}

func (p *StrengthReductionPass) isPowerOf2(v uint64) (bool, uint64) {
	if v == 0 {
		return false, 0
	}
	if (v & (v - 1)) != 0 {
		return false, 0
	}
	var log2 uint64 = 0
	for v > 1 {
		v >>= 1
		log2++
	}
	return true, log2
}
