package m68k

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

func alignVal(val, align int) int {
	return (val + align - 1) & ^(align - 1)
}

type rodataEntry struct {
	lbl string
	str string
}

type Backend struct {
	buf        bytes.Buffer
	program    *ir.Program
	slots      map[int]int    // instruction ID -> stack offset below A6 (positive offset, accessed as -offset(A6))
	jmpSlots   map[int]int    // SetJmp instruction ID -> jmpbuf stack offset below A6
	paramSlots map[string]int // param name -> stack offset above A6 (positive offset, accessed as offset(A6))
	typeMap    map[int]ir.Type
	stackSize  int
	retPtrSlot int
	lblCount   int
	rodata     []rodataEntry
}

func New() *Backend {
	return &Backend{
		slots:      make(map[int]int),
		jmpSlots:   make(map[int]int),
		paramSlots: make(map[string]int),
		typeMap:    make(map[int]ir.Type),
	}
}

func (b *Backend) nextLabel() string {
	b.lblCount++
	return fmt.Sprintf(".LL%d", b.lblCount)
}

func (b *Backend) getTypeAlignment(typ ir.Type) int {
	switch typ.Name {
	case "byte", "bool":
		return 1
	case "word", "int", "uint", "const_integer", "noreturn":
		return 2 // 68000 requires 2-byte alignment for words and longwords
	}
	if typ.IsByte() || typ.IsBool() {
		return 1
	}
	if typ.IsWord() || typ.IsInt() || typ.IsConstInt() || typ.IsNoReturn() {
		return 2
	}
	if typ.IsAnArray() {
		return b.getTypeAlignment(typ.ArrayElementType())
	}
	if typ.IsAPointer() || typ.IsAFuncPtr() {
		return 2
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ.Name]; ok {
			return b.getTypeAlignment(def)
		}
	}
	if typ.IsAStruct() {
		maxAlign := 1
		for _, f := range typ.FieldsOfStruct() {
			align := b.getTypeAlignment(f.Type)
			if align > maxAlign {
				maxAlign = align
			}
		}
		return maxAlign
	}
	return 2
}

func (b *Backend) getTypeSize(typ ir.Type) int {
	switch typ.Name {
	case "byte", "bool":
		return 1
	case "word", "int", "uint", "const_integer", "noreturn":
		return 4 // 32-bit longword on M68K
	}
	if typ.IsByte() || typ.IsBool() {
		return 1
	}
	if typ.IsWord() || typ.IsInt() || typ.IsConstInt() || typ.IsNoReturn() {
		return 4
	}
	if typ.IsAnArray() {
		return typ.ArrayLength() * b.getTypeSize(typ.ArrayElementType())
	}
	if typ.IsAPointer() || typ.IsAFuncPtr() {
		return 4
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ.Name]; ok {
			return b.getTypeSize(def)
		}
	}
	if typ.IsAStruct() {
		size := 0
		maxAlign := 1
		for _, f := range typ.FieldsOfStruct() {
			fSize := b.getTypeSize(f.Type)
			fAlign := b.getTypeAlignment(f.Type)
			size = alignVal(size, fAlign)
			size += fSize
			if fAlign > maxAlign {
				maxAlign = fAlign
			}
		}
		return alignVal(size, maxAlign)
	}
	return 4
}

func (b *Backend) getEltSize(arrType ir.Type) int {
	if arrType.IsAPointer() {
		arrType = arrType.PointedType()
	}
	if arrType.IsAnArray() {
		return b.getTypeSize(arrType.ArrayElementType())
	}
	return 4
}

func (b *Backend) getFieldOffsetAndSize(structType ir.Type, fieldIndex int) (int, int) {
	if structType.IsAPointer() {
		structType = structType.PointedType()
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[structType.Name]; ok {
			structType = def
		}
	}
	fields := structType.FieldsOfStruct()
	offset := 0
	for i := 0; i < fieldIndex; i++ {
		fAlign := b.getTypeAlignment(fields[i].Type)
		offset = alignVal(offset, fAlign)
		offset += b.getTypeSize(fields[i].Type)
	}
	targetAlign := b.getTypeAlignment(fields[fieldIndex].Type)
	offset = alignVal(offset, targetAlign)
	return offset, b.getTypeSize(fields[fieldIndex].Type)
}

func (b *Backend) allocateSlots(f *ir.Function) {
	b.slots = make(map[int]int)
	b.jmpSlots = make(map[int]int)
	b.paramSlots = make(map[string]int)
	b.typeMap = make(map[int]ir.Type)

	// In M68K standard calling convention:
	// Return address is at 4(A6).
	// Caller's pushed arguments start at 8(A6).
	paramOffset := 8
	for _, p := range f.Parameters {
		sz := b.getTypeSize(p.Typ)
		if p.Typ.IsAnArray() || p.Typ.IsAStruct() {
			b.paramSlots[p.Name] = paramOffset
		} else if sz == 1 {
			b.paramSlots[p.Name] = paramOffset + 3
		} else if sz == 2 {
			b.paramSlots[p.Name] = paramOffset + 2
		} else {
			b.paramSlots[p.Name] = paramOffset
		}
		// Arguments on the stack are at least 4-byte aligned
		paramOffset += alignVal(sz, 4)
	}

	currentOffset := 0
	retSize := b.getTypeSize(f.ReturnType)
	if retSize > 4 {
		currentOffset += 4
		b.retPtrSlot = currentOffset
	}

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			b.typeMap[instr.GetID()] = instr.Type()
			size := b.getTypeSize(instr.Type())
			currentOffset = alignVal(currentOffset, 4)
			currentOffset += alignVal(size, 4)
			b.slots[instr.GetID()] = currentOffset

			if _, ok := instr.(*ir.SetJmp); ok {
				currentOffset = alignVal(currentOffset, 4)
				currentOffset += 64
				b.jmpSlots[instr.GetID()] = currentOffset
			}
		}
	}
	// Align total local stack frame to 4 bytes for stack safety
	b.stackSize = alignVal(currentOffset, 4)
}

func (b *Backend) localAddr(id int) string {
	return fmt.Sprintf("-%d(a6)", b.slots[id])
}

func (b *Backend) paramAddr(name string) string {
	return fmt.Sprintf("%d(a6)", b.paramSlots[name])
}

func (b *Backend) getAddr(val ir.Value) string {
	switch v := val.(type) {
	case *ir.Parameter:
		return b.paramAddr(v.Name)
	case ir.Instruction:
		return b.localAddr(v.GetID())
	case *ir.Global:
		return fmt.Sprintf("v_%s", v.Name)
	default:
		log.Panicf("getAddr: unhandled value type %T", val)
	}
	return ""
}

func (b *Backend) loadVal(val ir.Value, reg string) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, %s\n", v.Val, reg))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, %s\n", v.Val, reg))
	case *ir.Parameter:
		sz := b.getTypeSize(v.Typ)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tmoveq   #0, %s\n", reg))
			b.buf.WriteString(fmt.Sprintf("\tmove.b  %s, %s\n", b.paramAddr(v.Name), reg))
		} else if sz == 2 {
			b.buf.WriteString(fmt.Sprintf("\tmoveq   #0, %s\n", reg))
			b.buf.WriteString(fmt.Sprintf("\tmove.w  %s, %s\n", b.paramAddr(v.Name), reg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tmove.l  %s, %s\n", b.paramAddr(v.Name), reg))
		}
	case ir.Instruction:
		sz := b.getTypeSize(v.Type())
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tmoveq   #0, %s\n", reg))
			b.buf.WriteString(fmt.Sprintf("\tmove.b  %s, %s\n", b.localAddr(v.GetID()), reg))
		} else if sz == 2 {
			b.buf.WriteString(fmt.Sprintf("\tmoveq   #0, %s\n", reg))
			b.buf.WriteString(fmt.Sprintf("\tmove.w  %s, %s\n", b.localAddr(v.GetID()), reg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tmove.l  %s, %s\n", b.localAddr(v.GetID()), reg))
		}
	default:
		log.Panicf("loadVal: unhandled value type %T", val)
	}
}

func (b *Backend) emitLoadAddr(reg string, addrStr string) {
	if addrStr == fmt.Sprintf("(%s)", reg) {
		return
	}
	if strings.HasPrefix(addrStr, "(") && strings.HasSuffix(addrStr, ")") {
		inner := addrStr[1 : len(addrStr)-1]
		if inner == "a0" || inner == "a1" || inner == "sp" || inner == "a6" {
			b.buf.WriteString(fmt.Sprintf("\tmove.l  %s, %s\n", inner, reg))
			return
		}
	}
	b.buf.WriteString(fmt.Sprintf("\tlea     %s, %s\n", addrStr, reg))
}

func (b *Backend) emitMemCopy(destAddr, srcAddr string, size int) {
	if size <= 0 {
		return
	}
	if size == 1 {
		b.buf.WriteString(fmt.Sprintf("\tmove.b  %s, d0\n", srcAddr))
		b.buf.WriteString(fmt.Sprintf("\tmove.b  d0, %s\n", destAddr))
		return
	}
	// Multi-byte copy using (a0)+ and (a1)+
	// A0 must hold source address, A1 must hold destination address.
	if destAddr == "(a0)" {
		b.buf.WriteString("\tmove.l  a0, a1\n")
		b.emitLoadAddr("a0", srcAddr)
	} else if srcAddr == "(a0)" {
		b.emitLoadAddr("a1", destAddr)
	} else {
		b.emitLoadAddr("a0", srcAddr)
		b.emitLoadAddr("a1", destAddr)
	}
	if size <= 16 {
		for i := 0; i < size; i++ {
			b.buf.WriteString("\tmove.b  (a0)+, (a1)+\n")
		}
	} else {
		lblLoop := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tmove.w  #%d, d0\n", size-1))
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblLoop))
		b.buf.WriteString("\tmove.b  (a0)+, (a1)+\n")
		b.buf.WriteString(fmt.Sprintf("\tdbra    d0, %s\n", lblLoop))
	}
}

func (b *Backend) storeToAddr(destAddr string, val ir.Value, size int) {
	if (val.Type().IsAnArray() || val.Type().IsAStruct()) && size > 1 {
		srcAddr := b.getAddr(val)
		b.emitMemCopy(destAddr, srcAddr, size)
		return
	}
	if size == 1 {
		b.loadVal(val, "d0")
		b.buf.WriteString(fmt.Sprintf("\tmove.b  d0, %s\n", destAddr))
	} else if size == 2 {
		b.loadVal(val, "d0")
		b.buf.WriteString(fmt.Sprintf("\tmove.w  d0, %s\n", destAddr))
	} else if size == 4 {
		b.loadVal(val, "d0")
		b.buf.WriteString(fmt.Sprintf("\tmove.l  d0, %s\n", destAddr))
	} else {
		srcAddr := b.getAddr(val)
		b.emitMemCopy(destAddr, srcAddr, size)
	}
}

func (b *Backend) storeResult(id int) {
	sz := b.getTypeSize(b.slotsType(id))
	if sz == 1 {
		b.buf.WriteString(fmt.Sprintf("\tmove.b  d0, %s\n", b.localAddr(id)))
	} else if sz == 2 {
		b.buf.WriteString(fmt.Sprintf("\tmove.w  d0, %s\n", b.localAddr(id)))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tmove.l  d0, %s\n", b.localAddr(id)))
	}
}

func (b *Backend) slotsType(id int) ir.Type {
	if t, ok := b.typeMap[id]; ok {
		return t
	}
	return ir.TypeInt
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					size := b.getTypeSize(phi.Typ)
					if size <= 4 {
						b.loadVal(edge.Value, "d0")
						if size == 1 {
							b.buf.WriteString(fmt.Sprintf("\tmove.b  d0, %s\n", b.localAddr(phi.GetID())))
						} else if size == 2 {
							b.buf.WriteString(fmt.Sprintf("\tmove.w  d0, %s\n", b.localAddr(phi.GetID())))
						} else {
							b.buf.WriteString(fmt.Sprintf("\tmove.l  d0, %s\n", b.localAddr(phi.GetID())))
						}
					} else {
						b.emitMemCopy(b.localAddr(phi.GetID()), b.getAddr(edge.Value), size)
					}
				}
			}
		}
	}
}

func (b *Backend) emitBinaryOp(i *ir.BinaryOp) {
	b.loadVal(i.Left, "d0")
	b.loadVal(i.Right, "d1")

	switch i.Op {
	case "add":
		b.buf.WriteString("\tadd.l   d1, d0\n")
	case "sub":
		b.buf.WriteString("\tsub.l   d1, d0\n")
	case "mul":
		b.buf.WriteString("\tjsr     __mul32\n")
	case "div":
		b.buf.WriteString("\tjsr     __div32\n")
	case "mod":
		b.buf.WriteString("\tjsr     __mod32\n")
	case "and", "bitand":
		b.buf.WriteString("\tand.l   d1, d0\n")
	case "or", "bitor":
		b.buf.WriteString("\tor.l    d1, d0\n")
	case "xor", "bitxor":
		b.buf.WriteString("\teor.l   d1, d0\n")
	case "andnot":
		b.buf.WriteString("\tnot.l   d1\n")
		b.buf.WriteString("\tand.l   d1, d0\n")
	case "shl":
		b.buf.WriteString("\tlsl.l   d1, d0\n")
	case "shr":
		if i.Left.Type().IsInt() {
			b.buf.WriteString("\tasr.l   d1, d0\n")
		} else {
			b.buf.WriteString("\tlsr.l   d1, d0\n")
		}
	default:
		log.Panicf("emitBinaryOp: unsupported op %s", i.Op)
	}

	b.storeResult(i.GetID())
}

func (b *Backend) emitCompare(i *ir.Compare) {
	b.loadVal(i.Left, "d0")
	b.loadVal(i.Right, "d1")
	b.buf.WriteString("\tcmp.l   d1, d0\n") // evaluates Left - Right

	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()
	trueLbl := b.nextLabel()
	doneLbl := b.nextLabel()

	switch i.Op {
	case "eq":
		b.buf.WriteString(fmt.Sprintf("\tbeq     %s\n", trueLbl))
	case "neq":
		b.buf.WriteString(fmt.Sprintf("\tbne     %s\n", trueLbl))
	case "lt":
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tbcs     %s\n", trueLbl))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tblt     %s\n", trueLbl))
		}
	case "lte":
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tbls     %s\n", trueLbl))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tble     %s\n", trueLbl))
		}
	case "gt":
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tbhi     %s\n", trueLbl))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tbgt     %s\n", trueLbl))
		}
	case "gte":
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tbcc     %s\n", trueLbl))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tbge     %s\n", trueLbl))
		}
	default:
		log.Panicf("emitCompare: unsupported op %s", i.Op)
	}

	b.buf.WriteString("\tmoveq   #0, d0\n")
	b.buf.WriteString(fmt.Sprintf("\tbra     %s\n", doneLbl))
	b.buf.WriteString(fmt.Sprintf("%s:\n", trueLbl))
	b.buf.WriteString("\tmoveq   #1, d0\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", doneLbl))

	b.storeResult(i.GetID())
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))

	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tmove.b  #%d, %s\n", i.Val, b.localAddr(id)))

	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, %s\n", i.Val, b.localAddr(id)))

	case *ir.Sizeof:
		sz := b.getTypeSize(i.TargetTyp)
		b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, %s\n", sz, b.localAddr(id)))

	case *ir.Load:
		sz := b.getTypeSize(i.Global.Typ)
		b.emitMemCopy(b.localAddr(id), fmt.Sprintf("v_%s", i.Global.Name), sz)

	case *ir.Store:
		sz := b.getTypeSize(i.Global.Typ)
		b.storeToAddr(fmt.Sprintf("v_%s", i.Global.Name), i.Val, sz)

	case *ir.ZeroInit:
		sz := b.getTypeSize(i.Typ)
		b.emitLoadAddr("a0", b.localAddr(id))
		longs := sz / 4
		for j := 0; j < longs; j++ {
			b.buf.WriteString("\tclr.l   (a0)+\n")
		}
		rem := sz % 4
		if rem >= 2 {
			b.buf.WriteString("\tclr.w   (a0)+\n")
			rem -= 2
		}
		if rem == 1 {
			b.buf.WriteString("\tclr.b   (a0)+\n")
		}

	case *ir.BinaryOp:
		b.emitBinaryOp(i)

	case *ir.Compare:
		b.emitCompare(i)

	case *ir.UnaryOp:
		b.loadVal(i.Operand, "d0")
		switch i.Op {
		case "neg":
			b.buf.WriteString("\tneg.l   d0\n")
		case "not", "bitnot":
			b.buf.WriteString("\tnot.l   d0\n")
		default:
			log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
		}
		b.storeResult(id)

	case *ir.Cast:
		b.loadVal(i.Operand, "d0")
		b.storeResult(id)

	case *ir.AddressOfLocal:
		b.emitLoadAddr("a0", b.getAddr(i.Local))
		b.buf.WriteString(fmt.Sprintf("\tmove.l  a0, %s\n", b.localAddr(id)))

	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tlea     v_%s, a0\n", i.Global.Name))
		b.buf.WriteString(fmt.Sprintf("\tmove.l  a0, %s\n", b.localAddr(id)))

	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tlea     %s, a0\n", i.Func.EmitName()))
		b.buf.WriteString(fmt.Sprintf("\tmove.l  a0, %s\n", b.localAddr(id)))

	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		b.loadVal(i.Ptr, "d0")
		b.buf.WriteString("\tmove.l  d0, a0\n")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd.l   #%d, a0\n", byteOffset))
		}
		b.buf.WriteString(fmt.Sprintf("\tmove.l  a0, %s\n", b.localAddr(id)))

	case *ir.AddressOfElement:
		eltSize := b.getEltSize(i.ArrayPtr.Type())
		b.loadVal(i.ArrayPtr, "d0")
		b.buf.WriteString("\tmove.l  d0, a0\n")
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tadd.l   #%d, a0\n", byteOffset))
			}
		} else {
			b.loadVal(i.Index, "d1")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, d0\n", eltSize))
				b.buf.WriteString("\tjsr     __mul32\n") // D0 = D0 * D1
				b.buf.WriteString("\tadd.l   d0, a0\n")
			} else {
				b.buf.WriteString("\tadd.l   d1, a0\n")
			}
		}
		b.buf.WriteString(fmt.Sprintf("\tmove.l  a0, %s\n", b.localAddr(id)))

	case *ir.LoadPtr:
		sz := b.getTypeSize(i.Typ)
		b.loadVal(i.Ptr, "d0")
		b.buf.WriteString("\tmove.l  d0, a0\n")
		if i.Typ.IsAnArray() || i.Typ.IsAStruct() {
			b.emitMemCopy(b.localAddr(id), "(a0)", sz)
		} else if sz == 1 {
			b.buf.WriteString("\tmoveq   #0, d0\n")
			b.buf.WriteString("\tmove.b  (a0), d0\n")
			b.buf.WriteString(fmt.Sprintf("\tmove.b  d0, %s\n", b.localAddr(id)))
		} else if sz == 2 {
			b.buf.WriteString("\tmoveq   #0, d0\n")
			b.buf.WriteString("\tmove.w  (a0), d0\n")
			b.buf.WriteString(fmt.Sprintf("\tmove.w  d0, %s\n", b.localAddr(id)))
		} else if sz == 4 {
			b.buf.WriteString("\tmove.l  (a0), d0\n")
			b.buf.WriteString(fmt.Sprintf("\tmove.l  d0, %s\n", b.localAddr(id)))
		} else {
			b.emitMemCopy(b.localAddr(id), "(a0)", sz)
		}

	case *ir.StorePtr:
		sz := b.getTypeSize(i.Val.Type())
		b.loadVal(i.Ptr, "d0")
		b.buf.WriteString("\tmove.l  d0, a0\n")
		if i.Val.Type().IsAnArray() || i.Val.Type().IsAStruct() {
			srcAddr := b.getAddr(i.Val)
			b.emitMemCopy("(a0)", srcAddr, sz)
		} else if sz == 1 {
			b.loadVal(i.Val, "d0")
			b.buf.WriteString("\tmove.b  d0, (a0)\n")
		} else if sz == 2 {
			b.loadVal(i.Val, "d0")
			b.buf.WriteString("\tmove.w  d0, (a0)\n")
		} else if sz == 4 {
			b.loadVal(i.Val, "d0")
			b.buf.WriteString("\tmove.l  d0, (a0)\n")
		} else {
			srcAddr := b.getAddr(i.Val)
			b.emitMemCopy("(a0)", srcAddr, sz)
		}

	case *ir.ExtractField:
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		structAddr := b.getAddr(i.Struct)
		b.emitLoadAddr("a0", structAddr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd.l   #%d, a0\n", byteOffset))
		}
		b.emitMemCopy(b.localAddr(id), "(a0)", fieldSize)

	case *ir.InsertField:
		structSize := b.getTypeSize(i.Struct.Type())
		b.emitMemCopy(b.localAddr(id), b.getAddr(i.Struct), structSize)
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		b.emitLoadAddr("a0", b.localAddr(id))
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tadd.l   #%d, a0\n", byteOffset))
		}
		b.storeToAddr("(a0)", i.Val, fieldSize)

	case *ir.ExtractElement:
		eltSize := b.getTypeSize(i.Typ)
		arrayAddr := b.getAddr(i.Array)
		b.emitLoadAddr("a0", arrayAddr)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tadd.l   #%d, a0\n", byteOffset))
			}
		} else {
			b.loadVal(i.Index, "d1")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, d0\n", eltSize))
				b.buf.WriteString("\tjsr     __mul32\n")
				b.buf.WriteString("\tadd.l   d0, a0\n")
			} else {
				b.buf.WriteString("\tadd.l   d1, a0\n")
			}
		}
		b.emitMemCopy(b.localAddr(id), "(a0)", eltSize)

	case *ir.InsertElement:
		arraySize := b.getTypeSize(i.Array.Type())
		b.emitMemCopy(b.localAddr(id), b.getAddr(i.Array), arraySize)
		eltSize := b.getEltSize(i.Array.Type())
		b.emitLoadAddr("a0", b.localAddr(id))
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tadd.l   #%d, a0\n", byteOffset))
			}
		} else {
			b.loadVal(i.Index, "d1")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tmove.l  #%d, d0\n", eltSize))
				b.buf.WriteString("\tjsr     __mul32\n")
				b.buf.WriteString("\tadd.l   d0, a0\n")
			} else {
				b.buf.WriteString("\tadd.l   d1, a0\n")
			}
		}
		b.storeToAddr("(a0)", i.Val, eltSize)

	case *ir.Call:
		// Push arguments in reverse order (right-to-left)
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			sz := b.getTypeSize(arg.Type())
			pushSize := alignVal(sz, 4)
			totalArgBytes += pushSize
			if (arg.Type().IsAnArray() || arg.Type().IsAStruct()) || sz > 4 {
				b.buf.WriteString(fmt.Sprintf("\tsub.l   #%d, sp\n", pushSize))
				b.emitMemCopy("(sp)", b.getAddr(arg), sz)
			} else {
				b.loadVal(arg, "d0")
				b.buf.WriteString("\tmove.l  d0, -(sp)\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		if retSize > 4 {
			b.emitLoadAddr("a0", b.localAddr(id))
		}
		b.buf.WriteString(fmt.Sprintf("\tjsr     %s\n", i.Func.EmitName()))
		if totalArgBytes > 0 {
			b.buf.WriteString(fmt.Sprintf("\tlea     %d(sp), sp\n", totalArgBytes))
		}
		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 4 {
			b.storeResult(id)
		}

	case *ir.IndirectCall:
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			sz := b.getTypeSize(arg.Type())
			pushSize := alignVal(sz, 4)
			totalArgBytes += pushSize
			if (arg.Type().IsAnArray() || arg.Type().IsAStruct()) || sz > 4 {
				b.buf.WriteString(fmt.Sprintf("\tsub.l   #%d, sp\n", pushSize))
				b.emitMemCopy("(sp)", b.getAddr(arg), sz)
			} else {
				b.loadVal(arg, "d0")
				b.buf.WriteString("\tmove.l  d0, -(sp)\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		if retSize > 4 {
			b.emitLoadAddr("a0", b.localAddr(id))
		}
		b.loadVal(i.FuncPtr, "d0")
		b.buf.WriteString("\tmove.l  d0, a1\n")
		b.buf.WriteString("\tjsr     (a1)\n")
		if totalArgBytes > 0 {
			b.buf.WriteString(fmt.Sprintf("\tlea     %d(sp), sp\n", totalArgBytes))
		}
		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 4 {
			b.storeResult(id)
		}

	case *ir.BuiltinCall:
		b.emitBuiltinCall(i)

	case *ir.SetJmp:
		b.emitSetJmp(i)
	case *ir.LongJmp:
		b.emitLongJmp(i)

	case *ir.Phi:
		// Phi nodes handled at branch/jump transitions

	default:
		log.Panicf("emitInstr: unhandled instruction %T", instr)
	}
}

func (b *Backend) emitSetJmp(i *ir.SetJmp) {
	id := i.GetID()
	jmpSlot := b.jmpSlots[id]
	lblResume := b.nextLabel()
	lblDone := b.nextLabel()

	b.buf.WriteString(fmt.Sprintf("\tlea     -%d(a6), a0\n", jmpSlot))
	b.buf.WriteString("\tmove.l  v_prelude._jmp_chain_, (a0)\n")
	b.buf.WriteString("\tmove.l  a0, v_prelude._jmp_chain_\n")
	b.buf.WriteString(fmt.Sprintf("\tmove.l  #%s, 4(a0)\n", lblResume))
	b.buf.WriteString("\tmove.l  sp, 8(a0)\n")
	b.buf.WriteString("\tmove.l  a6, 12(a0)\n")
	b.buf.WriteString("\tmoveq   #0, d0\n")
	b.buf.WriteString(fmt.Sprintf("\tbra     %s\n", lblDone))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblResume))
	b.buf.WriteString("\tmoveq   #1, d0\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	b.buf.WriteString(fmt.Sprintf("\tmove.l  d0, %s\n", b.localAddr(id)))
}

func (b *Backend) emitLongJmp(i *ir.LongJmp) {
	b.loadVal(i.JmpBuf, "d0")
	b.buf.WriteString("\tmove.l  d0, a0\n")
	b.buf.WriteString("\tmove.l  12(a0), a6\n")
	b.buf.WriteString("\tmove.l  8(a0), sp\n")
	b.buf.WriteString("\tmove.l  4(a0), a1\n")
	b.buf.WriteString("\tjmp     (a1)\n")
}

func (b *Backend) emitBuiltinCall(i *ir.BuiltinCall) {
	switch i.Name {
	case "print", "println":
		b.emitPrint(i.Name == "println", i.Args)
	case "exit":
		if len(i.Args) > 0 {
			b.loadVal(i.Args[0], "d0")
		} else {
			b.buf.WriteString("\tmoveq   #0, d0\n")
		}
		b.buf.WriteString("\tmove.l  d0, -(sp)\n")
		b.buf.WriteString("\tjsr     _exit\n")
	case "panic", "_panic_":
		if len(i.Args) > 0 {
			if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
				b.lblCount++
				lbl := fmt.Sprintf(".Lpanic%d", b.lblCount)
				b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: strLit.Value})
				b.buf.WriteString(fmt.Sprintf("\tpea     %s\n", lbl))
			} else {
				b.loadVal(i.Args[0], "d0")
				b.buf.WriteString("\tmove.l  d0, -(sp)\n")
			}
		} else {
			b.buf.WriteString("\tclr.l   -(sp)\n")
		}
		b.buf.WriteString("\tjsr     _panic\n")
		b.buf.WriteString("\tlea     4(sp), sp\n")
	case "_unlink_jmp_":
		lblDone := b.nextLabel()
		b.buf.WriteString("\tmove.l  v_prelude._jmp_chain_, d0\n")
		b.buf.WriteString(fmt.Sprintf("\tbeq     %s\n", lblDone))
		b.buf.WriteString("\tmove.l  d0, a0\n")
		b.buf.WriteString("\tmove.l  (a0), v_prelude._jmp_chain_\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	case "_propagate_panic_":
		lblDone := b.nextLabel()
		lblAbort := b.nextLabel()
		b.buf.WriteString("\tmove.l  v_prelude._panic_, d0\n")
		b.buf.WriteString(fmt.Sprintf("\tbeq     %s\n", lblDone))
		b.buf.WriteString("\tmove.l  v_prelude._jmp_chain_, d0\n")
		b.buf.WriteString(fmt.Sprintf("\tbeq     %s\n", lblAbort))
		b.buf.WriteString("\tmove.l  d0, a0\n")
		b.buf.WriteString("\tmove.l  12(a0), a6\n")
		b.buf.WriteString("\tmove.l  8(a0), sp\n")
		b.buf.WriteString("\tmove.l  4(a0), a1\n")
		b.buf.WriteString("\tjmp     (a1)\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblAbort))
		b.buf.WriteString("\tpea     .L_empty_chain_msg\n")
		b.buf.WriteString("\tjsr     _printf\n")
		b.buf.WriteString("\tlea     4(sp), sp\n")
		b.buf.WriteString("\tmove.l  #1, -(sp)\n")
		b.buf.WriteString("\tjsr     _exit\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	default:
		log.Panicf("emitBuiltinCall: unhandled builtin %s", i.Name)
	}
}

func (b *Backend) emitPrint(newline bool, args []ir.Value) {
	b.lblCount++
	fmtLabel := fmt.Sprintf(".Lfmt%d", b.lblCount)

	var formatStrs []string
	var dataArgs []ir.Value

	for _, arg := range args {
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, strLit)
		} else if arg.Type().Equals(ir.TypeInt) || arg.Type().IsInt() {
			formatStrs = append(formatStrs, "%d")
			dataArgs = append(dataArgs, arg)
		} else if strings.HasSuffix(arg.Type().Name, "slice_byte") || arg.Type().Name == "*byte" || arg.Type().Name == "string" {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, arg)
		} else {
			formatStrs = append(formatStrs, "%u")
			dataArgs = append(dataArgs, arg)
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\n"
	}

	b.rodata = append(b.rodata, rodataEntry{lbl: fmtLabel, str: format})

	for i := len(dataArgs) - 1; i >= 0; i-- {
		arg := dataArgs[i]
		if strLit, ok := arg.(*ir.StringLiteral); ok {
			b.lblCount++
			strLbl := fmt.Sprintf(".Lstr%d", b.lblCount)
			b.rodata = append(b.rodata, rodataEntry{lbl: strLbl, str: strLit.Value})
			b.buf.WriteString(fmt.Sprintf("\tpea     %s\n", strLbl))
		} else {
			b.loadVal(arg, "d0")
			b.buf.WriteString("\tmove.l  d0, -(sp)\n")
		}
	}

	b.buf.WriteString(fmt.Sprintf("\tpea     %s\n", fmtLabel))
	b.buf.WriteString("\tjsr     _printf\n")

	cleanup := (len(dataArgs) + 1) * 4
	b.buf.WriteString(fmt.Sprintf("\tlea     %d(sp), sp\n", cleanup))
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.allocateSlots(f)

	b.buf.WriteString("\n")
	b.buf.WriteString(fmt.Sprintf("; ============================================================================\n"))
	b.buf.WriteString(fmt.Sprintf("; func %s (frame: %d bytes)\n", f.Name, b.stackSize))
	b.buf.WriteString(fmt.Sprintf("; ============================================================================\n"))
	b.buf.WriteString(fmt.Sprintf("%s:\n", f.EmitName()))

	// Prologue
	b.buf.WriteString(fmt.Sprintf("\tlink    a6, #-%d\n", b.stackSize))
	retSize := b.getTypeSize(f.ReturnType)
	if retSize > 4 {
		b.buf.WriteString(fmt.Sprintf("\tmove.l  a0, -%d(a6)\n", b.retPtrSlot))
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
			b.buf.WriteString("\t; " + instr.String() + "\n")
			b.emitInstr(instr)
		}

		if blk.Terminator != nil {
			switch term := blk.Terminator.(type) {
			case *ir.Jump:
				b.emitPhiAssignments(blk, term.Target)
				b.buf.WriteString(fmt.Sprintf("\tbra     .L_%s_b%d\n", f.Name, term.Target.ID))

			case *ir.Branch:
				b.loadVal(term.Condition, "d0")
				b.buf.WriteString("\ttst.b   d0\n")
				trueLbl := fmt.Sprintf(".L_%s_b%d_true", f.Name, blk.ID)
				falseLbl := fmt.Sprintf(".L_%s_b%d_false", f.Name, blk.ID)

				b.buf.WriteString(fmt.Sprintf("\tbne     %s\n", trueLbl))
				b.buf.WriteString(fmt.Sprintf("\tbra     %s\n", falseLbl))

				b.buf.WriteString(fmt.Sprintf("%s:\n", trueLbl))
				b.emitPhiAssignments(blk, term.TrueBlock)
				b.buf.WriteString(fmt.Sprintf("\tbra     .L_%s_b%d\n", f.Name, term.TrueBlock.ID))

				b.buf.WriteString(fmt.Sprintf("%s:\n", falseLbl))
				b.emitPhiAssignments(blk, term.FalseBlock)
				b.buf.WriteString(fmt.Sprintf("\tbra     .L_%s_b%d\n", f.Name, term.FalseBlock.ID))

			case *ir.Return:
				if term.Val != nil {
					sz := b.getTypeSize(term.Val.Type())
					if sz <= 4 {
						b.loadVal(term.Val, "d0")
					} else {
						// Struct return: copy to caller return pointer
						b.buf.WriteString(fmt.Sprintf("\tmove.l  -%d(a6), a1\n", b.retPtrSlot))
						b.emitLoadAddr("a0", b.getAddr(term.Val))
						b.emitMemCopy("(a1)", "(a0)", sz)
					}
				}
				b.buf.WriteString("\tunlk    a6\n")
				b.buf.WriteString("\trts\n")

			default:
				log.Panicf("emitFunc: unhandled terminator %T", blk.Terminator)
			}
		}
	}
}

func (b *Backend) emitGlobals(prog *ir.Program) {
	b.buf.WriteString("\n; ── Global Data Section ───────────────────────────────────────────────────────\n")
	b.buf.WriteString("\teven\n")
	for _, g := range prog.Globals {
		b.buf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
		sz := b.getTypeSize(g.Typ)
		if g.IsInit {
			if g.InitVal != nil {
				b.emitData(g.InitVal)
			} else if g.InitString != "" {
				var bytesList []string
				for _, ch := range []byte(g.InitString) {
					bytesList = append(bytesList, fmt.Sprintf("$%02X", ch))
				}
				b.buf.WriteString(fmt.Sprintf("\tdc.b    %s\n", strings.Join(bytesList, ", ")))
				sz -= len(g.InitString)
				if sz > 0 {
					b.buf.WriteString(fmt.Sprintf("\tds.b    %d\n", sz))
				}
			}
		} else {
			if sz > 0 {
				b.buf.WriteString(fmt.Sprintf("\tds.b    %d\n", sz))
			}
		}
		b.buf.WriteString("\teven\n")
	}

	if len(b.rodata) > 0 {
		b.buf.WriteString("\n; ── Read-Only Data (Strings & Format Literals) ───────────────────────────────\n")
		b.buf.WriteString("\teven\n")
		for _, entry := range b.rodata {
			b.buf.WriteString(fmt.Sprintf("%s:\n", entry.lbl))
			var bytesList []string
			for _, ch := range []byte(entry.str) {
				bytesList = append(bytesList, fmt.Sprintf("$%02X", ch))
			}
			bytesList = append(bytesList, "$00")
			b.buf.WriteString(fmt.Sprintf("\tdc.b    %s\n", strings.Join(bytesList, ", ")))
			b.buf.WriteString("\teven\n")
		}
	}
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tdc.b    %d\n", v.Val))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tdc.l    %d\n", v.Val))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tdc.l    v_%s\n", v.Global.Name))
	case *ir.ConstStruct:
		structTyp := v.Type()
		if b.program != nil {
			if def, ok := b.program.TypeDefs[structTyp.Name]; ok {
				structTyp = def
			}
		}
		fields := structTyp.FieldsOfStruct()
		byteOffset := 0
		for fIdx, f := range fields {
			sz := b.getTypeSize(f.Type)
			align := b.getTypeAlignment(f.Type)
			paddedOffset := alignVal(byteOffset, align)
			if paddedOffset > byteOffset {
				b.buf.WriteString(fmt.Sprintf("\tds.b    %d\n", paddedOffset-byteOffset))
			}
			byteOffset = paddedOffset
			if fIdx < len(v.Fields) {
				b.emitData(v.Fields[fIdx])
			}
			byteOffset += sz
		}
		structSize := b.getTypeSize(v.Type())
		if structSize > byteOffset {
			b.buf.WriteString(fmt.Sprintf("\tds.b    %d\n", structSize-byteOffset))
		}
	case *ir.ConstArray:
		totalBytes := 0
		for _, el := range v.Elements {
			elSz := b.getTypeSize(el.Type())
			b.emitData(el)
			totalBytes += elSz
		}
		arrSize := b.getTypeSize(v.Type())
		if arrSize > totalBytes {
			b.buf.WriteString(fmt.Sprintf("\tds.b    %d\n", arrSize-totalBytes))
		}
	}
}

func (b *Backend) emitHelpers() {
	b.buf.WriteString(`
; ── Software Math Helpers (32-Bit) ───────────────────────────────────────────

; __mul32: D0 = D0 * D1
__mul32:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	; (D0_hi * 2^16 + D0_lo) * (D1_hi * 2^16 + D1_lo)
	; = D0_lo * D1_lo + (D0_hi * D1_lo + D0_lo * D1_hi) * 2^16
	move.w  d0, d2      ; D2 = D0_lo
	mulu.w  d1, d2      ; D2 = D0_lo * D1_lo
	move.l  d0, d3
	swap    d3          ; D3 = D0_hi
	mulu.w  d1, d3      ; D3 = D0_hi * D1_lo
	swap    d1          ; D1 = D1_hi
	mulu.w  d0, d1      ; D1 = D0_lo * D1_hi
	add.w   d3, d1      ; D1 = (D0_hi * D1_lo + D0_lo * D1_hi)
	swap    d1
	clr.w   d1
	add.l   d1, d2      ; D2 = full 32-bit product
	move.l  d2, d0
	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

; __div32: D0 = D0 / D1 (signed)
__div32:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	moveq   #0, d2      ; sign tracker
	tst.l   d0
	bge     .L_div_d0_pos
	neg.l   d0
	not.b   d2
.L_div_d0_pos:
	tst.l   d1
	bge     .L_div_d1_pos
	neg.l   d1
	not.b   d2
.L_div_d1_pos:
	bsr     __udiv32
	tst.b   d2
	beq     .L_div_done
	neg.l   d0
.L_div_done:
	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

; __mod32: D0 = D0 % D1 (signed)
__mod32:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	moveq   #0, d2
	tst.l   d0
	bge     .L_mod_d0_pos
	neg.l   d0
	not.b   d2          ; remainder sign follows dividend
.L_mod_d0_pos:
	tst.l   d1
	bge     .L_mod_d1_pos
	neg.l   d1
.L_mod_d1_pos:
	bsr     __umod32
	tst.b   d2
	beq     .L_mod_done
	neg.l   d0
.L_mod_done:
	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

; __udiv32: D0 = D0 / D1 (unsigned)
__udiv32:
	bsr     __udivmod32
	rts

; __umod32: D0 = D0 % D1 (unsigned)
__umod32:
	bsr     __udivmod32
	move.l  d1, d0      ; remainder in D1 -> return in D0
	rts

; __udivmod32: Unsigned 32-bit division/modulo (D0 / D1 -> quotient in D0, remainder in D1)
__udivmod32:
	tst.l   d1
	bne     .L_udiv_start
	pea     .L_div_zero_str
	jsr     _panic
.L_udiv_start:
	move.l  d2, -(sp)
	move.l  d3, -(sp)
	moveq   #0, d2      ; quotient
	moveq   #31, d3     ; bit counter
.L_udiv_loop:
	add.l   d0, d0      ; shift dividend left, MSB into carry
	addx.l  d2, d2      ; shift carry into partial remainder (D2)
	cmp.l   d1, d2      ; can we subtract divisor?
	bcs     .L_udiv_next
	sub.l   d1, d2
	addq.l  #1, d0      ; set bit in quotient
.L_udiv_next:
	dbra    d3, .L_udiv_loop
	move.l  d2, d1      ; remainder in D1
	move.l  (sp)+, d3
	move.l  (sp)+, d2
	rts

_panic:
	move.l  4(sp), d0
	move.l  d0, v_prelude._panic_
	move.l  d0, -(sp)
	pea     .L_panic_msg_fmt
	jsr     _printf
	lea     8(sp), sp
	move.l  v_prelude._jmp_chain_, d0
	beq     .L_panic_abort
	move.l  d0, a0
	move.l  12(a0), a6
	move.l  8(a0), sp
	move.l  4(a0), a1
	jmp     (a1)
.L_panic_abort:
	pea     .L_empty_chain_msg
	jsr     _printf
	lea     4(sp), sp
	move.l  #1, -(sp)
	jsr     _exit

	even
.L_div_zero_str:
	dc.b    "division by zero", 0
	even
.L_panic_msg_fmt:
	dc.b    $0A, "*PANIC* %s", $0A, 0
	even
.L_empty_chain_msg:
	dc.b    $0A, "*** ABORT", $0A, $0A, "*** EMPTY_RE_CHAIN", $0A, 0
	even
`)
}

func (b *Backend) Generate(prog *ir.Program) string {
	b.program = prog
	b.buf.Reset()
	b.rodata = nil

	b.buf.WriteString("; ── M68K Whole-Program Assembly (MiniGolf) ──────────────────────────────────\n")
	b.buf.WriteString("\teven\n")

	// Emit entry dispatch
	b.buf.WriteString("_main:\n")
	b.buf.WriteString("\tjsr     f_main__main\n")
	b.buf.WriteString("\tmoveq   #0, d0\n")
	b.buf.WriteString("\trts\n")

	for _, f := range prog.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
		}
	}

	b.emitGlobals(prog)
	b.emitHelpers()

	return b.buf.String()
}
