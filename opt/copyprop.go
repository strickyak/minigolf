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

	if isRightConstW {
		switch i.Op {
		case "add", "sub":
			if cRightW.Val == 0 {
				return i.Left
			}
		case "mul", "div":
			if cRightW.Val == 1 {
				return i.Left
			}
		}
	}

	if isLeftConstW {
		switch i.Op {
		case "add":
			if cLeftW.Val == 0 {
				return i.Right
			}
		case "mul":
			if cLeftW.Val == 1 {
				return i.Right
			}
		}
	}

	return nil
}
