package opt

import (
	"fmt"

	"github.com/strickyak/minigolf/ir"
)

type ConstFoldPass struct {
	WordSize int
}

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
	case *ir.ZeroInit:
		return p.foldZeroInit(i)
	case *ir.BinaryOp:
		return p.foldBinaryOp(i)
	case *ir.Compare:
		return p.foldCompare(i)
	case *ir.UnaryOp:
		return p.foldUnaryOp(i)
	case *ir.Cast:
		return p.foldCast(i)
	case *ir.Sizeof:
		return p.foldSizeof(i)
	}
	return nil
}

func (p *ConstFoldPass) foldZeroInit(i *ir.ZeroInit) ir.Instruction {
	typ := i.Typ
	if typ.IsByte() || typ.IsBool() || typ.Name == "byte" || typ.Name == "bool" {
		return &ir.ConstByte{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded ZeroInit"},
			Val:             0,
		}
	}
	if typ.IsWord() || typ.IsInt() || typ.Name == "word" || typ.Name == "int" || typ.IsAPointer() {
		return &ir.ConstWord{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded ZeroInit"},
			Val:             0,
		}
	}
	return nil
}

func (p *ConstFoldPass) foldSizeof(i *ir.Sizeof) ir.Instruction {
	typ := i.TargetTyp
	if typ.IsByte() || typ.IsBool() || typ.Name == "byte" || typ.Name == "bool" {
		return &ir.ConstWord{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded sizeof(byte)"},
			Val:             1,
		}
	}
	if p.WordSize > 0 {
		if typ.IsWord() || typ.IsInt() || typ.Name == "word" || typ.Name == "int" || typ.IsAPointer() || typ.IsAFuncPtr() {
			return &ir.ConstWord{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: fmt.Sprintf("Folded sizeof(%s)", typ.Name)},
				Val:             uint64(p.WordSize),
			}
		}
		if typ.IsAnArray() && typ.ArrayLen > 0 {
			elt := typ.ArrayElementType()
			if elt.IsByte() || elt.IsBool() || elt.Name == "byte" || elt.Name == "bool" {
				return &ir.ConstWord{
					BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: fmt.Sprintf("Folded sizeof(%s)", typ.Name)},
					Val:             uint64(typ.ArrayLen),
				}
			}
			if elt.IsWord() || elt.IsInt() || elt.Name == "word" || elt.Name == "int" || elt.IsAPointer() || elt.IsAFuncPtr() {
				return &ir.ConstWord{
					BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: fmt.Sprintf("Folded sizeof(%s)", typ.Name)},
					Val:             uint64(typ.ArrayLen * p.WordSize),
				}
			}
		}
	}
	return nil
}

func (p *ConstFoldPass) foldCast(i *ir.Cast) ir.Instruction {
	switch i.Op {
	case "trunc":
		if cW, ok := i.Operand.(*ir.ConstWord); ok {
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: ir.TypeByte, Comment: "Folded trunc"},
				Val:             uint8(cW.Val),
			}
		}
		if cB, ok := i.Operand.(*ir.ConstByte); ok {
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: ir.TypeByte, Comment: "Folded trunc"},
				Val:             cB.Val,
			}
		}
	case "zero_ext":
		if cB, ok := i.Operand.(*ir.ConstByte); ok {
			return &ir.ConstWord{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: ir.TypeWord, Comment: "Folded zero_ext"},
				Val:             uint64(cB.Val),
			}
		}
		if cW, ok := i.Operand.(*ir.ConstWord); ok {
			return &ir.ConstWord{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: ir.TypeWord, Comment: "Folded zero_ext"},
				Val:             cW.Val,
			}
		}
	case "word_to_ptr", "ptr_to_word", "bitcast":
		if cW, ok := i.Operand.(*ir.ConstWord); ok {
			return &ir.ConstWord{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op},
				Val:             cW.Val,
			}
		}
		if cB, ok := i.Operand.(*ir.ConstByte); ok {
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op},
				Val:             cB.Val,
			}
		}
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

	cLeftB, isLeftConstB := i.Left.(*ir.ConstByte)
	cRightB, isRightConstB := i.Right.(*ir.ConstByte)

	if isLeftConstB && isRightConstB {
		var result uint8
		switch i.Op {
		case "add":
			result = cLeftB.Val + cRightB.Val
		case "sub":
			result = cLeftB.Val - cRightB.Val
		case "mul":
			result = cLeftB.Val * cRightB.Val
		case "div":
			if cRightB.Val == 0 {
				return nil
			}
			result = cLeftB.Val / cRightB.Val
		case "mod":
			if cRightB.Val == 0 {
				return nil
			}
			result = cLeftB.Val % cRightB.Val
		case "and":
			result = cLeftB.Val & cRightB.Val
		case "or":
			result = cLeftB.Val | cRightB.Val
		case "xor":
			result = cLeftB.Val ^ cRightB.Val
		case "shl":
			result = cLeftB.Val << cRightB.Val
		case "shr":
			result = cLeftB.Val >> cRightB.Val
		default:
			return nil
		}
		return &ir.ConstByte{
			BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op},
			Val:             result,
		}
	}

	// Constant-producing algebraic identities:
	// x * 0 = 0, 0 * x = 0
	// x & 0 = 0, 0 & x = 0
	if i.Op == "mul" || i.Op == "and" {
		if (isRightConstW && cRightW.Val == 0) || (isLeftConstW && cLeftW.Val == 0) {
			return &ir.ConstWord{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op + " by 0"},
				Val:             0,
			}
		}
		if (isRightConstB && cRightB.Val == 0) || (isLeftConstB && cLeftB.Val == 0) {
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op + " by 0"},
				Val:             0,
			}
		}
	}

	// x - x = 0
	// x ^ x = 0
	if i.Left == i.Right {
		if i.Op == "sub" || i.Op == "xor" {
			if i.Typ.IsByte() {
				return &ir.ConstByte{
					BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op + " self"},
					Val:             0,
				}
			}
			return &ir.ConstWord{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded " + i.Op + " self"},
				Val:             0,
			}
		}
	}

	return nil
}

func (p *ConstFoldPass) foldCompare(i *ir.Compare) ir.Instruction {
	cLeftW, isLeftConstW := i.Left.(*ir.ConstWord)
	cRightW, isRightConstW := i.Right.(*ir.ConstWord)

	if isLeftConstW && isRightConstW {
		isInt := i.Left.Type().Equals(ir.TypeInt) || i.Left.Type().IsInt()
		var result bool
		if isInt {
			var l, r int64
			if p.WordSize == 2 {
				l = int64(int16(cLeftW.Val))
				r = int64(int16(cRightW.Val))
			} else if p.WordSize == 4 {
				l = int64(int32(cLeftW.Val))
				r = int64(int32(cRightW.Val))
			} else {
				l = int64(cLeftW.Val)
				r = int64(cRightW.Val)
			}
			switch i.Op {
			case "eq":
				result = l == r
			case "neq":
				result = l != r
			case "lt":
				result = l < r
			case "lte":
				result = l <= r
			case "gt":
				result = l > r
			case "gte":
				result = l >= r
			default:
				return nil
			}
		} else {
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

	cLeftB, isLeftConstB := i.Left.(*ir.ConstByte)
	cRightB, isRightConstB := i.Right.(*ir.ConstByte)

	if isLeftConstB && isRightConstB {
		var result bool
		switch i.Op {
		case "eq":
			result = cLeftB.Val == cRightB.Val
		case "neq":
			result = cLeftB.Val != cRightB.Val
		case "lt":
			result = cLeftB.Val < cRightB.Val
		case "lte":
			result = cLeftB.Val <= cRightB.Val
		case "gt":
			result = cLeftB.Val > cRightB.Val
		case "gte":
			result = cLeftB.Val >= cRightB.Val
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

	if i.Left == i.Right {
		switch i.Op {
		case "eq", "lte", "gte":
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded compare self true"},
				Val:             1,
			}
		case "neq", "lt", "gt":
			return &ir.ConstByte{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Folded compare self false"},
				Val:             0,
			}
		}
	}
	// Narrow comparisons of zero-extended bytes:
	// zero_ext(b1) op zero_ext(b2) -> b1 op b2
	castL, isCastL := i.Left.(*ir.Cast)
	castR, isCastR := i.Right.(*ir.Cast)
	if isCastL && castL.Op == "zero_ext" && (castL.Operand.Type().IsByte() || castL.Operand.Type().Name == "byte") {
		if isCastR && castR.Op == "zero_ext" && (castR.Operand.Type().IsByte() || castR.Operand.Type().Name == "byte") {
			return &ir.Compare{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Narrowed zero_ext compare"},
				Op:              i.Op,
				Left:            castL.Operand,
				Right:           castR.Operand,
			}
		}
		if cRightW, ok := i.Right.(*ir.ConstWord); ok {
			if cRightW.Val <= 255 {
				return &ir.Compare{
					BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Narrowed zero_ext compare"},
					Op:              i.Op,
					Left:            castL.Operand,
					Right: &ir.ConstByte{
						BaseInstruction: ir.BaseInstruction{Typ: ir.TypeByte},
						Val:             uint8(cRightW.Val),
					},
				}
			} else {
				if i.Op == "eq" {
					return &ir.ConstByte{BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ}, Val: 0}
				}
				if i.Op == "neq" {
					return &ir.ConstByte{BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ}, Val: 1}
				}
			}
		}
	}
	if isCastR && castR.Op == "zero_ext" && (castR.Operand.Type().IsByte() || castR.Operand.Type().Name == "byte") {
		if cLeftW, ok := i.Left.(*ir.ConstWord); ok && cLeftW.Val <= 255 {
			return &ir.Compare{
				BaseInstruction: ir.BaseInstruction{ID: i.ID, Typ: i.Typ, Comment: "Narrowed zero_ext compare"},
				Op:              i.Op,
				Left: &ir.ConstByte{
					BaseInstruction: ir.BaseInstruction{Typ: ir.TypeByte},
					Val:             uint8(cLeftW.Val),
				},
				Right: castR.Operand,
			}
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
