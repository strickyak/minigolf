package x86_64

import (
	"bytes"
	"fmt"
	"minigo/ir"
	"strconv"
	"strings"
)

func (b *Backend) getTypeSize(typ string) int {
	if typ == "byte" {
		return 1
	}
	if typ == "word" {
		return 8
	}
	if strings.HasPrefix(typ, "[") {
		idx := strings.Index(typ, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(typ[1:idx])
			eltSize := b.getTypeSize(typ[idx+1:])
			return length * eltSize
		}
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ]; ok {
			typ = def
		}
	}
	if strings.HasPrefix(typ, "struct{") {
		content := typ[7 : len(typ)-1]
		size := 0
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				size += b.getTypeSize(content[start:i])
				start = i + 1
			}
		}
		return size
	}
	return 8
}

func (b *Backend) getEltSize(arrType string) int {
	if strings.HasPrefix(arrType, "[") {
		idx := strings.Index(arrType, "]")
		if idx != -1 {
			return b.getTypeSize(arrType[idx+1:])
		}
	}
	return 8
}

func (b *Backend) getFieldOffsetAndSize(structName string, fieldIndex int) (int, int) {
	content := ""
	if def, ok := b.program.TypeDefs[structName]; ok {
		content = def[7 : len(def)-1]
	} else if strings.HasPrefix(structName, "struct{") {
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
			if fIdx < fieldIndex {
				byteOffset += sz
			} else if fIdx == fieldIndex {
				return byteOffset, sz
			}
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

	if len(program.Globals) > 0 {
		b.dataBuf.WriteString(".data\n")
		for _, g := range program.Globals {
			b.dataBuf.WriteString(fmt.Sprintf("\t.globl v_%s\n", g.Name))
			b.dataBuf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
			size := b.getTypeSize(string(g.Typ))
			b.dataBuf.WriteString(fmt.Sprintf("\t.zero %d\n", size))
		}
	}

	for _, f := range program.Functions {
		b.emitFunc(f)
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
	for _, p := range f.Parameters {
		size := b.getTypeSize(string(p.Typ))
		aligned := (size + 7) &^ 7
		if aligned < 8 {
			aligned = 8
		}
		b.stackOffset += aligned
		b.paramSlots[p.Name] = b.stackOffset
		if regsIdx < len(regs) {
			b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], %s\n", b.stackOffset, regs[regsIdx]))
			regsIdx++
			if size > 8 && regsIdx < len(regs) {
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d + 8], %s\n", b.stackOffset, regs[regsIdx]))
				regsIdx++
			}
		}
	}

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if instr.Type() != ir.TypeVoid && instr.Type() != ir.TypeUnknown {
				b.getSlot(instr.GetID(), string(instr.Type()))
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
				size := b.getTypeSize(string(term.Val.Type()))
				if size > 16 {
					panic("Unsupported: large value return")
				}
				b.loadVal(term.Val, "rax")
				if size > 8 {
					addr := b.getAddr(term.Val)
					b.buf.WriteString(fmt.Sprintf("\tmov rdx, qword ptr [%s + 8]\n", addr))
				}
			}
			b.buf.WriteString("\tmov rsp, rbp\n")
			b.buf.WriteString("\tpop rbp\n")
			b.buf.WriteString("\tret\n")
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
	}
	return ""
}

func (b *Backend) emitMemCopy(destAddr, srcAddr string, size int) {
	if size == 1 {
		b.buf.WriteString(fmt.Sprintf("\tmov al, byte ptr [%s]\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmov byte ptr [%s], al\n", destAddr))
	} else if size <= 8 {
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
					size := b.getTypeSize(string(phi.Typ))
					if size <= 8 {
						b.loadVal(edge.Value, "rax")
						if phi.Type() == ir.TypeByte {
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
	case *ir.Load:
		size := b.getTypeSize(string(i.Global.Typ))
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), fmt.Sprintf("rip + v_%s", i.Global.Name), size)
	case *ir.Store:
		size := b.getTypeSize(string(i.Global.Typ))
		b.storeToAddr(fmt.Sprintf("rip + v_%s", i.Global.Name), i.Val, size)
	case *ir.ZeroInit:
		size := b.getTypeSize(string(i.Typ))
		b.buf.WriteString(fmt.Sprintf("\tlea rdi, [rbp - %d]\n", offset))
		b.buf.WriteString("\txor al, al\n")
		b.buf.WriteString(fmt.Sprintf("\tmov rcx, %d\n", size))
		b.buf.WriteString("\trep stosb\n")
	case *ir.ExtractElement:
		eltSize := b.getTypeSize(string(i.Typ))
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
		arraySize := b.getTypeSize(string(i.Array.Type()))
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), b.getAddr(i.Array), arraySize)

		eltSize := b.getEltSize(string(i.Array.Type()))
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
		byteOffset, fieldSize := b.getFieldOffsetAndSize(string(i.Struct.Type()), i.FieldIndex)
		structAddr := b.getAddr(i.Struct)
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		b.buf.WriteString(fmt.Sprintf("\tlea rcx, [%s]\n", structAddr))
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", fieldSize)
	case *ir.InsertField:
		structSize := b.getTypeSize(string(i.Struct.Type()))
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), b.getAddr(i.Struct), structSize)

		byteOffset, fieldSize := b.getFieldOffsetAndSize(string(i.Struct.Type()), i.FieldIndex)
		b.buf.WriteString(fmt.Sprintf("\tlea rcx, [rbp - %d]\n", offset))
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.storeToAddr("rcx", i.Val, fieldSize)
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tlea rax, [rip + v_%s]\n", i.Global.Name))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.AddressOfLocal:
		localInstr := i.Local.(ir.Instruction)
		localOffset := b.slots[localInstr.GetID()]
		b.buf.WriteString(fmt.Sprintf("\tlea rax, [rbp - %d]\n", localOffset))
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.ExtractFieldPtr:
		structName := strings.TrimPrefix(string(i.Ptr.Type()), "*")
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structName, i.FieldIndex)
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		b.loadVal(i.Ptr, "rcx")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", fieldSize)
	case *ir.InsertFieldPtr:
		structName := strings.TrimPrefix(string(i.Ptr.Type()), "*")
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structName, i.FieldIndex)
		b.loadVal(i.Ptr, "rcx")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd rcx, %d\n", byteOffset))
		}
		b.storeToAddr("rcx", i.Val, fieldSize)
	case *ir.LoadPtr:
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], 0\n", offset))
		b.loadVal(i.Ptr, "rcx")
		b.emitMemCopy(fmt.Sprintf("rbp - %d", offset), "rcx", b.getTypeSize(string(i.Typ)))
	case *ir.StorePtr:
		ptrType := string(i.Ptr.Type())
		pointeeType := "word"
		if strings.HasPrefix(ptrType, "*") {
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
		case "div":
			b.buf.WriteString("\txor rdx, rdx\n\tdiv rcx\n")
		case "mod":
			b.buf.WriteString("\txor rdx, rdx\n\tdiv rcx\n\tmov rax, rdx\n")
		case "and":
			b.buf.WriteString("\tand rax, rcx\n")
		case "or":
			b.buf.WriteString("\tor rax, rcx\n")
		case "xor":
			b.buf.WriteString("\txor rax, rcx\n")
		case "shl":
			b.buf.WriteString("\tshl rax, cl\n")
		case "shr":
			b.buf.WriteString("\tshr rax, cl\n")
		}
		if i.Typ == ir.TypeByte {
			b.buf.WriteString("\tmovzx rax, al\n")
		}
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.Compare:
		b.loadVal(i.Left, "rax")
		b.loadVal(i.Right, "rcx")
		b.buf.WriteString("\tcmp rax, rcx\n")
		switch i.Op {
		case "eq":
			b.buf.WriteString("\tsete al\n")
		case "neq":
			b.buf.WriteString("\tsetne al\n")
		case "lt":
			b.buf.WriteString("\tsetb al\n")
		case "lte":
			b.buf.WriteString("\tsetbe al\n")
		case "gt":
			b.buf.WriteString("\tseta al\n")
		case "gte":
			b.buf.WriteString("\tsetae al\n")
		}
		b.buf.WriteString("\tmovzx rax, al\n")
		b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
	case *ir.Call:
		regs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		regsIdx := 0
		for _, arg := range i.Args {
			size := b.getTypeSize(string(arg.Type()))
			if regsIdx < len(regs) {
				b.loadVal(arg, regs[regsIdx])
				regsIdx++
				if size > 8 && regsIdx < len(regs) {
					addr := b.getAddr(arg)
					b.buf.WriteString(fmt.Sprintf("\tmov %s, qword ptr [%s + 8]\n", regs[regsIdx], addr))
					regsIdx++
				}
			}
		}
		b.buf.WriteString(fmt.Sprintf("\tcall f_%s\n", i.Func.Name))
		if i.Typ == ir.TypeByte {
			b.buf.WriteString("\tmovzx rax, al\n")
		}
		if i.Typ != ir.TypeVoid {
			size := b.getTypeSize(string(i.Typ))
			if size > 16 {
				panic("Unsupported: large value return")
			}
			b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d], rax\n", offset))
			if size > 8 {
				b.buf.WriteString(fmt.Sprintf("\tmov qword ptr [rbp - %d + 8], rdx\n", offset))
			}
		}
	case *ir.BuiltinCall:
		if i.Name == "print" || i.Name == "println" {
			b.emitPrint(i.Name == "println", i.Args)
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
			formatStrs = append(formatStrs, strLit.Value)
		} else {
			formatStrs = append(formatStrs, "%llu")
			dataArgs = append(dataArgs, arg)
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\\n"
	}

	if b.dataBuf.Len() == 0 {
		b.dataBuf.WriteString(".data\n")
	}
	b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.string \"%s\"\n", fmtLabel, format))

	b.buf.WriteString(fmt.Sprintf("\tlea rdi, [rip + %s]\n", fmtLabel))

	regs := []string{"rsi", "rdx", "rcx", "r8", "r9"}
	for idx, arg := range dataArgs {
		if idx < len(regs) {
			b.loadVal(arg, regs[idx])
		}
	}

	b.buf.WriteString("\txor eax, eax\n")
	b.buf.WriteString("\tcall printf@PLT\n")
}
