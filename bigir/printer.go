package bigir

import (
	"bytes"
	"fmt"
	"strings"
)

// PrintProgram formats a BIGIR Program into human-readable text.
func PrintProgram(prog *Program) string {
	var buf bytes.Buffer

	buf.WriteString("; ==========================================\n")
	buf.WriteString("; BIGIR Program (EMBIGGEN Process Mode)\n")
	buf.WriteString("; ==========================================\n\n")

	// Print Globals
	if len(prog.Globals) > 0 {
		buf.WriteString("; --- Globals ---\n")
		for _, g := range prog.Globals {
			location := "Fixed RAM (Slot 0)"
			if g.IsFar {
				location = "Far Data (Blocks 128..255)"
			}
			buf.WriteString(fmt.Sprintf("%s : %s [%s]", g.Name, g.Typ.String(), location))
			if g.InitString != "" {
				buf.WriteString(fmt.Sprintf(" = %q", g.InitString))
			}
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}

	// Print Trampolines
	if len(prog.Trampolines) > 0 {
		buf.WriteString("; --- Slot 6 Trampolines ---\n")
		for _, tramp := range prog.Trampolines {
			buf.WriteString(fmt.Sprintf("trampoline %s -> Block %d : 0x%04X\n",
				tramp.FuncName, tramp.TargetBlock, tramp.TargetAddr))
		}
		buf.WriteString("\n")
	}

	// Print Functions grouped by Block
	for _, fn := range prog.Functions {
		printFunction(&buf, fn)
		buf.WriteString("\n")
	}

	return buf.String()
}

func printFunction(buf *bytes.Buffer, fn *Function) {
	kind := "near"
	if fn.IsFar {
		kind = fmt.Sprintf("far(Block %d, Offset 0x%04X)", fn.BlockID, fn.Slot5Offset)
	}

	buf.WriteString(fmt.Sprintf("func %s(", fn.Name))
	for i, p := range fn.Parameters {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("%s %s", p.Name, p.Typ.String()))
	}
	buf.WriteString(fmt.Sprintf(") %s [%s] {\n", fn.ReturnType.String(), kind))

	for _, bb := range fn.Blocks {
		buf.WriteString(fmt.Sprintf("  %s:\n", bb.String()))
		for _, instr := range bb.Instructions {
			buf.WriteString(fmt.Sprintf("    %s = %s\n", instr.String(), formatInstruction(instr)))
		}
		if bb.Terminator != nil {
			buf.WriteString(fmt.Sprintf("    %s\n", formatInstruction(bb.Terminator)))
		}
	}

	buf.WriteString("}\n")
}

func formatInstruction(i Instruction) string {
	switch instr := i.(type) {
	case *ConstByte:
		return fmt.Sprintf("const_byte %d", instr.Val)
	case *ConstWord:
		return fmt.Sprintf("const_word %d", instr.Val)
	case *ConstFarRef:
		return fmt.Sprintf("const_far_ref Block=%d Chunk=%d", instr.BlockID, instr.ChunkIdx)
	case *ConstString:
		return fmt.Sprintf("const_string %q", instr.Val)
	case *BinaryOp:
		return fmt.Sprintf("%s %s, %s", instr.Op, instr.Left.String(), instr.Right.String())
	case *UnaryOp:
		return fmt.Sprintf("%s %s", instr.Op, instr.Operand.String())
	case *Compare:
		return fmt.Sprintf("cmp_%s %s, %s", instr.Op, instr.Left.String(), instr.Right.String())
	case *Phi:
		var edges []string
		for _, e := range instr.Edges {
			edges = append(edges, fmt.Sprintf("[%s from %s]", e.Value.String(), e.Block.String()))
		}
		return fmt.Sprintf("phi %s", strings.Join(edges, ", "))
	case *NearLoad:
		return fmt.Sprintf("near_load [%s]", instr.Addr.String())
	case *NearStore:
		return fmt.Sprintf("near_store [%s], %s", instr.Addr.String(), instr.Val.String())
	case *FarLoad:
		return fmt.Sprintf("far_load FarRef=%s, Offset=%s", instr.FarRef.String(), instr.Offset.String())
	case *FarStore:
		return fmt.Sprintf("far_store FarRef=%s, Offset=%s, Val=%s", instr.FarRef.String(), instr.Offset.String(), instr.Val.String())
	case *AddressOfGlobal:
		return fmt.Sprintf("addrof @%s", instr.Global.Name)
	case *AddressOfLocal:
		return fmt.Sprintf("addrof_local %s", instr.Local.String())
	case *SliceMake:
		return fmt.Sprintf("slice_make FarRef=%s, Offset=%s, Len=%s, Cap=%s",
			instr.FarRef.String(), instr.Offset.String(), instr.Length.String(), instr.Capacity.String())
	case *SliceGet:
		return fmt.Sprintf("slice_get %s[%s]", instr.Slice.String(), instr.Index.String())
	case *SlicePut:
		return fmt.Sprintf("slice_put %s[%s] = %s", instr.Slice.String(), instr.Index.String(), instr.Val.String())
	case *SliceChop:
		return fmt.Sprintf("slice_chop %s[%s:%s]", instr.Slice.String(), instr.Start.String(), instr.Limit.String())
	case *SliceField:
		fields := []string{"far_ref", "offset", "length", "capacity"}
		fName := fmt.Sprintf("field_%d", instr.FieldIdx)
		if instr.FieldIdx >= 0 && instr.FieldIdx < len(fields) {
			fName = fields[instr.FieldIdx]
		}
		return fmt.Sprintf("slice_field %s.%s", instr.Slice.String(), fName)
	case *FuncRef:
		return fmt.Sprintf("&%s", instr.FuncName)
	case *NearCall:
		return fmt.Sprintf("near_call %s(%v)", instr.Callee, formatValues(instr.Args))
	case *FarCall:
		return fmt.Sprintf("far_call %s[Block %d : 0x%04X](%v)",
			instr.Callee, instr.TargetBlock, instr.TargetAddr, formatValues(instr.Args))
	case *IndirectCall:
		return fmt.Sprintf("indirect_call %s(%v)", instr.FuncPtr.String(), formatValues(instr.Args))
	case *Return:
		if instr.Val != nil {
			return fmt.Sprintf("return %s", instr.Val.String())
		}
		return "return"
	case *FarReturn:
		if instr.Val != nil {
			return fmt.Sprintf("far_return %s", instr.Val.String())
		}
		return "far_return"
	case *Branch:
		return fmt.Sprintf("br %s", instr.Target.String())
	case *CondBranch:
		return fmt.Sprintf("cbr %s ? %s : %s", instr.Cond.String(), instr.TrueTarget.String(), instr.FalseTarget.String())
	case *Panic:
		return fmt.Sprintf("panic %q", instr.Message)
	default:
		return i.Opcode()
	}
}

func formatValues(vals []Value) string {
	var s string
	for i, v := range vals {
		if i > 0 {
			s += ", "
		}
		s += v.String()
	}
	return s
}
