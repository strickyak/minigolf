package opt

import (
	"github.com/strickyak/minigolf/ir"
)

type ConstFoldPass struct{}

func (p *ConstFoldPass) Name() string { return "ConstFold" }

func (p *ConstFoldPass) Run(f *ir.Function) bool {
	changed := false

	for _, b := range f.Blocks {
		for i, instr := range b.Instructions {
			newInstr := p.foldInstruction(instr, f)
			if newInstr != nil {
				// Replace instruction in the block
				b.Instructions[i] = newInstr
				// Replace uses globally in the function
				ReplaceUsesOf(f, instr, newInstr)
				changed = true
			}
		}

		if b.Terminator != nil {
			newTerm := p.foldInstruction(b.Terminator, f)
			if newTerm != nil {
				if term, ok := newTerm.(ir.Terminator); ok {
					b.Terminator = term
				} else {
					// We replaced a terminator with something that isn't a terminator?
					// This happens if we tried to fold a branch but that's for DBE.
					// ConstFold just folds expressions.
				}
			}
		}
	}

	return changed
}

func (p *ConstFoldPass) foldInstruction(instr ir.Instruction, f *ir.Function) ir.Instruction {
	switch i := instr.(type) {
	case *ir.BinaryOp:
		return p.foldBinaryOp(i)
	case *ir.Compare:
		return p.foldCompare(i)
	case *ir.UnaryOp:
		return p.foldUnaryOp(i)
	}
	return nil
}

func (p *ConstFoldPass) foldBinaryOp(i *ir.BinaryOp) ir.Instruction {
	cLeftW, isLeftConstW := i.Left.(*ir.ConstWord)
	cRightW, isRightConstW := i.Right.(*ir.ConstWord)

	if isLeftConstW && isRightConstW {
		var result uint64
		switch i.Op {
		case "add":
			result = cLeftW.Val + cRightW.Val
		case "sub":
			result = cLeftW.Val - cRightW.Val
		case "mul":
			result = cLeftW.Val * cRightW.Val
		case "div":
			if cRightW.Val == 0 {
				return nil // Don't fold division by zero
			}
			result = cLeftW.Val / cRightW.Val
		case "mod":
			if cRightW.Val == 0 {
				return nil
			}
			result = cLeftW.Val % cRightW.Val
		case "and":
			result = cLeftW.Val & cRightW.Val
		case "or":
			result = cLeftW.Val | cRightW.Val
		case "xor":
			result = cLeftW.Val ^ cRightW.Val
		case "shl":
			result = cLeftW.Val << cRightW.Val
		case "shr":
			result = cLeftW.Val >> cRightW.Val
		default:
			return nil
		}
		return &ir.ConstWord{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op},
			Val:             result,
		}
	}
	return nil
}

func (p *ConstFoldPass) foldCompare(i *ir.Compare) ir.Instruction {
	cLeftW, isLeftConstW := i.Left.(*ir.ConstWord)
	cRightW, isRightConstW := i.Right.(*ir.ConstWord)

	if isLeftConstW && isRightConstW {
		var result bool
		switch i.Op {
		case "eq":
			result = cLeftW.Val == cRightW.Val
		case "neq":
			result = cLeftW.Val != cRightW.Val
		case "lt":
			result = cLeftW.Val < cRightW.Val
		case "lte":
			result = cLeftW.Val <= cRightW.Val
		case "gt":
			result = cLeftW.Val > cRightW.Val
		case "gte":
			result = cLeftW.Val >= cRightW.Val
		default:
			return nil
		}
		var val uint8 = 0
		if result {
			val = 1
		}
		return &ir.ConstByte{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded compare " + i.Op},
			Val:             val,
		}
	}
	return nil
}

func (p *ConstFoldPass) foldUnaryOp(i *ir.UnaryOp) ir.Instruction {
	cW, isConstW := i.Operand.(*ir.ConstWord)

	if isConstW {
		var result uint64
		switch i.Op {
		case "not":
			result = ^cW.Val
		case "neg":
			result = -cW.Val
		default:
			return nil
		}
		return &ir.ConstWord{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded unary " + i.Op},
			Val:             result,
		}
	}

	cB, isConstB := i.Operand.(*ir.ConstByte)
	if isConstB {
		if i.Op == "not" {
			var result uint8 = 0
			if cB.Val == 0 {
				result = 1
			}
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded unary " + i.Op},
				Val:             result,
			}
		}
	}

	return nil
}
