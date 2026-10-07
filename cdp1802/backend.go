package cdp1802

import (
	_ "embed"
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

//go:embed cstart.asm
var CStartAsm string

func alignVal(val, align int) int {
	return (val + align - 1) & ^(align - 1)
}

type rodataEntry struct {
	lbl string
	str string
}

type Backend struct {
	buf           bytes.Buffer
	program       *ir.Program
	slots         map[int]int    // instruction ID -> byte offset below RB (accessed as RB - offset)
	jmpSlots      map[int]int    // SetJmp ID -> jmpbuf offset below RB
	paramSlots    map[string]int // param name -> offset above RB (Arg 1 starts at +5)
	typeMap       map[int]ir.Type
	stackSize     int
	retPtrSlot    int
	lblCount      int
	rodata        []rodataEntry
	IncludeCStart bool
}

func New() *Backend {
	return &Backend{
		slots:         make(map[int]int),
		jmpSlots:      make(map[int]int),
		paramSlots:    make(map[string]int),
		typeMap:       make(map[int]ir.Type),
		IncludeCStart: true,
	}
}

func (b *Backend) nextLabel() string {
	b.lblCount++
	return fmt.Sprintf(".LL%d", b.lblCount)
}

func (b *Backend) getTypeAlignment(typ ir.Type) int {
	return 1 // 1802 is strictly byte-addressable with no hardware alignment restrictions
}

func (b *Backend) getTypeSize(typ ir.Type) int {
	switch typ.Name {
	case "byte", "bool":
		return 1
	case "word", "int", "uint", "const_integer", "noreturn":
		return 2 // 16-bit word on 1802
	}
	if typ.IsByte() || typ.IsBool() {
		return 1
	}
	if typ.IsWord() || typ.IsInt() || typ.IsConstInt() || typ.IsNoReturn() {
		return 2
	}
	if typ.IsAnArray() {
		return typ.ArrayLength() * b.getTypeSize(typ.ArrayElementType())
	}
	if typ.IsAPointer() || typ.IsAFuncPtr() {
		return 2
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ.Name]; ok {
			return b.getTypeSize(def)
		}
	}
	if typ.IsAStruct() {
		size := 0
		for _, f := range typ.FieldsOfStruct() {
			size += b.getTypeSize(f.Type)
		}
		return size
	}
	return 2
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
		offset += b.getTypeSize(fields[i].Type)
	}
	return offset, b.getTypeSize(fields[fieldIndex].Type)
}

func (b *Backend) resolveSlot(f *ir.Function, id int) int {
	if f != nil && f.SlotAlias != nil {
		for {
			if alias, ok := f.SlotAlias[id]; ok {
				id = alias
			} else {
				break
			}
		}
	}
	return id
}

func (b *Backend) allocateSlots(f *ir.Function) {
	b.slots = make(map[int]int)
	b.jmpSlots = make(map[int]int)
	b.paramSlots = make(map[string]int)
	b.typeMap = make(map[int]ir.Type)

	// Caller's pushed arguments start at +5 above RB
	// (RB+1, RB+2: saved RB; RB+3, RB+4: saved R6)
	paramOffset := 5
	retSize := b.getTypeSize(f.ReturnType)
	if retSize > 2 {
		b.retPtrSlot = paramOffset
		paramOffset += 2
	}
	for _, p := range f.Parameters {
		sz := b.getTypeSize(p.Typ)
		b.paramSlots[p.Name] = paramOffset
		paramOffset += alignVal(sz, 2)
	}

	currentOffset := 0

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			id := instr.GetID()
			canon := b.resolveSlot(f, id)
			b.typeMap[id] = instr.Type()
			b.typeMap[canon] = instr.Type()
		}
	}

	addressTaken := make(map[int]bool)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if aol, ok := instr.(*ir.AddressOfLocal); ok {
				if aol.Local != nil {
					if localInstr, ok2 := aol.Local.(ir.Instruction); ok2 {
						addressTaken[localInstr.GetID()] = true
						addressTaken[b.resolveSlot(f, localInstr.GetID())] = true
					}
				}
			}
		}
	}

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			id := instr.GetID()
			canon := b.resolveSlot(f, id)

			switch instr.(type) {
			case *ir.ConstByte, *ir.ConstWord, *ir.Sizeof, *ir.AddressOfGlobal, *ir.AddressOfFunc:
				if !addressTaken[id] && !addressTaken[canon] {
					continue
				}
			}

			if off, ok := b.slots[canon]; ok {
				b.slots[id] = off
				continue
			}

			size := b.getTypeSize(instr.Type())
			if size == 0 {
				continue
			}
			currentOffset += size
			b.slots[canon] = currentOffset
			b.slots[id] = currentOffset

			if _, ok := instr.(*ir.SetJmp); ok {
				currentOffset += 16
				b.jmpSlots[id] = currentOffset
				b.jmpSlots[canon] = currentOffset
			}
		}
	}

	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			id := instr.GetID()
			canon := b.resolveSlot(f, id)
			if off, ok := b.slots[canon]; ok {
				b.slots[id] = off
			}
		}
	}

	b.stackSize = alignVal(currentOffset, 2)
}

func (b *Backend) emitAddrToRC(d int) {
	if d == 0 {
		b.buf.WriteString("\tGHI RB\n\tPHI RC\n\tGLO RB\n\tPLO RC\n")
	} else if d > 0 && d <= 3 {
		b.buf.WriteString("\tGHI RB\n\tPHI RC\n\tGLO RB\n\tPLO RC\n")
		for k := 0; k < d; k++ {
			b.buf.WriteString("\tINC RC\n")
		}
	} else if d < 0 && d >= -3 {
		b.buf.WriteString("\tGHI RB\n\tPHI RC\n\tGLO RB\n\tPLO RC\n")
		for k := 0; k < -d; k++ {
			b.buf.WriteString("\tDEC RC\n")
		}
	} else if d > 0 {
		b.buf.WriteString(fmt.Sprintf("\tGLO RB\n\tADI %d\n\tPLO RC\n\tGHI RB\n\tADCI %d\n\tPHI RC\n",
			d&0xFF, (d>>8)&0xFF))
	} else {
		neg := -d
		b.buf.WriteString(fmt.Sprintf("\tGLO RB\n\tSMI %d\n\tPLO RC\n\tGHI RB\n\tSMBI %d\n\tPHI RC\n",
			neg&0xFF, (neg>>8)&0xFF))
	}
}

func (b *Backend) emitAddConst(reg string, offset int) {
	if offset == 0 {
		return
	}
	if offset > 0 && offset <= 3 {
		for k := 0; k < offset; k++ {
			b.buf.WriteString(fmt.Sprintf("\tINC %s\n", reg))
		}
		return
	}
	if offset < 0 && offset >= -3 {
		for k := 0; k < -offset; k++ {
			b.buf.WriteString(fmt.Sprintf("\tDEC %s\n", reg))
		}
		return
	}
	if offset > 0 {
		b.buf.WriteString(fmt.Sprintf("\tGLO %s\n\tADI %d\n\tPLO %s\n\tGHI %s\n\tADCI %d\n\tPHI %s\n",
			reg, offset&0xFF, reg, reg, (offset>>8)&0xFF, reg))
	} else {
		neg := -offset
		b.buf.WriteString(fmt.Sprintf("\tGLO %s\n\tSMI %d\n\tPLO %s\n\tGHI %s\n\tSMBI %d\n\tPHI %s\n",
			reg, neg&0xFF, reg, reg, (neg>>8)&0xFF, reg))
	}
}

func (b *Backend) loadVal(val ir.Value, reg string) {
	switch v := val.(type) {
	case *ir.ConstWord:
		wVal := uint16(v.Val)
		if wVal == 0 {
			b.buf.WriteString(fmt.Sprintf("\tLDI 0\n\tPLO %s\n\tPHI %s\n", reg, reg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tLOAD %s, %d\n", reg, wVal))
		}
	case *ir.ConstByte:
		if v.Val == 0 {
			b.buf.WriteString(fmt.Sprintf("\tLDI 0\n\tPLO %s\n\tPHI %s\n", reg, reg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tLDI %d\n\tPLO %s\n\tLDI 0\n\tPHI %s\n", v.Val&0xFF, reg, reg))
		}
	case *ir.Sizeof:
		sz := b.getTypeSize(v.TargetTyp)
		b.buf.WriteString(fmt.Sprintf("\tLOAD %s, %d\n", reg, sz))
	case *ir.Parameter:
		off := b.paramSlots[v.Name]
		sz := b.getTypeSize(v.Typ)
		if sz == 1 {
			b.emitAddrToRC(off + 1)
			b.buf.WriteString(fmt.Sprintf("\tLDN RC\n\tPLO %s\n\tLDI 0\n\tPHI %s\n", reg, reg))
		} else {
			b.emitAddrToRC(off)
			if reg == "RC" {
				b.buf.WriteString("\tLDA RC\n\tPHI RA\n\tLDN RC\n\tPLO RC\n\tGHI RA\n\tPHI RC\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tLDA RC\n\tPHI %s\n\tLDN RC\n\tPLO %s\n", reg, reg))
			}
		}
	case *ir.Global:
		sz := b.getTypeSize(v.Typ)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tLOAD RC, v_%s\n\tLDN RC\n\tPLO %s\n\tLDI 0\n\tPHI %s\n", v.Name, reg, reg))
		} else {
			if reg == "RC" {
				b.buf.WriteString(fmt.Sprintf("\tLOAD RC, v_%s\n\tLDA RC\n\tPHI RA\n\tLDN RC\n\tPLO RC\n\tGHI RA\n\tPHI RC\n", v.Name))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tLOAD RC, v_%s\n\tLDA RC\n\tPHI %s\n\tLDN RC\n\tPLO %s\n", v.Name, reg, reg))
			}
		}
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tLOAD %s, v_%s\n", reg, v.Global.Name))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tLOAD %s, %s\n", reg, v.Func.EmitName()))
	case *ir.StringLiteral:
		b.lblCount++
		lbl := fmt.Sprintf(".Lstr%d", b.lblCount)
		b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: v.Value})
		b.buf.WriteString(fmt.Sprintf("\tLOAD %s, %s\n", reg, lbl))
	case ir.Instruction:
		id := v.GetID()
		slot := b.slots[id]
		sz := b.getTypeSize(v.Type())
		b.emitAddrToRC(-slot)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tLDN RC\n\tPLO %s\n\tLDI 0\n\tPHI %s\n", reg, reg))
		} else {
			if reg == "RC" {
				b.buf.WriteString("\tLDA RC\n\tPHI RA\n\tLDN RC\n\tPLO RC\n\tGHI RA\n\tPHI RC\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tLDA RC\n\tPHI %s\n\tLDN RC\n\tPLO %s\n", reg, reg))
			}
		}
	default:
		log.Panicf("loadVal: unsupported value %T", val)
	}
}

func (b *Backend) storeResult(id int) {
	slot, ok := b.slots[id]
	if !ok || slot == 0 {
		return
	}
	typ := b.typeMap[id]
	sz := b.getTypeSize(typ)
	b.emitAddrToRC(-slot)
	if sz == 1 {
		b.buf.WriteString("\tGLO R7\n\tSTR RC\n")
	} else {
		b.buf.WriteString("\tGHI R7\n\tSTR RC\n\tINC RC\n\tGLO R7\n\tSTR RC\n")
	}
}

func (b *Backend) emitLoadAddr(reg string, val ir.Value) {
	switch v := val.(type) {
	case *ir.Parameter:
		off := b.paramSlots[v.Name]
		b.emitAddrToRC(off)
		if reg != "RC" {
			b.buf.WriteString(fmt.Sprintf("\tGHI RC\n\tPHI %s\n\tGLO RC\n\tPLO %s\n", reg, reg))
		}
	case ir.Instruction:
		slot := b.slots[v.GetID()]
		b.emitAddrToRC(-slot)
		if reg != "RC" {
			b.buf.WriteString(fmt.Sprintf("\tGHI RC\n\tPHI %s\n\tGLO RC\n\tPLO %s\n", reg, reg))
		}
	case *ir.Global:
		b.buf.WriteString(fmt.Sprintf("\tLOAD %s, v_%s\n", reg, v.Name))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tLOAD %s, v_%s\n", reg, v.Global.Name))
	default:
		log.Panicf("emitLoadAddr: unsupported value %T", val)
	}
}

func (b *Backend) emitMemCopy(sz int) {
	if sz <= 0 {
		return
	}
	if sz == 1 {
		b.buf.WriteString("\tLDA RA\n\tSTR RC\n")
		return
	}
	if sz == 2 {
		b.buf.WriteString("\tLDA RA\n\tSTR RC\n\tINC RC\n\tLDA RA\n\tSTR RC\n")
		return
	}
	if sz == 4 {
		for k := 0; k < 4; k++ {
			if k > 0 {
				b.buf.WriteString("\tINC RC\n")
			}
			b.buf.WriteString("\tLDA RA\n\tSTR RC\n")
		}
		return
	}
	lbl := b.nextLabel()
	b.buf.WriteString(fmt.Sprintf("\tLOAD RD, %d\n", sz))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
	b.buf.WriteString("\tLDA RA\n\tSTR RC\n\tINC RC\n\tDEC RD\n\tGLO RD\n")
	b.buf.WriteString(fmt.Sprintf("\tBNZ %s\n\tGHI RD\n\tBNZ %s\n", lbl, lbl))
}

func (b *Backend) emitMemClear(sz int) {
	if sz <= 0 {
		return
	}
	b.buf.WriteString("\tLDI 0\n")
	if sz == 1 {
		b.buf.WriteString("\tSTR RC\n")
		return
	}
	if sz == 2 {
		b.buf.WriteString("\tSTR RC\n\tINC RC\n\tSTR RC\n")
		return
	}
	if sz <= 4 {
		for k := 0; k < sz; k++ {
			if k > 0 {
				b.buf.WriteString("\tINC RC\n")
			}
			b.buf.WriteString("\tSTR RC\n")
		}
		return
	}
	lbl := b.nextLabel()
	b.buf.WriteString(fmt.Sprintf("\tLOAD RD, %d\n", sz))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
	b.buf.WriteString("\tLDI 0\n\tSTR RC\n\tINC RC\n\tDEC RD\n\tGLO RD\n")
	b.buf.WriteString(fmt.Sprintf("\tBNZ %s\n\tGHI RD\n\tBNZ %s\n", lbl, lbl))
}

func (b *Backend) emitBinaryOp(i *ir.BinaryOp) {
	id := i.GetID()
	b.loadVal(i.Left, "R7")
	b.loadVal(i.Right, "R8")

	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()

	switch i.Op {
	case "add", "+":
		b.buf.WriteString("\tSEP R4\n\tDW  __add16\n")
	case "sub", "-":
		b.buf.WriteString("\tSEP R4\n\tDW  __sub16\n")
	case "mul", "*":
		b.buf.WriteString("\tSEP R4\n\tDW  __mul16\n")
	case "div", "/":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __udiv16\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __sdiv16\n")
		}
	case "mod", "%":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __umod16\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __smod16\n")
		}
	case "and", "bitand", "&":
		b.buf.WriteString("\tSEP R4\n\tDW  __and16\n")
	case "or", "bitor", "|":
		b.buf.WriteString("\tSEP R4\n\tDW  __or16\n")
	case "xor", "bitxor", "^":
		b.buf.WriteString("\tSEP R4\n\tDW  __xor16\n")
	case "andnot":
		b.buf.WriteString("\tGLO R8\n\tXRI $FF\n\tPLO R8\n\tGHI R8\n\tXRI $FF\n\tPHI R8\n")
		b.buf.WriteString("\tSEP R4\n\tDW  __and16\n")
	case "shl", "<<":
		b.buf.WriteString("\tSEP R4\n\tDW  __shl16\n")
	case "shr", ">>":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __shr16\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __sar16\n")
		}
	case "sar":
		b.buf.WriteString("\tSEP R4\n\tDW  __sar16\n")
	default:
		log.Panicf("emitBinaryOp: unsupported op %s", i.Op)
	}
	b.storeResult(id)
}

func (b *Backend) emitCompare(i *ir.Compare) {
	id := i.GetID()
	b.loadVal(i.Left, "R7")
	b.loadVal(i.Right, "R8")
	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()

	switch i.Op {
	case "eq":
		b.buf.WriteString("\tSEP R4\n\tDW  __cmpeq16\n")
	case "neq":
		b.buf.WriteString("\tSEP R4\n\tDW  __cmpne16\n")
	case "lt":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmplt16_u\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmplt16_s\n")
		}
	case "lte":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmplte16_u\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmplte16_s\n")
		}
	case "gt":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmpgt16_u\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmpgt16_s\n")
		}
	case "gte":
		if isUnsigned {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmpgte16_u\n")
		} else {
			b.buf.WriteString("\tSEP R4\n\tDW  __cmpgte16_s\n")
		}
	default:
		log.Panicf("emitCompare: unsupported op %s", i.Op)
	}
	b.storeResult(id)
}

func (b *Backend) emitUnaryOp(i *ir.UnaryOp) {
	id := i.GetID()
	b.loadVal(i.Operand, "R7")
	switch i.Op {
	case "neg":
		b.buf.WriteString("\tGLO R7\n\tSDI 0\n\tPLO R7\n\tGHI R7\n\tSDBI 0\n\tPHI R7\n")
	case "not":
		lblZero := b.nextLabel()
		lblDone := b.nextLabel()
		b.buf.WriteString("\tGLO R7\n\tBNZ " + lblZero + "\n\tGHI R7\n\tBNZ " + lblZero + "\n")
		b.buf.WriteString("\tLDI 1\n\tPLO R7\n\tLDI 0\n\tPHI R7\n\tLBR " + lblDone + "\n")
		b.buf.WriteString(lblZero + ":\n")
		b.buf.WriteString("\tLDI 0\n\tPLO R7\n\tPHI R7\n")
		b.buf.WriteString(lblDone + ":\n")
	case "bitnot":
		b.buf.WriteString("\tGLO R7\n\tXRI $FF\n\tPLO R7\n\tGHI R7\n\tXRI $FF\n\tPHI R7\n")
	default:
		log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
	}
	b.storeResult(id)
}

func (b *Backend) emitCallStackCleanup(totalArgBytes int) {
	if totalArgBytes <= 0 {
		return
	}
	if totalArgBytes <= 4 {
		for k := 0; k < totalArgBytes; k++ {
			b.buf.WriteString("\tINC R2\n")
		}
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tGLO R2\n\tADI %d\n\tPLO R2\n\tGHI R2\n\tADCI %d\n\tPHI R2\n",
		totalArgBytes&0xFF, (totalArgBytes>>8)&0xFF))
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
			b.buf.WriteString(fmt.Sprintf("\tLOAD R7, %s\n\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n", strLbl))
		} else {
			b.loadVal(arg, "R7")
			b.buf.WriteString("\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n")
		}
	}

	b.buf.WriteString(fmt.Sprintf("\tLOAD R7, %s\n\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n", fmtLabel))
	b.buf.WriteString("\tSEP R4\n\tDW  _printf\n")

	cleanup := (len(dataArgs) + 1) * 2
	b.emitCallStackCleanup(cleanup)
}

func (b *Backend) emitBuiltinCall(i *ir.BuiltinCall) {
	switch i.Name {
	case "print", "println":
		b.emitPrint(i.Name == "println", i.Args)
	case "exit":
		if len(i.Args) > 0 {
			b.loadVal(i.Args[0], "R7")
		} else {
			b.buf.WriteString("\tLOAD R7, 0\n")
		}
		b.buf.WriteString("\tGLO R7\n\tSTXD\n\tINC R2\n\tSEP R4\n\tDW  _exit\n")
	case "panic", "_panic_":
		if len(i.Args) > 0 {
			if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
				b.lblCount++
				lbl := fmt.Sprintf(".Lpanic%d", b.lblCount)
				b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: strLit.Value})
				b.buf.WriteString(fmt.Sprintf("\tLOAD R7, %s\n\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n", lbl))
			} else {
				b.loadVal(i.Args[0], "R7")
				b.buf.WriteString("\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n")
			}
		} else {
			b.buf.WriteString("\tLOAD R7, 0\n\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n")
		}
		b.buf.WriteString("\tSEP R4\n\tDW  _panic\n\tINC R2\n\tINC R2\n")
	case "_unlink_jmp_":
		lblDone := b.nextLabel()
		lblSkip := b.nextLabel()
		b.buf.WriteString("\tLOAD RC, v_prelude._jmp_chain_\n")
		b.buf.WriteString("\tLDA RC\n\tPHI R7\n\tLDN RC\n\tPLO R7\n")
		b.buf.WriteString(fmt.Sprintf("\tGLO R7\n\tBNZ %s\n\tGHI R7\n\tBZ %s\n%s:\n", lblSkip, lblDone, lblSkip))
		b.buf.WriteString("\tGHI R7\n\tPHI RC\n\tGLO R7\n\tPLO RC\n")
		b.buf.WriteString("\tLDA RC\n\tPHI R8\n\tLDN RC\n\tPLO R8\n")
		b.buf.WriteString("\tLOAD RC, v_prelude._jmp_chain_\n")
		b.buf.WriteString("\tGHI R8\n\tSTR RC\n\tINC RC\n\tGLO R8\n\tSTR RC\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	case "_propagate_panic_":
		lblDone := b.nextLabel()
		lblAbort := b.nextLabel()
		lblCheckChain := b.nextLabel()
		lblDoJmp := b.nextLabel()
		b.buf.WriteString("\tLOAD RC, v_prelude._panic_\n")
		b.buf.WriteString("\tLDA RC\n\tPHI R7\n\tLDN RC\n\tPLO R7\n")
		b.buf.WriteString(fmt.Sprintf("\tGLO R7\n\tBNZ %s\n\tGHI R7\n\tBZ %s\n%s:\n", lblCheckChain, lblDone, lblCheckChain))
		b.buf.WriteString("\tLOAD RC, v_prelude._jmp_chain_\n")
		b.buf.WriteString("\tLDA RC\n\tPHI R7\n\tLDN RC\n\tPLO R7\n")
		b.buf.WriteString(fmt.Sprintf("\tGLO R7\n\tBNZ %s\n\tGHI R7\n\tBZ %s\n%s:\n", lblDoJmp, lblAbort, lblDoJmp))
		// Longjmp to top chain jmpbuf (in R7)
		b.buf.WriteString("\tGHI R7\n\tPHI RC\n\tGLO R7\n\tPLO RC\n")
		// Resume PC from RC+2
		b.buf.WriteString("\tINC RC\n\tINC RC\n\tLDA RC\n\tPHI RD\n\tLDN RC\n\tPLO RD\n")
		// SP from RC+4
		b.buf.WriteString("\tINC RC\n\tLDA RC\n\tPHI R2\n\tLDN RC\n\tPLO R2\n")
		// FP (RB) from RC+6
		b.buf.WriteString("\tINC RC\n\tLDA RC\n\tPHI RB\n\tLDN RC\n\tPLO RB\n")
		b.buf.WriteString("\tLOAD R7, 1\n")
		b.buf.WriteString("\tGHI RD\n\tPHI R3\n\tGLO RD\n\tPLO R3\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblAbort))
		b.buf.WriteString("\tLOAD R7, .L_empty_chain_msg\n\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n\tSEP R4\n\tDW  _printf\n\tINC R2\n\tINC R2\n")
		b.buf.WriteString("\tLOAD R7, 1\n\tGLO R7\n\tSTXD\n\tINC R2\n\tSEP R4\n\tDW  _exit\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	default:
		log.Panicf("emitBuiltinCall: unhandled builtin %s", i.Name)
	}
}

func (b *Backend) emitSetJmp(i *ir.SetJmp) {
	id := i.GetID()
	jmpSlot := b.jmpSlots[id]
	lblResume := b.nextLabel()
	lblDone := b.nextLabel()

	// Compute jmpbuf address in RC
	b.emitAddrToRC(-jmpSlot)

	// Save prev chain at RC+0
	b.buf.WriteString("\tLOAD RD, v_prelude._jmp_chain_\n\tLDA RD\n\tPHI RA\n\tLDN RD\n\tPLO RA\n")
	b.buf.WriteString("\tGHI RA\n\tSTR RC\n\tINC RC\n\tGLO RA\n\tSTR RC\n")

	// Update v_prelude._jmp_chain_ = jmpbuf
	b.emitAddrToRC(-jmpSlot)
	b.buf.WriteString("\tLOAD RD, v_prelude._jmp_chain_\n\tGHI RC\n\tSTR RD\n\tINC RD\n\tGLO RC\n\tSTR RD\n")

	// Save resume PC at RC+2
	b.emitAddrToRC(-jmpSlot + 2)
	b.buf.WriteString(fmt.Sprintf("\tLOAD RD, %s\n\tGHI RD\n\tSTR RC\n\tINC RC\n\tGLO RD\n\tSTR RC\n", lblResume))

	// Save SP at RC+4
	b.emitAddrToRC(-jmpSlot + 4)
	b.buf.WriteString("\tGHI R2\n\tSTR RC\n\tINC RC\n\tGLO R2\n\tSTR RC\n")

	// Save FP (RB) at RC+6
	b.emitAddrToRC(-jmpSlot + 6)
	b.buf.WriteString("\tGHI RB\n\tSTR RC\n\tINC RC\n\tGLO RB\n\tSTR RC\n")

	b.buf.WriteString("\tLOAD R7, 0\n")
	b.buf.WriteString(fmt.Sprintf("\tLBR  %s\n", lblDone))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblResume))
	b.buf.WriteString("\tLOAD R7, 1\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	b.storeResult(id)
}

func (b *Backend) emitLongJmp(i *ir.LongJmp) {
	b.loadVal(i.JmpBuf, "RC")
	// RC points to jmpbuf
	// jmpbuf layout:
	// +0: prev chain (2 bytes)
	// +2: resume PC (2 bytes)
	// +4: SP (2 bytes)
	// +6: FP / RB (2 bytes)
	b.buf.WriteString("\tINC RC\n\tINC RC\n")
	b.buf.WriteString("\tLDA RC\n\tPHI RD\n\tLDN RC\n\tPLO RD\n") // RD = resume PC, RC at +3
	b.buf.WriteString("\tINC RC\n")
	b.buf.WriteString("\tLDA RC\n\tPHI R2\n\tLDN RC\n\tPLO R2\n") // R2 = SP, RC at +5
	b.buf.WriteString("\tINC RC\n")
	b.buf.WriteString("\tLDA RC\n\tPHI RB\n\tLDN RC\n\tPLO RB\n") // RB = FP
	// Jump to resume PC in RD via RA trampoline (since P=3, cannot change R3 directly)
	lblTrampoline := b.nextLabel()
	b.buf.WriteString(fmt.Sprintf("\tLOAD RA, %s\n\tSEP RA\n%s:\n\tGHI RD\n\tPHI R3\n\tGLO RD\n\tPLO R3\n\tSEP R3\n", lblTrampoline, lblTrampoline))
}

func (b *Backend) emitPhiAssignments(currentBlock *ir.BasicBlock, targetBlock *ir.BasicBlock) {
	for _, instr := range targetBlock.Instructions {
		phi, ok := instr.(*ir.Phi)
		if !ok {
			continue
		}
		for _, edge := range phi.Edges {
			if edge.Block == currentBlock {
				if edge.Value != nil {
					sz := b.getTypeSize(phi.Type())
					if sz > 2 {
						b.emitLoadAddr("RA", edge.Value)
						b.emitAddrToRC(-b.slots[phi.GetID()])
						b.emitMemCopy(sz)
					} else {
						b.loadVal(edge.Value, "R7")
						b.storeResult(phi.GetID())
					}
				}
				break
			}
		}
	}
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))

	case *ir.ConstByte, *ir.ConstWord, *ir.Sizeof:
		// Stored on demand by loadVal

	case *ir.ZeroInit:
		sz := b.getTypeSize(i.Typ)
		b.emitAddrToRC(-b.slots[id])
		b.emitMemClear(sz)

	case *ir.AddressOfLocal:
		b.emitLoadAddr("R7", i.Local)
		b.storeResult(id)

	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tLOAD R7, v_%s\n", i.Global.Name))
		b.storeResult(id)

	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tLOAD R7, %s\n", i.Func.EmitName()))
		b.storeResult(id)

	case *ir.Load:
		sz := b.getTypeSize(i.Global.Typ)
		b.buf.WriteString(fmt.Sprintf("\tLOAD RC, v_%s\n", i.Global.Name))
		if sz == 1 {
			b.buf.WriteString("\tLDN RC\n\tPLO R7\n\tLDI 0\n\tPHI R7\n")
		} else if sz == 2 {
			b.buf.WriteString("\tLDA RC\n\tPHI R7\n\tLDN RC\n\tPLO R7\n")
		} else {
			b.buf.WriteString("\tGHI RC\n\tPHI RA\n\tGLO RC\n\tPLO RA\n")
			b.emitAddrToRC(-b.slots[id])
			b.emitMemCopy(sz)
			return
		}
		b.storeResult(id)

	case *ir.Store:
		sz := b.getTypeSize(i.Global.Typ)
		if sz == 1 {
			b.loadVal(i.Val, "R7")
			b.buf.WriteString(fmt.Sprintf("\tLOAD RC, v_%s\n", i.Global.Name))
			b.buf.WriteString("\tGLO R7\n\tSTR RC\n")
		} else if sz == 2 {
			b.loadVal(i.Val, "R7")
			b.buf.WriteString(fmt.Sprintf("\tLOAD RC, v_%s\n", i.Global.Name))
			b.buf.WriteString("\tGHI R7\n\tSTR RC\n\tINC RC\n\tGLO R7\n\tSTR RC\n")
		} else {
			b.buf.WriteString(fmt.Sprintf("\tLOAD RD, v_%s\n", i.Global.Name))
			b.emitLoadAddr("RA", i.Val)
			b.buf.WriteString("\tGHI RD\n\tPHI RC\n\tGLO RD\n\tPLO RC\n")
			b.emitMemCopy(sz)
		}

	case *ir.LoadPtr:
		sz := b.getTypeSize(i.Typ)
		b.loadVal(i.Ptr, "RC")
		if sz == 1 {
			b.buf.WriteString("\tLDN RC\n\tPLO R7\n\tLDI 0\n\tPHI R7\n")
		} else if sz == 2 {
			b.buf.WriteString("\tLDA RC\n\tPHI R7\n\tLDN RC\n\tPLO R7\n")
		} else {
			b.buf.WriteString("\tGHI RC\n\tPHI RA\n\tGLO RC\n\tPLO RA\n")
			b.emitAddrToRC(-b.slots[id])
			b.emitMemCopy(sz)
			return
		}
		b.storeResult(id)

	case *ir.StorePtr:
		sz := b.getTypeSize(i.Val.Type())
		if sz == 1 {
			b.loadVal(i.Val, "R7")
			b.loadVal(i.Ptr, "RC")
			b.buf.WriteString("\tGLO R7\n\tSTR RC\n")
		} else if sz == 2 {
			b.loadVal(i.Val, "R7")
			b.loadVal(i.Ptr, "RC")
			b.buf.WriteString("\tGHI R7\n\tSTR RC\n\tINC RC\n\tGLO R7\n\tSTR RC\n")
		} else {
			b.loadVal(i.Ptr, "RD")
			b.emitLoadAddr("RA", i.Val)
			b.buf.WriteString("\tGHI RD\n\tPHI RC\n\tGLO RD\n\tPLO RC\n")
			b.emitMemCopy(sz)
		}

	case *ir.ExtractField:
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		b.emitLoadAddr("RC", i.Struct)
		b.emitAddConst("RC", byteOffset)
		if fieldSize == 1 {
			b.buf.WriteString("\tLDN RC\n\tPLO R7\n\tLDI 0\n\tPHI R7\n")
			b.storeResult(id)
		} else if fieldSize == 2 {
			b.buf.WriteString("\tLDA RC\n\tPHI R7\n\tLDN RC\n\tPLO R7\n")
			b.storeResult(id)
		} else {
			b.buf.WriteString("\tGHI RC\n\tPHI RA\n\tGLO RC\n\tPLO RA\n")
			b.emitAddrToRC(-b.slots[id])
			b.emitMemCopy(fieldSize)
		}

	case *ir.InsertField:
		structSize := b.getTypeSize(i.Struct.Type())
		b.emitLoadAddr("RA", i.Struct)
		b.emitAddrToRC(-b.slots[id])
		b.emitMemCopy(structSize)

		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal(i.Val, "R7")
			b.emitAddrToRC(-b.slots[id])
			b.emitAddConst("RC", byteOffset)
			b.buf.WriteString("\tGLO R7\n\tSTR RC\n")
		} else if fieldSize == 2 {
			b.loadVal(i.Val, "R7")
			b.emitAddrToRC(-b.slots[id])
			b.emitAddConst("RC", byteOffset)
			b.buf.WriteString("\tGHI R7\n\tSTR RC\n\tINC RC\n\tGLO R7\n\tSTR RC\n")
		} else {
			b.emitLoadAddr("RA", i.Val)
			b.emitAddrToRC(-b.slots[id])
			b.emitAddConst("RC", byteOffset)
			b.emitMemCopy(fieldSize)
		}

	case *ir.ExtractElement:
		eltSize := b.getTypeSize(i.Typ)
		b.emitLoadAddr("RA", i.Array)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConst("RA", byteOffset)
		} else {
			b.loadVal(i.Index, "R7")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tLOAD R8, %d\n\tSEP R4\n\tDW  __mul16\n", eltSize))
			}
			b.buf.WriteString("\tGHI RA\n\tPHI R8\n\tGLO RA\n\tPLO R8\n")
			b.buf.WriteString("\tSEP R4\n\tDW  __add16\n")
			b.buf.WriteString("\tGHI R7\n\tPHI RA\n\tGLO R7\n\tPLO RA\n")
		}
		if eltSize == 1 {
			b.buf.WriteString("\tLDN RA\n\tPLO R7\n\tLDI 0\n\tPHI R7\n")
			b.storeResult(id)
		} else if eltSize == 2 {
			b.buf.WriteString("\tLDA RA\n\tPHI R7\n\tLDN RA\n\tPLO R7\n")
			b.storeResult(id)
		} else {
			b.emitAddrToRC(-b.slots[id])
			b.emitMemCopy(eltSize)
		}

	case *ir.InsertElement:
		arraySize := b.getTypeSize(i.Array.Type())
		b.emitLoadAddr("RA", i.Array)
		b.emitAddrToRC(-b.slots[id])
		b.emitMemCopy(arraySize)

		eltSize := b.getTypeSize(i.Val.Type())
		b.emitAddrToRC(-b.slots[id])
		b.buf.WriteString("\tGHI RC\n\tPHI RD\n\tGLO RC\n\tPLO RD\n")
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConst("RD", byteOffset)
		} else {
			b.loadVal(i.Index, "R7")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tLOAD R8, %d\n\tSEP R4\n\tDW  __mul16\n", eltSize))
			}
			b.buf.WriteString("\tGHI RD\n\tPHI R8\n\tGLO RD\n\tPLO R8\n")
			b.buf.WriteString("\tSEP R4\n\tDW  __add16\n")
			b.buf.WriteString("\tGHI R7\n\tPHI RD\n\tGLO R7\n\tPLO RD\n")
		}
		if eltSize == 1 {
			b.loadVal(i.Val, "R7")
			b.buf.WriteString("\tGLO R7\n\tSTR RD\n")
		} else if eltSize == 2 {
			b.loadVal(i.Val, "R7")
			b.buf.WriteString("\tGHI R7\n\tSTR RD\n\tINC RD\n\tGLO R7\n\tSTR RD\n")
		} else {
			b.emitLoadAddr("RA", i.Val)
			b.buf.WriteString("\tGHI RD\n\tPHI RC\n\tGLO RD\n\tPLO RC\n")
			b.emitMemCopy(eltSize)
		}

	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		b.loadVal(i.Ptr, "R7")
		b.emitAddConst("R7", byteOffset)
		b.storeResult(id)

	case *ir.AddressOfElement:
		eltSize := b.getTypeSize(i.ArrayPtr.Type().PointedType().ArrayElementType())
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.loadVal(i.ArrayPtr, "R7")
			b.emitAddConst("R7", byteOffset)
		} else {
			b.loadVal(i.Index, "R7")
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tLOAD R8, %d\n\tSEP R4\n\tDW  __mul16\n", eltSize))
			}
			b.loadVal(i.ArrayPtr, "R8")
			b.buf.WriteString("\tSEP R4\n\tDW  __add16\n")
		}
		b.storeResult(id)

	case *ir.BinaryOp:
		b.emitBinaryOp(i)

	case *ir.Compare:
		b.emitCompare(i)

	case *ir.UnaryOp:
		b.emitUnaryOp(i)

	case *ir.Cast:
		b.loadVal(i.Operand, "R7")
		b.storeResult(id)

	case *ir.Call:
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			sz := b.getTypeSize(arg.Type())
			pushSize := alignVal(sz, 2)
			totalArgBytes += pushSize
			if sz > 2 {
				b.buf.WriteString(fmt.Sprintf("\tGLO R2\n\tSMI %d\n\tPLO R2\n\tGHI R2\n\tSMBI %d\n\tPHI R2\n", pushSize&0xFF, (pushSize>>8)&0xFF))
				b.emitLoadAddr("RA", arg)
				b.buf.WriteString("\tGLO R2\n\tADI 1\n\tPLO RC\n\tGHI R2\n\tADCI 0\n\tPHI RC\n")
				b.emitMemCopy(sz)
			} else if sz == 1 {
				b.loadVal(arg, "R7")
				b.buf.WriteString("\tGLO R7\n\tSTXD\n\tLDI 0\n\tSTXD\n")
			} else {
				b.loadVal(arg, "R7")
				b.buf.WriteString("\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		if retSize > 2 {
			b.emitAddrToRC(-b.slots[id])
			b.buf.WriteString("\tGLO RC\n\tSTXD\n\tGHI RC\n\tSTXD\n")
			totalArgBytes += 2
		}
		b.buf.WriteString(fmt.Sprintf("\tSEP R4\n\tDW  %s\n", i.Func.EmitName()))
		b.emitCallStackCleanup(totalArgBytes)
		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 2 {
			b.storeResult(id)
		}

	case *ir.IndirectCall:
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			sz := b.getTypeSize(arg.Type())
			pushSize := alignVal(sz, 2)
			totalArgBytes += pushSize
			if sz > 2 {
				b.buf.WriteString(fmt.Sprintf("\tGLO R2\n\tSMI %d\n\tPLO R2\n\tGHI R2\n\tSMBI %d\n\tPHI R2\n", pushSize&0xFF, (pushSize>>8)&0xFF))
				b.emitLoadAddr("RA", arg)
				b.buf.WriteString("\tGLO R2\n\tADI 1\n\tPLO RC\n\tGHI R2\n\tADCI 0\n\tPHI RC\n")
				b.emitMemCopy(sz)
			} else if sz == 1 {
				b.loadVal(arg, "R7")
				b.buf.WriteString("\tGLO R7\n\tSTXD\n\tLDI 0\n\tSTXD\n")
			} else {
				b.loadVal(arg, "R7")
				b.buf.WriteString("\tGLO R7\n\tSTXD\n\tGHI R7\n\tSTXD\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		if retSize > 2 {
			b.emitAddrToRC(-b.slots[id])
			b.buf.WriteString("\tGLO RC\n\tSTXD\n\tGHI RC\n\tSTXD\n")
			totalArgBytes += 2
		}
		b.loadVal(i.FuncPtr, "RC")
		b.buf.WriteString("\tSEP R4\n\tDW  __call_rc\n")
		b.emitCallStackCleanup(totalArgBytes)
		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 2 {
			b.storeResult(id)
		}

	case *ir.BuiltinCall:
		b.emitBuiltinCall(i)

	case *ir.SetJmp:
		b.emitSetJmp(i)

	case *ir.LongJmp:
		b.emitLongJmp(i)

	case *ir.Phi:
		// Handled at block transitions

	default:
		log.Panicf("emitInstr: unhandled instruction %T", instr)
	}
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.allocateSlots(f)
	fName := f.EmitName()

	b.buf.WriteString(fmt.Sprintf("\n; ── Function: %s ───────────────────────────────────────────────\n", fName))
	b.buf.WriteString(fmt.Sprintf("%s:\n", fName))

	// Function prologue: save old RB, set RB = R2, allocate local frame
	b.buf.WriteString("\tGLO RB\n\tSTXD\n\tGHI RB\n\tSTXD\n")
	b.buf.WriteString("\tGHI R2\n\tPHI RB\n\tGLO R2\n\tPLO RB\n")
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tGLO R2\n\tSMI %d\n\tPLO R2\n\tGHI R2\n\tSMBI %d\n\tPHI R2\n",
			b.stackSize&0xFF, (b.stackSize>>8)&0xFF))
	}

	for _, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", fName, blk.ID))

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
			b.buf.WriteString(fmt.Sprintf("\tLBR .L_%s_b%d\n", fName, term.Target.ID))

		case *ir.Branch:
			b.loadVal(term.Condition, "R7")
			trueLbl := fmt.Sprintf(".L_%s_b%d_true", fName, blk.ID)
			falseLbl := fmt.Sprintf(".L_%s_b%d_false", fName, blk.ID)

			b.buf.WriteString("\tGLO R7\n")
			b.buf.WriteString(fmt.Sprintf("\tLBNZ %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("\tLBR  %s\n", falseLbl))

			b.buf.WriteString(fmt.Sprintf("%s:\n", trueLbl))
			b.emitPhiAssignments(blk, term.TrueBlock)
			b.buf.WriteString(fmt.Sprintf("\tLBR  .L_%s_b%d\n", fName, term.TrueBlock.ID))

			b.buf.WriteString(fmt.Sprintf("%s:\n", falseLbl))
			b.emitPhiAssignments(blk, term.FalseBlock)
			b.buf.WriteString(fmt.Sprintf("\tLBR  .L_%s_b%d\n", fName, term.FalseBlock.ID))

		case *ir.Return:
			if term.Val != nil {
				sz := b.getTypeSize(term.Val.Type())
				if sz <= 2 {
					b.loadVal(term.Val, "R7")
				} else {
					b.emitAddrToRC(b.retPtrSlot)
					b.buf.WriteString("\tLDA RC\n\tPHI RA\n\tLDN RC\n\tPLO RA\n")
					b.emitLoadAddr("RC", term.Val)
					b.buf.WriteString("\tGHI RA\n\tPHI RD\n\tGLO RA\n\tPLO RD\n")
					b.buf.WriteString("\tGHI RC\n\tPHI RA\n\tGLO RC\n\tPLO RA\n")
					b.buf.WriteString("\tGHI RD\n\tPHI RC\n\tGLO RD\n\tPLO RC\n")
					b.emitMemCopy(sz)
				}
			}
			b.buf.WriteString("\tGHI RB\n\tPHI R2\n\tGLO RB\n\tPLO R2\n")
			b.buf.WriteString("\tINC R2\n\tLDA R2\n\tPHI RB\n\tLDN R2\n\tPLO RB\n")
			b.buf.WriteString("\tSEP R5\n")

		default:
			log.Panicf("emitFunc: unhandled terminator %T", blk.Terminator)
		}
	}
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tDB %d\n", v.Val))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tDW %d\n", uint16(v.Val)))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tDW v_%s\n", v.Global.Name))
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
			if fIdx < len(v.Fields) {
				b.emitData(v.Fields[fIdx])
			} else {
				b.buf.WriteString(fmt.Sprintf("\tDS %d\n", sz))
			}
			byteOffset += sz
		}
		structSize := b.getTypeSize(v.Type())
		if structSize > byteOffset {
			b.buf.WriteString(fmt.Sprintf("\tDS %d\n", structSize-byteOffset))
		}
	case *ir.ConstArray:
		for _, elem := range v.Elements {
			b.emitData(elem)
		}
	case *ir.StringLiteral:
		b.buf.WriteString(fmt.Sprintf("\tASCIZ %q\n", v.Value))
	default:
		log.Panicf("emitData: unsupported value %T", val)
	}
}

func (b *Backend) emitGlobals(prog *ir.Program) {
	b.buf.WriteString("\n; ── Global Data Section ───────────────────────────────────────────────────────\n")
	b.buf.WriteString("v_prelude._jmp_chain_:\n\tDW 0\n")
	b.buf.WriteString("v_prelude._panic_:\n\tDW 0\n")

	for _, g := range prog.Globals {
		if g.Name == "prelude._jmp_chain_" || g.Name == "prelude._panic_" {
			continue
		}
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
				b.buf.WriteString(fmt.Sprintf("\tDB %s\n", strings.Join(bytesList, ", ")))
				sz -= len(g.InitString)
				if sz > 0 {
					b.buf.WriteString(fmt.Sprintf("\tDS %d\n", sz))
				}
			}
		} else {
			if sz > 0 {
				b.buf.WriteString(fmt.Sprintf("\tDS %d\n", sz))
			}
		}
	}

	if len(b.rodata) > 0 {
		b.buf.WriteString("\n; ── Read-Only Data (Strings & Format Literals) ───────────────────────────────\n")
		b.buf.WriteString(".L_empty_chain_msg:\n\tASCIZ \"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"\n")
		for _, entry := range b.rodata {
			b.buf.WriteString(fmt.Sprintf("%s:\n\tASCIZ %q\n", entry.lbl, entry.str))
		}
	}
}

func (b *Backend) Generate(prog *ir.Program) string {
	b.program = prog
	b.buf.Reset()
	b.rodata = nil

	if b.IncludeCStart {
		b.buf.WriteString(CStartAsm)
		b.buf.WriteString("\n\n")
	}

	b.buf.WriteString("; ── CDP1802 Whole-Program Assembly (MiniGolf) ───────────────────────────────\n")

	hasFunc := func(name string) bool {
		for _, f := range prog.Functions {
			if f.EmitName() == name || f.Name == name {
				return true
			}
		}
		return false
	}

	if hasFunc("f_main__main") && !hasFunc("_main") && !hasFunc("main") {
		b.buf.WriteString("\n_main:\n")
		b.buf.WriteString("\tSEP R4\n\tDW  f_main__main\n")
		b.buf.WriteString("\tLOAD R7, 0\n")
		b.buf.WriteString("\tSEP R5\n")
	}

	for _, f := range prog.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
		}
	}

	b.emitGlobals(prog)

	return b.buf.String()
}
