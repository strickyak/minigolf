package ir

import (
	"bytes"
	"fmt"
	"strings"
)

func str(a fmt.Stringer) string {
	if a == nil {
		return "nil"
	}
	return a.String()
}

// PrintProgram generates a human-readable string representation of the SSA IR.
func PrintProgram(p *Program) string {
	var buf bytes.Buffer

	for _, g := range p.Globals {
		buf.WriteString(fmt.Sprintf("global %s %s\n", g.Name, g.Typ))
	}
	if len(p.Globals) > 0 {
		buf.WriteString("\n")
	}

	for _, f := range p.Functions {
		params := []string{}
		for _, param := range f.Parameters {
			params = append(params, fmt.Sprintf("%s %s", param.Typ, param.String()))
		}
		buf.WriteString(fmt.Sprintf("func %s @%s(%s) {\n", f.ReturnType, f.Name, strings.Join(params, ", ")))

		for _, b := range f.Blocks {
			buf.WriteString(fmt.Sprintf("b%d:\n", b.ID))
			for _, instr := range b.Instructions {
					buf.WriteString(PrintInstruction(instr))
					buf.WriteByte('\n')
			} // next instr
		} // next block
		buf.WriteString("}\n\n")
	}

	return buf.String()
}

func PrintInstruction(instr Instruction) string {
	op := instr.Opcode()

	var args []string
	switch i := instr.(type) {
	case *ConstByte:
		args = append(args, fmt.Sprintf("%d", i.Val))
	case *ConstWord:
		args = append(args, fmt.Sprintf("%d", i.Val))
	case *Load:
		args = append(args, i.Global.String())
	case *Store:
		args = append(args, i.Global.String(), i.Val.String())
	case *BinaryOp:
		args = append(args, i.Left.String(), i.Right.String())
	case *Compare:
		args = append(args, str(i.Left), str(i.Right))
	case *UnaryOp:
		args = append(args, i.Operand.String())
	case *Phi:
		for _, edge := range i.Edges {
			args = append(args, fmt.Sprintf("[b%d: %s]", edge.Block.ID, edge.Value.String()))
		}
	case *ExtractElement:
		args = append(args, i.Array.String(), i.Index.String())
	case *InsertElement:
		args = append(args, i.Array.String(), i.Index.String(), i.Val.String())
	case *ExtractField:
		args = append(args, i.Struct.String(), fmt.Sprintf("%d", i.FieldIndex))
	case *InsertField:
		args = append(args, i.Struct.String(), fmt.Sprintf("%d", i.FieldIndex), i.Val.String())
	case *AddressOfGlobal:
		args = append(args, i.Global.String())
	case *AddressOfLocal:
		args = append(args, i.Local.String())
	case *AddressOfFunc:
		args = append(args, "@"+i.Func.Name)
	case *AddressOfField:
		args = append(args, i.Ptr.String(), fmt.Sprintf("%d", i.FieldIndex))
	case *AddressOfElement:
		args = append(args, i.ArrayPtr.String(), i.Index.String())
	case *ExtractFieldPtr:
		args = append(args, i.Ptr.String(), fmt.Sprintf("%d", i.FieldIndex))
	case *InsertFieldPtr:
		args = append(args, i.Ptr.String(), fmt.Sprintf("%d", i.FieldIndex), i.Val.String())
	case *LoadPtr:
		args = append(args, i.Ptr.String())
	case *StorePtr:
		args = append(args, i.Ptr.String(), i.Val.String())
	case *ZeroInit:
		// no args
	case *Sizeof:
		args = append(args, i.TargetTyp.String())
	case *ConstStruct:
		for _, f := range i.Fields {
			args = append(args, f.String())
		}
	case *ConstArray:
		for _, e := range i.Elements {
			args = append(args, e.String())
		}
	case *SourceMarker:
		args = append(args, fmt.Sprintf("%q", i.Comment))
	case *Call:
		args = append(args, "@"+i.Func.Name)
		for _, a := range i.Args {
			args = append(args, a.String())
		}
	case *IndirectCall:
		args = append(args, i.FuncPtr.String())
		for _, a := range i.Args {
			args = append(args, a.String())
		}
	case *BuiltinCall:
		for _, a := range i.Args {
			args = append(args, a.String())
		}
	case *Cast:
		args = append(args, i.Operand.String())
	case *Jump:
		args = append(args, fmt.Sprintf("b%d", i.Target.ID))
	case *Branch:
		args = append(args, i.Condition.String(), fmt.Sprintf("b%d", i.TrueBlock.ID), fmt.Sprintf("b%d", i.FalseBlock.ID))
	case *Return:
		if i.Val != nil {
			args = append(args, i.Val.String())
		}
	}

	comment := instr.GetComment()
	if comment != "" {
		comment = "\t\t; " + comment
	}
	if !instr.Type().Equals(TypeVoid) && !instr.Type().Equals(TypeUnknown) {
		return fmt.Sprintf("  %s:%s = %s %s%s", instr.String(), instr.Type().Name, op, strings.Join(args, ", "), comment)
	} else {
		return fmt.Sprintf("  %s %s%s", op, strings.Join(args, ", "), comment)
	}
}
