package cbe

import (
	"bytes"
	"fmt"
	"log"
	"github.com/strickyak/minigolf/ir"
	"strings"
)

type CBE struct {
	buf        bytes.Buffer
	typedefBuf bytes.Buffer
	arrayTypes map[string]bool
}

func New() *CBE {
	return &CBE{
		arrayTypes: make(map[string]bool),
	}
}

func (c *CBE) mapType(typ string) string {
	if typ == "byte" || typ == "word" || typ == "void" || typ == "unknown" {
		return typ
	}
	if strings.HasPrefix(typ, "*") {
		return c.mapType(typ[1:]) + "*"
	}
	if strings.HasPrefix(typ, "[") {
		idx := strings.Index(typ, "]")
		if idx == -1 {
			return "word"
		}
		lenStr := typ[1:idx]
		eltType := typ[idx+1:]

		eltName := c.mapType(eltType)
		typeName := fmt.Sprintf("t_arr_%s_%s", lenStr, eltName)

		if !c.arrayTypes[typeName] {
			c.arrayTypes[typeName] = true
			c.typedefBuf.WriteString(fmt.Sprintf("typedef struct { %s data[%s]; } %s;\n", eltName, lenStr, typeName))
		}
		return typeName
	}
	if strings.HasPrefix(typ, "struct{") {
		content := typ[7 : len(typ)-1]
		typeName := "t_tuple_" + strings.ReplaceAll(content, ";", "_")
		typeName = strings.ReplaceAll(typeName, "*", "ptr_")
		typeName = strings.ReplaceAll(typeName, "[", "arr_")
		typeName = strings.ReplaceAll(typeName, "]", "_")

		if !c.arrayTypes[typeName] {
			c.arrayTypes[typeName] = true

			var fields string
			depth := 0
			start := 0
			fIdx := 0
			for i := 0; i < len(content); i++ {
				if content[i] == '{' {
					depth++
				} else if content[i] == '}' {
					depth--
				} else if content[i] == ';' && depth == 0 {
					fieldType := content[start:i]
					fields += fmt.Sprintf("%s f%d; ", c.mapType(fieldType), fIdx)
					start = i + 1
					fIdx++
				}
			}
			c.typedefBuf.WriteString(fmt.Sprintf("typedef struct { %s} %s;\n", fields, typeName))
		}
		return typeName
	}
	// Assume it's a named struct type if it reaches here
	typ = strings.ReplaceAll(typ, ".", "_")
	typ = strings.ReplaceAll(typ, "*", "ptr_")
	typ = strings.ReplaceAll(typ, "[", "_")
	typ = strings.ReplaceAll(typ, "]", "_")
	return typ
}

func (c *CBE) Generate(program *ir.Program) string {
	// Struct types
	for _, name := range program.TypeDefOrder {
		typStr := program.TypeDefs[name]
		if strings.HasPrefix(typStr, "struct{") {
			content := typStr[7 : len(typStr)-1]
			var fields string
			depth := 0
			start := 0
			fIdx := 0
			for i := 0; i < len(content); i++ {
				if content[i] == '{' {
					depth++
				} else if content[i] == '}' {
					depth--
				} else if content[i] == ';' && depth == 0 {
					fTyp := content[start:i]
					fields += fmt.Sprintf("%s f%d; ", c.mapType(fTyp), fIdx)
					fIdx++
					start = i + 1
				}
			}
			nameSanitized := c.mapType(name)
			c.typedefBuf.WriteString(fmt.Sprintf("typedef struct %s %s;\n", nameSanitized, nameSanitized))
			c.typedefBuf.WriteString(fmt.Sprintf("struct %s { %s};\n", nameSanitized, fields))
		}
	}

	// Globals
	for _, g := range program.Globals {
		gName := strings.ReplaceAll(g.Name, ".", "_")
		c.buf.WriteString(fmt.Sprintf("%s v_%s;\n", c.mapType(string(g.Typ)), gName))
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

	var finalBuf bytes.Buffer
	finalBuf.WriteString("#include <stdio.h>\n")
	finalBuf.WriteString("#include <stdint.h>\n\n")
	finalBuf.WriteString("typedef uint8_t byte;\n")
	finalBuf.WriteString("typedef uintptr_t word;\n\n")

	finalBuf.WriteString(c.typedefBuf.String())
	finalBuf.WriteString("\n")
	finalBuf.WriteString(c.buf.String())

	return finalBuf.String()
}

func (c *CBE) emitFuncSignature(f *ir.Function, isForward bool) {
	retType := "void"
	if f.ReturnType != ir.TypeVoid {
		retType = c.mapType(string(f.ReturnType))
	}

	var params []string
	for _, p := range f.Parameters {
		params = append(params, fmt.Sprintf("%s v_%s", c.mapType(string(p.Typ)), p.Name))
	}

	fName := strings.ReplaceAll(f.Name, ".", "_")
	c.buf.WriteString(fmt.Sprintf("%s f_%s(%s)", retType, fName, strings.Join(params, ", ")))
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
				c.buf.WriteString(fmt.Sprintf("\t%s v%d;\n", c.mapType(string(instr.Type())), instr.GetID()))
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

			if ins, ok := instr.(*ir.InsertElement); ok {
				c.buf.WriteString(fmt.Sprintf("\tv%d = %s;\n", ins.GetID(), c.formatVal(ins.Array)))
				c.buf.WriteString(fmt.Sprintf("\tv%d.data[%s] = %s;\n", ins.GetID(), c.formatVal(ins.Index), c.formatVal(ins.Val)))
				continue
			}

			if ins, ok := instr.(*ir.InsertField); ok {
				c.buf.WriteString(fmt.Sprintf("\tv%d = %s;\n", ins.GetID(), c.formatVal(ins.Struct)))
				c.buf.WriteString(fmt.Sprintf("\tv%d.f%d = %s;\n", ins.GetID(), ins.FieldIndex, c.formatVal(ins.Val)))
				continue
			}

			if ins, ok := instr.(*ir.InsertFieldPtr); ok {
				c.buf.WriteString(fmt.Sprintf("\t(%s->f%d) = %s;\n", c.formatVal(ins.Ptr), ins.FieldIndex, c.formatVal(ins.Val)))
				continue
			}

			if stPtr, ok := instr.(*ir.StorePtr); ok {
				c.buf.WriteString(fmt.Sprintf("\t(*%s) = %s;\n", c.formatVal(stPtr.Ptr), c.formatVal(stPtr.Val)))
				continue
			}

			if sm, ok := instr.(*ir.SourceMarker); ok {
				c.buf.WriteString(fmt.Sprintf("\t/* %s */\n", sm.Comment))
				continue
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
		default:
			log.Panicf("bad case: %T / %v", term, term)
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
		gName := strings.ReplaceAll(val.Name, ".", "_")
		return "v_" + gName
	case *ir.StringLiteral:
		return fmt.Sprintf("%q", val.Value)
	case ir.Instruction:
		return fmt.Sprintf("v%d", val.(interface{ GetID() int }).GetID())
	default:
		log.Panicf("bad case: %T / %v", val, val)
	}
	return v.String()
}

func (c *CBE) emitInstrExpr(instr ir.Instruction) string {
	switch i := instr.(type) {
	case *ir.ConstByte:
		return fmt.Sprintf("%d", i.Val)
	case *ir.ConstWord:
		return fmt.Sprintf("%d", i.Val)
	case *ir.Sizeof:
		return fmt.Sprintf("sizeof(%s)", c.mapType(string(i.TargetTyp)))
	case *ir.Load:
		return c.formatVal(i.Global)
	case *ir.Store:
		return fmt.Sprintf("%s = %s", c.formatVal(i.Global), c.formatVal(i.Val))
	case *ir.BinaryOp:
		var opStr string
		switch i.Op {
		case "add":
			opStr = "+"
		case "sub":
			opStr = "-"
		case "mul":
			opStr = "*"
		case "div":
			opStr = "/"
		case "mod":
			opStr = "%"
		case "and":
			opStr = "&"
		case "or":
			opStr = "|"
		case "xor":
			opStr = "^"
		case "shl":
			opStr = "<<"
		case "shr":
			opStr = ">>"
		default:
			opStr = "UNKNOWN_BINARY_OP(" + i.Op + ")"
		}
		return fmt.Sprintf("(%s %s %s)", c.formatVal(i.Left), opStr, c.formatVal(i.Right))
	case *ir.Compare:
		var opStr string
		switch i.Op {
		case "eq":
			opStr = "=="
		case "neq":
			opStr = "!="
		case "lt":
			opStr = "<"
		case "lte":
			opStr = "<="
		case "gt":
			opStr = ">"
		case "gte":
			opStr = ">="
		default:
			opStr = "UNKNOWN_COMPARE_OP(" + i.Op + ")"
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
		for idx, arg := range i.Args {
			argStr := c.formatVal(arg)
			if idx < len(i.Func.Parameters) {
				expectedTyp := string(i.Func.Parameters[idx].Typ)
				argTyp := string(arg.Type())
				if strings.HasPrefix(expectedTyp, "*") && !strings.HasPrefix(argTyp, "*") {
					argStr = "(&" + argStr + ")"
				}
			}
			args = append(args, argStr)
		}
		fName := strings.ReplaceAll(i.Func.Name, ".", "_")
		return fmt.Sprintf("f_%s(%s)", fName, strings.Join(args, ", "))
	case *ir.BuiltinCall:
		if i.Name == "print" || i.Name == "println" {
			return c.emitPrint(i.Name == "println", i.Args)
		}
	case *ir.Cast:
		switch i.Op {
		case "trunc":
			return fmt.Sprintf("(byte)(%s)", c.formatVal(i.Operand))
		case "zero_ext":
			return fmt.Sprintf("(word)(%s)", c.formatVal(i.Operand))
		case "word_to_ptr":
			return fmt.Sprintf("(%s)(%s)", c.mapType(string(i.Typ)), c.formatVal(i.Operand))
		default:
			log.Panicf("bad case: %v", i.Op)
		}
	case *ir.ZeroInit:
		return fmt.Sprintf("(%s){0}", c.mapType(string(i.Typ)))
	case *ir.ExtractElement:
		return fmt.Sprintf("(%s).data[%s]", c.formatVal(i.Array), c.formatVal(i.Index))
	case *ir.ExtractField:
		return fmt.Sprintf("(%s).f%d", c.formatVal(i.Struct), i.FieldIndex)
	case *ir.AddressOfGlobal:
		gName := strings.ReplaceAll(i.Global.Name, ".", "_")
		return fmt.Sprintf("(&v_%s)", gName)
	case *ir.AddressOfLocal:
		return fmt.Sprintf("(&%s)", c.formatVal(i.Local))
	case *ir.AddressOfField:
		return fmt.Sprintf("(&(%s->f%d))", c.formatVal(i.Ptr), i.FieldIndex)
	case *ir.LoadPtr:
		return fmt.Sprintf("(*%s)", c.formatVal(i.Ptr))
	case *ir.ExtractFieldPtr:
		return fmt.Sprintf("(%s->f%d)", c.formatVal(i.Ptr), i.FieldIndex)
	default:
		log.Panicf("bad case: %T / %v", i, i)
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
			if arg.Type() == ir.TypeInt {
				formatStrs = append(formatStrs, "%lld")
				argStrs = append(argStrs, fmt.Sprintf("(long long)%s", c.formatVal(arg)))
			} else {
				formatStrs = append(formatStrs, "%llu")
				argStrs = append(argStrs, fmt.Sprintf("(unsigned long long)%s", c.formatVal(arg)))
			}
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
