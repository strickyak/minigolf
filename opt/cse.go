package opt

import (
	"fmt"
	"github.com/strickyak/minigolf/ir"
)

// CSEPass (Common Subexpression Elimination) eliminates redundant identical calculations
// within a single Basic Block.
type CSEPass struct{}

func (p *CSEPass) Name() string {
	return "CSEPass"
}

func (p *CSEPass) Run(f *ir.Function) bool {
	changed := false

	for _, b := range f.Blocks {
		// Maps expression keys to the first instruction that computed them.
		seenExprs := make(map[string]ir.Value)

		for _, instr := range b.Instructions {
			key := p.exprKey(instr)
			if key == "" {
				continue // Instruction cannot be safely CSE'd
			}

			if existingVal, ok := seenExprs[key]; ok {
				// We have seen this exact computation before in this block!
				ReplaceUsesOf(f, instr, existingVal)
				changed = true
			} else {
				// Record it for future instructions in this block
				seenExprs[key] = instr
			}
		}
	}

	return changed
}

// exprKey generates a unique string identifying the computation performed by the instruction.
// If the instruction has side-effects or depends on memory state (like Load), it returns "".
func (p *CSEPass) exprKey(instr ir.Instruction) string {
	switch i := instr.(type) {
	case *ir.BinaryOp:
		k1 := p.valueKey(i.Left)
		k2 := p.valueKey(i.Right)
		if p.isCommutative(i.Op) && k1 > k2 {
			k1, k2 = k2, k1
		}
		return fmt.Sprintf("binop:%s:%s:%s", i.Op, k1, k2)

	case *ir.UnaryOp:
		return fmt.Sprintf("unop:%s:%s", i.Op, p.valueKey(i.Operand))

	case *ir.Compare:
		k1 := p.valueKey(i.Left)
		k2 := p.valueKey(i.Right)
		op := i.Op
		if op == "eq" || op == "ne" {
			if k1 > k2 {
				k1, k2 = k2, k1
			}
		}
		return fmt.Sprintf("cmp:%s:%s:%s", op, k1, k2)

	case *ir.Cast:
		return fmt.Sprintf("cast:%s:%s:%s", i.Op, p.valueKey(i.Operand), i.Typ.Name)

	case *ir.ExtractElement:
		return fmt.Sprintf("ext_elt:%s:%s", p.valueKey(i.Array), p.valueKey(i.Index))

	case *ir.ExtractField:
		return fmt.Sprintf("ext_fld:%s:%d", p.valueKey(i.Struct), i.FieldIndex)

	case *ir.AddressOfLocal:
		return fmt.Sprintf("addr_loc:%s", p.valueKey(i.Local))

	case *ir.AddressOfGlobal:
		return fmt.Sprintf("addr_glb:%p", i.Global)

	case *ir.AddressOfField:
		return fmt.Sprintf("addr_fld:%s:%d", p.valueKey(i.Ptr), i.FieldIndex)

	case *ir.AddressOfElement:
		return fmt.Sprintf("addr_elt:%s:%s", p.valueKey(i.ArrayPtr), p.valueKey(i.Index))

	case *ir.Sizeof:
		return fmt.Sprintf("sizeof:%s", i.TargetTyp.Name)

	default:
		// Instructions like Load, Store, Call, Phi, etc., are not safely CSE-able
		// without deeper alias/dataflow analysis.
		return ""
	}
}

func (p *CSEPass) valueKey(v ir.Value) string {
	if v == nil {
		return "nil"
	}
	switch c := v.(type) {
	case *ir.ConstWord:
		return fmt.Sprintf("cw:%d", c.Val)
	case *ir.ConstByte:
		return fmt.Sprintf("cb:%d", c.Val)
	case *ir.StringLiteral:
		return fmt.Sprintf("sl:%q", c.Value)
	default:
		// Since minigolf IR is in SSA form, an instruction's pointer
		// uniquely identifies the value it produces.
		return fmt.Sprintf("p:%p", v)
	}
}

func (p *CSEPass) isCommutative(op string) bool {
	switch op {
	case "add", "mul", "and", "or", "xor":
		return true
	}
	return false
}
