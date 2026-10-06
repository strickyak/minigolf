package z80

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
	slots      map[int]int    // instruction ID -> byte offset below IX (accessed as -(offset)(ix))
	jmpSlots   map[int]int    // SetJmp instruction ID -> jmpbuf stack offset below IX
	paramSlots map[string]int // param name -> stack offset above IX (accessed as offset(ix))
	typeMap       map[int]ir.Type
	fusedCompares map[int]bool
	stackSize     int
	frameBias     int
	retPtrSlot    int
	currentFunc   *ir.Function
	lblCount      int
	rodata        []rodataEntry
}

func ixDisp(d int) string {
	if d >= 0 {
		return fmt.Sprintf("(ix+%d)", d)
	}
	return fmt.Sprintf("(ix-%d)", -d)
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
		return 1 // Z80 is byte-addressable with no alignment restrictions
	}
	if typ.IsByte() || typ.IsBool() {
		return 1
	}
	if typ.IsWord() || typ.IsInt() || typ.IsConstInt() || typ.IsNoReturn() {
		return 1
	}
	if typ.IsAnArray() {
		return b.getTypeAlignment(typ.ArrayElementType())
	}
	if typ.IsAPointer() || typ.IsAFuncPtr() {
		return 1
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[typ.Name]; ok {
			return b.getTypeAlignment(def)
		}
	}
	return 1
}

func (b *Backend) getTypeSize(typ ir.Type) int {
	switch typ.Name {
	case "byte", "bool":
		return 1
	case "word", "int", "uint", "const_integer", "noreturn":
		return 2 // 16-bit word on Z80
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

func (b *Backend) getEltSize(arrType ir.Type) int {
	if arrType.IsAPointer() {
		arrType = arrType.PointedType()
	}
	if arrType.IsAnArray() {
		return b.getTypeSize(arrType.ArrayElementType())
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
	b.fusedCompares = make(map[int]bool)

	// Pre-scan for fused compare-and-branch sequences to avoid allocating slots for them
	for _, blk := range f.Blocks {
		if branch, ok := blk.Terminator.(*ir.Branch); ok {
			cond := branch.Condition
			if outerCmp, ok := cond.(*ir.Compare); ok && outerCmp.Op == "neq" {
				if isZero(outerCmp.Right) {
					if innerCmp, ok := outerCmp.Left.(*ir.Compare); ok {
						if countUses(f, outerCmp) <= 1 && countUses(f, innerCmp) <= 1 {
							b.fusedCompares[innerCmp.GetID()] = true
							b.fusedCompares[outerCmp.GetID()] = true
							if zeroInst, ok := outerCmp.Right.(ir.Instruction); ok {
								b.fusedCompares[zeroInst.GetID()] = true
							}
						}
					}
				}
			}
			if cmp, ok := cond.(*ir.Compare); ok && !b.fusedCompares[cmp.GetID()] {
				if countUses(f, cmp) <= 1 {
					b.fusedCompares[cmp.GetID()] = true
				}
			}
		}
	}

	// In Z80 calling convention:
	// Saved IX is at 0(ix), 1(ix).
	// Return address is at 2(ix), 3(ix).
	// Caller's pushed arguments start at 4(ix).
	paramOffset := 4
	for _, p := range f.Parameters {
		sz := b.getTypeSize(p.Typ)
		b.paramSlots[p.Name] = paramOffset
		// Arguments on stack are widened to at least 2 bytes
		paramOffset += alignVal(sz, 2)
	}

	currentOffset := 0
	retSize := b.getTypeSize(f.ReturnType)
	if retSize > 2 {
		currentOffset += 2
		b.retPtrSlot = currentOffset
	}

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

	// Align total local stack frame to 2 bytes
	b.stackSize = alignVal(currentOffset, 2)
}

func (b *Backend) slotsType(id int) ir.Type {
	if t, ok := b.typeMap[id]; ok {
		return t
	}
	return ir.TypeInt
}

func (b *Backend) emitAddConstToHL(n int) {
	if n == 0 {
		return
	}
	if n > 0 && n <= 3 {
		for k := 0; k < n; k++ {
			b.buf.WriteString("\tinc  hl\n")
		}
		return
	}
	if n < 0 && n >= -3 {
		for k := 0; k < -n; k++ {
			b.buf.WriteString("\tdec  hl\n")
		}
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tld   de, %d\n\tadd  hl, de\n", n))
}

func (b *Backend) emitLocalAddr(reg string, d int) {
	if reg == "hl" {
		if d == 0 {
			b.buf.WriteString("\tpush ix\n\tpop  hl\n")
		} else if d > 0 && d <= 3 {
			b.buf.WriteString("\tpush ix\n\tpop  hl\n")
			for k := 0; k < d; k++ {
				b.buf.WriteString("\tinc  hl\n")
			}
		} else if d < 0 && d >= -3 {
			b.buf.WriteString("\tpush ix\n\tpop  hl\n")
			for k := 0; k < -d; k++ {
				b.buf.WriteString("\tdec  hl\n")
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tpush ix\n\tpop  hl\n\tld   bc, %d\n\tadd  hl, bc\n", d))
		}
	} else if reg == "de" {
		if d == 0 {
			b.buf.WriteString("\tpush ix\n\tpop  de\n")
		} else if d > 0 && d <= 4 {
			b.buf.WriteString("\tpush ix\n\tpop  de\n")
			for k := 0; k < d; k++ {
				b.buf.WriteString("\tinc  de\n")
			}
		} else if d < 0 && d >= -4 {
			b.buf.WriteString("\tpush ix\n\tpop  de\n")
			for k := 0; k < -d; k++ {
				b.buf.WriteString("\tdec  de\n")
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tpush hl\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tex   de, hl\n\tpop  hl\n", d))
		}
	} else {
		log.Panicf("emitLocalAddr: unsupported register %s", reg)
	}
}

func (b *Backend) emitLoadAddr(reg string, val ir.Value) {
	switch v := val.(type) {
	case *ir.Parameter:
		off := b.paramSlots[v.Name]
		d := b.frameBias + off
		b.emitLocalAddr(reg, d)
	case ir.Instruction:
		canon := b.resolveSlot(b.currentFunc, v.GetID())
		off, ok := b.slots[canon]
		if !ok || off == 0 {
			off, ok = b.slots[v.GetID()]
		}
		if !ok || off == 0 {
			log.Panicf("emitLoadAddr: instruction %s (id=%d) has no stack slot in function %s", v, v.GetID(), b.currentFunc.Name)
		}
		d := b.frameBias - off
		b.emitLocalAddr(reg, d)
	case *ir.Global:
		b.buf.WriteString(fmt.Sprintf("\tld   %s, v_%s\n", reg, v.Name))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tld   %s, v_%s\n", reg, v.Global.Name))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tld   %s, %s\n", reg, v.Func.EmitName()))
	default:
		log.Panicf("emitLoadAddr: unhandled value type %T", val)
	}
}

func (b *Backend) getDirectGlobalAddr(val ir.Value) (string, bool) {
	switch v := val.(type) {
	case *ir.Global:
		return fmt.Sprintf("v_%s", v.Name), true
	case *ir.AddressOfGlobal:
		return fmt.Sprintf("v_%s", v.Global.Name), true
	case *ir.AddressOfField:
		if glob, ok := v.Ptr.(*ir.AddressOfGlobal); ok {
			structType := v.Ptr.Type().PointedType()
			byteOffset, _ := b.getFieldOffsetAndSize(structType, v.FieldIndex)
			if byteOffset > 0 {
				return fmt.Sprintf("v_%s + %d", glob.Global.Name, byteOffset), true
			}
			return fmt.Sprintf("v_%s", glob.Global.Name), true
		}
	case *ir.AddressOfElement:
		if glob, ok := v.ArrayPtr.(*ir.AddressOfGlobal); ok {
			if cIdx, ok2 := v.Index.(*ir.ConstWord); ok2 {
				eltSize := b.getEltSize(v.ArrayPtr.Type())
				byteOffset := int(cIdx.Val) * eltSize
				if byteOffset > 0 {
					return fmt.Sprintf("v_%s + %d", glob.Global.Name, byteOffset), true
				}
				return fmt.Sprintf("v_%s", glob.Global.Name), true
			}
		}
	}
	return "", false
}

func (b *Backend) loadLocal(off int, sz int, reg string) {
	d := b.frameBias - off
	fits := false
	if sz == 1 {
		fits = (d >= -126 && d <= 126)
	} else {
		fits = (d >= -126 && d <= 125)
	}

	if fits {
		switch reg {
		case "a":
			b.buf.WriteString(fmt.Sprintf("\tld   a, %s\n", ixDisp(d)))
		case "b":
			b.buf.WriteString(fmt.Sprintf("\tld   b, %s\n", ixDisp(d)))
		case "hl":
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   l, %s\n\tld   h, 0\n", ixDisp(d)))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   l, %s\n\tld   h, %s\n", ixDisp(d), ixDisp(d+1)))
			}
		case "de":
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   e, %s\n\tld   d, 0\n", ixDisp(d)))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   e, %s\n\tld   d, %s\n", ixDisp(d), ixDisp(d+1)))
			}
		}
		return
	}
	// off does not fit directly in IX displacement: compute address safely
	switch reg {
	case "a":
		b.buf.WriteString(fmt.Sprintf("\tpush de\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   a, (hl)\n\tpop  de\n", d))
	case "b":
		b.buf.WriteString(fmt.Sprintf("\tpush de\n\tpush hl\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   b, (hl)\n\tpop  hl\n\tpop  de\n", d))
	case "hl":
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tpush de\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   l, (hl)\n\tld   h, 0\n\tpop  de\n", d))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tpush de\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   a, (hl)\n\tinc  hl\n\tld   h, (hl)\n\tld   l, a\n\tpop  de\n", d))
		}
	case "de":
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tpush hl\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   e, (hl)\n\tld   d, 0\n\tpop  hl\n", d))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tpush hl\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n\tpop  hl\n", d))
		}
	}
}

func (b *Backend) loadVal(val ir.Value, reg string) {
	switch reg {
	case "a":
		switch v := val.(type) {
		case *ir.ConstByte:
			b.buf.WriteString(fmt.Sprintf("\tld   a, %d\n", v.Val))
		case *ir.ConstWord:
			b.buf.WriteString(fmt.Sprintf("\tld   a, %d\n", v.Val&0xFF))
		case *ir.Sizeof:
			sz := b.getTypeSize(v.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tld   a, %d\n", sz&0xFF))
		case *ir.Parameter:
			off := b.paramSlots[v.Name]
			d := b.frameBias + off
			b.buf.WriteString(fmt.Sprintf("\tld   a, %s\n", ixDisp(d)))
		case *ir.Global:
			b.buf.WriteString(fmt.Sprintf("\tld   a, (v_%s)\n", v.Name))
		case *ir.AddressOfGlobal:
			b.buf.WriteString(fmt.Sprintf("\tld   a, v_%s\n", v.Global.Name))
		case ir.Instruction:
			off := b.slots[v.GetID()]
			b.loadLocal(off, 1, "a")
		default:
			log.Panicf("loadVal(a): unhandled type %T", val)
		}

	case "b":
		switch v := val.(type) {
		case *ir.ConstByte:
			b.buf.WriteString(fmt.Sprintf("\tld   b, %d\n", v.Val))
		case *ir.ConstWord:
			b.buf.WriteString(fmt.Sprintf("\tld   b, %d\n", v.Val&0xFF))
		case *ir.Sizeof:
			sz := b.getTypeSize(v.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tld   b, %d\n", sz&0xFF))
		case *ir.Parameter:
			off := b.paramSlots[v.Name]
			d := b.frameBias + off
			b.buf.WriteString(fmt.Sprintf("\tld   b, %s\n", ixDisp(d)))
		case *ir.Global:
			b.buf.WriteString(fmt.Sprintf("\tld   a, (v_%s)\n\tld   b, a\n", v.Name))
		case *ir.AddressOfGlobal:
			b.buf.WriteString(fmt.Sprintf("\tld   b, v_%s\n", v.Global.Name))
		case ir.Instruction:
			off := b.slots[v.GetID()]
			b.loadLocal(off, 1, "b")
		default:
			log.Panicf("loadVal(b): unhandled type %T", val)
		}

	case "hl":
		switch v := val.(type) {
		case *ir.ConstByte:
			b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n", v.Val))
		case *ir.ConstWord:
			b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n", v.Val))
		case *ir.Sizeof:
			sz := b.getTypeSize(v.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n", sz))
		case *ir.AddressOfGlobal:
			b.buf.WriteString(fmt.Sprintf("\tld   hl, v_%s\n", v.Global.Name))
		case *ir.AddressOfFunc:
			b.buf.WriteString(fmt.Sprintf("\tld   hl, %s\n", v.Func.EmitName()))
		case *ir.AddressOfLocal:
			b.emitLoadAddr("hl", v.Local)
		case *ir.Parameter:
			sz := b.getTypeSize(v.Typ)
			off := b.paramSlots[v.Name]
			d := b.frameBias + off
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   l, %s\n\tld   h, 0\n", ixDisp(d)))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   l, %s\n\tld   h, %s\n", ixDisp(d), ixDisp(d+1)))
			}
		case *ir.Global:
			sz := b.getTypeSize(v.Typ)
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   a, (v_%s)\n\tld   l, a\n\tld   h, 0\n", v.Name))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, (v_%s)\n", v.Name))
			}
		case ir.Instruction:
			sz := b.getTypeSize(v.Type())
			off := b.slots[v.GetID()]
			b.loadLocal(off, sz, "hl")
		default:
			log.Panicf("loadVal(hl): unhandled type %T", val)
		}

	case "de":
		switch v := val.(type) {
		case *ir.ConstByte:
			b.buf.WriteString(fmt.Sprintf("\tld   de, %d\n", v.Val))
		case *ir.ConstWord:
			b.buf.WriteString(fmt.Sprintf("\tld   de, %d\n", v.Val))
		case *ir.Sizeof:
			sz := b.getTypeSize(v.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tld   de, %d\n", sz))
		case *ir.AddressOfGlobal:
			b.buf.WriteString(fmt.Sprintf("\tld   de, v_%s\n", v.Global.Name))
		case *ir.AddressOfFunc:
			b.buf.WriteString(fmt.Sprintf("\tld   de, %s\n", v.Func.EmitName()))
		case *ir.AddressOfLocal:
			b.emitLoadAddr("de", v.Local)
		case *ir.Parameter:
			sz := b.getTypeSize(v.Typ)
			off := b.paramSlots[v.Name]
			d := b.frameBias + off
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   e, %s\n\tld   d, 0\n", ixDisp(d)))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   e, %s\n\tld   d, %s\n", ixDisp(d), ixDisp(d+1)))
			}
		case *ir.Global:
			sz := b.getTypeSize(v.Typ)
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   a, (v_%s)\n\tld   e, a\n\tld   d, 0\n", v.Name))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   de, (v_%s)\n", v.Name))
			}
		case ir.Instruction:
			sz := b.getTypeSize(v.Type())
			off := b.slots[v.GetID()]
			b.loadLocal(off, sz, "de")
		default:
			log.Panicf("loadVal(de): unhandled type %T", val)
		}

	default:
		log.Panicf("loadVal: unsupported reg %s", reg)
	}
}

func (b *Backend) storeResult(id int) {
	sz := b.getTypeSize(b.slotsType(id))
	off, ok := b.slots[id]
	if !ok {
		log.Panicf("storeResult: instruction %d has no slot", id)
	}
	d := b.frameBias - off
	fits := false
	if sz == 1 {
		fits = (d >= -126 && d <= 126)
	} else {
		fits = (d >= -126 && d <= 125)
	}

	if sz == 1 {
		if fits {
			b.buf.WriteString(fmt.Sprintf("\tld   %s, a\n", ixDisp(d)))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   (hl), a\n", d))
		}
	} else {
		if fits {
			b.buf.WriteString(fmt.Sprintf("\tld   %s, l\n\tld   %s, h\n", ixDisp(d), ixDisp(d+1)))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld   c, l\n\tld   b, h\n\tpush ix\n\tpop  hl\n\tld   de, %d\n\tadd  hl, de\n\tld   (hl), c\n\tinc  hl\n\tld   (hl), b\n", d))
		}
	}
}

func (b *Backend) emitMemCopy(destAddr, srcAddr string, size int) {
	if size <= 0 {
		return
	}
	// HL = source, DE = dest
	if size == 1 {
		b.buf.WriteString(fmt.Sprintf("\tld   a, %s\n\tld   %s, a\n", srcAddr, destAddr))
		return
	}
	if size == 2 {
		b.buf.WriteString(fmt.Sprintf("\tld   a, %s\n\tld   %s, a\n\tinc  hl\n\tinc  de\n\tld   a, %s\n\tld   %s, a\n", srcAddr, destAddr, srcAddr, destAddr))
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tld   bc, %d\n\tldir\n", size))
}

func (b *Backend) storeToAddr(destAddr string, val ir.Value, size int) {
	if destAddr == "(de)" {
		if size == 1 {
			b.loadVal(val, "a")
			b.buf.WriteString("\tld   (de), a\n")
			return
		} else if size == 2 {
			b.loadVal(val, "hl")
			b.buf.WriteString("\tld   a, l\n\tld   (de), a\n\tinc  de\n\tld   a, h\n\tld   (de), a\n")
			return
		}
	}
	if size == 1 {
		b.loadVal(val, "a")
		b.buf.WriteString(fmt.Sprintf("\tld   %s, a\n", destAddr))
	} else if size == 2 {
		b.loadVal(val, "hl")
		if strings.HasPrefix(destAddr, "(v_") {
			b.buf.WriteString(fmt.Sprintf("\tld   %s, hl\n", destAddr))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld   a, l\n\tld   %s, a\n\tinc  de\n\tld   a, h\n\tld   %s, a\n", destAddr, destAddr))
		}
	} else {
		// Multi-byte copy
		if strings.HasPrefix(destAddr, "(v_") && strings.HasSuffix(destAddr, ")") {
			globalName := destAddr[1 : len(destAddr)-1]
			b.buf.WriteString(fmt.Sprintf("\tld   de, %s\n", globalName))
		}
		b.emitLoadAddr("hl", val)
		b.buf.WriteString(fmt.Sprintf("\tld   bc, %d\n\tldir\n", size))
	}
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					size := b.getTypeSize(phi.Typ)
					if size == 1 {
						b.loadVal(edge.Value, "a")
						b.storeResult(phi.GetID())
					} else if size == 2 {
						b.loadVal(edge.Value, "hl")
						b.storeResult(phi.GetID())
					} else {
						b.emitLoadAddr("hl", edge.Value)
						b.emitLoadAddr("de", phi)
						b.emitMemCopy("(de)", "(hl)", size)
					}
				}
			}
		}
	}
}

func (b *Backend) emitBinaryOp(i *ir.BinaryOp) {
	sz := b.getTypeSize(i.Typ)
	if sz == 1 {
		b.loadVal(i.Left, "a")
		if cb, ok := i.Right.(*ir.ConstByte); ok {
			switch i.Op {
			case "add":
				if cb.Val == 0 {
					// no-op
				} else if cb.Val == 1 {
					b.buf.WriteString("\tinc  a\n")
				} else if cb.Val == 2 {
					b.buf.WriteString("\tinc  a\n\tinc  a\n")
				} else {
					b.buf.WriteString(fmt.Sprintf("\tadd  a, %d\n", cb.Val))
				}
			case "sub":
				if cb.Val == 0 {
					// no-op
				} else if cb.Val == 1 {
					b.buf.WriteString("\tdec  a\n")
				} else if cb.Val == 2 {
					b.buf.WriteString("\tdec  a\n\tdec  a\n")
				} else {
					b.buf.WriteString(fmt.Sprintf("\tsub  %d\n", cb.Val))
				}
			case "and", "bitand":
				b.buf.WriteString(fmt.Sprintf("\tand  %d\n", cb.Val))
			case "or", "bitor":
				b.buf.WriteString(fmt.Sprintf("\tor   %d\n", cb.Val))
			case "xor", "bitxor":
				b.buf.WriteString(fmt.Sprintf("\txor  %d\n", cb.Val))
			default:
				b.loadVal(i.Right, "b")
				switch i.Op {
				case "andnot":
					b.buf.WriteString("\tld   a, b\n\tcpl\n\tld   b, a\n")
					b.loadVal(i.Left, "a")
					b.buf.WriteString("\tand  b\n")
				default:
					sz = 2
				}
			}
		} else if cw, ok := i.Right.(*ir.ConstWord); ok {
			v := cw.Val & 0xFF
			switch i.Op {
			case "add":
				if v == 0 {
					// no-op
				} else if v == 1 {
					b.buf.WriteString("\tinc  a\n")
				} else if v == 2 {
					b.buf.WriteString("\tinc  a\n\tinc  a\n")
				} else {
					b.buf.WriteString(fmt.Sprintf("\tadd  a, %d\n", v))
				}
			case "sub":
				if v == 0 {
					// no-op
				} else if v == 1 {
					b.buf.WriteString("\tdec  a\n")
				} else if v == 2 {
					b.buf.WriteString("\tdec  a\n\tdec  a\n")
				} else {
					b.buf.WriteString(fmt.Sprintf("\tsub  %d\n", v))
				}
			case "and", "bitand":
				b.buf.WriteString(fmt.Sprintf("\tand  %d\n", v))
			case "or", "bitor":
				b.buf.WriteString(fmt.Sprintf("\tor   %d\n", v))
			case "xor", "bitxor":
				b.buf.WriteString(fmt.Sprintf("\txor  %d\n", v))
			default:
				b.loadVal(i.Right, "b")
				switch i.Op {
				case "andnot":
					b.buf.WriteString("\tld   a, b\n\tcpl\n\tld   b, a\n")
					b.loadVal(i.Left, "a")
					b.buf.WriteString("\tand  b\n")
				default:
					sz = 2
				}
			}
		} else {
			b.loadVal(i.Right, "b")
			switch i.Op {
			case "add":
				b.buf.WriteString("\tadd  a, b\n")
			case "sub":
				b.buf.WriteString("\tsub  b\n")
			case "and", "bitand":
				b.buf.WriteString("\tand  b\n")
			case "or", "bitor":
				b.buf.WriteString("\tor   b\n")
			case "xor", "bitxor":
				b.buf.WriteString("\txor  b\n")
			case "andnot":
				b.buf.WriteString("\tld   a, b\n\tcpl\n\tld   b, a\n")
				b.loadVal(i.Left, "a")
				b.buf.WriteString("\tand  b\n")
			default:
				// Fall back to 16-bit operation
				sz = 2
			}
		}
		if sz == 1 {
			b.storeResult(i.GetID())
			return
		}
	}

	// 16-bit binary operations
	b.loadVal(i.Left, "hl")

	switch i.Op {
	case "add":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			switch cw.Val {
			case 0:
				// no-op
			case 1:
				b.buf.WriteString("\tinc  hl\n")
			case 2:
				b.buf.WriteString("\tinc  hl\n\tinc  hl\n")
			case 3:
				b.buf.WriteString("\tinc  hl\n\tinc  hl\n\tinc  hl\n")
			case 4:
				b.buf.WriteString("\tinc  hl\n\tinc  hl\n\tinc  hl\n\tinc  hl\n")
			default:
				b.loadVal(i.Right, "de")
				b.buf.WriteString("\tadd  hl, de\n")
			}
		} else {
			b.loadVal(i.Right, "de")
			b.buf.WriteString("\tadd  hl, de\n")
		}
	case "sub":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			switch cw.Val {
			case 0:
				// no-op
			case 1:
				b.buf.WriteString("\tdec  hl\n")
			case 2:
				b.buf.WriteString("\tdec  hl\n\tdec  hl\n")
			case 3:
				b.buf.WriteString("\tdec  hl\n\tdec  hl\n\tdec  hl\n")
			case 4:
				b.buf.WriteString("\tdec  hl\n\tdec  hl\n\tdec  hl\n\tdec  hl\n")
			default:
				b.loadVal(i.Right, "de")
				b.buf.WriteString("\tor   a\n\tsbc  hl, de\n")
			}
		} else {
			b.loadVal(i.Right, "de")
			b.buf.WriteString("\tor   a\n\tsbc  hl, de\n")
		}
	case "mul":
		b.loadVal(i.Right, "de")
		b.buf.WriteString("\tcall __mul16\n")
	case "div":
		b.loadVal(i.Right, "de")
		if i.Left.Type().IsInt() {
			b.buf.WriteString("\tcall __div16\n")
		} else {
			b.buf.WriteString("\tcall __udiv16\n")
		}
	case "mod":
		b.loadVal(i.Right, "de")
		if i.Left.Type().IsInt() {
			b.buf.WriteString("\tcall __mod16\n")
		} else {
			b.buf.WriteString("\tcall __umod16\n")
		}
	case "and", "bitand":
		b.loadVal(i.Right, "de")
		b.buf.WriteString("\tld   a, l\n\tand  e\n\tld   l, a\n\tld   a, h\n\tand  d\n\tld   h, a\n")
	case "or", "bitor":
		b.loadVal(i.Right, "de")
		b.buf.WriteString("\tld   a, l\n\tor   e\n\tld   l, a\n\tld   a, h\n\tor   d\n\tld   h, a\n")
	case "xor", "bitxor":
		b.loadVal(i.Right, "de")
		b.buf.WriteString("\tld   a, l\n\txor  e\n\tld   l, a\n\tld   a, h\n\txor  d\n\tld   h, a\n")
	case "andnot":
		b.loadVal(i.Right, "de")
		b.buf.WriteString("\tld   a, e\n\tcpl\n\tand  l\n\tld   l, a\n\tld   a, d\n\tcpl\n\tand  h\n\tld   h, a\n")
	case "shl":
		if cVal, ok := i.Right.(*ir.ConstWord); ok && cVal.Val <= 4 {
			for j := uint64(0); j < cVal.Val; j++ {
				b.buf.WriteString("\tadd  hl, hl\n")
			}
		} else {
			b.loadVal(i.Right, "de")
			b.buf.WriteString("\tld   c, e\n\tcall __shl16\n")
		}
	case "shr":
		b.loadVal(i.Right, "de")
		if i.Left.Type().IsInt() {
			b.buf.WriteString("\tld   c, e\n\tcall __sar16\n")
		} else {
			b.buf.WriteString("\tld   c, e\n\tcall __shr16\n")
		}
	default:
		log.Panicf("emitBinaryOp: unsupported op %s", i.Op)
	}

	b.storeResult(i.GetID())
}

func (b *Backend) emitCompare(i *ir.Compare) {
	sz := b.getTypeSize(i.Left.Type())
	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()

	if sz == 1 {
		b.loadVal(i.Left, "a")
		if cb, ok := i.Right.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tcp   %d\n", cb.Val))
		} else if cw, ok := i.Right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tcp   %d\n", cw.Val&0xFF))
		} else {
			b.loadVal(i.Right, "b")
			b.buf.WriteString("\tcp   b\n")
		}
	} else {
		b.loadVal(i.Left, "hl")
		if cw, ok := i.Right.(*ir.ConstWord); ok && cw.Val == 0 && (i.Op == "eq" || i.Op == "neq") {
			b.buf.WriteString("\tld   a, h\n\tor   l\n")
		} else {
			b.loadVal(i.Right, "de")
			b.buf.WriteString("\tor   a\n\tsbc  hl, de\n")
		}
	}

	doneLbl := b.nextLabel()

	switch i.Op {
	case "eq":
		b.buf.WriteString("\tld   a, 0\n")
		b.buf.WriteString(fmt.Sprintf("\tjmp  nz, %s\n", doneLbl))
		b.buf.WriteString("\tinc  a\n")
	case "neq":
		b.buf.WriteString("\tld   a, 0\n")
		b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", doneLbl))
		b.buf.WriteString("\tinc  a\n")
	case "lt":
		if isUnsigned {
			b.buf.WriteString("\tld   a, 0\n")
			b.buf.WriteString(fmt.Sprintf("\tjmp  nc, %s\n", doneLbl))
			b.buf.WriteString("\tinc  a\n")
		} else {
			trueLbl := b.nextLabel()
			falseLbl := b.nextLabel()
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\txor  a\n\tjmp  %s\n", falseLbl, doneLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\tld   a, 1\n", trueLbl))
		}
	case "lte":
		if isUnsigned {
			b.buf.WriteString("\tld   a, 1\n")
			b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", doneLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  c, %s\n", doneLbl))
			b.buf.WriteString("\tdec  a\n")
		} else {
			trueLbl := b.nextLabel()
			falseLbl := b.nextLabel()
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\txor  a\n\tjmp  %s\n", falseLbl, doneLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\tld   a, 1\n", trueLbl))
		}
	case "gt":
		if isUnsigned {
			b.buf.WriteString("\tld   a, 0\n")
			b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", doneLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  c, %s\n", doneLbl))
			b.buf.WriteString("\tinc  a\n")
		} else {
			trueLbl := b.nextLabel()
			falseLbl := b.nextLabel()
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", falseLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\txor  a\n\tjmp  %s\n", falseLbl, doneLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\tld   a, 1\n", trueLbl))
		}
	case "gte":
		if isUnsigned {
			b.buf.WriteString("\tld   a, 0\n")
			b.buf.WriteString(fmt.Sprintf("\tjmp  c, %s\n", doneLbl))
			b.buf.WriteString("\tinc  a\n")
		} else {
			trueLbl := b.nextLabel()
			falseLbl := b.nextLabel()
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\txor  a\n\tjmp  %s\n", falseLbl, doneLbl))
			b.buf.WriteString(fmt.Sprintf("%s:\n\tld   a, 1\n", trueLbl))
		}
	default:
		log.Panicf("emitCompare: unsupported op %s", i.Op)
	}

	b.buf.WriteString(fmt.Sprintf("%s:\n", doneLbl))

	b.storeResult(i.GetID())
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))

	case *ir.ConstByte, *ir.ConstWord, *ir.Sizeof:
		// Emitted on-demand at use sites in loadVal
		return

	case *ir.Load:
		sz := b.getTypeSize(i.Global.Typ)
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tld   a, (v_%s)\n", i.Global.Name))
			b.storeResult(id)
		} else if sz == 2 {
			b.buf.WriteString(fmt.Sprintf("\tld   hl, (v_%s)\n", i.Global.Name))
			b.storeResult(id)
		} else {
			b.buf.WriteString(fmt.Sprintf("\tld   hl, v_%s\n", i.Global.Name))
			b.emitLoadAddr("de", i)
			b.emitMemCopy("(de)", "(hl)", sz)
		}

	case *ir.Store:
		sz := b.getTypeSize(i.Global.Typ)
		b.storeToAddr(fmt.Sprintf("(v_%s)", i.Global.Name), i.Val, sz)

	case *ir.ZeroInit:
		sz := b.getTypeSize(i.Typ)
		if sz == 1 {
			b.buf.WriteString("\txor  a\n")
			b.storeResult(id)
		} else if sz == 2 {
			b.buf.WriteString("\tld   hl, 0\n")
			b.storeResult(id)
		} else {
			b.emitLoadAddr("de", i)
			b.buf.WriteString(fmt.Sprintf("\txor  a\n\tld   bc, %d\n", sz))
			lblLoop := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n\tld   (de), a\n\tinc  de\n\tdec  bc\n\tld   a, b\n\tor   c\n\tjmp  nz, %s\n", lblLoop, lblLoop))
		}

	case *ir.BinaryOp:
		b.emitBinaryOp(i)

	case *ir.Compare:
		b.emitCompare(i)

	case *ir.UnaryOp:
		sz := b.getTypeSize(i.Typ)
		if sz == 1 {
			b.loadVal(i.Operand, "a")
			switch i.Op {
			case "neg":
				b.buf.WriteString("\tneg\n")
			case "not", "bitnot":
				b.buf.WriteString("\tcpl\n")
			default:
				log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
			}
		} else {
			b.loadVal(i.Operand, "hl")
			switch i.Op {
			case "neg":
				b.buf.WriteString("\tcall __neg_hl\n")
			case "not", "bitnot":
				b.buf.WriteString("\tld   a, h\n\tcpl\n\tld   h, a\n\tld   a, l\n\tcpl\n\tld   l, a\n")
			default:
				log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
			}
		}
		b.storeResult(id)

	case *ir.Cast:
		srcSz := b.getTypeSize(i.Operand.Type())
		dstSz := b.getTypeSize(i.Typ)
		if dstSz == 1 {
			if srcSz == 1 {
				b.loadVal(i.Operand, "a")
			} else {
				b.loadVal(i.Operand, "hl")
				b.buf.WriteString("\tld   a, l\n")
			}
		} else {
			if srcSz == 1 {
				b.loadVal(i.Operand, "a")
				b.buf.WriteString("\tld   l, a\n\tld   h, 0\n")
			} else {
				b.loadVal(i.Operand, "hl")
			}
		}
		b.storeResult(id)

	case *ir.AddressOfLocal:
		b.emitLoadAddr("hl", i.Local)
		b.storeResult(id)

	case *ir.AddressOfGlobal, *ir.AddressOfFunc:
		// Emitted on-demand at use sites in loadVal
		return

	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		if glob, ok := i.Ptr.(*ir.AddressOfGlobal); ok {
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, v_%s + %d\n", glob.Global.Name, byteOffset))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, v_%s\n", glob.Global.Name))
			}
			b.storeResult(id)
			return
		}
		b.loadVal(i.Ptr, "hl")
		b.emitAddConstToHL(byteOffset)
		b.storeResult(id)

	case *ir.AddressOfElement:
		eltSize := b.getEltSize(i.ArrayPtr.Type())
		if glob, ok := i.ArrayPtr.(*ir.AddressOfGlobal); ok {
			if cIdx, ok2 := i.Index.(*ir.ConstWord); ok2 {
				byteOffset := int(cIdx.Val) * eltSize
				if byteOffset > 0 {
					b.buf.WriteString(fmt.Sprintf("\tld   hl, v_%s + %d\n", glob.Global.Name, byteOffset))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tld   hl, v_%s\n", glob.Global.Name))
				}
				b.storeResult(id)
				return
			}
		}
		b.loadVal(i.ArrayPtr, "hl")
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConstToHL(byteOffset)
		} else {
			b.loadVal(i.Index, "de")
			if eltSize == 1 {
				b.buf.WriteString("\tadd  hl, de\n")
			} else if eltSize == 2 {
				b.buf.WriteString("\tex   de, hl\n\tadd  hl, hl\n\tadd  hl, de\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tpush hl\n\tld   hl, %d\n\tcall __mul16\n\tex   de, hl\n\tpop  hl\n\tadd  hl, de\n", eltSize))
			}
		}
		b.storeResult(id)

	case *ir.LoadPtr:
		sz := b.getTypeSize(i.Typ)
		if addrStr, ok := b.getDirectGlobalAddr(i.Ptr); ok {
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tld   a, (%s)\n", addrStr))
			} else if sz == 2 {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, (%s)\n", addrStr))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, %s\n", addrStr))
				b.emitLoadAddr("de", i)
				b.emitMemCopy("(de)", "(hl)", sz)
			}
			b.storeResult(id)
			return
		}
		b.loadVal(i.Ptr, "hl")
		if sz == 1 {
			b.buf.WriteString("\tld   a, (hl)\n")
		} else if sz == 2 {
			b.buf.WriteString("\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n\tex   de, hl\n")
		} else {
			b.emitLoadAddr("de", i)
			b.emitMemCopy("(de)", "(hl)", sz)
		}
		b.storeResult(id)

	case *ir.StorePtr:
		sz := b.getTypeSize(i.Val.Type())
		if addrStr, ok := b.getDirectGlobalAddr(i.Ptr); ok {
			if sz == 1 {
				b.loadVal(i.Val, "a")
				b.buf.WriteString(fmt.Sprintf("\tld   (%s), a\n", addrStr))
				return
			} else if sz == 2 {
				b.loadVal(i.Val, "hl")
				b.buf.WriteString(fmt.Sprintf("\tld   (%s), hl\n", addrStr))
				return
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   de, %s\n", addrStr))
				b.emitLoadAddr("hl", i.Val)
				b.buf.WriteString(fmt.Sprintf("\tld   bc, %d\n\tldir\n", sz))
				return
			}
		}
		b.loadVal(i.Ptr, "de") // DE = destination address
		if sz == 1 {
			b.loadVal(i.Val, "a")
			b.buf.WriteString("\tld   (de), a\n")
		} else if sz == 2 {
			b.loadVal(i.Val, "hl")
			b.buf.WriteString("\tld   a, l\n\tld   (de), a\n\tinc  de\n\tld   a, h\n\tld   (de), a\n")
		} else {
			b.emitLoadAddr("hl", i.Val)
			b.buf.WriteString(fmt.Sprintf("\tld   bc, %d\n\tldir\n", sz))
		}

	case *ir.ExtractField:
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		b.emitLoadAddr("hl", i.Struct)
		b.emitAddConstToHL(byteOffset)
		b.emitLoadAddr("de", i)
		b.emitMemCopy("(de)", "(hl)", fieldSize)

	case *ir.InsertField:
		structSize := b.getTypeSize(i.Struct.Type())
		b.emitLoadAddr("hl", i.Struct)
		b.emitLoadAddr("de", i)
		b.emitMemCopy("(de)", "(hl)", structSize)
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		b.emitLoadAddr("de", i)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n\tadd  hl, de\n\tex   de, hl\n", byteOffset))
		}
		b.storeToAddr("(de)", i.Val, fieldSize)

	case *ir.ExtractElement:
		eltSize := b.getTypeSize(i.Typ)
		b.emitLoadAddr("hl", i.Array)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConstToHL(byteOffset)
		} else {
			b.loadVal(i.Index, "de")
			if eltSize == 1 {
				b.buf.WriteString("\tadd  hl, de\n")
			} else if eltSize == 2 {
				b.buf.WriteString("\tex   de, hl\n\tadd  hl, hl\n\tadd  hl, de\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tpush hl\n\tld   hl, %d\n\tcall __mul16\n\tex   de, hl\n\tpop  hl\n\tadd  hl, de\n", eltSize))
			}
		}
		b.emitLoadAddr("de", i)
		b.emitMemCopy("(de)", "(hl)", eltSize)

	case *ir.InsertElement:
		arraySize := b.getTypeSize(i.Array.Type())
		b.emitLoadAddr("hl", i.Array)
		b.emitLoadAddr("de", i)
		b.emitMemCopy("(de)", "(hl)", arraySize)
		eltSize := b.getEltSize(i.Array.Type())
		b.emitLoadAddr("de", i)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n\tadd  hl, de\n\tex   de, hl\n", byteOffset))
			}
		} else {
			b.loadVal(i.Index, "hl")
			if eltSize == 1 {
				b.buf.WriteString("\tadd  hl, de\n\tex   de, hl\n")
			} else if eltSize == 2 {
				b.buf.WriteString("\tadd  hl, hl\n\tadd  hl, de\n\tex   de, hl\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tld   de, %d\n\tcall __mul16\n\tex   de, hl\n\tpop  hl\n\tadd  hl, de\n\tex   de, hl\n", eltSize))
			}
		}
		b.storeToAddr("(de)", i.Val, eltSize)

	case *ir.Call:
		// Push arguments in reverse order (right-to-left)
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			sz := b.getTypeSize(arg.Type())
			pushSize := alignVal(sz, 2)
			totalArgBytes += pushSize
			if sz > 2 {
				b.buf.WriteString(fmt.Sprintf("\tld   hl, -%d\n\tadd  hl, sp\n\tld   sp, hl\n", pushSize))
				b.emitLoadAddr("hl", arg)
				b.buf.WriteString("\tld   d, h\n\tld   e, l\n") // copy from arg to (sp)
				// DE = sp
				b.buf.WriteString(fmt.Sprintf("\tld   hl, 0\n\tadd  hl, sp\n\tex   de, hl\n\tld   bc, %d\n\tldir\n", sz))
			} else {
				b.loadVal(arg, "hl")
				b.buf.WriteString("\tpush hl\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		b.buf.WriteString(fmt.Sprintf("\tcall %s\n", i.Func.EmitName()))
		hasRet := !i.Typ.Equals(ir.TypeVoid) && retSize <= 2
		b.emitCallStackCleanup(totalArgBytes, hasRet)
		if hasRet {
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
				b.buf.WriteString(fmt.Sprintf("\tld   hl, -%d\n\tadd  hl, sp\n\tld   sp, hl\n", pushSize))
				b.emitLoadAddr("hl", arg)
				b.buf.WriteString(fmt.Sprintf("\tld   hl, 0\n\tadd  hl, sp\n\tex   de, hl\n\tld   bc, %d\n\tldir\n", sz))
			} else {
				b.loadVal(arg, "hl")
				b.buf.WriteString("\tpush hl\n")
			}
		}
		retSize := b.getTypeSize(i.Typ)
		b.loadVal(i.FuncPtr, "hl")
		b.buf.WriteString("\tcall __call_hl\n")
		hasRet := !i.Typ.Equals(ir.TypeVoid) && retSize <= 2
		b.emitCallStackCleanup(totalArgBytes, hasRet)
		if hasRet {
			b.storeResult(id)
		}

	case *ir.BuiltinCall:
		b.emitBuiltinCall(i)

	case *ir.SetJmp:
		b.emitSetJmp(i)
	case *ir.LongJmp:
		b.emitLongJmp(i)

	case *ir.Phi:
		// Phi handled at branch/jump transitions

	default:
		log.Panicf("emitInstr: unhandled instruction %T", instr)
	}
}

func (b *Backend) emitCallStackCleanup(totalArgBytes int, hasRet bool) {
	if totalArgBytes <= 0 {
		return
	}
	if totalArgBytes <= 24 {
		for a := 0; a < totalArgBytes; a += 2 {
			b.buf.WriteString("\tpop  bc\n")
		}
	} else if hasRet {
		b.buf.WriteString(fmt.Sprintf("\tex   de, hl\n\tld   hl, %d\n\tadd  hl, sp\n\tld   sp, hl\n\tex   de, hl\n", totalArgBytes))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n\tadd  hl, sp\n\tld   sp, hl\n", totalArgBytes))
	}
}

func (b *Backend) emitSetJmp(i *ir.SetJmp) {
	id := i.GetID()
	jmpSlot := b.jmpSlots[id]
	lblResume := b.nextLabel()
	lblDone := b.nextLabel()

	// Compute jmpbuf address in HL
	b.buf.WriteString(fmt.Sprintf("\tpush ix\n\tpop  hl\n\tld   de, -%d\n\tadd  hl, de\n", jmpSlot))
	// Save prev chain
	b.buf.WriteString("\tld   de, (v_prelude._jmp_chain_)\n\tld   (hl), e\n\tinc  hl\n\tld   (hl), d\n\tdec  hl\n")
	// Update chain to point to this jmpbuf
	b.buf.WriteString("\tld   (v_prelude._jmp_chain_), hl\n")
	// Save resume PC at offset 2
	b.buf.WriteString(fmt.Sprintf("\tld   de, %s\n\tinc  hl\n\tinc  hl\n\tld   (hl), e\n\tinc  hl\n\tld   (hl), d\n", lblResume))
	// Save SP at offset 4
	b.buf.WriteString("\tinc  hl\n\tpush hl\n\tld   hl, 2\n\tadd  hl, sp\n\tex   de, hl\n\tpop  hl\n\tld   (hl), e\n\tinc  hl\n\tld   (hl), d\n")
	// Save IX at offset 6
	b.buf.WriteString("\tinc  hl\n\tpush ix\n\tpop  de\n\tld   (hl), e\n\tinc  hl\n\tld   (hl), d\n")

	b.buf.WriteString("\txor  a\n\tld   hl, 0\n")
	b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", lblDone))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblResume))
	b.buf.WriteString("\tld   a, 1\n\tld   hl, 1\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	b.storeResult(id)
}

func (b *Backend) emitLongJmp(i *ir.LongJmp) {
	b.loadVal(i.JmpBuf, "hl")
	// Restore IX from offset 6
	b.buf.WriteString("\tld   de, 6\n\tadd  hl, de\n\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n\tpush de\n\tpop  ix\n")
	// Resume PC from offset 2
	b.loadVal(i.JmpBuf, "hl")
	b.buf.WriteString("\tinc  hl\n\tinc  hl\n\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n")
	b.buf.WriteString("\tpush de\n\tret\n") // Jump to resume PC
}

func (b *Backend) emitBuiltinCall(i *ir.BuiltinCall) {
	switch i.Name {
	case "print", "println":
		b.emitPrint(i.Name == "println", i.Args)
	case "exit":
		if len(i.Args) > 0 {
			b.loadVal(i.Args[0], "hl")
		} else {
			b.buf.WriteString("\tld   hl, 0\n")
		}
		b.buf.WriteString("\tpush hl\n\tcall _exit\n")
	case "panic", "_panic_":
		if len(i.Args) > 0 {
			if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
				b.lblCount++
				lbl := fmt.Sprintf(".Lpanic%d", b.lblCount)
				b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: strLit.Value})
				b.buf.WriteString(fmt.Sprintf("\tld   hl, %s\n\tpush hl\n", lbl))
			} else {
				b.loadVal(i.Args[0], "hl")
				b.buf.WriteString("\tpush hl\n")
			}
		} else {
			b.buf.WriteString("\tld   hl, 0\n\tpush hl\n")
		}
		b.buf.WriteString("\tcall _panic\n\tpop  bc\n")
	case "_unlink_jmp_":
		lblDone := b.nextLabel()
		b.buf.WriteString("\tld   hl, (v_prelude._jmp_chain_)\n")
		b.buf.WriteString("\tld   a, h\n\tor   l\n")
		b.buf.WriteString(fmt.Sprintf("\tjr   z, %s\n", lblDone))
		b.buf.WriteString("\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n")
		b.buf.WriteString("\tld   (v_prelude._jmp_chain_), de\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	case "_propagate_panic_":
		lblDone := b.nextLabel()
		lblAbort := b.nextLabel()
		b.buf.WriteString("\tld   hl, (v_prelude._panic_)\n")
		b.buf.WriteString("\tld   a, h\n\tor   l\n")
		b.buf.WriteString(fmt.Sprintf("\tjr   z, %s\n", lblDone))
		b.buf.WriteString("\tld   hl, (v_prelude._jmp_chain_)\n")
		b.buf.WriteString("\tld   a, h\n\tor   l\n")
		b.buf.WriteString(fmt.Sprintf("\tjr   z, %s\n", lblAbort))
		// Restore IX from offset 6
		b.buf.WriteString("\tpush hl\n")
		b.buf.WriteString("\tld   de, 6\n\tadd  hl, de\n\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n\tpush de\n\tpop  ix\n")
		// Restore SP from offset 4
		b.buf.WriteString("\tpop  hl\n\tpush hl\n")
		b.buf.WriteString("\tld   de, 4\n\tadd  hl, de\n\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n\tex   de, hl\n\tld   sp, hl\n")
		// Load resume PC from offset 2
		b.buf.WriteString("\tpop  hl\n")
		b.buf.WriteString("\tinc  hl\n\tinc  hl\n\tld   e, (hl)\n\tinc  hl\n\tld   d, (hl)\n")
		// Return 1 in A / HL from setjmp
		b.buf.WriteString("\tld   a, 1\n\tld   hl, 1\n")
		b.buf.WriteString("\tpush de\n\tret\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblAbort))
		b.buf.WriteString("\tld   hl, .L_empty_chain_msg\n\tpush hl\n\tcall _printf\n\tpop  bc\n")
		b.buf.WriteString("\tld   hl, 1\n\tpush hl\n\tcall _exit\n")
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
			b.buf.WriteString(fmt.Sprintf("\tld   hl, %s\n\tpush hl\n", strLbl))
		} else {
			b.loadVal(arg, "hl")
			b.buf.WriteString("\tpush hl\n")
		}
	}

	b.buf.WriteString(fmt.Sprintf("\tld   hl, %s\n\tpush hl\n", fmtLabel))
	b.buf.WriteString("\tcall _printf\n")

	cleanup := (len(dataArgs) + 1) * 2
	if cleanup == 2 {
		b.buf.WriteString("\tpop  bc\n")
	} else if cleanup == 4 {
		b.buf.WriteString("\tpop  bc\n\tpop  bc\n")
	} else if cleanup == 6 {
		b.buf.WriteString("\tpop  bc\n\tpop  bc\n\tpop  bc\n")
	} else {
		b.buf.WriteString(fmt.Sprintf("\tld   hl, %d\n\tadd  hl, sp\n\tld   sp, hl\n", cleanup))
	}
}

func isZero(val ir.Value) bool {
	if val == nil {
		return false
	}
	if cb, ok := val.(*ir.ConstByte); ok && cb.Val == 0 {
		return true
	}
	if cw, ok := val.(*ir.ConstWord); ok && cw.Val == 0 {
		return true
	}
	if _, ok := val.(*ir.ZeroInit); ok {
		return true
	}
	return false
}

func getOperands(instr ir.Instruction) []ir.Value {
	var ops []ir.Value
	switch i := instr.(type) {
	case *ir.Store:
		ops = append(ops, i.Val)
	case *ir.BinaryOp:
		ops = append(ops, i.Left, i.Right)
	case *ir.Compare:
		ops = append(ops, i.Left, i.Right)
	case *ir.UnaryOp:
		ops = append(ops, i.Operand)
	case *ir.ExtractElement:
		ops = append(ops, i.Array, i.Index)
	case *ir.InsertElement:
		ops = append(ops, i.Array, i.Index, i.Val)
	case *ir.ExtractField:
		ops = append(ops, i.Struct)
	case *ir.InsertField:
		ops = append(ops, i.Struct, i.Val)
	case *ir.AddressOfLocal:
		ops = append(ops, i.Local)
	case *ir.AddressOfField:
		ops = append(ops, i.Ptr)
	case *ir.AddressOfElement:
		ops = append(ops, i.ArrayPtr, i.Index)
	case *ir.ExtractFieldPtr:
		ops = append(ops, i.Ptr)
	case *ir.InsertFieldPtr:
		ops = append(ops, i.Ptr, i.Val)
	case *ir.LoadPtr:
		ops = append(ops, i.Ptr)
	case *ir.StorePtr:
		ops = append(ops, i.Ptr, i.Val)
	case *ir.Phi:
		for _, e := range i.Edges {
			if e.Value != nil {
				ops = append(ops, e.Value)
			}
		}
	case *ir.Call:
		ops = append(ops, i.Args...)
	case *ir.IndirectCall:
		ops = append(ops, i.FuncPtr)
		ops = append(ops, i.Args...)
	case *ir.BuiltinCall:
		ops = append(ops, i.Args...)
	case *ir.Cast:
		ops = append(ops, i.Operand)
	case *ir.Branch:
		if i.Condition != nil {
			ops = append(ops, i.Condition)
		}
	case *ir.Return:
		if i.Val != nil {
			ops = append(ops, i.Val)
		}
	case *ir.SetJmp:
		if i.JmpBuf != nil {
			ops = append(ops, i.JmpBuf)
		}
	case *ir.LongJmp:
		if i.JmpBuf != nil {
			ops = append(ops, i.JmpBuf)
		}
	case *ir.ConstStruct:
		ops = append(ops, i.Fields...)
	case *ir.ConstArray:
		ops = append(ops, i.Elements...)
	}
	return ops
}

func countUses(f *ir.Function, val ir.Value) int {
	count := 0
	for _, blk := range f.Blocks {
		for _, inst := range blk.Instructions {
			for _, op := range getOperands(inst) {
				if op == val {
					count++
				}
			}
		}
	}
	return count
}

func (b *Backend) hasPhiAssignments(from, to *ir.BasicBlock) bool {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					return true
				}
			}
		}
	}
	return false
}

func (b *Backend) emitFusedCompareAndBranch(f *ir.Function, blk *ir.BasicBlock, branch *ir.Branch, cmp *ir.Compare) {
	sz := b.getTypeSize(cmp.Left.Type())
	isUnsigned := cmp.Left.Type().IsWord() || cmp.Left.Type().IsByte() || cmp.Left.Type().IsAPointer()

	if sz == 1 {
		b.loadVal(cmp.Left, "a")
		if cb, ok := cmp.Right.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tcp   %d\n", cb.Val))
		} else if cw, ok := cmp.Right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tcp   %d\n", cw.Val&0xFF))
		} else {
			b.loadVal(cmp.Right, "b")
			b.buf.WriteString("\tcp   b\n")
		}
	} else {
		b.loadVal(cmp.Left, "hl")
		if cw, ok := cmp.Right.(*ir.ConstWord); ok && cw.Val == 0 && (cmp.Op == "eq" || cmp.Op == "neq") {
			b.buf.WriteString("\tld   a, h\n\tor   l\n")
		} else {
			b.loadVal(cmp.Right, "de")
			b.buf.WriteString("\tor   a\n\tsbc  hl, de\n")
		}
	}

	trueTarget := fmt.Sprintf(".L_%s_b%d", f.Name, branch.TrueBlock.ID)
	falseTarget := fmt.Sprintf(".L_%s_b%d", f.Name, branch.FalseBlock.ID)

	hasTruePhi := b.hasPhiAssignments(blk, branch.TrueBlock)
	hasFalsePhi := b.hasPhiAssignments(blk, branch.FalseBlock)

	if hasTruePhi {
		trueTarget = fmt.Sprintf(".L_%s_b%d_true", f.Name, blk.ID)
	}
	if hasFalsePhi {
		falseTarget = fmt.Sprintf(".L_%s_b%d_false", f.Name, blk.ID)
	}

	switch cmp.Op {
	case "eq":
		b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", trueTarget))
		b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
	case "neq":
		b.buf.WriteString(fmt.Sprintf("\tjmp  nz, %s\n", trueTarget))
		b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
	case "lt":
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tjmp  c, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		} else {
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		}
	case "lte":
		b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", trueTarget))
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tjmp  c, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		} else {
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		}
	case "gt":
		b.buf.WriteString(fmt.Sprintf("\tjmp  z, %s\n", falseTarget))
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tjmp  nc, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		} else {
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		}
	case "gte":
		if isUnsigned {
			b.buf.WriteString(fmt.Sprintf("\tjmp  nc, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		} else {
			overLbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tjmp  pe, %s\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  p, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
			b.buf.WriteString(fmt.Sprintf("%s:\n", overLbl))
			b.buf.WriteString(fmt.Sprintf("\tjmp  m, %s\n", trueTarget))
			b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
		}
	}

	if hasTruePhi {
		b.buf.WriteString(fmt.Sprintf("%s:\n", trueTarget))
		b.emitPhiAssignments(blk, branch.TrueBlock)
		b.buf.WriteString(fmt.Sprintf("\tjmp  .L_%s_b%d\n", f.Name, branch.TrueBlock.ID))
	}

	if hasFalsePhi {
		b.buf.WriteString(fmt.Sprintf("%s:\n", falseTarget))
		b.emitPhiAssignments(blk, branch.FalseBlock)
		b.buf.WriteString(fmt.Sprintf("\tjmp  .L_%s_b%d\n", f.Name, branch.FalseBlock.ID))
	}
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.currentFunc = f
	b.allocateSlots(f)

	if b.stackSize > 120 {
		b.frameBias = b.stackSize / 2
		if b.frameBias > 110 {
			b.frameBias = 110
		}
	} else {
		b.frameBias = 0
	}

	b.buf.WriteString("\n")
	b.buf.WriteString(fmt.Sprintf("; ============================================================================\n"))
	b.buf.WriteString(fmt.Sprintf("; func %s (frame: %d bytes)\n", f.Name, b.stackSize))
	b.buf.WriteString(fmt.Sprintf("; ============================================================================\n"))
	b.buf.WriteString(fmt.Sprintf("%s:\n", f.EmitName()))

	// Prologue
	b.buf.WriteString("\tpush ix\n\tld   ix, 0\n\tadd  ix, sp\n")
	if b.frameBias > 0 {
		b.buf.WriteString(fmt.Sprintf("\tld   de, -%d\n\tadd  ix, de\n", b.frameBias))
	}
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tld   hl, -%d\n\tadd  hl, sp\n\tld   sp, hl\n", b.stackSize))
	}

	for blkIdx, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", f.Name, blk.ID))

		var fusedCompare *ir.Compare
		var redundantZero ir.Instruction
		var redundantOuterCmp ir.Instruction

		if branch, ok := blk.Terminator.(*ir.Branch); ok {
			cond := branch.Condition
			if outerCmp, ok := cond.(*ir.Compare); ok && outerCmp.Op == "neq" {
				if isZero(outerCmp.Right) {
					if innerCmp, ok := outerCmp.Left.(*ir.Compare); ok {
						if countUses(f, outerCmp) <= 1 && countUses(f, innerCmp) <= 1 {
							fusedCompare = innerCmp
							redundantOuterCmp = outerCmp
							if zeroInst, ok := outerCmp.Right.(ir.Instruction); ok {
								redundantZero = zeroInst
							}
						}
					}
				}
			}
			if fusedCompare == nil {
				if cmp, ok := cond.(*ir.Compare); ok {
					if countUses(f, cmp) <= 1 {
						fusedCompare = cmp
					}
				}
			}
		}

		for _, instr := range blk.Instructions {
			if _, isPhi := instr.(*ir.Phi); isPhi {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			if instr == fusedCompare || instr == redundantOuterCmp || instr == redundantZero {
				continue
			}
			b.buf.WriteString("\t; " + instr.String() + "\n")
			b.emitInstr(instr)
		}

		if blk.Terminator != nil {
			switch term := blk.Terminator.(type) {
			case *ir.Jump:
				b.emitPhiAssignments(blk, term.Target)
				if blkIdx+1 < len(f.Blocks) && term.Target == f.Blocks[blkIdx+1] {
					// Fall through directly into next basic block
				} else {
					b.buf.WriteString(fmt.Sprintf("\tjmp  .L_%s_b%d\n", f.Name, term.Target.ID))
				}

			case *ir.Branch:
				if fusedCompare != nil {
					b.emitFusedCompareAndBranch(f, blk, term, fusedCompare)
				} else {
					trueTarget := fmt.Sprintf(".L_%s_b%d", f.Name, term.TrueBlock.ID)
					falseTarget := fmt.Sprintf(".L_%s_b%d", f.Name, term.FalseBlock.ID)

					hasTruePhi := b.hasPhiAssignments(blk, term.TrueBlock)
					hasFalsePhi := b.hasPhiAssignments(blk, term.FalseBlock)

					trueLbl := fmt.Sprintf(".L_%s_b%d_true", f.Name, blk.ID)
					falseLbl := fmt.Sprintf(".L_%s_b%d_false", f.Name, blk.ID)

					if hasTruePhi {
						trueTarget = trueLbl
					}
					if hasFalsePhi {
						falseTarget = falseLbl
					}

					b.loadVal(term.Condition, "a")
					b.buf.WriteString("\tor   a\n")
					b.buf.WriteString(fmt.Sprintf("\tjmp  nz, %s\n", trueTarget))

					isFalseNext := (blkIdx+1 < len(f.Blocks) && term.FalseBlock == f.Blocks[blkIdx+1])
					if !hasFalsePhi && isFalseNext {
						// Falls through directly to false block
					} else {
						b.buf.WriteString(fmt.Sprintf("\tjmp  %s\n", falseTarget))
					}

					if hasTruePhi {
						b.buf.WriteString(fmt.Sprintf("%s:\n", trueLbl))
						b.emitPhiAssignments(blk, term.TrueBlock)
						b.buf.WriteString(fmt.Sprintf("\tjmp  .L_%s_b%d\n", f.Name, term.TrueBlock.ID))
					}

					if hasFalsePhi {
						b.buf.WriteString(fmt.Sprintf("%s:\n", falseLbl))
						b.emitPhiAssignments(blk, term.FalseBlock)
						b.buf.WriteString(fmt.Sprintf("\tjmp  .L_%s_b%d\n", f.Name, term.FalseBlock.ID))
					}
				}

			case *ir.Return:
				if term.Val != nil {
					sz := b.getTypeSize(term.Val.Type())
					if sz == 1 {
						b.loadVal(term.Val, "a")
						b.buf.WriteString("\tld   l, a\n\tld   h, 0\n")
					} else if sz == 2 {
						b.loadVal(term.Val, "hl")
					}
				}
				if b.frameBias > 0 {
					b.buf.WriteString(fmt.Sprintf("\tld   de, %d\n\tadd  ix, de\n", b.frameBias))
				}
				b.buf.WriteString("\tld   sp, ix\n\tpop  ix\n\tret\n")

			default:
				log.Panicf("emitFunc: unhandled terminator %T", blk.Terminator)
			}
		}
	}
}

func (b *Backend) emitGlobals(prog *ir.Program) {
	b.buf.WriteString("\n; ── Global Data Section ───────────────────────────────────────────────────────\n")
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
				b.buf.WriteString(fmt.Sprintf("\tdefb %s\n", strings.Join(bytesList, ", ")))
				sz -= len(g.InitString)
				if sz > 0 {
					b.buf.WriteString(fmt.Sprintf("\tdefs %d, 0\n", sz))
				}
			}
		} else {
			if sz > 0 {
				b.buf.WriteString(fmt.Sprintf("\tdefs %d, 0\n", sz))
			}
		}
	}

	if len(b.rodata) > 0 {
		b.buf.WriteString("\n; ── Read-Only Data (Strings & Format Literals) ───────────────────────────────\n")
		for _, entry := range b.rodata {
			b.buf.WriteString(fmt.Sprintf("%s:\n", entry.lbl))
			var bytesList []string
			for _, ch := range []byte(entry.str) {
				bytesList = append(bytesList, fmt.Sprintf("$%02X", ch))
			}
			bytesList = append(bytesList, "$00")
			b.buf.WriteString(fmt.Sprintf("\tdefb %s\n", strings.Join(bytesList, ", ")))
		}
	}
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tdefb %d\n", v.Val))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tdefw %d\n", v.Val))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tdefw v_%s\n", v.Global.Name))
	case *ir.ConstStruct:
		structTyp := v.Type()
		if b.program != nil {
			if def, ok := b.program.TypeDefs[structTyp.Name]; ok {
				structTyp = def
			}
		}
		fields := structTyp.FieldsOfStruct()
		for fIdx := range fields {
			if fIdx < len(v.Fields) {
				b.emitData(v.Fields[fIdx])
			}
		}
	case *ir.ConstArray:
		for _, el := range v.Elements {
			b.emitData(el)
		}
	}
}

func (b *Backend) emitHelpers() {
	b.buf.WriteString(`
; ── Software Math Helpers (Z80 16-Bit) ──────────────────────────────────────

; Indirect function call via HL: call __call_hl
__call_hl:
	jp   (hl)

; __mul16: HL = HL * DE
__mul16:
	ld   b, h
	ld   c, l
	ld   hl, 0
	ld   a, 16
.L_mul_loop:
	srl  d
	rr   e
	jmp  nc, .L_mul_noadd
	add  hl, bc
.L_mul_noadd:
	sla  c
	rl   b
	dec  a
	jmp  nz, .L_mul_loop
	ret

; __udivmod16: Unsigned division HL / DE -> quotient in BC, remainder in HL
__udivmod16:
	ld   a, d
	or   e
	jmp  nz, .L_udiv_start
	ld   hl, .L_div_zero_str
	push hl
	call _panic
.L_udiv_start:
	ld   b, h
	ld   c, l
	ld   hl, 0
	ld   a, 16
.L_udiv_loop:
	sla  c
	rl   b
	rl   l
	rl   h
	or   a
	sbc  hl, de
	jmp  nc, .L_udiv_sub_ok
	add  hl, de
	jmp  .L_udiv_next
.L_udiv_sub_ok:
	inc  c
.L_udiv_next:
	dec  a
	jmp  nz, .L_udiv_loop
	ret

; __udiv16: HL = HL / DE (unsigned)
__udiv16:
	call __udivmod16
	ld   h, b
	ld   l, c
	ret

; __umod16: HL = HL % DE (unsigned)
__umod16:
	call __udivmod16
	ret

; __div16: HL = HL / DE (signed)
__div16:
	ld   a, 0
	ld   (.L_sign_div), a
	bit  7, h
	jmp  z, .L_div_hl_pos
	call __neg_hl
	ld   a, (.L_sign_div)
	cpl
	ld   (.L_sign_div), a
.L_div_hl_pos:
	bit  7, d
	jmp  z, .L_div_de_pos
	call __neg_de
	ld   a, (.L_sign_div)
	cpl
	ld   (.L_sign_div), a
.L_div_de_pos:
	call __udiv16
	ld   a, (.L_sign_div)
	or   a
	ret  z
	call __neg_hl
	ret

; __mod16: HL = HL % DE (signed, remainder sign follows dividend)
__mod16:
	ld   a, 0
	ld   (.L_sign_mod), a
	bit  7, h
	jmp  z, .L_mod_hl_pos
	call __neg_hl
	ld   a, 1
	ld   (.L_sign_mod), a
.L_mod_hl_pos:
	bit  7, d
	jmp  z, .L_mod_de_pos
	call __neg_de
.L_mod_de_pos:
	call __umod16
	ld   a, (.L_sign_mod)
	or   a
	ret  z
	call __neg_hl
	ret

__neg_hl:
	ld   a, h
	cpl
	ld   h, a
	ld   a, l
	cpl
	ld   l, a
	inc  hl
	ret

__neg_de:
	ld   a, d
	cpl
	ld   d, a
	ld   a, e
	cpl
	ld   e, a
	inc  de
	ret

__shl16:
	ld   a, c
	or   a
	ret  z
.L_shl_loop:
	add  hl, hl
	dec  a
	jmp  nz, .L_shl_loop
	ret

__shr16:
	ld   a, c
	or   a
	ret  z
.L_shr_loop:
	srl  h
	rr   l
	dec  a
	jmp  nz, .L_shr_loop
	ret

__sar16:
	ld   a, c
	or   a
	ret  z
.L_sar_loop:
	sra  h
	rr   l
	dec  a
	jmp  nz, .L_sar_loop
	ret

_panic:
	ld   hl, .L_panic_msg_fmt
	push hl
	call _printf
	pop  bc
	ld   hl, 1
	push hl
	call _exit

.L_sign_div:
	defb 0
.L_sign_mod:
	defb 0
.L_div_zero_str:
	defb "division by zero", 0
.L_panic_msg_fmt:
	defb "\n*PANIC*\n", 0
.L_empty_chain_msg:
	defb "\n*** ABORT\n\n*** EMPTY_RE_CHAIN\n", 0
`)
}

func (b *Backend) Generate(prog *ir.Program) string {
	b.program = prog
	b.buf.Reset()
	b.rodata = nil

	b.buf.WriteString("; ── Z80 Whole-Program Assembly (MiniGolf) ───────────────────────────────────\n")

	// Emit entry dispatch
	b.buf.WriteString("_main:\n")
	b.buf.WriteString("\tcall f_main__main\n")
	b.buf.WriteString("\tld   hl, 0\n")
	b.buf.WriteString("\tret\n")

	for _, f := range prog.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
		}
	}

	b.emitGlobals(prog)
	b.emitHelpers()

	return optimizeAsm(b.buf.String())
}

func normLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func optimizeAsm(asm string) string {
	lines := strings.Split(asm, "\n")

	for pass := 0; pass < 5; pass++ {
		changed := false
		var out []string
		n := len(lines)

		for i := 0; i < n; i++ {
			line := lines[i]
			trimmed := strings.TrimSpace(line)

			// Skip comment-only or blank lines directly to out
			if trimmed == "" || strings.HasPrefix(trimmed, ";") {
				out = append(out, line)
				continue
			}

			norm := normLine(trimmed)

			// Find next non-empty, non-comment line index
			nextIdx := -1
			for j := i + 1; j < n; j++ {
				t := strings.TrimSpace(lines[j])
				if t != "" && !strings.HasPrefix(t, ";") {
					nextIdx = j
					break
				}
			}

			// 1. Redundant unconditional jump to next label:
			//    jmp target
			//    target:
			if strings.HasPrefix(norm, "jmp ") && !strings.Contains(norm, ",") {
				target := strings.TrimSpace(strings.TrimPrefix(norm, "jmp "))
				if nextIdx >= 0 && normLine(lines[nextIdx]) == target+":" {
					changed = true
					continue
				}
			}

			// 2. Redundant 8-bit store then reload:
			//    ld (ix+d), a
			//    ld a, (ix+d)
			if strings.HasPrefix(norm, "ld (ix") && strings.HasSuffix(norm, "), a") {
				slot := norm[3 : len(norm)-3] // "(ix...)"
				if nextIdx >= 0 {
					nextNorm := normLine(lines[nextIdx])
					if nextNorm == "ld a, "+slot {
						lines[nextIdx] = ";" + lines[nextIdx]
						changed = true
					}
				}
			}

			// 3. Redundant 16-bit store then reload:
			//    ld (ix+d), l
			//    ld (ix+d+1), h
			//    ld l, (ix+d)
			//    ld h, (ix+d+1)
			if strings.HasPrefix(norm, "ld (ix") && strings.HasSuffix(norm, "), l") {
				slotL := norm[3 : len(norm)-3]
				if nextIdx >= 0 {
					next1Norm := normLine(lines[nextIdx])
					if strings.HasPrefix(next1Norm, "ld (ix") && strings.HasSuffix(next1Norm, "), h") {
						slotH := next1Norm[3 : len(next1Norm)-3]
						next2Idx := -1
						for j := nextIdx + 1; j < n; j++ {
							t := strings.TrimSpace(lines[j])
							if t != "" && !strings.HasPrefix(t, ";") {
								next2Idx = j
								break
							}
						}
						if next2Idx >= 0 {
							next2Norm := normLine(lines[next2Idx])
							if next2Norm == "ld l, "+slotL {
								next3Idx := -1
								for j := next2Idx + 1; j < n; j++ {
									t := strings.TrimSpace(lines[j])
									if t != "" && !strings.HasPrefix(t, ";") {
										next3Idx = j
										break
									}
								}
								if next3Idx >= 0 {
									next3Norm := normLine(lines[next3Idx])
									if next3Norm == "ld h, "+slotH {
										lines[next2Idx] = ";" + lines[next2Idx]
										lines[next3Idx] = ";" + lines[next3Idx]
										changed = true
									}
								}
							}
						}
					}
				}
			}

			// 4. Branch inversion:
			//    jmp cond, L1
			//    jmp L2
			//    L1:
			//    ->
			//    jmp invCond, L2
			//    L1:
			if strings.HasPrefix(norm, "jmp ") && strings.Contains(norm, ",") && nextIdx >= 0 {
				parts := strings.Split(strings.TrimPrefix(norm, "jmp "), ",")
				if len(parts) == 2 {
					cond := strings.TrimSpace(parts[0])
					target1 := strings.TrimSpace(parts[1])
					next1Norm := normLine(lines[nextIdx])
					if strings.HasPrefix(next1Norm, "jmp ") && !strings.Contains(next1Norm, ",") {
						target2 := strings.TrimSpace(strings.TrimPrefix(next1Norm, "jmp "))
						next2Idx := -1
						for j := nextIdx + 1; j < n; j++ {
							t := strings.TrimSpace(lines[j])
							if t != "" && !strings.HasPrefix(t, ";") {
								next2Idx = j
								break
							}
						}
						if next2Idx >= 0 && normLine(lines[next2Idx]) == target1+":" {
							invCond := ""
							switch cond {
							case "z":
								invCond = "nz"
							case "nz":
								invCond = "z"
							case "c":
								invCond = "nc"
							case "nc":
								invCond = "c"
							}
							if invCond != "" {
								out = append(out, fmt.Sprintf("\tjmp  %s, %s", invCond, target2))
								lines[nextIdx] = ";" + lines[nextIdx]
								changed = true
								continue
							}
						}
					}
				}
			}

			// 5. cp 0 -> or a
			if norm == "cp 0" {
				out = append(out, "\tor   a")
				changed = true
				continue
			}

			// 6. Redundant register moves: ld r, r
			if norm == "ld a, a" || norm == "ld b, b" || norm == "ld c, c" ||
				norm == "ld d, d" || norm == "ld e, e" || norm == "ld h, h" || norm == "ld l, l" {
				changed = true
				continue
			}

			// 7. Redundant consecutive ex de, hl
			if norm == "ex de, hl" && nextIdx >= 0 {
				if normLine(lines[nextIdx]) == "ex de, hl" {
					lines[nextIdx] = ";" + lines[nextIdx]
					changed = true
					continue
				}
			}

			// 8. Redundant push R / pop R
			if strings.HasPrefix(norm, "push ") && nextIdx >= 0 {
				reg := strings.TrimSpace(strings.TrimPrefix(norm, "push "))
				if normLine(lines[nextIdx]) == "pop "+reg {
					lines[nextIdx] = ";" + lines[nextIdx]
					changed = true
					continue
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
