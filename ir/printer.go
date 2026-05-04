package ir

import (
	"bytes"
	"fmt"
	"strings"
)

// PrintProgram generates a human-readable string representation of the SSA IR.
func PrintProgram(p *Program) string {
	var buf bytes.Buffer
	
	for _, g := range p.Globals {
		buf.WriteString(fmt.Sprintf("global %s %s\n", g.Name, g.Typ))
	}
	if len(p.Globals) > 0 { buf.WriteString("\n") }
	
	for _, f := range p.Functions {
		params := []string{}
		for _, param := range f.Parameters {
			params = append(params, fmt.Sprintf("%s %s", param.Typ, param.String()))
		}
		buf.WriteString(fmt.Sprintf("func %s @%s(%s) {\n", f.ReturnType, f.Name, strings.Join(params, ", ")))
		
		for _, b := range f.Blocks {
			buf.WriteString(fmt.Sprintf("b%d:\n", b.ID))
			for _, instr := range b.Instructions {
				op := instr.Opcode()
				
				var args []string
				switch i := instr.(type) {
				case *ConstByte: args = append(args, fmt.Sprintf("%d", i.Val))
				case *ConstWord: args = append(args, fmt.Sprintf("%d", i.Val))
				case *Load: args = append(args, i.Global.String())
				case *Store: args = append(args, i.Global.String(), i.Val.String())
				case *BinaryOp: args = append(args, i.Left.String(), i.Right.String())
				case *Compare: args = append(args, i.Left.String(), i.Right.String())
				case *UnaryOp: args = append(args, i.Operand.String())
				case *Phi:
					for _, edge := range i.Edges {
						args = append(args, fmt.Sprintf("[b%d: %s]", edge.Block.ID, edge.Value.String()))
					}
				case *Call:
					args = append(args, "@"+i.Func.Name)
					for _, a := range i.Args { args = append(args, a.String()) }
				case *BuiltinCall:
					for _, a := range i.Args { args = append(args, a.String()) }
				case *Cast: args = append(args, i.Operand.String())
				case *Jump: args = append(args, fmt.Sprintf("b%d", i.Target.ID))
				case *Branch: args = append(args, i.Condition.String(), fmt.Sprintf("b%d", i.TrueBlock.ID), fmt.Sprintf("b%d", i.FalseBlock.ID))
				case *Return:
					if i.Val != nil { args = append(args, i.Val.String()) }
				}
				
				if instr.Type() != TypeVoid && instr.Type() != TypeUnknown {
					buf.WriteString(fmt.Sprintf("  %s:%s = %s %s\n", instr.String(), instr.Type(), op, strings.Join(args, ", ")))
				} else {
					buf.WriteString(fmt.Sprintf("  %s %s\n", op, strings.Join(args, ", ")))
				}
			}
		}
		buf.WriteString("}\n\n")
	}
	
	return buf.String()
}
