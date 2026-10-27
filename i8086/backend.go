package i8086

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/strickyak/minigolf/ir"
	"github.com/strickyak/minigolf/m6809"
)

func alignVal(val, align int) int {
	return (val + align - 1) & ^(align - 1)
}

type rodataEntry struct {
	lbl string
	str string
}

type Backend struct {
	buf             bytes.Buffer
	program         *ir.Program
	slots           map[int]int    // instruction ID -> positive byte offset below BP (accessed as [BP-offset])
	jmpSlots        map[int]int    // SetJmp instruction ID -> positive jmpbuf offset below BP
	paramSlots      map[string]int // param name -> positive byte offset above BP (accessed as [BP+offset])
	typeMap         map[int]ir.Type
	fusedCompares   map[int]bool
	stackSize       int
	retPtrSlot      int
	currentFunc     *ir.Function
	lblCount        int
	rodata          []rodataEntry
	GlobalVarOffset int
}

func bpDisp(d int) string {
	if d > 0 {
		return fmt.Sprintf("[BP+%d]", d)
	}
	if d < 0 {
		return fmt.Sprintf("[BP-%d]", -d)
	}
	return "[BP]"
}

func New() *Backend {
	offset := 0x0200
	if m6809.GLOBAL_VAR_OFFSET != nil && *m6809.GLOBAL_VAR_OFFSET != 16 && *m6809.GLOBAL_VAR_OFFSET > 0 {
		offset = *m6809.GLOBAL_VAR_OFFSET
	}
	return &Backend{
		slots:           make(map[int]int),
		jmpSlots:        make(map[int]int),
		paramSlots:      make(map[string]int),
		typeMap:         make(map[int]ir.Type),
		GlobalVarOffset: offset,
	}
}

func (b *Backend) nextLabel() string {
	b.lblCount++
	return fmt.Sprintf(".LL%d", b.lblCount)
}

func (b *Backend) getTypeAlignment(typ ir.Type) int {
	return 1 // 8086 is byte-addressable with no alignment restrictions
}

func (b *Backend) getTypeSize(typ ir.Type) int {
	switch typ.Name {
	case "byte", "bool":
		return 1
	case "word", "int", "uint", "const_integer", "noreturn":
		return 2 // 16-bit word on Intel 8086
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

func instrIndexInBlock(blk *ir.BasicBlock, target ir.Instruction) int {
	if target == nil {
		return -1
	}
	for idx, inst := range blk.Instructions {
		if inst == target {
			return idx
		}
	}
	return -1
}

func (b *Backend) findBlockFusedCompare(f *ir.Function, blk *ir.BasicBlock) (fusedCompare *ir.Compare, redundantOuterCmp ir.Instruction, redundantZero ir.Instruction) {
	branch, ok := blk.Terminator.(*ir.Branch)
	if !ok {
		return nil, nil, nil
	}
	canFuseInst := func(inst ir.Instruction) bool {
		if inst == nil {
			return false
		}
		if instrIndexInBlock(blk, inst) < 0 {
			return false
		}
		if b.resolveSlot(f, inst.GetID()) != inst.GetID() {
			return false
		}
		return countUses(f, inst) == 1
	}

	cond := branch.Condition
	if outerCmp, ok := cond.(*ir.Compare); ok && outerCmp.Op == "neq" && isZero(outerCmp.Right) {
		if innerCmp, ok := outerCmp.Left.(*ir.Compare); ok {
			if canFuseInst(outerCmp) && canFuseInst(innerCmp) {
				var zeroInst ir.Instruction
				if zi, ok := outerCmp.Right.(ir.Instruction); ok && canFuseInst(zi) {
					zeroInst = zi
				}
				innerIdx := instrIndexInBlock(blk, innerCmp)
				safe := true
				for i := innerIdx + 1; i < len(blk.Instructions); i++ {
					inst := blk.Instructions[i]
					if _, isTerm := inst.(ir.Terminator); isTerm {
						continue
					}
					if inst != outerCmp && inst != zeroInst {
						safe = false
						break
					}
				}
				if safe {
					return innerCmp, outerCmp, zeroInst
				}
			}
		}
	}
	if cmp, ok := cond.(*ir.Compare); ok && canFuseInst(cmp) {
		cmpIdx := instrIndexInBlock(blk, cmp)
		safe := true
		for i := cmpIdx + 1; i < len(blk.Instructions); i++ {
			inst := blk.Instructions[i]
			if _, isTerm := inst.(ir.Terminator); isTerm {
				continue
			}
			safe = false
			break
		}
		if safe {
			return cmp, nil, nil
		}
	}
	return nil, nil, nil
}

func (b *Backend) allocateSlots(f *ir.Function) {
	b.slots = make(map[int]int)
	b.jmpSlots = make(map[int]int)
	b.paramSlots = make(map[string]int)
	b.typeMap = make(map[int]ir.Type)
	b.fusedCompares = make(map[int]bool)

	// Pre-scan for fused compare-and-branch sequences to avoid allocating slots for them
	for _, blk := range f.Blocks {
		fusedCmp, outerCmp, zeroInst := b.findBlockFusedCompare(f, blk)
		if fusedCmp != nil {
			b.fusedCompares[fusedCmp.GetID()] = true
		}
		if outerCmp != nil {
			b.fusedCompares[outerCmp.GetID()] = true
		}
		if zeroInst != nil {
			b.fusedCompares[zeroInst.GetID()] = true
		}
	}

	// On 8086 Small model:
	// [BP+0] = saved caller BP
	// [BP+2] = 16-bit near return IP
	// [BP+4] = first caller argument
	paramOffset := 4
	for _, p := range f.Parameters {
		sz := b.getTypeSize(p.Typ)
		b.paramSlots[p.Name] = paramOffset
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

	// Allocate small (<= 2 byte) scalar slots in pass 0 so scalars stay within
	// 8-bit signed displacement [BP-2]..[BP-128], then larger structs/arrays in pass 1.
	for pass := 0; pass < 2; pass++ {
		for _, blk := range f.Blocks {
			for _, instr := range blk.Instructions {
				id := instr.GetID()
				canon := b.resolveSlot(f, id)

				if _, ok := instr.(*ir.SetJmp); ok && pass == 1 {
					currentOffset += 8
					b.jmpSlots[id] = currentOffset
				}

				if b.fusedCompares[id] && !addressTaken[id] && !addressTaken[canon] {
					continue
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
				isScalar := size <= 2 && !instr.Type().IsAStruct() && !instr.Type().IsAnArray()
				if (pass == 0 && isScalar) || (pass == 1 && !isScalar) {
					currentOffset += alignVal(size, 2)
					b.slots[canon] = currentOffset
					b.slots[id] = currentOffset
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

	b.stackSize = alignVal(currentOffset, 2)
}

func (b *Backend) slotsType(id int) ir.Type {
	if t, ok := b.typeMap[id]; ok {
		return t
	}
	return ir.TypeInt
}

func (b *Backend) emitAddConst(reg string, n int) {
	uReg := strings.ToUpper(reg)
	if n == 0 {
		return
	}
	if n == 1 {
		b.buf.WriteString(fmt.Sprintf("\tINC  %s\n", uReg))
		return
	}
	if n == 2 {
		b.buf.WriteString(fmt.Sprintf("\tINC  %s\n\tINC  %s\n", uReg, uReg))
		return
	}
	if n == -1 {
		b.buf.WriteString(fmt.Sprintf("\tDEC  %s\n", uReg))
		return
	}
	if n == -2 {
		b.buf.WriteString(fmt.Sprintf("\tDEC  %s\n\tDEC  %s\n", uReg, uReg))
		return
	}
	if n > 0 {
		b.buf.WriteString(fmt.Sprintf("\tADD  %s, %d\n", uReg, n))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tSUB  %s, %d\n", uReg, -n))
	}
}

func (b *Backend) emitLoadAddr(reg string, val ir.Value) {
	uReg := strings.ToUpper(reg)
	switch v := val.(type) {
	case *ir.Parameter:
		off := b.paramSlots[v.Name]
		b.buf.WriteString(fmt.Sprintf("\tLEA  %s, %s\n", uReg, bpDisp(off)))
	case ir.Instruction:
		canon := b.resolveSlot(b.currentFunc, v.GetID())
		off, ok := b.slots[canon]
		if !ok || off == 0 {
			off, ok = b.slots[v.GetID()]
		}
		if !ok || off == 0 {
			log.Panicf("emitLoadAddr: instruction %s (id=%d) has no stack slot in function %s", v, v.GetID(), b.currentFunc.Name)
		}
		b.buf.WriteString(fmt.Sprintf("\tLEA  %s, %s\n", uReg, bpDisp(-off)))
	case *ir.Global:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, v_%s\n", uReg, v.Name))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, v_%s\n", uReg, v.Global.Name))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, v.Func.EmitName()))
	case *ir.StringLiteral:
		b.lblCount++
		lbl := fmt.Sprintf(".Lstr%d", b.lblCount)
		b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: v.Value})
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, lbl))
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

func reg8Halves(reg16 string) (string, string) {
	switch strings.ToUpper(reg16) {
	case "AX":
		return "AL", "AH"
	case "BX":
		return "BL", "BH"
	case "CX":
		return "CL", "CH"
	case "DX":
		return "DL", "DH"
	}
	return "", ""
}

func (b *Backend) loadVal(val ir.Value, reg string) {
	uReg := strings.ToUpper(reg)
	is8 := (uReg == "AL" || uReg == "BL" || uReg == "CL" || uReg == "DL")

	if is8 {
		switch v := val.(type) {
		case *ir.ConstByte:
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %d\n", uReg, v.Val))
		case *ir.ConstWord:
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %d\n", uReg, v.Val&0xFF))
		case *ir.Sizeof:
			sz := b.getTypeSize(v.TargetTyp)
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %d\n", uReg, sz&0xFF))
		case *ir.Parameter:
			off := b.paramSlots[v.Name]
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, bpDisp(off)))
		case *ir.Global:
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, [v_%s]\n", uReg, v.Name))
		case *ir.AddressOfGlobal:
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, v_%s\n", uReg, v.Global.Name))
		case ir.Instruction:
			off := b.slots[v.GetID()]
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, bpDisp(-off)))
		default:
			log.Panicf("loadVal(%s): unhandled type %T", uReg, val)
		}
		return
	}

	// 16-bit register: AX, BX, CX, DX, SI, DI
	lo8, hi8 := reg8Halves(uReg)
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %d\n", uReg, v.Val))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %d\n", uReg, v.Val&0xFFFF))
	case *ir.Sizeof:
		sz := b.getTypeSize(v.TargetTyp)
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %d\n", uReg, sz))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, v_%s\n", uReg, v.Global.Name))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, v.Func.EmitName()))
	case *ir.AddressOfLocal:
		b.emitLoadAddr(uReg, v.Local)
	case *ir.StringLiteral:
		b.emitLoadAddr(uReg, v)
	case *ir.Parameter:
		sz := b.getTypeSize(v.Typ)
		off := b.paramSlots[v.Name]
		if sz == 1 {
			if lo8 != "" {
				b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n\tMOV  %s, 0\n", lo8, bpDisp(off), hi8))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AL, %s\n\tMOV  AH, 0\n\tMOV  %s, AX\n", bpDisp(off), uReg))
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, bpDisp(off)))
		}
	case *ir.Global:
		sz := b.getTypeSize(v.Typ)
		if sz == 1 {
			if lo8 != "" {
				b.buf.WriteString(fmt.Sprintf("\tMOV  %s, [v_%s]\n\tMOV  %s, 0\n", lo8, v.Name, hi8))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AL, [v_%s]\n\tMOV  AH, 0\n\tMOV  %s, AX\n", v.Name, uReg))
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, [v_%s]\n", uReg, v.Name))
		}
	case ir.Instruction:
		sz := b.getTypeSize(v.Type())
		off := b.slots[v.GetID()]
		if sz == 1 {
			if lo8 != "" {
				b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n\tMOV  %s, 0\n", lo8, bpDisp(-off), hi8))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AL, %s\n\tMOV  AH, 0\n\tMOV  %s, AX\n", bpDisp(-off), uReg))
			}
		} else {
			b.buf.WriteString(fmt.Sprintf("\tMOV  %s, %s\n", uReg, bpDisp(-off)))
		}
	default:
		log.Panicf("loadVal(%s): unhandled type %T", uReg, val)
	}
}

func (b *Backend) storeResult(id int) {
	sz := b.getTypeSize(b.slotsType(id))
	off, ok := b.slots[id]
	if !ok {
		log.Panicf("storeResult: instruction %d has no slot", id)
	}
	if sz == 1 {
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, AL\n", bpDisp(-off)))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, AX\n", bpDisp(-off)))
	}
}

// emitMemCopy copies `size` bytes from DS:[SI] to ES:[DI].
func (b *Backend) emitMemCopy(size int) {
	if size <= 0 {
		return
	}
	if size == 1 {
		b.buf.WriteString("\tMOV  AL, [SI]\n\tMOV  [DI], AL\n")
		return
	}
	if size == 2 {
		b.buf.WriteString("\tMOV  AX, [SI]\n\tMOV  [DI], AX\n")
		return
	}
	if size == 4 {
		b.buf.WriteString("\tMOV  AX, [SI]\n\tMOV  [DI], AX\n\tMOV  AX, [SI+2]\n\tMOV  [DI+2], AX\n")
		return
	}
	b.buf.WriteString(fmt.Sprintf("\tMOV  CX, %d\n\tREP  MOVSB\n", size))
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from && edge.Value != nil {
					size := b.getTypeSize(phi.Typ)
					if size == 1 {
						b.loadVal(edge.Value, "al")
						b.storeResult(phi.GetID())
					} else if size == 2 {
						b.loadVal(edge.Value, "ax")
						b.storeResult(phi.GetID())
					} else {
						b.emitLoadAddr("si", edge.Value)
						b.emitLoadAddr("di", phi)
						b.emitMemCopy(size)
					}
				}
			}
		}
	}
}

func (b *Backend) emitBinaryOp(i *ir.BinaryOp) {
	origSz := b.getTypeSize(i.Typ)
	sz := origSz
	if sz == 1 {
		switch i.Op {
		case "add", "sub", "and", "bitand", "or", "bitor", "xor", "bitxor", "andnot":
			b.loadVal(i.Left, "al")
			if i.Op == "andnot" {
				b.loadVal(i.Right, "dl")
				b.buf.WriteString("\tNOT  DL\n\tAND  AL, DL\n")
				b.storeResult(i.GetID())
				return
			}
			var immVal int
			hasImm := false
			if cb, ok := i.Right.(*ir.ConstByte); ok {
				immVal = int(cb.Val)
				hasImm = true
			} else if cw, ok := i.Right.(*ir.ConstWord); ok {
				immVal = int(cw.Val & 0xFF)
				hasImm = true
			}
			if hasImm {
				switch i.Op {
				case "add":
					if immVal == 1 {
						b.buf.WriteString("\tINC  AL\n")
					} else if immVal != 0 {
						b.buf.WriteString(fmt.Sprintf("\tADD  AL, %d\n", immVal))
					}
				case "sub":
					if immVal == 1 {
						b.buf.WriteString("\tDEC  AL\n")
					} else if immVal != 0 {
						b.buf.WriteString(fmt.Sprintf("\tSUB  AL, %d\n", immVal))
					}
				case "and", "bitand":
					b.buf.WriteString(fmt.Sprintf("\tAND  AL, %d\n", immVal))
				case "or", "bitor":
					b.buf.WriteString(fmt.Sprintf("\tOR   AL, %d\n", immVal))
				case "xor", "bitxor":
					b.buf.WriteString(fmt.Sprintf("\tXOR  AL, %d\n", immVal))
				}
			} else {
				b.loadVal(i.Right, "dl")
				switch i.Op {
				case "add":
					b.buf.WriteString("\tADD  AL, DL\n")
				case "sub":
					b.buf.WriteString("\tSUB  AL, DL\n")
				case "and", "bitand":
					b.buf.WriteString("\tAND  AL, DL\n")
				case "or", "bitor":
					b.buf.WriteString("\tOR   AL, DL\n")
				case "xor", "bitxor":
					b.buf.WriteString("\tXOR  AL, DL\n")
				}
			}
			b.storeResult(i.GetID())
			return
		default:
			sz = 2
		}
	}

	// 16-bit binary operations in AX
	b.loadVal(i.Left, "ax")

	switch i.Op {
	case "add":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			b.emitAddConst("ax", int(int16(uint16(cw.Val))))
		} else {
			b.loadVal(i.Right, "dx")
			b.buf.WriteString("\tADD  AX, DX\n")
		}
	case "sub":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			b.emitAddConst("ax", -int(int16(uint16(cw.Val))))
		} else {
			b.loadVal(i.Right, "dx")
			b.buf.WriteString("\tSUB  AX, DX\n")
		}
	case "mul":
		b.loadVal(i.Right, "dx")
		b.buf.WriteString("\tMUL  DX\n")
	case "div":
		b.loadVal(i.Right, "bx")
		if i.Left.Type().IsInt() {
			b.buf.WriteString("\tCALL __div16\n")
		} else {
			b.buf.WriteString("\tCALL __udiv16\n")
		}
	case "mod":
		b.loadVal(i.Right, "bx")
		if i.Left.Type().IsInt() {
			b.buf.WriteString("\tCALL __mod16\n")
		} else {
			b.buf.WriteString("\tCALL __umod16\n")
		}
	case "and", "bitand":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tAND  AX, %d\n", cw.Val&0xFFFF))
		} else {
			b.loadVal(i.Right, "dx")
			b.buf.WriteString("\tAND  AX, DX\n")
		}
	case "or", "bitor":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tOR   AX, %d\n", cw.Val&0xFFFF))
		} else {
			b.loadVal(i.Right, "dx")
			b.buf.WriteString("\tOR   AX, DX\n")
		}
	case "xor", "bitxor":
		if cw, ok := i.Right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tXOR  AX, %d\n", cw.Val&0xFFFF))
		} else {
			b.loadVal(i.Right, "dx")
			b.buf.WriteString("\tXOR  AX, DX\n")
		}
	case "andnot":
		b.loadVal(i.Right, "dx")
		b.buf.WriteString("\tNOT  DX\n\tAND  AX, DX\n")
	case "shl":
		if cw, ok := i.Right.(*ir.ConstWord); ok && cw.Val <= 3 {
			for k := uint64(0); k < cw.Val; k++ {
				b.buf.WriteString("\tSHL  AX, 1\n")
			}
		} else if cb, ok := i.Right.(*ir.ConstByte); ok && cb.Val <= 3 {
			for k := uint8(0); k < cb.Val; k++ {
				b.buf.WriteString("\tSHL  AX, 1\n")
			}
		} else {
			b.loadVal(i.Right, "cl")
			b.buf.WriteString("\tSHL  AX, CL\n")
		}
	case "shr":
		shiftMnemonic := "SHR"
		if i.Left.Type().IsInt() {
			shiftMnemonic = "SAR"
		}
		if cw, ok := i.Right.(*ir.ConstWord); ok && cw.Val <= 3 {
			for k := uint64(0); k < cw.Val; k++ {
				b.buf.WriteString(fmt.Sprintf("\t%s  AX, 1\n", shiftMnemonic))
			}
		} else if cb, ok := i.Right.(*ir.ConstByte); ok && cb.Val <= 3 {
			for k := uint8(0); k < cb.Val; k++ {
				b.buf.WriteString(fmt.Sprintf("\t%s  AX, 1\n", shiftMnemonic))
			}
		} else {
			b.loadVal(i.Right, "cl")
			b.buf.WriteString(fmt.Sprintf("\t%s  AX, CL\n", shiftMnemonic))
		}
	default:
		log.Panicf("emitBinaryOp: unsupported op %s", i.Op)
	}

	b.storeResult(i.GetID())
}

func condJumpMnemonic(op string, isUnsigned bool) string {
	switch op {
	case "eq":
		return "JE"
	case "neq":
		return "JNE"
	case "lt":
		if isUnsigned {
			return "JB"
		}
		return "JL"
	case "lte":
		if isUnsigned {
			return "JBE"
		}
		return "JLE"
	case "gt":
		if isUnsigned {
			return "JA"
		}
		return "JG"
	case "gte":
		if isUnsigned {
			return "JAE"
		}
		return "JGE"
	default:
		log.Panicf("condJumpMnemonic: unsupported op %s", op)
		return ""
	}
}

func (b *Backend) emitCmpOperands(left, right ir.Value) {
	sz := b.getTypeSize(left.Type())
	if sz == 1 {
		b.loadVal(left, "al")
		if cb, ok := right.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tCMP  AL, %d\n", cb.Val))
		} else if cw, ok := right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tCMP  AL, %d\n", cw.Val&0xFF))
		} else {
			b.loadVal(right, "dl")
			b.buf.WriteString("\tCMP  AL, DL\n")
		}
	} else {
		b.loadVal(left, "ax")
		if cw, ok := right.(*ir.ConstWord); ok {
			b.buf.WriteString(fmt.Sprintf("\tCMP  AX, %d\n", cw.Val&0xFFFF))
		} else if cb, ok := right.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tCMP  AX, %d\n", cb.Val))
		} else {
			b.loadVal(right, "dx")
			b.buf.WriteString("\tCMP  AX, DX\n")
		}
	}
}

func (b *Backend) emitCompare(i *ir.Compare) {
	isUnsigned := i.Left.Type().IsWord() || i.Left.Type().IsByte() || i.Left.Type().IsAPointer()
	b.emitCmpOperands(i.Left, i.Right)

	doneLbl := b.nextLabel()
	jcc := condJumpMnemonic(i.Op, isUnsigned)

	b.buf.WriteString("\tMOV  AL, 1\n")
	b.buf.WriteString(fmt.Sprintf("\t%s   %s\n", jcc, doneLbl))
	b.buf.WriteString("\tXOR  AL, AL\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", doneLbl))

	b.storeResult(i.GetID())
}

func (b *Backend) emitFusedCompareAndBranch(f *ir.Function, blk *ir.BasicBlock, branch *ir.Branch, cmp *ir.Compare) {
	isUnsigned := cmp.Left.Type().IsWord() || cmp.Left.Type().IsByte() || cmp.Left.Type().IsAPointer()
	b.emitCmpOperands(cmp.Left, cmp.Right)

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

	jcc := condJumpMnemonic(cmp.Op, isUnsigned)
	b.buf.WriteString(fmt.Sprintf("\t%s   %s\n", jcc, trueTarget))
	b.buf.WriteString(fmt.Sprintf("\tJMP  %s\n", falseTarget))

	if hasTruePhi {
		b.buf.WriteString(fmt.Sprintf("%s:\n", trueTarget))
		b.emitPhiAssignments(blk, branch.TrueBlock)
		b.buf.WriteString(fmt.Sprintf("\tJMP  .L_%s_b%d\n", f.Name, branch.TrueBlock.ID))
	}

	if hasFalsePhi {
		b.buf.WriteString(fmt.Sprintf("%s:\n", falseTarget))
		b.emitPhiAssignments(blk, branch.FalseBlock)
		b.buf.WriteString(fmt.Sprintf("\tJMP  .L_%s_b%d\n", f.Name, branch.FalseBlock.ID))
	}
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))

	case *ir.ConstByte:
		if _, ok := b.slots[id]; ok {
			b.loadVal(i, "al")
			b.storeResult(id)
		}

	case *ir.ConstWord, *ir.Sizeof:
		if _, ok := b.slots[id]; ok {
			b.loadVal(i, "ax")
			b.storeResult(id)
		}

	case *ir.Load:
		sz := b.getTypeSize(i.Global.Typ)
		if sz == 0 {
			return
		} else if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tMOV  AL, [v_%s]\n", i.Global.Name))
			b.storeResult(id)
		} else if sz == 2 {
			b.buf.WriteString(fmt.Sprintf("\tMOV  AX, [v_%s]\n", i.Global.Name))
			b.storeResult(id)
		} else {
			b.buf.WriteString(fmt.Sprintf("\tMOV  SI, v_%s\n", i.Global.Name))
			b.emitLoadAddr("di", i)
			b.emitMemCopy(sz)
		}

	case *ir.Store:
		sz := b.getTypeSize(i.Global.Typ)
		if sz == 0 {
			return
		} else if sz == 1 {
			b.loadVal(i.Val, "al")
			b.buf.WriteString(fmt.Sprintf("\tMOV  [v_%s], AL\n", i.Global.Name))
		} else if sz == 2 {
			b.loadVal(i.Val, "ax")
			b.buf.WriteString(fmt.Sprintf("\tMOV  [v_%s], AX\n", i.Global.Name))
		} else {
			b.emitLoadAddr("si", i.Val)
			b.buf.WriteString(fmt.Sprintf("\tMOV  DI, v_%s\n", i.Global.Name))
			b.emitMemCopy(sz)
		}

	case *ir.ZeroInit:
		sz := b.getTypeSize(i.Typ)
		if sz == 0 {
			return
		} else if sz == 1 {
			off := b.slots[id]
			b.buf.WriteString(fmt.Sprintf("\tMOV  BYTE PTR %s, 0\n", bpDisp(-off)))
		} else if sz == 2 {
			off := b.slots[id]
			b.buf.WriteString(fmt.Sprintf("\tMOV  WORD PTR %s, 0\n", bpDisp(-off)))
		} else {
			b.emitLoadAddr("di", i)
			b.buf.WriteString(fmt.Sprintf("\tXOR  AL, AL\n\tMOV  CX, %d\n\tREP  STOSB\n", sz))
		}

	case *ir.BinaryOp:
		b.emitBinaryOp(i)

	case *ir.Compare:
		b.emitCompare(i)

	case *ir.UnaryOp:
		sz := b.getTypeSize(i.Typ)
		if sz == 1 {
			b.loadVal(i.Operand, "al")
			switch i.Op {
			case "neg":
				b.buf.WriteString("\tNEG  AL\n")
			case "not", "bitnot":
				if i.Typ.IsBool() {
					b.buf.WriteString("\tXOR  AL, 1\n")
				} else {
					b.buf.WriteString("\tNOT  AL\n")
				}
			default:
				log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
			}
		} else {
			b.loadVal(i.Operand, "ax")
			switch i.Op {
			case "neg":
				b.buf.WriteString("\tNEG  AX\n")
			case "not", "bitnot":
				b.buf.WriteString("\tNOT  AX\n")
			default:
				log.Panicf("emitUnaryOp: unsupported op %s", i.Op)
			}
		}
		b.storeResult(id)

	case *ir.Cast:
		srcSz := b.getTypeSize(i.Operand.Type())
		dstSz := b.getTypeSize(i.Typ)
		if dstSz > 2 {
			b.emitLoadAddr("si", i.Operand)
			b.emitLoadAddr("di", i)
			b.emitMemCopy(dstSz)
			return
		}
		if dstSz == 1 {
			if srcSz == 1 {
				b.loadVal(i.Operand, "al")
			} else {
				b.loadVal(i.Operand, "ax")
			}
		} else {
			b.loadVal(i.Operand, "ax")
		}
		b.storeResult(id)

	case *ir.AddressOfLocal:
		b.emitLoadAddr("ax", i.Local)
		b.storeResult(id)

	case *ir.AddressOfGlobal, *ir.AddressOfFunc:
		if _, ok := b.slots[id]; ok {
			b.loadVal(i, "ax")
			b.storeResult(id)
		}

	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		if glob, ok := i.Ptr.(*ir.AddressOfGlobal); ok {
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AX, v_%s + %d\n", glob.Global.Name, byteOffset))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AX, v_%s\n", glob.Global.Name))
			}
			b.storeResult(id)
			return
		}
		b.loadVal(i.Ptr, "ax")
		b.emitAddConst("ax", byteOffset)
		b.storeResult(id)

	case *ir.AddressOfElement:
		eltSize := b.getEltSize(i.ArrayPtr.Type())
		if glob, ok := i.ArrayPtr.(*ir.AddressOfGlobal); ok {
			if cIdx, ok2 := i.Index.(*ir.ConstWord); ok2 {
				byteOffset := int(cIdx.Val) * eltSize
				if byteOffset > 0 {
					b.buf.WriteString(fmt.Sprintf("\tMOV  AX, v_%s + %d\n", glob.Global.Name, byteOffset))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tMOV  AX, v_%s\n", glob.Global.Name))
				}
				b.storeResult(id)
				return
			}
		}
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			b.loadVal(i.ArrayPtr, "ax")
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConst("ax", byteOffset)
		} else {
			b.loadVal(i.Index, "ax")
			if eltSize == 1 {
				b.loadVal(i.ArrayPtr, "dx")
				b.buf.WriteString("\tADD  AX, DX\n")
			} else if eltSize == 2 {
				b.buf.WriteString("\tSHL  AX, 1\n")
				b.loadVal(i.ArrayPtr, "dx")
				b.buf.WriteString("\tADD  AX, DX\n")
			} else if eltSize == 4 {
				b.buf.WriteString("\tSHL  AX, 1\n\tSHL  AX, 1\n")
				b.loadVal(i.ArrayPtr, "dx")
				b.buf.WriteString("\tADD  AX, DX\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  DX, %d\n\tMUL  DX\n", eltSize))
				b.loadVal(i.ArrayPtr, "dx")
				b.buf.WriteString("\tADD  AX, DX\n")
			}
		}
		b.storeResult(id)

	case *ir.LoadPtr:
		sz := b.getTypeSize(i.Typ)
		if sz == 0 {
			return
		}
		if addrStr, ok := b.getDirectGlobalAddr(i.Ptr); ok {
			if sz == 1 {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AL, [%s]\n", addrStr))
				b.storeResult(id)
			} else if sz == 2 {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AX, [%s]\n", addrStr))
				b.storeResult(id)
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  SI, %s\n", addrStr))
				b.emitLoadAddr("di", i)
				b.emitMemCopy(sz)
			}
			return
		}
		if sz == 1 {
			b.loadVal(i.Ptr, "bx")
			b.buf.WriteString("\tMOV  AL, [BX]\n")
			b.storeResult(id)
		} else if sz == 2 {
			b.loadVal(i.Ptr, "bx")
			b.buf.WriteString("\tMOV  AX, [BX]\n")
			b.storeResult(id)
		} else {
			b.loadVal(i.Ptr, "si")
			b.emitLoadAddr("di", i)
			b.emitMemCopy(sz)
		}

	case *ir.StorePtr:
		sz := b.getTypeSize(i.Val.Type())
		if sz == 0 {
			return
		}
		if addrStr, ok := b.getDirectGlobalAddr(i.Ptr); ok {
			if sz == 1 {
				b.loadVal(i.Val, "al")
				b.buf.WriteString(fmt.Sprintf("\tMOV  [%s], AL\n", addrStr))
			} else if sz == 2 {
				b.loadVal(i.Val, "ax")
				b.buf.WriteString(fmt.Sprintf("\tMOV  [%s], AX\n", addrStr))
			} else {
				b.emitLoadAddr("si", i.Val)
				b.buf.WriteString(fmt.Sprintf("\tMOV  DI, %s\n", addrStr))
				b.emitMemCopy(sz)
			}
			return
		}
		if sz == 1 {
			b.loadVal(i.Val, "al")
			b.loadVal(i.Ptr, "bx")
			b.buf.WriteString("\tMOV  [BX], AL\n")
		} else if sz == 2 {
			b.loadVal(i.Val, "ax")
			b.loadVal(i.Ptr, "bx")
			b.buf.WriteString("\tMOV  [BX], AX\n")
		} else {
			b.emitLoadAddr("si", i.Val)
			b.loadVal(i.Ptr, "di")
			b.emitMemCopy(sz)
		}

	case *ir.ExtractField:
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		if fieldSize == 1 {
			b.emitLoadAddr("bx", i.Struct)
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  AL, [BX]\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AL, [BX+%d]\n", byteOffset))
			}
			b.storeResult(id)
		} else if fieldSize == 2 {
			b.emitLoadAddr("bx", i.Struct)
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  AX, [BX]\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AX, [BX+%d]\n", byteOffset))
			}
			b.storeResult(id)
		} else {
			b.emitLoadAddr("si", i.Struct)
			b.emitAddConst("si", byteOffset)
			b.emitLoadAddr("di", i)
			b.emitMemCopy(fieldSize)
		}

	case *ir.InsertField:
		structSize := b.getTypeSize(i.Struct.Type())
		b.emitLoadAddr("si", i.Struct)
		b.emitLoadAddr("di", i)
		b.emitMemCopy(structSize)
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type(), i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal(i.Val, "al")
			b.emitLoadAddr("bx", i)
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  [BX], AL\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  [BX+%d], AL\n", byteOffset))
			}
		} else if fieldSize == 2 {
			b.loadVal(i.Val, "ax")
			b.emitLoadAddr("bx", i)
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  [BX], AX\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  [BX+%d], AX\n", byteOffset))
			}
		} else {
			b.emitLoadAddr("si", i.Val)
			b.emitLoadAddr("di", i)
			b.emitAddConst("di", byteOffset)
			b.emitMemCopy(fieldSize)
		}

	case *ir.ExtractFieldPtr:
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal(i.Ptr, "bx")
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  AL, [BX]\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AL, [BX+%d]\n", byteOffset))
			}
			b.storeResult(id)
		} else if fieldSize == 2 {
			b.loadVal(i.Ptr, "bx")
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  AX, [BX]\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  AX, [BX+%d]\n", byteOffset))
			}
			b.storeResult(id)
		} else {
			b.loadVal(i.Ptr, "si")
			b.emitAddConst("si", byteOffset)
			b.emitLoadAddr("di", i)
			b.emitMemCopy(fieldSize)
		}

	case *ir.InsertFieldPtr:
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structType, i.FieldIndex)
		if fieldSize == 1 {
			b.loadVal(i.Val, "al")
			b.loadVal(i.Ptr, "bx")
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  [BX], AL\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  [BX+%d], AL\n", byteOffset))
			}
		} else if fieldSize == 2 {
			b.loadVal(i.Val, "ax")
			b.loadVal(i.Ptr, "bx")
			if byteOffset == 0 {
				b.buf.WriteString("\tMOV  [BX], AX\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tMOV  [BX+%d], AX\n", byteOffset))
			}
		} else {
			b.emitLoadAddr("si", i.Val)
			b.loadVal(i.Ptr, "di")
			b.emitAddConst("di", byteOffset)
			b.emitMemCopy(fieldSize)
		}

	case *ir.ExtractElement:
		eltSize := b.getTypeSize(i.Typ)
		b.emitLoadAddr("si", i.Array)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConst("si", byteOffset)
		} else {
			b.loadVal(i.Index, "ax")
			if eltSize == 2 {
				b.buf.WriteString("\tSHL  AX, 1\n")
			} else if eltSize > 2 {
				b.buf.WriteString(fmt.Sprintf("\tMOV  DX, %d\n\tMUL  DX\n", eltSize))
			}
			b.buf.WriteString("\tADD  SI, AX\n")
		}
		if eltSize == 1 {
			b.buf.WriteString("\tMOV  AL, [SI]\n")
			b.storeResult(id)
		} else if eltSize == 2 {
			b.buf.WriteString("\tMOV  AX, [SI]\n")
			b.storeResult(id)
		} else {
			b.emitLoadAddr("di", i)
			b.emitMemCopy(eltSize)
		}

	case *ir.InsertElement:
		arraySize := b.getTypeSize(i.Array.Type())
		b.emitLoadAddr("si", i.Array)
		b.emitLoadAddr("di", i)
		b.emitMemCopy(arraySize)
		eltSize := b.getEltSize(i.Array.Type())
		b.emitLoadAddr("di", i)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			b.emitAddConst("di", byteOffset)
		} else {
			b.loadVal(i.Index, "ax")
			if eltSize == 2 {
				b.buf.WriteString("\tSHL  AX, 1\n")
			} else if eltSize > 2 {
				b.buf.WriteString(fmt.Sprintf("\tMOV  DX, %d\n\tMUL  DX\n", eltSize))
			}
			b.buf.WriteString("\tADD  DI, AX\n")
		}
		if eltSize == 1 {
			b.loadVal(i.Val, "al")
			b.buf.WriteString("\tMOV  [DI], AL\n")
		} else if eltSize == 2 {
			b.loadVal(i.Val, "ax")
			b.buf.WriteString("\tMOV  [DI], AX\n")
		} else {
			b.emitLoadAddr("si", i.Val)
			b.emitMemCopy(eltSize)
		}

	case *ir.Call:
		totalArgBytes := b.emitPushCallArgs(i.Args)
		retSize := b.getTypeSize(i.Typ)
		if retSize > 2 {
			b.emitLoadAddr("cx", i)
		}
		b.buf.WriteString(fmt.Sprintf("\tCALL %s\n", i.Func.EmitName()))
		b.emitCallStackCleanup(totalArgBytes)
		hasRet := !i.Typ.Equals(ir.TypeVoid) && retSize <= 2
		if hasRet {
			b.storeResult(id)
		}

	case *ir.IndirectCall:
		totalArgBytes := b.emitPushCallArgs(i.Args)
		retSize := b.getTypeSize(i.Typ)
		if retSize > 2 {
			b.emitLoadAddr("cx", i)
		}
		b.loadVal(i.FuncPtr, "bx")
		b.buf.WriteString("\tCALL BX\n")
		b.emitCallStackCleanup(totalArgBytes)
		hasRet := !i.Typ.Equals(ir.TypeVoid) && retSize <= 2
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
		// Handled at block transitions

	default:
		log.Panicf("emitInstr: unhandled instruction %T", instr)
	}
}

func (b *Backend) emitPushCallArgs(args []ir.Value) int {
	totalArgBytes := 0
	for idx := len(args) - 1; idx >= 0; idx-- {
		arg := args[idx]
		sz := b.getTypeSize(arg.Type())
		pushSize := alignVal(sz, 2)
		totalArgBytes += pushSize
		if sz > 2 {
			b.buf.WriteString(fmt.Sprintf("\tSUB  SP, %d\n", pushSize))
			b.emitLoadAddr("si", arg)
			b.buf.WriteString("\tMOV  DI, SP\n")
			b.emitMemCopy(sz)
		} else {
			b.loadVal(arg, "ax")
			b.buf.WriteString("\tPUSH AX\n")
		}
	}
	return totalArgBytes
}

func (b *Backend) emitCallStackCleanup(totalArgBytes int) {
	if totalArgBytes <= 0 {
		return
	}
	if totalArgBytes == 2 {
		b.buf.WriteString("\tPOP  CX\n")
	} else if totalArgBytes == 4 {
		b.buf.WriteString("\tPOP  CX\n\tPOP  CX\n")
	} else {
		b.buf.WriteString(fmt.Sprintf("\tADD  SP, %d\n", totalArgBytes))
	}
}

func (b *Backend) emitSetJmp(i *ir.SetJmp) {
	id := i.GetID()
	jmpSlot := b.jmpSlots[id]
	lblResume := b.nextLabel()
	lblDone := b.nextLabel()

	b.buf.WriteString(fmt.Sprintf("\tLEA  BX, %s\n", bpDisp(-jmpSlot)))
	b.buf.WriteString("\tMOV  AX, [v_prelude._jmp_chain_]\n")
	b.buf.WriteString("\tMOV  [BX], AX\n")
	b.buf.WriteString("\tMOV  [v_prelude._jmp_chain_], BX\n")
	b.buf.WriteString(fmt.Sprintf("\tMOV  WORD PTR [BX+2], %s\n", lblResume))
	b.buf.WriteString("\tMOV  [BX+4], SP\n")
	b.buf.WriteString("\tMOV  [BX+6], BP\n")
	b.buf.WriteString("\tXOR  AX, AX\n")
	b.buf.WriteString(fmt.Sprintf("\tJMP  %s\n", lblDone))
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblResume))
	b.buf.WriteString("\tMOV  AX, 1\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	b.storeResult(id)
}

func (b *Backend) emitLongJmp(i *ir.LongJmp) {
	b.loadVal(i.JmpBuf, "ax")
	b.buf.WriteString("\tJMP  __longjmp_ax\n")
}

func (b *Backend) emitBuiltinCall(i *ir.BuiltinCall) {
	switch i.Name {
	case "print", "println":
		b.emitPrint(i.Name == "println", i.Args)
	case "exit":
		if len(i.Args) > 0 {
			b.loadVal(i.Args[0], "ax")
		} else {
			b.buf.WriteString("\tXOR  AX, AX\n")
		}
		b.buf.WriteString("\tPUSH AX\n\tCALL _exit\n")
	case "panic", "_panic_":
		if len(i.Args) > 0 {
			if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
				b.lblCount++
				lbl := fmt.Sprintf(".Lpanic%d", b.lblCount)
				b.rodata = append(b.rodata, rodataEntry{lbl: lbl, str: strLit.Value})
				b.buf.WriteString(fmt.Sprintf("\tMOV  AX, %s\n\tPUSH AX\n", lbl))
			} else {
				b.loadVal(i.Args[0], "ax")
				b.buf.WriteString("\tPUSH AX\n")
			}
		} else {
			b.buf.WriteString("\tXOR  AX, AX\n\tPUSH AX\n")
		}
		b.buf.WriteString("\tCALL _panic\n\tPOP  CX\n")
	case "_unlink_jmp_":
		lblDone := b.nextLabel()
		b.buf.WriteString("\tMOV  BX, [v_prelude._jmp_chain_]\n")
		b.buf.WriteString("\tTEST BX, BX\n")
		b.buf.WriteString(fmt.Sprintf("\tJZ   %s\n", lblDone))
		b.buf.WriteString("\tMOV  AX, [BX]\n")
		b.buf.WriteString("\tMOV  [v_prelude._jmp_chain_], AX\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
	case "_propagate_panic_":
		lblDone := b.nextLabel()
		b.buf.WriteString("\tMOV  AX, [v_prelude._panic_]\n")
		b.buf.WriteString("\tTEST AX, AX\n")
		b.buf.WriteString(fmt.Sprintf("\tJZ   %s\n", lblDone))
		b.buf.WriteString("\tMOV  AX, [v_prelude._jmp_chain_]\n")
		b.buf.WriteString("\tTEST AX, AX\n")
		b.buf.WriteString("\tJNZ  __longjmp_ax\n")
		b.buf.WriteString("\tJMP  .L_panic_abort\n")
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
			b.buf.WriteString(fmt.Sprintf("\tMOV  AX, %s\n\tPUSH AX\n", strLbl))
		} else if arg.Type().Name == "string" || strings.HasSuffix(arg.Type().Name, "slice_byte") {
			// First field of string / slice[byte] is Base (*byte) at offset 0
			b.emitLoadAddr("bx", arg)
			b.buf.WriteString("\tPUSH WORD PTR [BX]\n")
		} else {
			b.loadVal(arg, "ax")
			b.buf.WriteString("\tPUSH AX\n")
		}
	}

	b.buf.WriteString(fmt.Sprintf("\tMOV  AX, %s\n\tPUSH AX\n", fmtLabel))
	b.buf.WriteString("\tCALL _printf\n")
	b.emitCallStackCleanup((len(dataArgs) + 1) * 2)
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.currentFunc = f
	b.allocateSlots(f)

	b.buf.WriteString("\n")
	b.buf.WriteString("; ============================================================================\n")
	b.buf.WriteString(fmt.Sprintf("; func %s (frame: %d bytes)\n", f.Name, b.stackSize))
	b.buf.WriteString("; ============================================================================\n")
	b.buf.WriteString(fmt.Sprintf("%s:\n", f.EmitName()))

	// Prologue
	b.buf.WriteString("\tPUSH BP\n\tMOV  BP, SP\n")
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tSUB  SP, %d\n", b.stackSize))
	}
	retSize := b.getTypeSize(f.ReturnType)
	if retSize > 2 {
		b.buf.WriteString(fmt.Sprintf("\tMOV  %s, CX\n", bpDisp(-b.retPtrSlot)))
	}

	for blkIdx, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", f.Name, blk.ID))

		fusedCompare, redundantOuterCmp, redundantZero := b.findBlockFusedCompare(f, blk)

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
					b.buf.WriteString(fmt.Sprintf("\tJMP  .L_%s_b%d\n", f.Name, term.Target.ID))
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

					b.loadVal(term.Condition, "al")
					b.buf.WriteString("\tTEST AL, AL\n")
					b.buf.WriteString(fmt.Sprintf("\tJNZ  %s\n", trueTarget))
					b.buf.WriteString(fmt.Sprintf("\tJMP  %s\n", falseTarget))

					if hasTruePhi {
						b.buf.WriteString(fmt.Sprintf("%s:\n", trueLbl))
						b.emitPhiAssignments(blk, term.TrueBlock)
						b.buf.WriteString(fmt.Sprintf("\tJMP  .L_%s_b%d\n", f.Name, term.TrueBlock.ID))
					}

					if hasFalsePhi {
						b.buf.WriteString(fmt.Sprintf("%s:\n", falseLbl))
						b.emitPhiAssignments(blk, term.FalseBlock)
						b.buf.WriteString(fmt.Sprintf("\tJMP  .L_%s_b%d\n", f.Name, term.FalseBlock.ID))
					}
				}

			case *ir.Return:
				if term.Val != nil {
					sz := b.getTypeSize(term.Val.Type())
					if sz == 1 {
						b.loadVal(term.Val, "al")
						b.buf.WriteString("\tMOV  AH, 0\n")
					} else if sz == 2 {
						b.loadVal(term.Val, "ax")
					} else if sz > 2 {
						b.emitLoadAddr("si", term.Val)
						b.buf.WriteString(fmt.Sprintf("\tMOV  DI, %s\n", bpDisp(-b.retPtrSlot)))
						b.emitMemCopy(sz)
					}
				}
				if b.stackSize > 0 {
					b.buf.WriteString("\tMOV  SP, BP\n")
				}
				b.buf.WriteString("\tPOP  BP\n\tRET\n")

			default:
				log.Panicf("emitFunc: unhandled terminator %T", blk.Terminator)
			}
		}
	}
}

func (b *Backend) emitHelpers() {
	b.buf.WriteString(`
; ── Runtime & Hardware Math Helpers (Intel 8086, SEGMENT CODE) ──────────────

; __udiv16: AX = AX / BX (unsigned)
__udiv16:
	TEST BX, BX
	JZ   __div_zero
	XOR  DX, DX
	DIV  BX
	RET

; __umod16: AX = AX % BX (unsigned)
__umod16:
	TEST BX, BX
	JZ   __div_zero
	XOR  DX, DX
	DIV  BX
	MOV  AX, DX
	RET

; __div16: AX = AX / BX (signed)
__div16:
	TEST BX, BX
	JZ   __div_zero
	CWD
	IDIV BX
	RET

; __mod16: AX = AX % BX (signed, remainder sign follows dividend)
__mod16:
	TEST BX, BX
	JZ   __div_zero
	CWD
	IDIV BX
	MOV  AX, DX
	RET

__div_zero:
	MOV  AX, .L_div_zero_str
	PUSH AX
	CALL _panic

f_prelude__mul_byte:
	MOV  BX, SP
	MOV  AL, [BX+2]
	MOV  DL, [BX+4]
	MUL  DL
	RET

__longjmp_ax:
	MOV  BX, AX
	MOV  SP, [BX+4]
	MOV  BP, [BX+6]
	JMP  WORD PTR [BX+2]

_panic:
	MOV  BX, SP
	MOV  AX, [BX+2]
	MOV  [v_prelude._panic_], AX
	PUSH AX
	MOV  AX, .L_panic_msg_fmt
	PUSH AX
	CALL _printf
	POP  CX
	POP  CX
	MOV  AX, [v_prelude._jmp_chain_]
	TEST AX, AX
	JNZ  __longjmp_ax
.L_panic_abort:
	MOV  AX, .L_empty_chain_msg
	PUSH AX
	CALL _printf
	POP  CX
	MOV  AX, 1
	PUSH AX
	CALL _exit
`)
}

func (b *Backend) emitGlobals(prog *ir.Program) {
	b.buf.WriteString("\n; ── Global & Read-Only Data Section (SEGMENT DATA, DS = $0000) ──────────────\n")
	b.buf.WriteString("\tSEGMENT DATA\n")
	b.buf.WriteString(fmt.Sprintf("\tORG  $%04X\n", b.GlobalVarOffset))

	hasJmpChain := false
	hasPanicVar := false
	for _, g := range prog.Globals {
		if g.Name == "prelude._jmp_chain_" {
			hasJmpChain = true
		}
		if g.Name == "prelude._panic_" {
			hasPanicVar = true
		}
	}
	if !hasJmpChain {
		b.buf.WriteString("v_prelude._jmp_chain_:\n\tDW   0\n")
	}
	if !hasPanicVar {
		b.buf.WriteString("v_prelude._panic_:\n\tDW   0\n")
	}

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
				b.buf.WriteString(fmt.Sprintf("\tDB   %s\n", strings.Join(bytesList, ", ")))
				sz -= len(g.InitString)
				if sz > 0 {
					b.buf.WriteString(fmt.Sprintf("\tDS   %d\n", sz))
				}
			}
		} else {
			if sz > 0 {
				b.buf.WriteString(fmt.Sprintf("\tDS   %d\n", sz))
			}
		}
	}

	if len(b.rodata) > 0 {
		b.buf.WriteString("\n; ── Read-Only Data (Strings & Format Literals in SEGMENT DATA) ──────────────\n")
		for _, entry := range b.rodata {
			b.buf.WriteString(fmt.Sprintf("%s:\n", entry.lbl))
			var bytesList []string
			for _, ch := range []byte(entry.str) {
				bytesList = append(bytesList, fmt.Sprintf("$%02X", ch))
			}
			bytesList = append(bytesList, "$00")
			b.buf.WriteString(fmt.Sprintf("\tDB   %s\n", strings.Join(bytesList, ", ")))
		}
	}

	b.buf.WriteString(`
.L_div_zero_str:
	DB   "division by zero", 0
.L_panic_msg_fmt:
	DB   10, "*PANIC* %s", 10, 0
.L_empty_chain_msg:
	DB   10, "*** ABORT", 10, 10, "*** EMPTY_RE_CHAIN", 10, 0
`)
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tDB   %d\n", v.Val))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tDW   %d\n", v.Val&0xFFFF))
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tDW   v_%s\n", v.Global.Name))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tDW   %s\n", v.Func.EmitName()))
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
				b.buf.WriteString(fmt.Sprintf("\tDS   %d\n", sz))
			}
			byteOffset += sz
		}
		structSize := b.getTypeSize(v.Type())
		if structSize > byteOffset {
			b.buf.WriteString(fmt.Sprintf("\tDS   %d\n", structSize-byteOffset))
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
			b.buf.WriteString(fmt.Sprintf("\tDS   %d\n", arrSize-totalBytes))
		}
	}
}

func (b *Backend) Generate(prog *ir.Program) string {
	b.program = prog
	b.buf.Reset()
	b.rodata = nil

	b.buf.WriteString("; ── Intel 8086 Whole-Program Assembly (MiniGolf, Small Split I&D) ──────────\n")
	b.buf.WriteString("\tSEGMENT CODE\n")

	hasFunc := func(name string) bool {
		for _, f := range prog.Functions {
			if f.EmitName() == name || f.Name == name {
				return true
			}
		}
		return false
	}

	if hasFunc("f_main__main") && !hasFunc("_main") && !hasFunc("main") {
		b.buf.WriteString("_main:\n")
		b.buf.WriteString("\tCALL f_main__main\n")
		b.buf.WriteString("\tXOR  AX, AX\n")
		b.buf.WriteString("\tRET\n")
	}

	for _, f := range prog.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
		}
	}

	b.emitHelpers()
	b.emitGlobals(prog)

	return optimizeAsm(b.buf.String())
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
		termInInstructions := false
		for _, inst := range blk.Instructions {
			if inst == blk.Terminator {
				termInInstructions = true
			}
			for _, op := range getOperands(inst) {
				if op == val {
					count++
				}
			}
		}
		if blk.Terminator != nil && !termInInstructions {
			for _, op := range getOperands(blk.Terminator) {
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
				if edge.Block == from && edge.Value != nil {
					return true
				}
			}
		}
	}
	return false
}

func normLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func invertJcc(mnem string) string {
	switch mnem {
	case "JE", "JZ":
		return "JNE"
	case "JNE", "JNZ":
		return "JE"
	case "JB", "JC":
		return "JAE"
	case "JAE", "JNC":
		return "JB"
	case "JBE":
		return "JA"
	case "JA":
		return "JBE"
	case "JL":
		return "JGE"
	case "JGE":
		return "JL"
	case "JLE":
		return "JG"
	case "JG":
		return "JLE"
	}
	return ""
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

			if trimmed == "" || strings.HasPrefix(trimmed, ";") {
				out = append(out, line)
				continue
			}

			norm := normLine(trimmed)

			nextIdx := -1
			for j := i + 1; j < n; j++ {
				t := strings.TrimSpace(lines[j])
				if t != "" && !strings.HasPrefix(t, ";") {
					nextIdx = j
					break
				}
			}

			// 1. Redundant unconditional JMP to immediate next label:
			//    JMP target
			//    target:
			if strings.HasPrefix(norm, "JMP ") && !strings.Contains(norm, "[") {
				target := strings.TrimSpace(strings.TrimPrefix(norm, "JMP "))
				if nextIdx >= 0 && normLine(lines[nextIdx]) == target+":" {
					changed = true
					continue
				}
			}

			// 2. Redundant 16-bit store then reload:
			//    MOV [BP-d], AX
			//    MOV AX, [BP-d]
			if strings.HasPrefix(norm, "MOV [BP") && strings.HasSuffix(norm, "], AX") {
				slot := norm[4 : len(norm)-4]
				if nextIdx >= 0 && normLine(lines[nextIdx]) == "MOV AX, "+slot {
					lines[nextIdx] = ";" + lines[nextIdx]
					changed = true
				}
			}

			// 3. Redundant 8-bit store then reload:
			//    MOV [BP-d], AL
			//    MOV AL, [BP-d]
			if strings.HasPrefix(norm, "MOV [BP") && strings.HasSuffix(norm, "], AL") {
				slot := norm[4 : len(norm)-4]
				if nextIdx >= 0 && normLine(lines[nextIdx]) == "MOV AL, "+slot {
					lines[nextIdx] = ";" + lines[nextIdx]
					changed = true
				}
			}

			// 4. Branch inversion over unconditional JMP:
			//    Jcc L1
			//    JMP L2
			//    L1:
			fields := strings.Fields(norm)
			if len(fields) == 2 && nextIdx >= 0 {
				if inv := invertJcc(fields[0]); inv != "" {
					target1 := fields[1]
					next1Norm := normLine(lines[nextIdx])
					if strings.HasPrefix(next1Norm, "JMP ") && !strings.Contains(next1Norm, "[") {
						target2 := strings.TrimSpace(strings.TrimPrefix(next1Norm, "JMP "))
						next2Idx := -1
						for j := nextIdx + 1; j < n; j++ {
							t := strings.TrimSpace(lines[j])
							if t != "" && !strings.HasPrefix(t, ";") {
								next2Idx = j
								break
							}
						}
						if next2Idx >= 0 && normLine(lines[next2Idx]) == target1+":" {
							out = append(out, fmt.Sprintf("\t%-4s %s", inv, target2))
							lines[nextIdx] = ";" + lines[nextIdx]
							changed = true
							continue
						}
					}
				}
			}

			// 5. Redundant PUSH R / POP R
			if strings.HasPrefix(norm, "PUSH ") && nextIdx >= 0 {
				reg := strings.TrimSpace(strings.TrimPrefix(norm, "PUSH "))
				if normLine(lines[nextIdx]) == "POP "+reg {
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
