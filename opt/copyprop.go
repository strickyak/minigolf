package opt

import (
	"github.com/strickyak/minigolf/ir"
)

// CopyPropPass finds instructions that act as direct copies (identities)
// and propagates the original value to all uses of the copy.
// It handles redundant casts, and algebraic identities (e.g. x + 0).
type CopyPropPass struct{}

func (p *CopyPropPass) Name() string {
	return "CopyPropPass"
}

func (p *CopyPropPass) Run(f *ir.Function) bool {
	changed := false

	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			var replacement ir.Value

			switch i := instr.(type) {
			case *ir.Cast:
				if i.Typ.Equals(i.Operand.Type()) {
					replacement = i.Operand
				}

			case *ir.BinaryOp:
				replacement = p.simplifyBinaryOp(i)
			}

			if replacement != nil {
				ReplaceUsesOf(f, instr, replacement)
				changed = true
			}
		}
	}

	return changed
}

func (p *CopyPropPass) simplifyBinaryOp(i *ir.BinaryOp) ir.Value {
	cLeftW, isLeftConstW := i.Left.(*ir.ConstWord)
	cRightW, isRightConstW := i.Right.(*ir.ConstWord)
	cLeftB, isLeftConstB := i.Left.(*ir.ConstByte)
	cRightB, isRightConstB := i.Right.(*ir.ConstByte)

	var rightVal uint64
	hasRightConst := false
	if isRightConstW {
		rightVal = cRightW.Val
		hasRightConst = true
	} else if isRightConstB {
		rightVal = uint64(cRightB.Val)
		hasRightConst = true
	}

	var leftVal uint64
	hasLeftConst := false
	if isLeftConstW {
		leftVal = cLeftW.Val
		hasLeftConst = true
	} else if isLeftConstB {
		leftVal = uint64(cLeftB.Val)
		hasLeftConst = true
	}

	if hasRightConst {
		switch i.Op {
		case "add", "sub", "or", "xor", "shl", "shr":
			if rightVal == 0 {
				return i.Left
			}
		case "mul", "div":
			if rightVal == 1 {
				return i.Left
			}
		}
	}

	if hasLeftConst {
		switch i.Op {
		case "add", "or", "xor":
			if leftVal == 0 {
				return i.Right
			}
		case "mul":
			if leftVal == 1 {
				return i.Right
			}
		}
	}

	// Idempotent operations on same value: x & x -> x, x | x -> x
	if i.Left == i.Right {
		switch i.Op {
		case "and", "or":
			return i.Left
		}
	}

	return nil
}
