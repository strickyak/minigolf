package x86_64

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

func alignVal(val, align int) int {
	return (val + align - 1) & ^(align - 1)
}

func (b *Backend) getTypeAlignment(typ string) int {
	if typ == "byte" {
		return 1
	}
	if typ == "word" || typ == "int" || typ == "uint" || typ == "const_integer" {
		return 8
	}
	if (ir.Type{Name: typ}).IsAnArray() {
		idx := strings.Index(typ, "]")
		if idx != -1 {
			return b.getTypeAlignment(typ[idx+1:])
		}
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ]; ok {
			typ = def.Name
		}
	}
	if (ir.Type{Name: typ}).IsAStruct() {
		content := typ[7 : len(typ)-1]
		maxAlign := 1
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				align := b.getTypeAlignment(content[start:i])
				if align > maxAlign {
					maxAlign = align
				}
				start = i + 1
			}
		}
		return maxAlign
	}
	return 8
}

func (b *Backend) getTypeSize(typ string) int {
	if typ == "byte" {
		return 1
	}
	if typ == "word" || typ == "int" || typ == "uint" || typ == "const_integer" {
		return 8
	}
	if (ir.Type{Name: typ}).IsAnArray() {
		idx := strings.Index(typ, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(typ[1:idx])
			eltSize := b.getTypeSize(typ[idx+1:])
			return length * eltSize
		}
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ]; ok {
			typ = def.Name
		}
	}
	if (ir.Type{Name: typ}).IsAStruct() {
		content := typ[7 : len(typ)-1]
		size := 0
		maxAlign := 1
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				fTyp := content[start:i]
				fSize := b.getTypeSize(fTyp)
				fAlign := b.getTypeAlignment(fTyp)
				size = alignVal(size, fAlign)
				size += fSize
				if fAlign > maxAlign {
					maxAlign = fAlign
				}
				start = i + 1
			}
		}
		return alignVal(size, maxAlign)
	}
	return 8
}

func (b *Backend) getEltSize(arrType string) int {
	if strings.HasPrefix(arrType, "*") {
		arrType = arrType[1:]
	}
	if strings.HasPrefix(arrType, "[") {
		idx := strings.Index(arrType, "]")
		if idx != -1 {
			return b.getTypeSize(arrType[idx+1:])
		}
	}
	// Fallback, should not happen for valid IR
	return 8
}

func (b *Backend) getFieldOffsetAndSize(structName string, fieldIndex int) (int, int) {
	content := ""
	if def, ok := b.program.TypeDefs[structName]; ok {
		content = def.Name[7 : len(def.Name)-1]
	} else if (ir.Type{Name: structName}).IsAStruct() {
		content = structName[7 : len(structName)-1]
	} else {
		return 0, 8
	}

	byteOffset := 0
	depth := 0
	start := 0
	fIdx := 0
	for idx := 0; idx < len(content); idx++ {
		if content[idx] == '{' {
			depth++
		} else if content[idx] == '}' {
			depth--
		} else if content[idx] == ';' && depth == 0 {
			fTyp := content[start:idx]
			sz := b.getTypeSize(fTyp)
			align := b.getTypeAlignment(fTyp)
			byteOffset = alignVal(byteOffset, align)

			if fIdx == fieldIndex {
				return byteOffset, sz
			}
			byteOffset += sz
			fIdx++
			start = idx + 1
		}
	}
	return 0, 8
}

type Backend struct {
	program     *ir.Program
	buf         bytes.Buffer
	dataBuf     bytes.Buffer
	stackOffset int
	retPtrSlot  int
	slots       map[int]int
	paramSlots  map[string]int
	fmtCount    int
}

func New() *Backend {
	return &Backend{
		slots:      make(map[int]int),
		paramSlots: make(map[string]int),
	}
}

func (b *Backend) Generate(program *ir.Program) string {
	b.program = program
	b.buf.WriteString(".intel_syntax noprefix\n")
	b.buf.WriteString(".text\n")

	b.buf.WriteString("\t.globl f_prelude.shl_word\n")
	b.buf.WriteString("f_prelude.shl_word:\n")
	b.buf.WriteString("\tmov rax, rdi\n")
	b.buf.WriteString("\tmov rcx, rsi\n")
	b.buf.WriteString("\tshl rax, cl\n")
	b.buf.WriteString("\tret\n")

	b.buf.WriteString("\t.globl f_prelude.shr_word\n")
	b.buf.WriteString("f_prelude.shr_word:\n")
	b.buf.WriteString("\tmov rax, rdi\n")
	b.buf.WriteString("\tmov rcx, rsi\n")
	b.buf.WriteString("\tshr rax, cl\n")
	b.buf.WriteString("\tret\n")

	b.buf.WriteString("\t.globl f_prelude.mul_byte\n")
	b.buf.WriteString("f_prelude.mul_byte:\n")
	b.buf.WriteString("\tmovzx rax, dil\n")
	b.buf.WriteString("\tmovzx rcx, sil\n")
	b.buf.WriteString("\timul rax, rcx\n")
	b.buf.WriteString("\tret\n")

	if len(program.Globals) > 0 {
		b.dataBuf.WriteString(".data\n")
		for _, g := range program.Globals {
			b.dataBuf.WriteString(fmt.Sprintf("\t.globl v_%s\n", g.Name))
			b.dataBuf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
			if g.IsInit {
				if g.InitVal != nil {
					b.emitData(g.InitVal)
				} else {
					b.dataBuf.WriteString(fmt.Sprintf("\t.ascii %q\n", g.InitString))
				}
			} else {
				size := b.getTypeSize(g.Typ.Name)
				b.dataBuf.WriteString(fmt.Sprintf("\t.zero %d\n", size))
			}
		}
	}

	for _, f := range program.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
		}
	}

	b.buf.WriteString("\n\t.globl main\n")
	b.buf.WriteString("\t.globl _main\n")
	b.buf.WriteString("main:\n")
	b.buf.WriteString("_main:\n")
	b.buf.WriteString("\tpush rbp\n")
	b.buf.WriteString("\tmov rbp, rsp\n")
	b.buf.WriteString("\tand rsp, -16\n")
	b.buf.WriteString("\tcall f_main\n")

	// TODO -- fix this, to call fflush(stdout), where stdout is `extern FILE* stdout;`
	// b.buf.WriteString("\tlea eax, [rip + stdout]\n")
	// b.buf.WriteString("\tcall fflush@PLT\n")

	b.buf.WriteString("\txor rax, rax\n")
	b.buf.WriteString("\tmov rsp, rbp\n")
	b.buf.WriteString("\tpop rbp\n")
	b.buf.WriteString("\tret\n")

	b.buf.WriteString("\n# GNU/Linux stack compliance\n")
	b.buf.WriteString(".section .note.GNU-stack,\"\",@progbits\n")

	return b.dataBuf.String() + "\n" + b.buf.String()
}

func (b *Backend) getSlot(id int, typ string) int {
	if offset, ok := b.slots[id]; ok {
		return offset
	}
	size := b.getTypeSize(typ)
	aligned := (size + 7) &^ 7
	if aligned < 8 {
		aligned = 8
	}
	b.stackOffset += aligned
	b.slots[id] = b.stackOffset
	return b.stackOffset
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.buf.WriteString(fmt.Sprintf("\n\t.globl f_%s\n", f.Name))
	b.buf.WriteString(fmt.Sprintf("f_%s:\n", f.Name))
	b.buf.WriteString("\tpush rbp\n")
	b.buf.WriteString("\tmov rbp, rsp\n")

	b.stackOffset = 0
	b.slots = make(map[int]int)
	b.paramSlots = make(map[string]int)

	regs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
	regsIdx := 0

	retSize := b.getTypeSize(f.ReturnType.Name)
	b.retPtrSlot = 0
	if retSize > 16 {
		b.stackOffset += 8
		b.retPtrSlot = b.stackOffset
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], %s\n", b.stackOffset, regs[regsIdx]))
		regsIdx++
	}

	for _, p := range f.Parameters {
		size := b.getTypeSize(p.Typ.Name)
		aligned := (size + 7) &^ 7
		if aligned < 8 {
			aligned = 8
		}
		b.stackOffset += aligned
		b.paramSlots[p.Name] = b.stackOffset

		words := aligned / 8
		for w := 0; w < words; w++ {
			byteOffset := w * 8
			if regsIdx < 6 {
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d + %d], %s\n", b.stackOffset, byteOffset, regs[regsIdx]))
			} else {
				stackArgOffset := 16 + (regsIdx-6)*8
				b.buf.WriteString(fmt.Sprintf("\tmov r10, qword ptr [rbp + %d]\n", stackArgOffset))
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d + %d], r10\n", b.stackOffset, byteOffset))
			}
			regsIdx++
		}
	}

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if !instr.Type().Equals(ir.TypeVoid) && !instr.Type().Equals(ir.TypeUnknown) {
				b.getSlot(instr.GetID(), instr.Type().Name)
			}
		}
	}

	stackSize := (b.stackOffset + 15) &^ 15
	if stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tsub rsp, %d\n", stackSize))
	}

	for _, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", f.Name, blk.ID))

		for _, instr := range blk.Instructions {
			if _, isPhi := instr.(*ir.Phi); isPhi {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			b.emitInstr(instr)
		}

		switch term := blk.Terminator.(type) {
		case *ir.Jump:
			b.emitPhiAssignments(blk, term.Target)
			b.buf.WriteString(fmt.Sprintf("\tjmp .L_%s_b%d\n", f.Name, term.Target.ID))
		case *ir.Branch:
			b.loadVal(term.Condition, "rax")
			b.buf.WriteString("\ttest rax, rax\n")
			b.buf.WriteString(fmt.Sprintf("\tjnz .L_%s_b%d_true\n", f.Name, blk.ID))
			b.buf.WriteString(fmt.Sprintf("\tjmp .L_%s_b%d_false\n", f.Name, blk.ID))

			b.buf.WriteString(fmt.Sprintf(".L_%s_b%d_true:\n", f.Name, blk.ID))
			b.emitPhiAssignments(blk, term.TrueBlock)
			b.buf.WriteString(fmt.Sprintf("\tjmp .L_%s_b%d\n", f.Name, term.TrueBlock.ID))

			b.buf.WriteString(fmt.Sprintf(".L_%s_b%d_false:\n", f.Name, blk.ID))
			b.emitPhiAssignments(blk, term.FalseBlock)
			b.buf.WriteString(fmt.Sprintf("\tjmp .L_%s_b%d\n", f.Name, term.FalseBlock.ID))

		case *ir.Return:
			if term.Val != nil {
				size := b.getTypeSize(term.Val.Type().Name)
				if size > 16 {
					addr := b.getAddr(term.Val)
					b.buf.WriteString(fmt.Sprintf("\tmov rax, qword ptr [rbp - %d]\n", b.retPtrSlot))
					for i := 0; i < size; i += 8 {
						b.buf.WriteString(fmt.Sprintf("\tmov rcx, qword ptr [%s + %d]\n", addr, i))
						b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rax + %d], rcx\n", i))
					}
				} else {
					b.loadVal(term.Val, "rax")
					if size > 8 {
						addr := b.getAddr(term.Val)
						b.buf.WriteString(fmt.Sprintf("\tmov rdx, qword ptr [%s + 8]\n", addr))
					}
				}
			}
			b.buf.WriteString("\tmov rsp, rbp\n")
			b.buf.WriteString("\tpop rbp\n")
			b.buf.WriteString("\tret\n")
		default:
			log.Panicf("bad case: %T / %v", term, term)
		}
	}
}

func (b *Backend) loadVal(val ir.Value, reg string) {
	switch v := val.(type) {
	case *ir.Parameter:
		b.buf.WriteString(fmt.Sprintf("\tmov %s, qword ptr [rbp - %d]\n", reg, b.paramSlots[v.Name]))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tmov %s, %d\n", reg, v.Val))
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tmov %s, %d\n", reg, v.Val))
	case ir.Instruction:
		b.buf.WriteString(fmt.Sprintf("\tmov %s, qword ptr [rbp - %d]\n", reg, b.slots[v.GetID()]))
	default:
		log.Panicf("bad case: %T / %v", v, v)
	}
}

func (b *Backend) getAddr(val ir.Value) string {
	switch v := val.(type) {
	case *ir.Parameter:
		return fmt.Sprintf("rbp - %d", b.paramSlots[v.Name])
	case ir.Instruction:
		return fmt.Sprintf("rbp - %d", b.slots[v.GetID()])
	case *ir.Global:
		return fmt.Sprintf("rip + v_%s", v.Name)
	default:
		log.Panicf("bad case: %T / %v", v, v)
	}
	return ""
}

func (b *Backend) emitMemCopy(destAddr, srcAddr string, size int) {
	if size == 1 {
		b.buf.WriteString(fmt.Sprintf("\tmov al, byte ptr [%s]\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmov byte ptr [%s], al\n", destAddr))
	} else if size == 2 {
		b.buf.WriteString(fmt.Sprintf("\tmov ax, word ptr [%s]\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmov word ptr [%s], ax\n", destAddr))
	} else if size == 4 {
		b.buf.WriteString(fmt.Sprintf("\tmov eax, dword ptr [%s]\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmov dword ptr [%s], eax\n", destAddr))
	} else if size == 8 {
		b.buf.WriteString(fmt.Sprintf("\tmov rax, qword ptr [%s]\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [%s], rax\n", destAddr))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tlea rdi, [%s]\n", destAddr))
		b.buf.WriteString(fmt.Sprintf("\tlea rsi, [%s]\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmov rcx, %d\n", size))
		b.buf.WriteString("\trep movsb\n")
	}
}

func (b *Backend) storeToAddr(destAddr string, val ir.Value, size int) {
	srcAddr := b.getAddr(val)
	if srcAddr == "" {
		b.loadVal(val, "rax")
		if size == 1 {
			b.buf.WriteString(fmt.Sprintf("\tmov byte ptr [%s], al\n", destAddr))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [%s], rax\n", destAddr))
		}
	} else {
		b.emitMemCopy(destAddr, srcAddr, size)
	}
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					size := b.getTypeSize(phi.Typ.Name)
					if size <= 8 {
						b.loadVal(edge.Value, "rax")
						if phi.Type().Equals(ir.TypeByte) {
							b.buf.WriteString("\tmovzx rax, al\n")
						}
						b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", b.slots[phi.GetID()]))
					} else {
						b.emitMemCopy(fmt.Sprintf("rbp - %d", b.slots[phi.GetID()]), b.getAddr(edge.Value), size)
					}
				}
			}
		}
	}
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()
	offset := b.slots[id]

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t# %s\n", i.Comment))
	case *ir.ConstByte, *ir.ConstWord:
		b.loadVal(i, "rax")
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.Sizeof:
		size := b.getTypeSize(i.TargetTyp.Name)
		b.buf.WriteString(fmt.Sprintf("\tmov rax, %d\n", size))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.Load:
		size := b.getTypeSize(i.Global.Typ.Name)
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), fmt.Sprintf("rip + v_%s", i.Global.Name), size)
	case *ir.Store:
		size := b.getTypeSize(i.Global.Typ.Name)
		b.storeToAddr(fmt.Sprintf("rip + v_%s", i.Global.Name), i.Val, size)
	case *ir.ZeroInit:
		size := b.getTypeSize(i.Typ.Name)
		b.buf.WriteString(fmt.Sprintf("\tlea rdi, [rbp - %d]\n", offset))
		b.buf.WriteString("\txor al, al\n")
		b.buf.WriteString(fmt.Sprintf("\tmov rcx, %d\n", size))
		b.buf.WriteString("\trep stosb\n")
	case *ir.ExtractElement:
		eltSize := b.getTypeSize(i.Typ.Name)
		arrayAddr := b.getAddr(i.Array)
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.buf.WriteString(fmt.Sprintf("\tlea rcx, [%s]\n", arrayAddr))
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
			b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", eltSize)
		} else {
			b.loadVal(i.Index, "rax")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\timul rax, %d\n", eltSize))
			}
			b.buf.WriteString(fmt.Sprintf("\tlea rcx, [%s]\n", arrayAddr))
			b.buf.WriteString("\tadd rcx, rax\n")
			b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", eltSize)
		}
	case *ir.InsertElement:
		arraySize := b.getTypeSize(i.Array.Type().Name)
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), b.getAddr(i.Array), arraySize)

		eltSize := b.getEltSize(i.Array.Type().Name)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.buf.WriteString(fmt.Sprintf("\tlea rcx, [rbp - %d]\n", offset))
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
			b.storeToAddr("rcx", i.Val, eltSize)
		} else {
			b.loadVal(i.Index, "rax")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\timul rax, %d\n", eltSize))
			}
			b.buf.WriteString(fmt.Sprintf("\tlea rcx, [rbp - %d]\n", offset))
			b.buf.WriteString("\tadd rcx, rax\n")
			b.storeToAddr("rcx", i.Val, eltSize)
		}
	case *ir.ExtractField:
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type().Name, i.FieldIndex)
		structAddr := b.getAddr(i.Struct)
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		b.buf.WriteString(fmt.Sprintf("\tlea rcx, [%s]\n", structAddr))
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", fieldSize)
	case *ir.InsertField:
		structSize := b.getTypeSize(i.Struct.Type().Name)
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), b.getAddr(i.Struct), structSize)

		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type().Name, i.FieldIndex)
		b.buf.WriteString(fmt.Sprintf("\tlea rcx, [rbp - %d]\n", offset))
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.storeToAddr("rcx", i.Val, fieldSize)
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tlea rax, [rip + v_%s]\n", i.Global.Name))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tlea rax, [rip + f_%s]\n", i.Func.Name))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.AddressOfLocal:
		var localOffset int
		if p, ok := i.Local.(*ir.Parameter); ok {
			localOffset = b.paramSlots[p.Name]
		} else {
			localInstr := i.Local.(ir.Instruction)
			localOffset = b.slots[localInstr.GetID()]
		}
		b.buf.WriteString(fmt.Sprintf("\tlea rax, [rbp - %d]\n", localOffset))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.AddressOfField:
		structName := strings.TrimPrefix(i.Ptr.Type().Name, "*")
		byteOffset, _ := b.getFieldOffsetAndSize(structName, i.FieldIndex)
		b.loadVal(i.Ptr, "rax")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rax, %d\n", byteOffset))
		}
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.AddressOfElement:
		b.loadVal(i.ArrayPtr, "rax")
		eltSize := b.getEltSize(i.ArrayPtr.Type().Name)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tadd rax, %d\n", byteOffset))
			}
		} else {
			b.loadVal(i.Index, "rcx")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\timul rcx, %d\n", eltSize))
			}
			b.buf.WriteString("\tadd rax, rcx\n")
		}
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.ExtractFieldPtr:
		structName := strings.TrimPrefix(i.Ptr.Type().Name, "*")
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structName, i.FieldIndex)
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		b.loadVal(i.Ptr, "rcx")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", fieldSize)
	case *ir.InsertFieldPtr:
		structName := strings.TrimPrefix(i.Ptr.Type().Name, "*")
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structName, i.FieldIndex)
		b.loadVal(i.Ptr, "rcx")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.storeToAddr("rcx", i.Val, fieldSize)
	case *ir.LoadPtr:
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		b.loadVal(i.Ptr, "rcx")
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", b.getTypeSize(i.Typ.Name))
	case *ir.StorePtr:
		ptrType := i.Ptr.Type().Name
		pointeeType := "word"
		if (ir.Type{Name: ptrType}).IsAPointer() {
			pointeeType = ptrType[1:]
		}
		b.loadVal(i.Ptr, "rcx")
		b.storeToAddr("rcx", i.Val, b.getTypeSize(pointeeType))
	case *ir.BinaryOp:
		b.loadVal(i.Left, "rax")
		b.loadVal(i.Right, "rcx")
		switch i.Op {
		case "add":
			b.buf.WriteString("\tadd rax, rcx\n")
		case "sub":
			b.buf.WriteString("\tsub rax, rcx\n")
		case "mul":
			b.buf.WriteString("\timul rax, rcx\n")
		case "div", "mod":
			if i.Typ.Equals(ir.TypeInt) {
				b.buf.WriteString("\tcqo\n\tidiv rcx\n")
			} else {
				b.buf.WriteString("\txor rdx, rdx\n\tdiv rcx\n")
			}
			if i.Op == "mod" {
				b.buf.WriteString("\tmov rax, rdx\n")
			}
		case "and":
			b.buf.WriteString("\tand rax, rcx\n")
		case "or":
			b.buf.WriteString("\tor rax, rcx\n")
		case "xor":
			b.buf.WriteString("\txor rax, rcx\n")
		case "andnot":
			b.buf.WriteString("\tnot rcx\n\tand rax, rcx\n")
		case "shl":
			b.buf.WriteString("\tshl rax, cl\n")
		case "shr":
			if i.Typ.Equals(ir.TypeInt) {
				b.buf.WriteString("\tsar rax, cl\n")
			} else {
				b.buf.WriteString("\tshr rax, cl\n")
			}
		default:
			log.Panicf("bad case: %v", i.Op)
		}
		if i.Typ.Equals(ir.TypeByte) {
			b.buf.WriteString("\tmovzx rax, al\n")
		}
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.Compare:
		b.loadVal(i.Left, "rax")
		b.loadVal(i.Right, "rcx")
		b.buf.WriteString("\tcmp rax, rcx\n")
		isInt := i.Left.Type().Equals(ir.TypeInt)
		switch i.Op {
		case "eq":
			b.buf.WriteString("\tsete al\n")
		case "neq":
			b.buf.WriteString("\tsetne al\n")
		case "lt":
			if isInt {
				b.buf.WriteString("\tsetl al\n")
			} else {
				b.buf.WriteString("\tsetb al\n")
			}
		case "lte":
			if isInt {
				b.buf.WriteString("\tsetle al\n")
			} else {
				b.buf.WriteString("\tsetbe al\n")
			}
		case "gt":
			if isInt {
				b.buf.WriteString("\tsetg al\n")
			} else {
				b.buf.WriteString("\tseta al\n")
			}
		case "gte":
			if isInt {
				b.buf.WriteString("\tsetge al\n")
			} else {
				b.buf.WriteString("\tsetae al\n")
			}
		default:
			log.Panicf("bad case: %v", i.Op)
		}
		b.buf.WriteString("\tmovzx rax, al\n")
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.Call:
		regs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		retSize := 0
		if !i.Typ.Equals(ir.TypeVoid) {
			retSize = b.getTypeSize(i.Typ.Name)
		}

		totalWords := 0
		if retSize > 16 {
			totalWords++
		}
		for _, arg := range i.Args {
			size := b.getTypeSize(arg.Type().Name)
			totalWords += (size + 7) / 8
		}

		extraWords := 0
		if totalWords > 6 {
			extraWords = totalWords - 6
		}
		paddingWords := extraWords % 2
		stackSub := (extraWords + paddingWords) * 8
		if stackSub > 0 {
			b.buf.WriteString(fmt.Sprintf("\tsub rsp, %d\n", stackSub))
		}

		wordIdx := 0

		if retSize > 16 {
			if wordIdx < 6 {
				b.buf.WriteString(fmt.Sprintf("\tlea %s, [rbp - %d]\n", regs[wordIdx], offset))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tlea rax, [rbp - %d]\n", offset))
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rsp + %d], rax\n", (wordIdx-6)*8))
			}
			wordIdx++
		}

		for _, arg := range i.Args {
			size := b.getTypeSize(arg.Type().Name)
			words := (size + 7) / 8
			addr := b.getAddr(arg)
			for w := 0; w < words; w++ {
				if w == 0 {
					b.loadVal(arg, "r10")
				} else {
					if addr != "" {
						b.buf.WriteString(fmt.Sprintf("\tmov r10, qword ptr [%s + %d]\n", addr, w*8))
					} else {
						b.buf.WriteString("\tmov r10, 0\n")
					}
				}

				if wordIdx < 6 {
					b.buf.WriteString(fmt.Sprintf("\tmov %s, r10\n", regs[wordIdx]))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rsp + %d], r10\n", (wordIdx-6)*8))
				}
				wordIdx++
			}
		}

		b.buf.WriteString(fmt.Sprintf("\tcall f_%s\n", i.Func.Name))

		if stackSub > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rsp, %d\n", stackSub))
		}

		if i.Typ.Equals(ir.TypeByte) {
			b.buf.WriteString("\tmovzx rax, al\n")
		}
		if !i.Typ.Equals(ir.TypeVoid) {
			if retSize <= 16 {
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
				if retSize > 8 {
					b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d + 8], rdx\n", offset))
				}
			}
		}
	case *ir.IndirectCall:
		regs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		retSize := 0
		if !i.Typ.Equals(ir.TypeVoid) {
			retSize = b.getTypeSize(i.Typ.Name)
		}

		totalWords := 0
		if retSize > 16 {
			totalWords++
		}
		for _, arg := range i.Args {
			size := b.getTypeSize(arg.Type().Name)
			totalWords += (size + 7) / 8
		}

		extraWords := 0
		if totalWords > 6 {
			extraWords = totalWords - 6
		}
		paddingWords := extraWords % 2
		stackSub := (extraWords + paddingWords) * 8
		if stackSub > 0 {
			b.buf.WriteString(fmt.Sprintf("\tsub rsp, %d\n", stackSub))
		}

		wordIdx := 0

		if retSize > 16 {
			if wordIdx < 6 {
				b.buf.WriteString(fmt.Sprintf("\tlea %s, [rbp - %d]\n", regs[wordIdx], offset))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tlea rax, [rbp - %d]\n", offset))
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rsp + %d], rax\n", (wordIdx-6)*8))
			}
			wordIdx++
		}

		for _, arg := range i.Args {
			size := b.getTypeSize(arg.Type().Name)
			words := (size + 7) / 8
			addr := b.getAddr(arg)
			for w := 0; w < words; w++ {
				if w == 0 {
					b.loadVal(arg, "r10")
				} else {
					if addr != "" {
						b.buf.WriteString(fmt.Sprintf("\tmov r10, qword ptr [%s + %d]\n", addr, w*8))
					} else {
						b.buf.WriteString("\tmov r10, 0\n")
					}
				}

				if wordIdx < 6 {
					b.buf.WriteString(fmt.Sprintf("\tmov %s, r10\n", regs[wordIdx]))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rsp + %d], r10\n", (wordIdx-6)*8))
				}
				wordIdx++
			}
		}

		b.loadVal(i.FuncPtr, "r11")
		b.buf.WriteString("\tcall r11\n")

		if stackSub > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rsp, %d\n", stackSub))
		}

		if i.Typ.Equals(ir.TypeByte) {
			b.buf.WriteString("\tmovzx rax, al\n")
		}
		if !i.Typ.Equals(ir.TypeVoid) {
			if retSize <= 16 {
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
				if retSize > 8 {
					b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d + 8], rdx\n", offset))
				}
			}
		}
	case *ir.BuiltinCall:
		if i.Name == "print" || i.Name == "println" {
			b.emitPrint(i.Name == "println", i.Args)
		} else if i.Name == "exit" {
			b.buf.WriteString("\tud2\n")
		}
	case *ir.Cast:
		b.loadVal(i.Operand, "rax")
		if i.Op == "trunc" {
			b.buf.WriteString("\tmovzx rax, al\n")
		}
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	}
}

func (b *Backend) emitPrint(newline bool, args []ir.Value) {
	b.fmtCount++
	fmtLabel := fmt.Sprintf(".Lfmt%d", b.fmtCount)

	formatStrs := []string{}
	var dataArgs []ir.Value

	for _, arg := range args {
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, strLit)
		} else if arg.Type().Equals(ir.TypeInt) {
			formatStrs = append(formatStrs, "%lld")
			dataArgs = append(dataArgs, arg)
		} else if arg.Type().Name == "prelude.slice_byte" || arg.Type().Name == "slice_byte" {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, arg)
		} else {
			formatStrs = append(formatStrs, "%llu")
			dataArgs = append(dataArgs, arg)
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\n"
	}

	if b.dataBuf.Len() == 0 {
		b.dataBuf.WriteString(".data\n")
	}
	b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.string %q\n", fmtLabel, format))

	b.buf.WriteString(fmt.Sprintf("\tlea rdi, [rip + %s]\n", fmtLabel))

	regs := []string{"rsi", "rdx", "rcx", "r8", "r9"}
	stackArgs := 0

	for i := len(dataArgs) - 1; i >= len(regs); i-- {
		arg := dataArgs[i]
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			b.fmtCount++
			lbl := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.string %q\n", lbl, strLit.Value))
			b.buf.WriteString(fmt.Sprintf("\tlea rax, [rip + %s]\n", lbl))
			b.buf.WriteString("\tpush rax\n")
		} else if arg.Type().Name == "prelude.slice_byte" || arg.Type().Name == "slice_byte" {
			addr := b.getAddr(arg)
			if addr != "" {
				b.buf.WriteString(fmt.Sprintf("\tmov rax, qword ptr [%s]\n", addr))
				b.buf.WriteString("\tpush rax\n")
			} else {
				b.loadVal(arg, "rax")
				b.buf.WriteString("\tpush rax\n")
			}
		} else {
			b.loadVal(arg, "rax")
			b.buf.WriteString("\tpush rax\n")
		}
		stackArgs++
	}

	for idx, arg := range dataArgs {
		if idx >= len(regs) {
			break
		}
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			b.fmtCount++
			lbl := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.string %q\n", lbl, strLit.Value))
			b.buf.WriteString(fmt.Sprintf("\tlea %s, [rip + %s]\n", regs[idx], lbl))
		} else if arg.Type().Name == "prelude.slice_byte" || arg.Type().Name == "slice_byte" {
			addr := b.getAddr(arg)
			if addr != "" {
				b.buf.WriteString(fmt.Sprintf("\tmov %s, qword ptr [%s]\n", regs[idx], addr))
			} else {
				b.loadVal(arg, regs[idx])
			}
		} else {
			b.loadVal(arg, regs[idx])
		}
	}

	b.buf.WriteString("\txor eax, eax\n")
	b.buf.WriteString("\tcall printf@PLT\n")
	if stackArgs > 0 {
		b.buf.WriteString(fmt.Sprintf("\tadd rsp, %d\n", stackArgs*8))
	}
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.dataBuf.WriteString(fmt.Sprintf("\t.byte %d\n", v.Val))
	case *ir.ConstWord:
		b.dataBuf.WriteString(fmt.Sprintf("\t.quad %d\n", v.Val))
	case *ir.AddressOfGlobal:
		b.dataBuf.WriteString(fmt.Sprintf("\t.quad v_%s\n", v.Global.Name))
	case *ir.ConstStruct:
		structName := v.Type().Name
		if def, ok := b.program.TypeDefs[structName]; ok {
			structName = def.Name
		}
		content := ""
		if strings.HasPrefix(structName, "struct{") {
			content = structName[7 : len(structName)-1]
		}

		byteOffset := 0
		depth := 0
		start := 0
		fIdx := 0

		for idx := 0; idx < len(content); idx++ {
			if content[idx] == '{' {
				depth++
			} else if content[idx] == '}' {
				depth--
			} else if content[idx] == ';' && depth == 0 {
				fTyp := content[start:idx]
				sz := b.getTypeSize(fTyp)
				align := b.getTypeAlignment(fTyp)

				paddedOffset := alignVal(byteOffset, align)
				if paddedOffset > byteOffset {
					b.dataBuf.WriteString(fmt.Sprintf("\t.zero %d\n", paddedOffset-byteOffset))
				}
				byteOffset = paddedOffset

				b.emitData(v.Fields[fIdx])

				byteOffset += sz
				fIdx++
				start = idx + 1
			}
		}

		structSize := b.getTypeSize(v.Type().Name)
		if structSize > byteOffset {
			b.dataBuf.WriteString(fmt.Sprintf("\t.zero %d\n", structSize-byteOffset))
		}
	default:
		log.Panicf("unsupported init value type %T", val)
	}
}
