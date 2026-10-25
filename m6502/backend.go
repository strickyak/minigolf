package m6502

import (
	"bytes"
	_ "embed"
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
	slots         map[int]int    // instruction ID -> positive byte offset from ZP_FP (>= 2)
	jmpSlots      map[int]int    // SetJmp ID -> positive jmpbuf offset from ZP_FP (>= 2)
	paramSlots    map[string]int // param name -> positive offset from ZP_FP (>= frameSize)
	typeMap       map[int]ir.Type
	frameSize     int
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
	return 1 // 6502 is byte-addressable with no hardware alignment restrictions
}

func (b *Backend) getTypeSize(typ ir.Type) int {
	switch typ.Name {
	case "byte", "bool":
		return 1
	case "word", "int", "uint", "const_integer", "noreturn":
		return 2
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

	// Bytes 0..1 at (ZP_FP)+0..1 hold the saved caller ZP_FP.
	// Local variable and instruction slots start at positive offset 2.
	currentOffset := 2

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

	// Allocate small (<= 2 byte) slots first so scalar locals/temps stay at
	// offsets <= 254 even when the function has large struct/array locals.
	for pass := 0; pass < 2; pass++ {
		for _, blk := range f.Blocks {
			for _, instr := range blk.Instructions {
				id := instr.GetID()
				canon := b.resolveSlot(f, id)

				if _, ok := instr.(*ir.SetJmp); ok && pass == 1 {
					b.jmpSlots[id] = currentOffset
					currentOffset += 16
				}

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

				if instr.Type().Equals(ir.TypeVoid) || instr.Type().Name == "" {
					continue
				}
				size := b.getTypeSize(instr.Type())
				if size == 0 {
					size = 1
				}
				if (pass == 0 && size <= 2) || (pass == 1 && size > 2) {
					b.slots[canon] = currentOffset
					b.slots[id] = currentOffset
					currentOffset += alignVal(size, 2)
				}
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

	b.frameSize = alignVal(currentOffset, 2)

	// Caller's pushed arguments sit at ZP_FP + b.frameSize
	paramOffset := b.frameSize
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
}

func (b *Backend) emitLoadConst(reg string, val uint16) {
	if val == 0 && reg == "ZP_R0" {
		b.buf.WriteString("\tJSR __zero_r0\n")
		return
	}
	if val <= 255 {
		switch reg {
		case "ZP_R0":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __const_r0_y\n", val))
			return
		case "ZP_R1":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __const_r1_y\n", val))
			return
		}
	}
	lo := byte(val & 0xFF)
	hi := byte((val >> 8) & 0xFF)
	if lo == hi {
		b.buf.WriteString(fmt.Sprintf("\tLDA #$%02X\n\tSTA %s\n\tSTA %s+1\n", lo, reg, reg))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tLDA #$%02X\n\tSTA %s\n\tLDA #$%02X\n\tSTA %s+1\n", lo, reg, hi, reg))
	}
}

func (b *Backend) emitLoadSym(reg string, sym string) {
	b.buf.WriteString(fmt.Sprintf("\tLDA #<%s\n\tSTA %s\n\tLDA #>%s\n\tSTA %s+1\n", sym, reg, sym, reg))
}

func (b *Backend) emitCopyReg(dst, src string) {
	if dst == src {
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tLDA %s\n\tSTA %s\n\tLDA %s+1\n\tSTA %s+1\n", src, dst, src, dst))
}

// emitAddrToReg computes reg = ZP_FP + off (where off >= 0).
// Uses only A/Y; never clobbers any other ZP register.
func (b *Backend) emitAddrToReg(reg string, off int) {
	if off == 0 {
		b.emitCopyReg(reg, "ZP_FP")
		return
	}
	if off <= 255 {
		switch reg {
		case "ZP_R0":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __addr_r0_y\n", off))
			return
		case "ZP_R1":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __addr_r1_y\n", off))
			return
		case "ZP_RA":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __addr_ra_y\n", off))
			return
		case "ZP_RC":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __addr_rc_y\n", off))
			return
		case "ZP_RD":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __addr_rd_y\n", off))
			return
		}
	}
	switch reg {
	case "ZP_R0":
		b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __addr_r0_ay\n", off&0xFF, (off>>8)&0xFF))
		return
	case "ZP_R1":
		b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __addr_r1_ay\n", off&0xFF, (off>>8)&0xFF))
		return
	case "ZP_RA":
		b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __addr_ra_ay\n", off&0xFF, (off>>8)&0xFF))
		return
	case "ZP_RC":
		b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __addr_rc_ay\n", off&0xFF, (off>>8)&0xFF))
		return
	case "ZP_RD":
		b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __addr_rd_ay\n", off&0xFF, (off>>8)&0xFF))
		return
	case "ZP_RE":
		b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __addr_re_ay\n", off&0xFF, (off>>8)&0xFF))
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tCLC\n\tLDA ZP_FP\n\tADC #$%02X\n\tSTA %s\n\tLDA ZP_FP+1\n\tADC #$%02X\n\tSTA %s+1\n",
		off&0xFF, reg, (off>>8)&0xFF, reg))
}

// emitAddConst adds offset to 16-bit Zero-Page register reg.
func (b *Backend) emitAddConst(reg string, offset int) {
	if offset == 0 {
		return
	}
	if offset > 0 && offset <= 255 {
		switch reg {
		case "ZP_R0":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __add_r0_y\n", offset))
			return
		case "ZP_RA":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __add_ra_y\n", offset))
			return
		case "ZP_RC":
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __add_rc_y\n", offset))
			return
		}
	}
	if offset == 1 {
		lbl := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tINC %s\n\tBNE %s\n\tINC %s+1\n%s:\n", reg, lbl, reg, lbl))
		return
	}
	if offset > 0 {
		b.buf.WriteString(fmt.Sprintf("\tCLC\n\tLDA %s\n\tADC #$%02X\n\tSTA %s\n\tLDA %s+1\n\tADC #$%02X\n\tSTA %s+1\n",
			reg, offset&0xFF, reg, reg, (offset>>8)&0xFF, reg))
	} else {
		neg := -offset
		b.buf.WriteString(fmt.Sprintf("\tSEC\n\tLDA %s\n\tSBC #$%02X\n\tSTA %s\n\tLDA %s+1\n\tSBC #$%02X\n\tSTA %s+1\n",
			reg, neg&0xFF, reg, reg, (neg>>8)&0xFF, reg))
	}
}

// emitLoadFromFrame loads 1 or 2 bytes at ZP_FP + off into reg.
// Uses only A, Y (and ZP_RE if off > 254).
func (b *Backend) emitLoadFromFrame(reg string, off int, sz int) {
	if sz == 1 {
		if off <= 255 {
			switch reg {
			case "ZP_R0":
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __load_r0_b_y\n", off))
				return
			case "ZP_R1":
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __load_r1_b_y\n", off))
				return
			}
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tLDA (ZP_FP),Y\n\tSTA %s\n\tLDA #0\n\tSTA %s+1\n", off, reg, reg))
		} else {
			if reg == "ZP_R0" {
				b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __load_r0_b_ay\n", off&0xFF, (off>>8)&0xFF))
				return
			}
			b.emitAddrToReg("ZP_RE", off)
			b.buf.WriteString(fmt.Sprintf("\tLDY #0\n\tLDA (ZP_RE),Y\n\tSTA %s\n\tLDA #0\n\tSTA %s+1\n", reg, reg))
		}
	} else {
		if off <= 254 {
			switch reg {
			case "ZP_R0":
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __load_r0_y\n", off))
				return
			case "ZP_R1":
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __load_r1_y\n", off))
				return
			case "ZP_RA":
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __load_ra_y\n", off))
				return
			case "ZP_RC":
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __load_rc_y\n", off))
				return
			}
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tLDA (ZP_FP),Y\n\tSTA %s\n\tINY\n\tLDA (ZP_FP),Y\n\tSTA %s+1\n", off, reg, reg))
		} else {
			switch reg {
			case "ZP_R0":
				b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __load_r0_ay\n", off&0xFF, (off>>8)&0xFF))
				return
			case "ZP_R1":
				b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __load_r1_ay\n", off&0xFF, (off>>8)&0xFF))
				return
			case "ZP_RA":
				b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __load_ra_ay\n", off&0xFF, (off>>8)&0xFF))
				return
			case "ZP_RC":
				b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __load_rc_ay\n", off&0xFF, (off>>8)&0xFF))
				return
			}
			b.emitAddrToReg("ZP_RE", off)
			b.buf.WriteString(fmt.Sprintf("\tLDY #0\n\tLDA (ZP_RE),Y\n\tSTA %s\n\tINY\n\tLDA (ZP_RE),Y\n\tSTA %s+1\n", reg, reg))
		}
	}
}

// emitStoreToFrame stores 1 or 2 bytes from ZP_R0 into ZP_FP + off.
func (b *Backend) emitStoreToFrame(off int, sz int) {
	if sz == 1 {
		if off <= 255 {
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __store_r0_b_y\n", off))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __store_r0_b_ay\n", off&0xFF, (off>>8)&0xFF))
		}
	} else {
		if off <= 254 {
			b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __store_r0_y\n", off))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tLDY #$%02X\n\tLDA #$%02X\n\tJSR __store_r0_ay\n", off&0xFF, (off>>8)&0xFF))
		}
	}
}

func (b *Backend) loadVal(val ir.Value, reg string) {
	switch v := val.(type) {
	case *ir.ConstWord:
		b.emitLoadConst(reg, uint16(v.Val))
	case *ir.ConstByte:
		b.emitLoadConst(reg, uint16(v.Val))
	case *ir.Sizeof:
		sz := b.getTypeSize(v.TargetTyp)
		b.emitLoadConst(reg, uint16(sz))
	case *ir.Parameter:
		off := b.paramSlots[v.Name]
		sz := b.getTypeSize(v.Typ)
		b.emitLoadFromFrame(reg, off, sz)
	case *ir.Global:
		sz := b.getTypeSize(v.Typ)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tLDA v_%s\n\tSTA %s\n\tLDA #0\n\tSTA %s+1\n", v.Name, reg, reg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tLDA v_%s\n\tSTA %s\n\tLDA v_%s+1\n\tSTA %s+1\n", v.Name, reg, v.Name, reg))
		}
	case *ir.AddressOfGlobal:
		b.emitLoadSym(reg, "v_"+v.Global.Name)
	case *ir.AddressOfFunc:
		b.emitLoadSym(reg, v.Func.EmitName())
	case *ir.StringLiteral:
		b.lblCount++
		lbl := fmt.Sprintf(".Lstr%d", b.lblCount)
		b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: v.Value})
		b.emitLoadSym(reg, lbl)
	case ir.Instruction:
		id := v.GetID()
		slot := b.slots[id]
		sz := b.getTypeSize(v.Type())
		b.emitLoadFromFrame(reg, slot, sz)
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
	b.emitStoreToFrame(slot, sz)
}

func (b *Backend) emitLoadAddr(reg string, val ir.Value) {
	switch v := val.(type) {
	case *ir.Parameter:
		off := b.paramSlots[v.Name]
		b.emitAddrToReg(reg, off)
	case ir.Instruction:
		slot := b.slots[v.GetID()]
		b.emitAddrToReg(reg, slot)
	case *ir.Global:
		b.emitLoadSym(reg, "v_"+v.Name)
	case *ir.AddressOfGlobal:
		b.emitLoadSym(reg, "v_"+v.Global.Name)
	case *ir.AddressOfFunc:
		b.emitLoadSym(reg, v.Func.EmitName())
	default:
		log.Panicf("emitLoadAddr: unsupported value %T", val)
	}
}

// emitMemCopy copies sz bytes from (ZP_RA) to (ZP_RC).
func (b *Backend) emitMemCopy(sz int) {
	if sz <= 0 {
		return
	}
	if sz == 1 {
		b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RA),Y\n\tSTA (ZP_RC),Y\n")
		return
	}
	if sz == 2 {
		b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RA),Y\n\tSTA (ZP_RC),Y\n\tINY\n\tLDA (ZP_RA),Y\n\tSTA (ZP_RC),Y\n")
		return
	}
	if sz <= 255 {
		b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __memcpy_y\n", sz))
		return
	}
	b.emitLoadConst("ZP_RD", uint16(sz))
	b.buf.WriteString("\tJSR __memcpy_rd\n")
}

// emitMemClear clears sz bytes at (ZP_RC) to 0.
func (b *Backend) emitMemClear(sz int) {
	if sz <= 0 {
		return
	}
	if sz == 1 {
		b.buf.WriteString("\tLDY #0\n\tLDA #0\n\tSTA (ZP_RC),Y\n")
		return
	}
	if sz == 2 {
		b.buf.WriteString("\tLDY #0\n\tLDA #0\n\tSTA (ZP_RC),Y\n\tINY\n\tSTA (ZP_RC),Y\n")
		return
	}
	if sz <= 255 {
		b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __memclear_y\n", sz))
		return
	}
	b.emitLoadConst("ZP_RD", uint16(sz))
	b.buf.WriteString("\tJSR __memclear_rd\n")
}

func (b *Backend) emitBinaryOp(i *ir.BinaryOp) {
	id := i.GetID()
	b.loadVal(i.Left, "ZP_R0")
	b.loadVal(i.Right, "ZP_R1")

	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()

	switch i.Op {
	case "add", "+":
		b.buf.WriteString("\tJSR __add16\n")
	case "sub", "-":
		b.buf.WriteString("\tJSR __sub16\n")
	case "mul", "*":
		b.buf.WriteString("\tJSR __mul16\n")
	case "div", "/":
		if isUnsigned {
			b.buf.WriteString("\tJSR __udiv16\n")
		} else {
			b.buf.WriteString("\tJSR __sdiv16\n")
		}
	case "mod", "%":
		if isUnsigned {
			b.buf.WriteString("\tJSR __umod16\n")
		} else {
			b.buf.WriteString("\tJSR __smod16\n")
		}
	case "and", "bitand", "&":
		b.buf.WriteString("\tJSR __and16\n")
	case "or", "bitor", "|":
		b.buf.WriteString("\tJSR __or16\n")
	case "xor", "bitxor", "^":
		b.buf.WriteString("\tJSR __xor16\n")
	case "andnot":
		b.buf.WriteString("\tLDA ZP_R1\n\tEOR #$FF\n\tSTA ZP_R1\n\tLDA ZP_R1+1\n\tEOR #$FF\n\tSTA ZP_R1+1\n\tJSR __and16\n")
	case "shl", "<<":
		b.buf.WriteString("\tJSR __shl16\n")
	case "shr", ">>":
		if isUnsigned {
			b.buf.WriteString("\tJSR __shr16\n")
		} else {
			b.buf.WriteString("\tJSR __sar16\n")
		}
	case "sar":
		b.buf.WriteString("\tJSR __sar16\n")
	default:
		log.Panicf("emitBinaryOp: unsupported op %s", i.Op)
	}
	b.storeResult(id)
}

func isZeroVal(val ir.Value) bool {
	if val == nil {
		return false
	}
	if cb, ok := val.(*ir.ConstByte); ok && cb.Val == 0 {
		return true
	}
	if cw, ok := val.(*ir.ConstWord); ok && cw.Val == 0 {
		return true
	}
	return false
}

func (b *Backend) emitCompare(i *ir.Compare) {
	id := i.GetID()
	if i.Op == "neq" && isZeroVal(i.Right) {
		if _, isCmp := i.Left.(*ir.Compare); isCmp || i.Left.Type().IsBool() {
			b.loadVal(i.Left, "ZP_R0")
			b.storeResult(id)
			return
		}
	}
	b.loadVal(i.Left, "ZP_R0")
	b.loadVal(i.Right, "ZP_R1")
	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()

	switch i.Op {
	case "eq":
		b.buf.WriteString("\tJSR __cmpeq16\n")
	case "neq":
		b.buf.WriteString("\tJSR __cmpne16\n")
	case "lt":
		if isUnsigned {
			b.buf.WriteString("\tJSR __cmplt16_u\n")
		} else {
			b.buf.WriteString("\tJSR __cmplt16_s\n")
		}
	case "lte":
		if isUnsigned {
			b.buf.WriteString("\tJSR __cmplte16_u\n")
		} else {
			b.buf.WriteString("\tJSR __cmplte16_s\n")
		}
	case "gt":
		if isUnsigned {
			b.buf.WriteString("\tJSR __cmpgt16_u\n")
		} else {
			b.buf.WriteString("\tJSR __cmpgt16_s\n")
		}
	case "gte":
		if isUnsigned {
			b.buf.WriteString("\tJSR __cmpgte16_u\n")
		} else {
			b.buf.WriteString("\tJSR __cmpgte16_s\n")
		}
	default:
		log.Panicf("emitCompare: unsupported op %s", i.Op)
	}
	b.storeResult(id)
}

func (b *Backend) emitUnaryOp(i *ir.UnaryOp) {
	id := i.GetID()
	b.loadVal(i.Operand, "ZP_R0")
	switch i.Op {
	case "neg":
		b.buf.WriteString("\tJSR __neg_r0\n")
	case "not", "bitnot":
		b.buf.WriteString("\tLDA ZP_R0\n\tEOR #$FF\n\tSTA ZP_R0\n\tLDA ZP_R0+1\n\tEOR #$FF\n\tSTA ZP_R0+1\n")
	default:
		log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
	}
	b.storeResult(id)
}

func (b *Backend) emitSubSP(n int) {
	if n <= 0 {
		return
	}
	if n <= 255 {
		b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __sub_sp_y\n", n))
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tSEC\n\tLDA ZP_SP\n\tSBC #$%02X\n\tSTA ZP_SP\n\tSTA ZP_RC\n\tLDA ZP_SP+1\n\tSBC #$%02X\n\tSTA ZP_SP+1\n\tSTA ZP_RC+1\n",
		n&0xFF, (n>>8)&0xFF))
}

func (b *Backend) emitCallStackCleanup(totalArgBytes int) {
	if totalArgBytes <= 0 {
		return
	}
	if totalArgBytes <= 255 {
		b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __add_sp_y\n", totalArgBytes))
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tCLC\n\tLDA ZP_SP\n\tADC #$%02X\n\tSTA ZP_SP\n\tLDA ZP_SP+1\n\tADC #$%02X\n\tSTA ZP_SP+1\n",
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
			b.emitLoadSym("ZP_R0", strLbl)
			b.buf.WriteString("\tJSR __push_r0\n")
		} else {
			b.loadVal(arg, "ZP_R0")
			b.buf.WriteString("\tJSR __push_r0\n")
		}
	}

	b.emitLoadSym("ZP_R0", fmtLabel)
	b.buf.WriteString("\tJSR __push_r0\n")
	b.buf.WriteString("\tJSR _printf\n")

	cleanup := (len(dataArgs) + 1) * 2
	b.emitCallStackCleanup(cleanup)
}

func (b *Backend) emitBuiltinCall(i *ir.BuiltinCall) {
	switch i.Name {
	case "print", "println":
		b.emitPrint(i.Name == "println", i.Args)
	case "exit":
		if len(i.Args) > 0 {
			b.loadVal(i.Args[0], "ZP_R0")
		} else {
			b.emitLoadConst("ZP_R0", 0)
		}
		b.buf.WriteString("\tJSR __push_r0\n\tJSR _exit\n")
	case "panic", "_panic_":
		if len(i.Args) > 0 {
			if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
				b.lblCount++
				lbl := fmt.Sprintf(".Lpanic%d", b.lblCount)
				b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: strLit.Value})
				b.emitLoadSym("ZP_R0", lbl)
				b.buf.WriteString("\tJSR __push_r0\n")
			} else {
				b.loadVal(i.Args[0], "ZP_R0")
				b.buf.WriteString("\tJSR __push_r0\n")
			}
		} else {
			b.emitLoadConst("ZP_R0", 0)
			b.buf.WriteString("\tJSR __push_r0\n")
		}
		b.buf.WriteString("\tJSR _panic\n\tLDY #2\n\tJSR __add_sp_y\n")
	case "_unlink_jmp_":
		lblDone := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tLDA v_prelude._jmp_chain_\n\tORA v_prelude._jmp_chain_+1\n\tBEQ %s\n", lblDone))
		b.buf.WriteString("\tLDA v_prelude._jmp_chain_\n\tSTA ZP_RC\n\tLDA v_prelude._jmp_chain_+1\n\tSTA ZP_RC+1\n")
		b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RC),Y\n\tSTA v_prelude._jmp_chain_\n\tINY\n\tLDA (ZP_RC),Y\n\tSTA v_prelude._jmp_chain_+1\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	case "_propagate_panic_":
		lblDone := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tLDA v_prelude._panic_\n\tORA v_prelude._panic_+1\n\tBEQ %s\n\tJMP __longjmp_chain\n%s:\n", lblDone, lblDone))
	default:
		log.Panicf("emitBuiltinCall: unhandled builtin %s", i.Name)
	}
}

func (b *Backend) emitSetJmp(i *ir.SetJmp) {
	id := i.GetID()
	jmpSlot := b.jmpSlots[id]
	lblResume := b.nextLabel()
	lblDone := b.nextLabel()

	// Compute jmpbuf address in ZP_RC
	b.emitAddrToReg("ZP_RC", jmpSlot)

	// Save prev chain at ZP_RC+0..1
	b.buf.WriteString("\tLDY #0\n\tLDA v_prelude._jmp_chain_\n\tSTA (ZP_RC),Y\n\tINY\n\tLDA v_prelude._jmp_chain_+1\n\tSTA (ZP_RC),Y\n")

	// Update v_prelude._jmp_chain_ = ZP_RC
	b.buf.WriteString("\tLDA ZP_RC\n\tSTA v_prelude._jmp_chain_\n\tLDA ZP_RC+1\n\tSTA v_prelude._jmp_chain_+1\n")

	// Save resume PC at ZP_RC+2..3
	b.buf.WriteString(fmt.Sprintf("\tINY\n\tLDA #<%s\n\tSTA (ZP_RC),Y\n\tINY\n\tLDA #>%s\n\tSTA (ZP_RC),Y\n", lblResume, lblResume))

	// Save ZP_SP at ZP_RC+4..5
	b.buf.WriteString("\tINY\n\tLDA ZP_SP\n\tSTA (ZP_RC),Y\n\tINY\n\tLDA ZP_SP+1\n\tSTA (ZP_RC),Y\n")

	// Save ZP_FP at ZP_RC+6..7
	b.buf.WriteString("\tINY\n\tLDA ZP_FP\n\tSTA (ZP_RC),Y\n\tINY\n\tLDA ZP_FP+1\n\tSTA (ZP_RC),Y\n")

	// Save hardware stack pointer S at ZP_RC+8
	b.buf.WriteString("\tINY\n\tTSX\n\tTXA\n\tSTA (ZP_RC),Y\n")

	b.emitLoadConst("ZP_R0", 0)
	b.buf.WriteString(fmt.Sprintf("\tJMP %s\n", lblDone))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblResume))
	b.emitLoadConst("ZP_R0", 1)
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	b.storeResult(id)
}

func (b *Backend) emitLongJmp(i *ir.LongJmp) {
	b.loadVal(i.JmpBuf, "ZP_RC")
	b.buf.WriteString("\tJMP __longjmp_rc\n")
}

func (b *Backend) hasPhiAssignments(currentBlock *ir.BasicBlock, targetBlock *ir.BasicBlock) bool {
	for _, instr := range targetBlock.Instructions {
		phi, ok := instr.(*ir.Phi)
		if !ok {
			continue
		}
		for _, edge := range phi.Edges {
			if edge.Block == currentBlock && edge.Value != nil {
				return true
			}
		}
	}
	return false
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
						b.emitLoadAddr("ZP_RA", edge.Value)
						b.emitAddrToReg("ZP_RC", b.slots[phi.GetID()])
						b.emitMemCopy(sz)
					} else {
						b.loadVal(edge.Value, "ZP_R0")
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
		if _, ok := b.slots[id]; ok {
			b.loadVal(i, "ZP_R0")
			b.storeResult(id)
		}

	case *ir.ZeroInit:
		sz := b.getTypeSize(i.Typ)
		b.emitAddrToReg("ZP_RC", b.slots[id])
		b.emitMemClear(sz)

	case *ir.AddressOfLocal:
		b.emitLoadAddr("ZP_R0", i.Local)
		b.storeResult(id)

	case *ir.AddressOfGlobal:
		b.emitLoadSym("ZP_R0", "v_"+i.Global.Name)
		b.storeResult(id)

	case *ir.AddressOfFunc:
		b.emitLoadSym("ZP_R0", i.Func.EmitName())
		b.storeResult(id)

	case *ir.Load:
		sz := b.getTypeSize(i.Global.Typ)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tLDA v_%s\n\tSTA ZP_R0\n\tLDA #0\n\tSTA ZP_R0+1\n", i.Global.Name))
			b.storeResult(id)
		} else if sz == 2 {
			b.buf.WriteString(fmt.Sprintf("\tLDA v_%s\n\tSTA ZP_R0\n\tLDA v_%s+1\n\tSTA ZP_R0+1\n", i.Global.Name, i.Global.Name))
			b.storeResult(id)
		} else {
			b.emitLoadSym("ZP_RA", "v_"+i.Global.Name)
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.emitMemCopy(sz)
		}

	case *ir.Store:
		sz := b.getTypeSize(i.Global.Typ)
		if sz == 1 {
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString(fmt.Sprintf("\tLDA ZP_R0\n\tSTA v_%s\n", i.Global.Name))
		} else if sz == 2 {
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString(fmt.Sprintf("\tLDA ZP_R0\n\tSTA v_%s\n\tLDA ZP_R0+1\n\tSTA v_%s+1\n", i.Global.Name, i.Global.Name))
		} else {
			b.emitLoadAddr("ZP_RA", i.Val)
			b.emitLoadSym("ZP_RC", "v_"+i.Global.Name)
			b.emitMemCopy(sz)
		}

	case *ir.LoadPtr:
		sz := b.getTypeSize(i.Typ)
		if sz == 1 {
			b.loadVal(i.Ptr, "ZP_RC")
			b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RC),Y\n\tSTA ZP_R0\n\tLDA #0\n\tSTA ZP_R0+1\n")
			b.storeResult(id)
		} else if sz == 2 {
			b.loadVal(i.Ptr, "ZP_RC")
			b.buf.WriteString("\tJSR __load_r0_rc\n")
			b.storeResult(id)
		} else {
			b.loadVal(i.Ptr, "ZP_RA")
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.emitMemCopy(sz)
		}

	case *ir.StorePtr:
		sz := b.getTypeSize(i.Val.Type())
		if sz == 1 {
			b.loadVal(i.Ptr, "ZP_RC")
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString("\tLDY #0\n\tLDA ZP_R0\n\tSTA (ZP_RC),Y\n")
		} else if sz == 2 {
			b.loadVal(i.Ptr, "ZP_RC")
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString("\tJSR __store_r0_rc\n")
		} else {
			b.loadVal(i.Ptr, "ZP_RC")
			b.emitLoadAddr("ZP_RA", i.Val)
			b.emitMemCopy(sz)
		}

	case *ir.ExtractField:
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		b.emitLoadAddr("ZP_RA", i.Struct)
		b.emitAddConst("ZP_RA", byteOffset)
		if fieldSize == 1 {
			b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RA),Y\n\tSTA ZP_R0\n\tLDA #0\n\tSTA ZP_R0+1\n")
			b.storeResult(id)
		} else if fieldSize == 2 {
			b.buf.WriteString("\tJSR __load_r0_ra\n")
			b.storeResult(id)
		} else {
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.emitMemCopy(fieldSize)
		}

	case *ir.InsertField:
		structSize := b.getTypeSize(i.Struct.Type())
		b.emitLoadAddr("ZP_RA", i.Struct)
		b.emitAddrToReg("ZP_RC", b.slots[id])
		b.emitMemCopy(structSize)

		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal(i.Val, "ZP_R0")
			b.emitAddrToReg("ZP_RC", b.slots[id]+byteOffset)
			b.buf.WriteString("\tLDY #0\n\tLDA ZP_R0\n\tSTA (ZP_RC),Y\n")
		} else if fieldSize == 2 {
			b.loadVal(i.Val, "ZP_R0")
			b.emitAddrToReg("ZP_RC", b.slots[id]+byteOffset)
			b.buf.WriteString("\tJSR __store_r0_rc\n")
		} else {
			b.emitLoadAddr("ZP_RA", i.Val)
			b.emitAddrToReg("ZP_RC", b.slots[id]+byteOffset)
			b.emitMemCopy(fieldSize)
		}

	case *ir.ExtractFieldPtr:
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		b.loadVal(i.Ptr, "ZP_RA")
		b.emitAddConst("ZP_RA", byteOffset)
		if fieldSize == 1 {
			b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RA),Y\n\tSTA ZP_R0\n\tLDA #0\n\tSTA ZP_R0+1\n")
			b.storeResult(id)
		} else if fieldSize == 2 {
			b.buf.WriteString("\tJSR __load_r0_ra\n")
			b.storeResult(id)
		} else {
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.emitMemCopy(fieldSize)
		}

	case *ir.InsertFieldPtr:
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		b.loadVal(i.Ptr, "ZP_RC")
		b.emitAddConst("ZP_RC", byteOffset)
		if fieldSize == 1 {
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString("\tLDY #0\n\tLDA ZP_R0\n\tSTA (ZP_RC),Y\n")
		} else if fieldSize == 2 {
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString("\tJSR __store_r0_rc\n")
		} else {
			b.emitLoadAddr("ZP_RA", i.Val)
			b.emitMemCopy(fieldSize)
		}

	case *ir.ExtractElement:
		eltSize := b.getTypeSize(i.Typ)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			b.emitLoadAddr("ZP_RA", i.Array)
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConst("ZP_RA", byteOffset)
		} else {
			b.loadVal(i.Index, "ZP_R0")
			if eltSize > 1 {
				b.emitLoadConst("ZP_R1", uint16(eltSize))
				b.buf.WriteString("\tJSR __mul16\n")
			}
			b.emitLoadAddr("ZP_R1", i.Array)
			b.buf.WriteString("\tJSR __add16\n")
			b.emitCopyReg("ZP_RA", "ZP_R0")
		}
		if eltSize == 1 {
			b.buf.WriteString("\tLDY #0\n\tLDA (ZP_RA),Y\n\tSTA ZP_R0\n\tLDA #0\n\tSTA ZP_R0+1\n")
			b.storeResult(id)
		} else if eltSize == 2 {
			b.buf.WriteString("\tJSR __load_r0_ra\n")
			b.storeResult(id)
		} else {
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.emitMemCopy(eltSize)
		}

	case *ir.InsertElement:
		arraySize := b.getTypeSize(i.Array.Type())
		b.emitLoadAddr("ZP_RA", i.Array)
		b.emitAddrToReg("ZP_RC", b.slots[id])
		b.emitMemCopy(arraySize)

		eltSize := b.getTypeSize(i.Val.Type())
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddrToReg("ZP_RD", b.slots[id]+byteOffset)
		} else {
			b.loadVal(i.Index, "ZP_R0")
			if eltSize > 1 {
				b.emitLoadConst("ZP_R1", uint16(eltSize))
				b.buf.WriteString("\tJSR __mul16\n")
			}
			b.emitAddrToReg("ZP_R1", b.slots[id])
			b.buf.WriteString("\tJSR __add16\n")
			b.emitCopyReg("ZP_RD", "ZP_R0")
		}
		if eltSize == 1 {
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString("\tLDY #0\n\tLDA ZP_R0\n\tSTA (ZP_RD),Y\n")
		} else if eltSize == 2 {
			b.loadVal(i.Val, "ZP_R0")
			b.buf.WriteString("\tJSR __store_r0_rd\n")
		} else {
			b.emitLoadAddr("ZP_RA", i.Val)
			b.emitCopyReg("ZP_RC", "ZP_RD")
			b.emitMemCopy(eltSize)
		}

	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		b.loadVal(i.Ptr, "ZP_R0")
		b.emitAddConst("ZP_R0", byteOffset)
		b.storeResult(id)

	case *ir.AddressOfElement:
		eltSize := b.getTypeSize(i.ArrayPtr.Type().PointedType().ArrayElementType())
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.loadVal(i.ArrayPtr, "ZP_R0")
			b.emitAddConst("ZP_R0", byteOffset)
		} else {
			b.loadVal(i.Index, "ZP_R0")
			if eltSize > 1 {
				b.emitLoadConst("ZP_R1", uint16(eltSize))
				b.buf.WriteString("\tJSR __mul16\n")
			}
			b.loadVal(i.ArrayPtr, "ZP_R1")
			b.buf.WriteString("\tJSR __add16\n")
		}
		b.storeResult(id)

	case *ir.BinaryOp:
		b.emitBinaryOp(i)

	case *ir.Compare:
		b.emitCompare(i)

	case *ir.UnaryOp:
		b.emitUnaryOp(i)

	case *ir.Cast:
		dstSz := b.getTypeSize(i.Typ)
		if dstSz > 2 {
			b.emitLoadAddr("ZP_RA", i.Operand)
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.emitMemCopy(dstSz)
		} else {
			b.loadVal(i.Operand, "ZP_R0")
			if dstSz == 1 {
				b.buf.WriteString("\tLDA #0\n\tSTA ZP_R0+1\n")
			}
			b.storeResult(id)
		}

	case *ir.Call:
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			sz := b.getTypeSize(arg.Type())
			pushSize := alignVal(sz, 2)
			totalArgBytes += pushSize
			if sz > 2 {
				b.emitLoadAddr("ZP_RA", arg)
				b.emitSubSP(pushSize)
				b.emitMemCopy(sz)
			} else {
				b.loadVal(arg, "ZP_R0")
				b.buf.WriteString("\tJSR __push_r0\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		if retSize > 2 {
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.buf.WriteString("\tJSR __push_rc\n")
			totalArgBytes += 2
		}
		b.buf.WriteString(fmt.Sprintf("\tJSR %s\n", i.Func.EmitName()))
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
				b.emitLoadAddr("ZP_RA", arg)
				b.emitSubSP(pushSize)
				b.emitMemCopy(sz)
			} else {
				b.loadVal(arg, "ZP_R0")
				b.buf.WriteString("\tJSR __push_r0\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		if retSize > 2 {
			b.emitAddrToReg("ZP_RC", b.slots[id])
			b.buf.WriteString("\tJSR __push_rc\n")
			totalArgBytes += 2
		}
		b.loadVal(i.FuncPtr, "ZP_RC")
		b.buf.WriteString("\tJSR __call_rc\n")
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

	b.buf.WriteString(fmt.Sprintf("\n; ── Function: %s (frameSize=%d) ─────────────────────────────────────────\n", fName, b.frameSize))
	b.buf.WriteString(fmt.Sprintf("%s:\n", fName))

	// Function prologue
	if b.frameSize <= 255 {
		b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJSR __enter_frame_y\n", b.frameSize))
	} else {
		b.emitLoadConst("ZP_RE", uint16(b.frameSize))
		b.buf.WriteString("\tJSR __enter_frame_re\n")
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
			b.buf.WriteString(fmt.Sprintf("\tJMP .L_%s_b%d\n", fName, term.Target.ID))

		case *ir.Branch:
			b.loadVal(term.Condition, "ZP_R0")
			hasTruePhi := b.hasPhiAssignments(blk, term.TrueBlock)
			hasFalsePhi := b.hasPhiAssignments(blk, term.FalseBlock)

			trueTarget := fmt.Sprintf(".L_%s_b%d", fName, term.TrueBlock.ID)
			falseTarget := fmt.Sprintf(".L_%s_b%d", fName, term.FalseBlock.ID)
			if hasTruePhi {
				trueTarget = fmt.Sprintf(".L_%s_b%d_true", fName, blk.ID)
			}
			if hasFalsePhi {
				falseTarget = fmt.Sprintf(".L_%s_b%d_false", fName, blk.ID)
			}

			b.buf.WriteString("\tLDA ZP_R0\n")
			b.buf.WriteString(fmt.Sprintf("\tBNE %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tJMP %s\n", falseTarget))

			if hasTruePhi {
				b.buf.WriteString(fmt.Sprintf("%s:\n", trueTarget))
				b.emitPhiAssignments(blk, term.TrueBlock)
				b.buf.WriteString(fmt.Sprintf("\tJMP .L_%s_b%d\n", fName, term.TrueBlock.ID))
			}

			if hasFalsePhi {
				b.buf.WriteString(fmt.Sprintf("%s:\n", falseTarget))
				b.emitPhiAssignments(blk, term.FalseBlock)
				b.buf.WriteString(fmt.Sprintf("\tJMP .L_%s_b%d\n", fName, term.FalseBlock.ID))
			}

		case *ir.Return:
			if term.Val != nil {
				sz := b.getTypeSize(term.Val.Type())
				if sz <= 2 {
					b.loadVal(term.Val, "ZP_R0")
				} else {
					b.emitLoadFromFrame("ZP_RC", b.retPtrSlot, 2)
					b.emitLoadAddr("ZP_RA", term.Val)
					b.emitMemCopy(sz)
				}
			}
			if b.frameSize <= 255 {
				b.buf.WriteString(fmt.Sprintf("\tLDY #%d\n\tJMP __leave_frame_y\n", b.frameSize))
			} else {
				b.emitLoadConst("ZP_RE", uint16(b.frameSize))
				b.buf.WriteString("\tJMP __leave_frame_re\n")
			}

		case *ir.LongJmp:
			b.emitLongJmp(term)

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

	b.buf.WriteString("; ── MOS 6502 Whole-Program Assembly (MiniGolf) ──────────────────────────────\n")

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
		b.buf.WriteString("\tJSR f_main__main\n")
		b.buf.WriteString("\tLDA #0\n\tSTA ZP_R0\n\tSTA ZP_R0+1\n")
		b.buf.WriteString("\tRTS\n")
	}

	for _, f := range prog.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
		}
	}

	b.emitGlobals(prog)

	return optimizeAsm(b.buf.String())
}

func normLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func optimizeAsm(asm string) string {
	lines := strings.Split(asm, "\n")

	for pass := 0; pass < 4; pass++ {
		changed := false
		var out []string
		n := len(lines)

		for i := 0; i < n; i++ {
			line := lines[i]
			trimmed := strings.TrimSpace(line)

			if trimmed == "" || strings.HasPrefix(trimmed, ";") {
				out = append(out, line)
				continue
			}

			norm := normLine(trimmed)

			findNext := func(from int) int {
				for j := from; j < n; j++ {
					t := strings.TrimSpace(lines[j])
					if t != "" && !strings.HasPrefix(t, ";") {
						return j
					}
				}
				return -1
			}

			next1 := findNext(i + 1)

			// 1. Redundant JMP to immediate next label:
			//    JMP .L_foo
			//    .L_foo:
			if strings.HasPrefix(norm, "JMP ") && !strings.HasPrefix(norm, "JMP (") {
				target := strings.TrimSpace(strings.TrimPrefix(norm, "JMP "))
				if next1 >= 0 && normLine(lines[next1]) == target+":" {
					changed = true
					continue
				}
			}

			// 2. Redundant 1-byte or 2-byte store to slot followed immediately by reload of same slot into ZP_R0 or ZP_RC:
			//    LDY #k
			//    JSR __store_r0_y (or __store_r0_b_y)
			//    LDY #k
			//    JSR __load_r0_y / JSR __load_rc_y (or __load_r0_b_y)
			if strings.HasPrefix(norm, "LDY #") && next1 >= 0 {
				n1Norm := normLine(lines[next1])
				if n1Norm == "JSR __store_r0_y" {
					next2 := findNext(next1 + 1)
					if next2 >= 0 && normLine(lines[next2]) == norm {
						next3 := findNext(next2 + 1)
						if next3 >= 0 {
							n3Norm := normLine(lines[next3])
							if n3Norm == "JSR __load_r0_y" || n3Norm == "JSR __load_rc_y" {
								lines[next2] = ";" + lines[next2]
								lines[next3] = ";" + lines[next3]
								changed = true
							}
						}
					}
				} else if n1Norm == "JSR __store_r0_b_y" {
					next2 := findNext(next1 + 1)
					if next2 >= 0 && normLine(lines[next2]) == norm {
						next3 := findNext(next2 + 1)
						if next3 >= 0 && normLine(lines[next3]) == "JSR __load_r0_b_y" {
							lines[next2] = ";" + lines[next2]
							lines[next3] = ";" + lines[next3]
							changed = true
						}
					}
				}
			}

			// 3. Branch inversion over unconditional JMP:
			//    BNE L1
			//    JMP L2
			//    L1:
			// -> BEQ L2
			//    L1:
			if (strings.HasPrefix(norm, "BNE ") || strings.HasPrefix(norm, "BEQ ")) && next1 >= 0 {
				parts := strings.Fields(norm)
				if len(parts) == 2 {
					bcc := parts[0]
					t1 := parts[1]
					n1Norm := normLine(lines[next1])
					if strings.HasPrefix(n1Norm, "JMP ") && !strings.HasPrefix(n1Norm, "JMP (") {
						t2 := strings.TrimSpace(strings.TrimPrefix(n1Norm, "JMP "))
						next2 := findNext(next1 + 1)
						if next2 >= 0 && normLine(lines[next2]) == t1+":" {
							inv := "BEQ"
							if bcc == "BEQ" {
								inv = "BNE"
							}
							out = append(out, fmt.Sprintf("\t%s %s", inv, t2))
							lines[next1] = ";" + lines[next1]
							changed = true
							continue
						}
					}
				}
			}

			out = append(out, line)
		}
		lines = out
		if !changed {
			break
		}
	}

	return strings.Join(lines, "\n")
}
