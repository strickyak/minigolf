package cbe

import (
	"bytes"
	"fmt"
	"minigo/ir"
	"strings"
)

type CBE struct {
	buf bytes.Buffer
}

func New() *CBE {
	return &CBE{}
}

func (c *CBE) Generate(program *ir.Program) string {
	c.buf.WriteString("#include <stdio.h>\n")
	c.buf.WriteString("#include <stdint.h>\n\n")

	c.buf.WriteString("typedef uint8_t byte;\n")
	c.buf.WriteString("typedef uintptr_t word;\n\n")

	// Globals
	for _, g := range program.Globals {
		c.buf.WriteString(fmt.Sprintf("%s v_%s;\n", g.Typ, g.Name))
	}
	if len(program.Globals) > 0 {
		c.buf.WriteString("\n")
	}

	// Forward declarations for functions
	for _, f := range program.Functions {
		c.emitFuncSignature(f, true)
	}
	c.buf.WriteString("\n")

	// Function bodies
	for _, f := range program.Functions {
		c.emitFunc(f)
	}

	// C main
	c.buf.WriteString("int main() {\n")
	c.buf.WriteString("\tf_main();\n")
	c.buf.WriteString("\treturn 0;\n")
	c.buf.WriteString("}\n")

	return c.buf.String()
}

func (c *CBE) emitFuncSignature(f *ir.Function, isForward bool) {
	retType := "void"
	if f.ReturnType != ir.TypeVoid {
		retType = f.ReturnType.String()
	}

	var params []string
	for _, p := range f.Parameters {
		params = append(params, fmt.Sprintf("%s v_%s", p.Typ, p.Name))
	}

	c.buf.WriteString(fmt.Sprintf("%s f_%s(%s)", retType, f.Name, strings.Join(params, ", ")))
	if isForward {
		c.buf.WriteString(";\n")
	} else {
		c.buf.WriteString(" {\n")
	}
}

func (c *CBE) emitFunc(f *ir.Function) {
	c.emitFuncSignature(f, false)

	// Declare all local variables (values) at the top of the function
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			if instr.Type() != ir.TypeVoid && instr.Type() != ir.TypeUnknown {
				c.buf.WriteString(fmt.Sprintf("\t%s v%d;\n", instr.Type(), instr.GetID()))
			}
		}
	}
	c.buf.WriteString("\n")

	// Emit blocks
	for _, b := range f.Blocks {
		c.buf.WriteString(fmt.Sprintf("b%d:\n", b.ID))

		for _, instr := range b.Instructions {
			if _, isPhi := instr.(*ir.Phi); isPhi {
				continue // Phis are handled at the end of predecessor blocks
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue // Handled below
			}

			c.buf.WriteString("\t")
			if instr.Type() != ir.TypeVoid && instr.Type() != ir.TypeUnknown {
				c.buf.WriteString(fmt.Sprintf("v%d = ", instr.GetID()))
			}
			c.buf.WriteString(c.emitInstrExpr(instr) + ";\n")
		}

		// Terminator and Phi edge assignments
		switch term := b.Terminator.(type) {
		case *ir.Jump:
			c.emitPhiAssignments(b, term.Target, "\t")
			c.buf.WriteString(fmt.Sprintf("\tgoto b%d;\n", term.Target.ID))
		case *ir.Branch:
			c.buf.WriteString(fmt.Sprintf("\tif (%s) {\n", c.formatVal(term.Condition)))
			c.emitPhiAssignments(b, term.TrueBlock, "\t\t")
			c.buf.WriteString(fmt.Sprintf("\t\tgoto b%d;\n", term.TrueBlock.ID))
			c.buf.WriteString("\t} else {\n")
			c.emitPhiAssignments(b, term.FalseBlock, "\t\t")
			c.buf.WriteString(fmt.Sprintf("\t\tgoto b%d;\n", term.FalseBlock.ID))
			c.buf.WriteString("\t}\n")
		case *ir.Return:
			if term.Val != nil {
				c.buf.WriteString(fmt.Sprintf("\treturn %s;\n", c.formatVal(term.Val)))
			} else {
				c.buf.WriteString("\treturn;\n")
			}
		}
	}

	c.buf.WriteString("}\n\n")
}

func (c *CBE) emitPhiAssignments(from, to *ir.BasicBlock, indent string) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					c.buf.WriteString(fmt.Sprintf("%sv%d = %s;\n", indent, phi.GetID(), c.formatVal(edge.Value)))
				}
			}
		}
	}
}

func (c *CBE) formatVal(v ir.Value) string {
	switch val := v.(type) {
	case *ir.Parameter:
		return "v_" + val.Name
	case *ir.Global:
		return "v_" + val.Name
	case *ir.StringLiteral:
		return fmt.Sprintf("%q", val.Value)
	case ir.Instruction:
		return fmt.Sprintf("v%d", val.(interface{ GetID() int }).GetID())
	}
	return v.String()
}

func (c *CBE) emitInstrExpr(instr ir.Instruction) string {
	switch i := instr.(type) {
	case *ir.ConstByte:
		return fmt.Sprintf("%d", i.Val)
	case *ir.ConstWord:
		return fmt.Sprintf("%d", i.Val)
	case *ir.Load:
		return c.formatVal(i.Global)
	case *ir.Store:
		return fmt.Sprintf("%s = %s", c.formatVal(i.Global), c.formatVal(i.Val))
	case *ir.BinaryOp:
		var opStr string
		switch i.Op {
		case "add": opStr = "+"
		case "sub": opStr = "-"
		case "mul": opStr = "*"
		case "div": opStr = "/"
		case "mod": opStr = "%"
		case "and": opStr = "&"
		case "or":  opStr = "|"
		case "xor": opStr = "^"
		case "shl": opStr = "<<"
		case "shr": opStr = ">>"
		}
		return fmt.Sprintf("(%s %s %s)", c.formatVal(i.Left), opStr, c.formatVal(i.Right))
	case *ir.Compare:
		var opStr string
		switch i.Op {
		case "eq": opStr = "=="
		case "neq": opStr = "!="
		case "lt": opStr = "<"
		case "lte": opStr = "<="
		case "gt": opStr = ">"
		case "gte": opStr = ">="
		}
		// Cast to byte to ensure strictly byte-level boolean properties
		return fmt.Sprintf("(byte)(%s %s %s)", c.formatVal(i.Left), opStr, c.formatVal(i.Right))
	case *ir.UnaryOp:
		if i.Op == "not" {
			return fmt.Sprintf("(~%s)", c.formatVal(i.Operand))
		}
		return fmt.Sprintf("(-%s)", c.formatVal(i.Operand))
	case *ir.Call:
		var args []string
		for _, arg := range i.Args {
			args = append(args, c.formatVal(arg))
		}
		return fmt.Sprintf("f_%s(%s)", i.Func.Name, strings.Join(args, ", "))
	case *ir.BuiltinCall:
		if i.Name == "print" || i.Name == "println" {
			return c.emitPrint(i.Name == "println", i.Args)
		}
	case *ir.Cast:
		if i.Op == "trunc" {
			return fmt.Sprintf("(byte)(%s)", c.formatVal(i.Operand))
		} else if i.Op == "zero_ext" {
			return fmt.Sprintf("(word)(%s)", c.formatVal(i.Operand))
		}
	}
	return "/* unsupported instruction */"
}

func (c *CBE) emitPrint(newline bool, args []ir.Value) string {
	formatStrs := []string{}
	var argStrs []string

	for _, arg := range args {
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			formatStrs = append(formatStrs, strLit.Value)
		} else {
			formatStrs = append(formatStrs, "%llu")
			argStrs = append(argStrs, fmt.Sprintf("(unsigned long long)%s", c.formatVal(arg)))
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\\n"
	}

	if len(argStrs) > 0 {
		return fmt.Sprintf("printf(\"%s\", %s)", format, strings.Join(argStrs, ", "))
	}
	return fmt.Sprintf("printf(\"%s\")", format)
}
