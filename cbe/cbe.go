package cbe

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

type CBE struct {
	buf        bytes.Buffer
	typedefBuf bytes.Buffer
	arrayTypes map[string]bool
	f          *ir.Function
}

func New() *CBE {
	return &CBE{
		arrayTypes: make(map[string]bool),
	}
}

func (c *CBE) getSlot(id int) int {
	if c.f != nil && c.f.SlotAlias != nil {
		for {
			if alias, ok := c.f.SlotAlias[id]; ok {
				id = alias
			} else {
				break
			}
		}
	}
	return id
}

func (c *CBE) mapType(typ string) string {
	if typ == "byte" || typ == "word" || typ == "void" || typ == "unknown" || typ == "bool" {
		if typ == "bool" {
			return "byte"
		}
		return typ
	}
	if strings.HasPrefix(typ, "func_ptr_") || typ == "func" {
		return "word"
	}
	if typ == "const_integer" {
		return "word"
	}
	if typ == "int" {
		return "int16_t"
	}
	if typ == "any" {
		return "struct any_struct"
	}
	if typ == "noreturn" {
		return "int"
	}
	if typ == "uint" {
		return "uint16_t"
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
		typeName := fmt.Sprintf("t_arr_%s_%s", lenStr, ir.MangleName(eltType))

		if !c.arrayTypes[typeName] {
			c.arrayTypes[typeName] = true
			c.typedefBuf.WriteString(fmt.Sprintf("struct %s { %s data[%s]; };\n", typeName, eltName, lenStr))
		}
		return "struct " + typeName
	}
	if strings.HasPrefix(typ, "struct{") || strings.HasPrefix(typ, "tuple_") {
		content := typ[7 : len(typ)-1]
		typeName := "t_tuple_" + ir.MangleName(content)

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
			c.typedefBuf.WriteString(fmt.Sprintf("struct %s { %s};\n", typeName, fields))
		}
		return "struct " + typeName
	}
	// Assume it's a named struct type if it reaches here
	return "struct " + ir.MangleName(typ)
}

func (c *CBE) Generate(program *ir.Program) string {
	// Struct types forward declarations
	for _, name := range program.TypeDefOrder {
		typStr := program.TypeDefs[name]
		if typStr.IsAStruct() {
			nameSanitized := c.mapType(name)
			c.typedefBuf.WriteString(fmt.Sprintf("%s;\n", nameSanitized))
		}
	}

	// Struct types bodies
	var pending []string
	for _, name := range program.TypeDefOrder {
		if program.TypeDefs[name].IsAStruct() {
			pending = append(pending, name)
		}
	}

	for len(pending) > 0 {
		var nextPending []string
		madeProgress := false
		for _, name := range pending {
			typStr := program.TypeDefs[name]
			content := typStr.Name[7 : len(typStr.Name)-1]

			// Check dependencies
			canEmit := true
			depth := 0
			start := 0
			for i := 0; i < len(content); i++ {
				if content[i] == '{' {
					depth++
				} else if content[i] == '}' {
					depth--
				} else if content[i] == ';' && depth == 0 {
					fTyp := content[start:i]
					if !strings.HasSuffix(fTyp, "*") {
						for _, p := range pending {
							if p == fTyp && p != name {
								canEmit = false
								break
							}
						}
					}
					start = i + 1
				}
			}

			if canEmit {
				nameSanitized := c.mapType(name)
				var fields string
				depth = 0
				start = 0
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
				c.typedefBuf.WriteString(fmt.Sprintf("%s { %s};\n", nameSanitized, fields))
				madeProgress = true
			} else {
				nextPending = append(nextPending, name)
			}
		}
		if !madeProgress {
			// Cycle detected or unresolvable dependencies; just emit remaining in current order
			for _, name := range pending {
				typStr := program.TypeDefs[name]
				content := typStr.Name[7 : len(typStr.Name)-1]
				nameSanitized := c.mapType(name)
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
				c.typedefBuf.WriteString(fmt.Sprintf("%s { %s};\n", nameSanitized, fields))
			}
			break
		}
		pending = nextPending
	}

	// Globals
	for _, g := range program.Globals {
		gName := strings.ReplaceAll(g.Name, ".", "_")
		if g.IsInit {
			if g.InitVal != nil {
				c.buf.WriteString(fmt.Sprintf("%s v_%s = %s;\n", c.mapType(g.Typ.Name), gName, c.formatVal(g.InitVal)))
			} else {
				var byteStrs []string
				for i := 0; i < len(g.InitString); i++ {
					byteStrs = append(byteStrs, fmt.Sprintf("%d", g.InitString[i]))
				}
				c.buf.WriteString(fmt.Sprintf("%s v_%s = { .data = { %s } };\n", c.mapType(g.Typ.Name), gName, strings.Join(byteStrs, ", ")))
			}
		} else {
			c.buf.WriteString(fmt.Sprintf("%s v_%s;\n", c.mapType(g.Typ.Name), gName))
		}
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
		if len(f.Blocks) > 0 {
			c.emitFunc(f)
		}
	}

	// Check if panic is used
	usesPanic := false
	for _, f := range program.Functions {
		for _, b := range f.Blocks {
			for _, i := range b.Instructions {
				switch instr := i.(type) {
				case *ir.SetJmp, *ir.LongJmp:
					usesPanic = true
				case *ir.BuiltinCall:
					if instr.Name == "panic" || instr.Name == "_propagate_panic_" || instr.Name == "_unlink_jmp_" {
						usesPanic = true
					}
				}
			}
		}
	}

	// C main
	c.buf.WriteString("int main() {\n")
	if usesPanic {
		c.buf.WriteString("\tstruct jmp_struct jumper_main;\n")
		c.buf.WriteString("\tjumper_main.prev = NULL;\n")
		c.buf.WriteString("\tv_prelude__jmp_chain_ = (byte*)(&jumper_main);\n")
		c.buf.WriteString("\tint val = setjmp(jumper_main.jmpbuf);\n")
		c.buf.WriteString("\tif (val != 0) {\n")
		c.buf.WriteString("\t\tprintf(\"\\n*** UNCAUGHT_PANIC\\n\");\n")
		c.buf.WriteString("\t\tif (v_prelude__panic_) {\n")
		c.buf.WriteString("\t\t\tprintf(\"*** %s\\n\", (char*)v_prelude__panic_);\n")
		c.buf.WriteString("\t\t}\n")
		c.buf.WriteString("\t\tabort();\n")
		c.buf.WriteString("\t}\n")
	}
	c.buf.WriteString("\tf_main__main();\n")
	c.buf.WriteString("\treturn 0;\n")
	c.buf.WriteString("}\n")

	var finalBuf bytes.Buffer
	finalBuf.WriteString("#include <stdio.h>\n#include <stdint.h>\n#include <stdlib.h>\n#include <string.h>\n#include <setjmp.h>\n\nstruct jmp_struct {\n\tjmp_buf jmpbuf;\n\tstruct jmp_struct *prev;\n};\n\n// Types\n\n")
	finalBuf.WriteString("typedef uint8_t byte;\n")
	finalBuf.WriteString("typedef uintptr_t word;\n\n")

	finalBuf.WriteString("word f_prelude__shl_word(word x, word n) { return x << n; }\n")
	finalBuf.WriteString("word f_prelude__shr_word(word x, word n) { return x >> n; }\n")
	finalBuf.WriteString("word f_prelude__mul_byte(byte a, byte b) { return (word)a * (word)b; }\n\n")

	finalBuf.WriteString(c.typedefBuf.String())
	finalBuf.WriteString("\n")
	finalBuf.WriteString(c.buf.String())

	return finalBuf.String()
}

func (c *CBE) emitFuncSignature(f *ir.Function, isForward bool) {
	// If this function has a linkage override and no body, it's an external
	// C symbol (e.g. putchar from <stdio.h>). Don't emit a conflicting
	// forward declaration — the system headers already declare it.
	if isForward && f.Linkage != "" && len(f.Blocks) == 0 {
		return
	}

	retType := "void"
	if !f.ReturnType.Equals(ir.TypeVoid) {
		retType = c.mapType(f.ReturnType.Name)
	}

	var params []string
	for _, p := range f.Parameters {
		params = append(params, fmt.Sprintf("%s v_%s", c.mapType(p.Typ.Name), p.Name))
	}

	c.buf.WriteString(fmt.Sprintf("%s %s(%s)", retType, f.EmitName(), strings.Join(params, ", ")))
	if isForward {
		c.buf.WriteString(";\n")
	} else {
		c.buf.WriteString(" {\n")
	}
}

func (c *CBE) emitFunc(f *ir.Function) {
	c.f = f
	c.emitFuncSignature(f, false)

	// Declare all local variables (values) at the top of the function
	for _, b := range f.Blocks {
		for _, instr := range b.Instructions {
			if !instr.Type().Equals(ir.TypeVoid) && !instr.Type().Equals(ir.TypeUnknown) {
				id := instr.GetID()
				if _, ok := instr.(*ir.SetJmp); ok {
					c.buf.WriteString(fmt.Sprintf("\tstruct jmp_struct jumper_%d;\n", instr.GetID()))
				}
				if c.f != nil && c.f.SlotAlias != nil {
					if _, ok := c.f.SlotAlias[id]; ok {
						continue // Alias, already declared by target
					}
				}
				c.buf.WriteString(fmt.Sprintf("\t%s v%d;\n", c.mapType(instr.Type().Name), id))
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

			c.buf.WriteString("\t// " + instr.String() + "\n")

			if ins, ok := instr.(*ir.InsertElement); ok {
				c.buf.WriteString(fmt.Sprintf("\tv%d = %s;\n", c.getSlot(ins.GetID()), c.formatVal(ins.Array)))
				c.buf.WriteString(fmt.Sprintf("\tv%d.data[%s] = %s;\n", c.getSlot(ins.GetID()), c.formatVal(ins.Index), c.formatVal(ins.Val)))
				continue
			}

			if ins, ok := instr.(*ir.InsertField); ok {
				c.buf.WriteString(fmt.Sprintf("\tv%d = %s;\n", c.getSlot(ins.GetID()), c.formatVal(ins.Struct)))
				c.buf.WriteString(fmt.Sprintf("\tv%d.f%d = %s;\n", c.getSlot(ins.GetID()), ins.FieldIndex, c.formatVal(ins.Val)))
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
			if !instr.Type().Equals(ir.TypeVoid) && !instr.Type().Equals(ir.TypeUnknown) {
				c.buf.WriteString(fmt.Sprintf("v%d = ", c.getSlot(instr.GetID())))
			}
			c.buf.WriteString(c.emitInstrExpr(instr) + ";\n")
		}

		if b.Terminator != nil {
			c.buf.WriteString("\t// " + b.Terminator.String() + "\n")
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
					c.buf.WriteString(fmt.Sprintf("%sv%d = %s;\n", indent, c.getSlot(phi.GetID()), c.formatVal(edge.Value)))
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
	case *ir.ConstByte:
		return fmt.Sprintf("%d", val.Val)
	case *ir.ConstWord:
		return fmt.Sprintf("%dULL", val.Val)
	case *ir.ConstStruct:
		var fields []string
		for _, f := range val.Fields {
			fields = append(fields, c.formatVal(f))
		}
		return fmt.Sprintf("{ %s }", strings.Join(fields, ", "))
	case *ir.ConstArray:
		var elems []string
		for _, el := range val.Elements {
			elems = append(elems, c.formatVal(el))
		}
		return fmt.Sprintf("{ %s }", strings.Join(elems, ", "))
	case *ir.AddressOfGlobal:
		gName := strings.ReplaceAll(val.Global.Name, ".", "_")
		return fmt.Sprintf("(&v_%s)", gName)
	case *ir.AddressOfFunc:
		return fmt.Sprintf("((word)(&%s))", val.Func.EmitName())
	case ir.Instruction:
		return fmt.Sprintf("v%d", c.getSlot(val.(interface{ GetID() int }).GetID()))
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
		return fmt.Sprintf("%dULL", i.Val)
	case *ir.Sizeof:
		return fmt.Sprintf("sizeof(%s)", c.mapType(i.TargetTyp.Name))
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
		case "andnot":
			opStr = "& ~"
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
		tName := c.mapType(i.Left.Type().Name)
		return fmt.Sprintf("(byte)((%s)(%s) %s (%s)(%s))", tName, c.formatVal(i.Left), opStr, tName, c.formatVal(i.Right))
	case *ir.Return:
		if i.Val != nil {
			valStr := c.formatVal(i.Val)
			return fmt.Sprintf("v%d = %s", i.GetID(), valStr)
		}
		return ""
	case *ir.SetJmp:
		id := i.GetID()
		return fmt.Sprintf("(jumper_%d.prev = (struct jmp_struct*)v_prelude__jmp_chain_, v_prelude__jmp_chain_ = (byte*)(&jumper_%d), setjmp(jumper_%d.jmpbuf))", id, id, id)
	case *ir.LongJmp:
		return fmt.Sprintf("longjmp(((struct jmp_struct*)%s)->jmpbuf, 1)", c.formatVal(i.JmpBuf))
	case *ir.UnaryOp:
		if i.Op == "not" {
			return fmt.Sprintf("(~%s)", c.formatVal(i.Operand))
		}
		return fmt.Sprintf("(-%s)", c.formatVal(i.Operand))
	case *ir.Call:
		if i.Func.Name == "prelude.mul_word" {
			return fmt.Sprintf("(%s * %s)", c.formatVal(i.Args[0]), c.formatVal(i.Args[1]))
		}
		if i.Func.Name == "prelude.div_word" {
			return fmt.Sprintf("(%s / %s)", c.formatVal(i.Args[0]), c.formatVal(i.Args[1]))
		}
		if i.Func.Name == "prelude.mod_word" {
			return fmt.Sprintf("(%s %% %s)", c.formatVal(i.Args[0]), c.formatVal(i.Args[1]))
		}
		var args []string
		for idx, arg := range i.Args {
			argStr := c.formatVal(arg)
			if idx < len(i.Func.Parameters) {
				expectedTyp := i.Func.Parameters[idx].Typ.Name
				argTyp := arg.Type().Name
				if strings.HasPrefix(expectedTyp, "*") && !strings.HasPrefix(argTyp, "*") {
					argStr = fmt.Sprintf("(%s)(%s)", c.mapType(expectedTyp), argStr)
				}
			}
			args = append(args, argStr)
		}
		return fmt.Sprintf("%s(%s)", i.Func.EmitName(), strings.Join(args, ", "))
	case *ir.IndirectCall:
		var args []string
		var argTypes []string
		for _, arg := range i.Args {
			args = append(args, c.formatVal(arg))
			argTypes = append(argTypes, c.mapType(arg.Type().Name))
		}
		retType := c.mapType(i.Type().Name)
		castType := fmt.Sprintf("%s (*)(%s)", retType, strings.Join(argTypes, ", "))
		return fmt.Sprintf("((%s)(%s))(%s)", castType, c.formatVal(i.FuncPtr), strings.Join(args, ", "))
	case *ir.BuiltinCall:
		if i.Name == "print" || i.Name == "println" {
			return c.emitPrint(i.Name == "println", i.Args)
		} else if i.Name == "panic" {
			return c.emitPanic(i)
		} else if i.Name == "_unlink_jmp_" {
			return "(v_prelude__jmp_chain_ ? (v_prelude__jmp_chain_ = (byte*)(((struct jmp_struct*)v_prelude__jmp_chain_)->prev), 0) : 0)"
		} else if i.Name == "_propagate_panic_" {
			return "(v_prelude__panic_ ? (v_prelude__jmp_chain_ ? (longjmp(((struct jmp_struct*)v_prelude__jmp_chain_)->jmpbuf, 1), 0) : (printf(\"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"), abort(), 0)) : 0)"
		} else if i.Name == "exit" {
			return fmt.Sprintf("exit((int)%s)", c.formatVal(i.Args[0]))
		}
		return fmt.Sprintf("/* builtin %s */", i.Name)
	case *ir.Cast:
		switch i.Op {
		case "trunc":
			return fmt.Sprintf("(byte)(%s)", c.formatVal(i.Operand))
		case "zero_ext":
			return fmt.Sprintf("(word)(%s)", c.formatVal(i.Operand))
		case "word_to_ptr":
			return fmt.Sprintf("(%s)(%s)", c.mapType(i.Typ.Name), c.formatVal(i.Operand))
		case "ptr_to_word":
			operandStr := c.formatVal(i.Operand)
			// If the operand is a pointer to an array struct (e.g. *[N]byte →
			// t_arr_N_byte*), we want the address of element 0, not the struct.
			operandTyp := i.Operand.Type()
			if operandTyp.IsAPointer() && operandTyp.PointedType().IsAnArray() {
				// operandStr is a ptr expression like (&(ctx->f2)); use -> to deref.
				return fmt.Sprintf("(word)(&(%s)->data[0])", operandStr)
			}
			if operandTyp.IsAnArray() {
				// operandStr is a value expression; use & + .data[0]
				return fmt.Sprintf("(word)(&(%s).data[0])", operandStr)
			}
			return fmt.Sprintf("(word)(%s)", operandStr)
		case "bitcast":
			return fmt.Sprintf("(%s)(%s)", c.mapType(i.Typ.Name), c.formatVal(i.Operand))
		default:
			log.Panicf("bad case: %v", i.Op)
		}
	case *ir.ZeroInit:
		return fmt.Sprintf("(%s){0}", c.mapType(i.Typ.Name))
	case *ir.ExtractElement:
		return fmt.Sprintf("(%s).data[%s]", c.formatVal(i.Array), c.formatVal(i.Index))
	case *ir.ExtractField:
		return fmt.Sprintf("(%s).f%d", c.formatVal(i.Struct), i.FieldIndex)
	case *ir.AddressOfGlobal:
		gName := strings.ReplaceAll(i.Global.Name, ".", "_")
		return fmt.Sprintf("(&v_%s)", gName)
	case *ir.ConstStruct:
		var fields []string
		for _, f := range i.Fields {
			fields = append(fields, c.formatVal(f))
		}
		return fmt.Sprintf("{ %s }", strings.Join(fields, ", "))
	case *ir.ConstArray:
		var elems []string
		for _, el := range i.Elements {
			elems = append(elems, c.formatVal(el))
		}
		return fmt.Sprintf("{ %s }", strings.Join(elems, ", "))
	case *ir.AddressOfLocal:
		if p, ok := i.Local.(*ir.Parameter); ok {
			return fmt.Sprintf("(&v_%s)", p.Name)
		} else if inst, ok := i.Local.(ir.Instruction); ok {
			return fmt.Sprintf("(&v%d)", c.getSlot(inst.GetID()))
		} else {
			return fmt.Sprintf("(&%s)", c.formatVal(i.Local))
		}
	case *ir.AddressOfFunc:
		return fmt.Sprintf("((word)(&%s))", i.Func.EmitName())
	case *ir.AddressOfField:
		return fmt.Sprintf("(&(%s->f%d))", c.formatVal(i.Ptr), i.FieldIndex)
	case *ir.AddressOfElement:
		return fmt.Sprintf("(&(%s->data[%s]))", c.formatVal(i.ArrayPtr), c.formatVal(i.Index))
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
			formatStrs = append(formatStrs, "%s")
			argStrs = append(argStrs, fmt.Sprintf("(char*)(%q)", strLit.Value))
		} else if arg.Type().Equals(ir.TypeInt) {
			formatStrs = append(formatStrs, "%lld")
			argStrs = append(argStrs, fmt.Sprintf("(long long)%s", c.formatVal(arg)))
		} else if arg.Type().Name == "prelude.slice_byte" || arg.Type().Name == "slice_byte" {
			formatStrs = append(formatStrs, "%s")
			argStrs = append(argStrs, fmt.Sprintf("(char*)(%s.f0)", c.formatVal(arg)))
		} else if arg.Type().Name == "*byte" {
			formatStrs = append(formatStrs, "%s")
			argStrs = append(argStrs, fmt.Sprintf("(char*)(%s)", c.formatVal(arg)))
		} else {
			formatStrs = append(formatStrs, "%llu")
			argStrs = append(argStrs, fmt.Sprintf("(unsigned long long)%s", c.formatVal(arg)))
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\n"
	}

	if len(argStrs) > 0 {
		return fmt.Sprintf("printf(%q, %s)", format, strings.Join(argStrs, ", "))
	}
	return fmt.Sprintf("printf(%q)", format)
}

func (c *CBE) emitPanic(i *ir.BuiltinCall) string {
	msg := ""
	if len(i.Args) > 0 {
		if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
			msg = fmt.Sprintf("%q", strLit.Value)
		} else {
			msg = c.formatVal(i.Args[0])
		}
	} else {
		msg = "\"<nil>\""
	}
	return fmt.Sprintf("(printf(\"\\n*PANIC* %%s\\n\", (char*)(%s)), v_prelude__panic_ = (byte*)(%s), (v_prelude__jmp_chain_ ? (longjmp(((struct jmp_struct*)v_prelude__jmp_chain_)->jmpbuf, 1), 0) : (printf(\"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"), abort(), 0)))", msg, msg)
}
