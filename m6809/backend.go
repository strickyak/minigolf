package m6809

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

const needs_clra = true

var GLOBAL_VAR_OFFSET = flag.Int("global_var_offset", 16, "must be positive, so address 0 is not used, that is nil")

func align(sz int) int {
	if sz == 0 {
		return 1
	}
	return sz
}

func (b *Backend) getTypeSizeUsingIrt(irt *ir.Type) int {
	z := b.getTypeSizeUsingIrt9(irt)
	// log.Printf("NANDO99 getTypeSizeUsingIrt %#v -> %#v", irt, z)
	return z
}
func (b *Backend) getTypeSizeUsingIrt9(irt *ir.Type) int {
	if irt.IsAPointer() {
		return 2
	}
	if irt.IsAnArray() {
		et := irt.ArrayElementType()
		length := 0
		idx := strings.Index(irt.Name, "]")
		if idx != -1 {
			length, _ = strconv.Atoi(irt.Name[1:idx])
		}
		return length * b.getTypeSizeByType(et)
	}
	if irt.IsAStruct() {
		fields := irt.FieldsOfStruct()
		if len(fields) > 0 {
			size := 0
			for _, f := range fields {
				size += b.getTypeSizeByType(f.Type)
			}
			return size
		}

		// Fallback for missing fields (should not happen for full types, but maybe for tuple_...)
		content := irt.Name[7 : len(irt.Name)-1]
		if strings.HasPrefix(irt.Name, "tuple_") {
			content = irt.Name[6 : len(irt.Name)-1]
		}
		size := 0
		depth := 0
		start := 0
		for i := 0; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
			} else if content[i] == ';' && depth == 0 {
				// This will panic TIZENEGY if TIZENEGY is on!
				size += b.getTypeSize(content[start:i], nil)
				start = i + 1
			}
		}
		return size
	}

	switch irt.Name {
	case "void", "byte", "bool":
		return 1
	case "word", "int", "const_integer", "uint", "noreturn":
		return 2
	}
	if strings.HasPrefix(irt.Name, "func") {
		return 2
	}
	if b.program != nil {
		if def, ok := b.program.TypeDefs[irt.Name]; ok {
			return b.getTypeSizeByType(def)
		}
	}
	log.Panicf("M6809 getTypeSizeUsingIrt: unknown case: %#v", *irt)
	panic(0)
}

// func (b *Backend) getTypeSize(typ string) int //

func (b *Backend) getTypeSizeByType(irt ir.Type) int {
	return b.getTypeSize(irt.Name, &irt)
}

func (b *Backend) getTypeSize(typ string, irt *ir.Type) int {
	z := b.getTypeSize9(typ, irt)
	// log.Printf("NANDO9 getTypeSize %#v %#v -> %#v", typ, irt, z)
	return z
}
func (b *Backend) getTypeSize9(typ string, irt *ir.Type) int {

	if irt != nil {
		return b.getTypeSizeUsingIrt(irt)
	}
	log.Panicf("TIZENEGY getTypeSize9: %q, %#v", typ, irt)
	panic("NOT REACHED")
}

func (b *Backend) getEltSizeUsingIrt(irt ir.Type) int {
	switch {
	case irt.IsAPointer(): // THEIRS (correct?)
		pt := irt.PointedType()
		// Replicate the old behavior where we strip BOTH the pointer AND the array
		if pt.IsAnArray() {
			et := pt.ArrayElementType()
			return b.getTypeSizeUsingIrt(&et)
		}
		log.Panicf("getEltSizeUsingIrt: called on pointer that does not point to an array: irt=%#v", irt)
		// WAS: return b.getTypeSizeUsingIrt(&pt)

	case irt.IsAnArray():
		et := irt.ArrayElementType()
		// log.Printf("NANDO ARRAY %v ELEMENT %v", irt, et)
		fmt.Fprintf(os.Stderr, "F-NANDO ARRAY %v ELEMENT %v", irt, et)
		return b.getTypeSizeUsingIrt(&et)
	default:
		log.Panicf("M6809 getEltSizeUsingIrt: unknown case: %#v", irt)
	}
	panic("NOT REACHED")
}

func (b *Backend) getFieldOffsetAndSizeUsingIrt(irt ir.Type, fieldIndex int) (int, int) {
	if b.program != nil {
		if def, ok := b.program.TypeDefs[irt.Name]; ok {
			irt = def
		}
	}
	fields := irt.FieldsOfStruct()
	if len(fields) > 0 {
		offset := 0
		for i := 0; i < fieldIndex; i++ {
			offset += b.getTypeSizeByType(fields[i].Type)
		}
		size := b.getTypeSizeByType(fields[fieldIndex].Type)
		return offset, size
	}
	log.Panicf("getFieldOffsetAndSizeUsingIrt: no fields found for %#v", irt)
	panic(0)
}

func (b *Backend) getFieldOffsetAndSize(structName string, fieldIndex int) (int, int) {
	content := ""
	if def, ok := b.program.TypeDefs[structName]; ok {
		content = def.Name[7 : len(def.Name)-1]
	} else if strings.HasPrefix(structName, "struct{") || strings.HasPrefix(structName, "tuple_") {
		content = structName[7 : len(structName)-1]
	} else {
		log.Panicf("getFieldOffsetAndSize: not a struct: %q", structName)
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
			sz := b.getTypeSize(fTyp, nil)
			if fIdx < fieldIndex {
				byteOffset += sz
			} else if fIdx == fieldIndex {
				// log.Printf("NAN getFieldOffsetAndSize: struct=%q field=%d off=%d sz=%d", structName, fieldIndex, byteOffset, sz)
				return byteOffset, sz
			}
			fIdx++
			start = idx + 1
		}
	}
	log.Panicf("getFieldOffsetAndSize: field not found: %q . %d", structName, fieldIndex)
	panic(0)
}

type Backend struct {
	program         *ir.Program
	useFramePointer bool
	globalsAtY      bool
	picMode         bool
	frameOffset     int
	buf             bytes.Buffer
	dataBuf         bytes.Buffer
	rodataBuf       bytes.Buffer
	stackSize       int
	pushedBytes     int
	slots           map[int]int
	slotSizes       map[int]int
	paramSlots      map[string]int
	jmpSlots        map[int]int
	globalOffsets   map[string]int
	activeRegs      map[string]int
	valInReg        map[int]string
	slotOwner       map[int]int
	freeRegs        []string
	fmtCount        int
	lblCount        int
	retSlot         int
	f               *ir.Function
	levelBases      map[int]string
	levelYOffsets   map[int]int
	paramPseudoIDs  map[string]int
}

func New(useFramePointer bool, globalsAtY bool, picMode bool) *Backend {
	frameOff := 0
	if useFramePointer {
		frameOff = 2
	}
	return &Backend{
		useFramePointer: useFramePointer,
		globalsAtY:      globalsAtY,
		picMode:         picMode,
		frameOffset:     frameOff,
		slots:           make(map[int]int),
		slotSizes:       make(map[int]int),
		paramSlots:      make(map[string]int),
		jmpSlots:        make(map[int]int),
		globalOffsets:   make(map[string]int),
		slotOwner:       make(map[int]int),
		levelBases:      make(map[int]string),
		levelYOffsets:   make(map[int]int),
		paramPseudoIDs:  make(map[string]int),
	}
}

func (b *Backend) availableRegisters() []string {
	regs := []string{"X"}
	if !b.globalsAtY {
		regs = append(regs, "Y")
	}
	if !b.useFramePointer {
		regs = append(regs, "U")
	}
	return regs
}

func (b *Backend) flushRegisters() {
	if len(b.activeRegs) == 0 {
		return
	}
	b.buf.WriteString("\t\t\t; flushing registers {\n")
	var regs []string
	for r := range b.activeRegs {
		regs = append(regs, r)
	}
	sort.Strings(regs)
	for _, reg := range regs {
		id := b.activeRegs[reg]
		switch reg {
		case "X":
			b.buf.WriteString("\ttfr x,d\n")
		case "Y":
			b.buf.WriteString("\ttfr y,d\n")
		case "U":
			b.buf.WriteString("\ttfr u,d\n")
		case "B":
			// already in B
		case "D":
			// already in D
		default:
			panic(reg)
		}
		offset, ok := b.getSlotOffset(id)
		if !ok {
			continue
		}
		if owner, hasOwner := b.slotOwner[offset]; hasOwner && owner != id {
			b.buf.WriteString(fmt.Sprintf("\t\t\t; skipped flush for id=%v reg=%v because slot is owned by id=%v\n", id, reg, owner))
			continue
		}
		sz := b.slotSizes[id]
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tstb %s\t; reg=%v id=%v\n", b.memAccess(offset), reg, id))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tstd %s\t; reg=%v id=%v\n", b.memAccess(offset), reg, id))
		}
	}
	b.activeRegs = map[string]int{}
	b.valInReg = map[int]string{}
	b.freeRegs = b.availableRegisters()
	b.buf.WriteString("\t\t\t; registers flushed }\n")
}

func (b *Backend) allocateReg(id int) string {
	if len(b.freeRegs) > 0 {
		reg := b.freeRegs[0]
		b.freeRegs = b.freeRegs[1:]
		b.activeRegs[reg] = id
		b.valInReg[id] = reg
		return reg
	}

	var regToSpill string
	var spilledId int
	var regs []string
	for r := range b.activeRegs {
		regs = append(regs, r)
	}
	sort.Strings(regs)
	regToSpill = regs[0]
	spilledId = b.activeRegs[regToSpill]

	b.buf.WriteString(fmt.Sprintf("\t; spilling %s (val %d) to stack\n", regToSpill, spilledId))
	b.buf.WriteString("\tpshs d\n")
	b.pushBytes(2)
	if regToSpill == "X" {
		b.buf.WriteString("\ttfr x,d\n")
	}
	if regToSpill == "Y" {
		b.buf.WriteString("\ttfr y,d\n")
	}
	if regToSpill == "U" {
		b.buf.WriteString("\ttfr u,d\n")
	}
	offset, ok := b.getSlotOffset(spilledId)
	if !ok {
		panic("Cannot spill register holding unallocated ID")
	}
	if owner, hasOwner := b.slotOwner[offset]; hasOwner && owner != spilledId {
		b.buf.WriteString(fmt.Sprintf("\t\t\t; skipped spill for id=%v reg=%v because slot is owned by id=%v\n", spilledId, regToSpill, owner))
	} else if b.slotSizes[spilledId] == 1 {
		b.buf.WriteString(fmt.Sprintf("\tstb %s\n", b.memAccess(offset)))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(offset)))
	}
	b.buf.WriteString("\tpuls d\n")
	b.popBytes(2)

	delete(b.valInReg, spilledId)
	b.activeRegs[regToSpill] = id
	b.valInReg[id] = regToSpill
	return regToSpill
}

func (b *Backend) storeResult(id int) {
	reg := b.allocateReg(id)
	switch reg {
	case "X":
		b.buf.WriteString("\ttfr d,x\n")
	case "Y":
		b.buf.WriteString("\ttfr d,y\n")
	case "U":
		b.buf.WriteString("\ttfr d,u\n")
	default:
		log.Panicf("bad case in storeResult: %v", reg)
	}
}

func (b *Backend) nextLabel() string {
	b.lblCount++
	return fmt.Sprintf(".LL%d", b.lblCount)
}

func (b *Backend) memAccess(offsetFromEntry int) string {
	if b.useFramePointer {
		return fmt.Sprintf("%d,u", offsetFromEntry+2)
	}
	sOffset := b.frameOffset + b.stackSize + b.pushedBytes + offsetFromEntry
	return fmt.Sprintf("%d,s", sOffset)
}

func offsetAddrStr(valStr string, offset int) string {
	if idx := strings.Index(valStr, ","); idx != -1 {
		return fmt.Sprintf("%d+%s", offset, valStr)
	}
	return fmt.Sprintf("%s+%d", valStr, offset)
}

func (b *Backend) emitLoadAddr(reg string, addrStr string) {
	if strings.HasPrefix(addrStr, "v_") && !strings.Contains(addrStr, ",") {
		b.buf.WriteString(fmt.Sprintf("\tld%s #%s\n", reg, addrStr))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tlea%s %s\n", reg, addrStr))
	}
}

func (b *Backend) resolveVal(val ir.Value) ir.Value {
	for {
		if cast, ok := val.(*ir.Cast); ok && (cast.Op == "word_to_ptr" || cast.Op == "ptr_to_word" || cast.Op == "bitcast") {
			val = cast.Operand
		} else {
			break
		}
	}
	return val
}

func (b *Backend) getAddrStr(val ir.Value) string {
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.Parameter:
		return b.memAccess(b.paramSlots[v.Name])
	case ir.Instruction:
		return b.memAccess(b.slots[v.GetID()])
	case *ir.Global:
		if b.globalsAtY {
			return fmt.Sprintf("%d,y", b.globalOffsets[v.Name])
		}
		if b.picMode {
			return fmt.Sprintf("v_%s,pcr", v.Name)
		}
		return fmt.Sprintf("v_%s", v.Name)
	default:
		log.Panicf("bad case: %T / %v", v, v)
	}
	return ""
}

func (b *Backend) pushBytes(n int) {
	b.pushedBytes += n
	fmt.Fprintf(&b.buf, "\t\t\t; pushBytes: %d -> %d\n", n, b.pushedBytes)
}
func (b *Backend) popBytes(n int) {
	b.pushedBytes -= n
	fmt.Fprintf(&b.buf, "\t\t\t; popBytes: %d -> %d\n", n, b.pushedBytes)
}

func (b *Backend) getSlotOffset(id int) (int, bool) {
	origId := id
	if b.f != nil && b.f.SlotAlias != nil {
		for {
			if alias, ok := b.f.SlotAlias[id]; ok {
				id = alias
			} else {
				break
			}
		}
	}
	if offset, ok := b.slots[id]; ok {
		if origId != id {
			b.slots[origId] = offset
			b.slotSizes[origId] = b.slotSizes[id]
		}
		return offset, true
	}
	return 0, false
}

func (b *Backend) getSlot(id int, irt ir.Type) int {
	typ := irt.Name
	if offset, ok := b.slots[id]; ok {
		fmt.Fprintf(&b.buf, "\t\t\t; getSlot(%d, %q): found: offset=%d\n", id, typ, offset)
		return offset
	}
	size := b.getTypeSizeByType(irt)
	aligned := align(size)
	b.stackSize += aligned
	offset := -(b.frameOffset + b.stackSize)
	b.slots[id] = offset
	b.slotSizes[id] = size
	fmt.Fprintf(&b.buf, "\t\t\t; getSlot(%d, %q): setting: offset=%d; frame=%d  size=%d newStackSize: %d\n", id, typ, offset, b.frameOffset, aligned, b.stackSize)
	return offset
}

func (b *Backend) Generate(program *ir.Program) string {
	b.program = program
	b.buf.WriteString("\tpragma cescapes\n")
	//no-section// b.buf.WriteString("\tpragma undefextern\n")
	//no-section// b.buf.WriteString("\tsection code\n")

	b.globalOffsets = make(map[string]int)
	if !b.globalsAtY && len(program.Globals) > 0 {
		//no-section// b.dataBuf.WriteString("\tsection data ; start program.Globals\n")
		addr := *GLOBAL_VAR_OFFSET
		for _, g := range program.Globals {

			if g.IsInit {
				//no-section// b.dataBuf.WriteString("\n\tsection code\n")
				//no-section// b.dataBuf.WriteString(fmt.Sprintf("\texport v_%s\n", g.Name))

				b.dataBuf.WriteString(fmt.Sprintf("*** global var init: name=%q type=%q init=%#v\n", g.Name, g.Typ.Name, g.InitString))
				b.dataBuf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
				if g.InitVal != nil {
					b.emitData(g.InitVal)
				} else {
					for i := 0; i < len(g.InitString); i++ {
						x := g.InitString[i]
						c := byte('~')
						if ' ' <= x && x < '~' {
							c = x
						}
						b.dataBuf.WriteString(fmt.Sprintf("\tfcb %d ; [%d] <%c>\n", g.InitString[i], i, c))
					}
				}
			} else {
				//no-section// b.dataBuf.WriteString("\n\tsection data\n")
				//no-section// b.dataBuf.WriteString(fmt.Sprintf("\texport v_%s\n", g.Name))

				size := b.getTypeSizeByType(g.Typ)
				// Use `equ` to avoid producing 0 bytes which do not belong in our ROM image.
				b.dataBuf.WriteString(fmt.Sprintf("v_%s\tequ\t%d\t; size=%d type=%q [no init]\n\n", g.Name, addr, size, g.Typ.Name))
				addr += size
				/*
				   for j := 0; j < size; j++ {
				       b.dataBuf.WriteString("\tfcb 0\n")
				   }
				*/
			}
		}
	} else if b.globalsAtY {
		offset := *GLOBAL_VAR_OFFSET
		for _, g := range program.Globals {
			b.globalOffsets[g.Name] = offset
			if g.IsInit {
				if g.InitVal != nil {
					offset += b.getTypeSizeByType(g.Typ)
				} else {
					offset += len(g.InitString)
				}
			} else {
				size := b.getTypeSizeByType(g.Typ)
				offset += size
			}
		}
	}
	//no-section// b.dataBuf.WriteString("\tsection code ; finished program.Globals\n")

	for _, f := range program.Functions {
		if len(f.Blocks) > 0 {
			b.emitFunc(f)
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

	//no-section// b.buf.WriteString("\n\texport _main\n")
	b.buf.WriteString("_main:\n")

	if usesPanic {
		b.buf.WriteString("\tleas -10,s\t; Allocate 10 bytes for jumper_main\n")
		b.buf.WriteString("\tldd #0\n")
		b.buf.WriteString("\tstd 0,s\t; jumper_main.prev = NULL\n")

		b.buf.WriteString("\tleax ,s\n")
		if b.picMode {
			b.buf.WriteString("\tstx v_prelude._jmp_chain_,pcr\n")
		} else {
			b.buf.WriteString("\tstx v_prelude._jmp_chain_\n")
		}

		lblNext := b.nextLabel()
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleay %s,pcr\n", lblNext))
			b.buf.WriteString("\tsty 2,x\n")
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd #%s\n", lblNext))
			b.buf.WriteString("\tstd 2,x\n")
		}

		b.buf.WriteString("\ttfr s,d\n")
		b.buf.WriteString("\tstd 4,x\n")
		b.buf.WriteString("\ttfr u,d\n")
		b.buf.WriteString("\tstd 6,x\n")
		b.buf.WriteString("\ttfr y,d\n")
		b.buf.WriteString("\tstd 8,x\n")
		b.buf.WriteString("\tclra\n")
		b.buf.WriteString("\tclrb\n")
		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext))
		b.buf.WriteString("\tcmpd #0\n")
		lblCallMain := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblCallMain))

		// Uncaught Panic
		b.fmtCount++
		lblUncaught := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** UNCAUGHT_PANIC\\n\"\n", lblUncaught))
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblUncaught))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblUncaught))
		}
		b.buf.WriteString("\tstx ,--s\n")
		if b.picMode {
			b.buf.WriteString("\tlbsr _printf\n")
		} else {
			b.buf.WriteString("\tjsr _printf\n")
		}
		b.buf.WriteString("\tleas 2,s\n")

		if b.picMode {
			b.buf.WriteString("\tldd v_prelude._panic_,pcr\n")
		} else {
			b.buf.WriteString("\tldd v_prelude._panic_\n")
		}
		b.buf.WriteString("\tcmpd #0\n")
		lblAbort := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblAbort))

		b.fmtCount++
		lblPanicMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"*** %%s\\n\"\n", lblPanicMsg))

		b.buf.WriteString("\tstd ,--s\n")
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblPanicMsg))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblPanicMsg))
		}
		b.buf.WriteString("\tstx ,--s\n")
		if b.picMode {
			b.buf.WriteString("\tlbsr _printf\n")
		} else {
			b.buf.WriteString("\tjsr _printf\n")
		}
		b.buf.WriteString("\tleas 4,s\n")

		b.buf.WriteString(fmt.Sprintf("%s:\n", lblAbort))
		b.buf.WriteString("\tldx #1\n")
		b.buf.WriteString("\tjmp __exit\n")

		b.buf.WriteString(fmt.Sprintf("%s:\n", lblCallMain))
	}

	if b.picMode {
		b.buf.WriteString("\tlbsr f_main__main\n")
	} else {
		b.buf.WriteString("\tjsr f_main__main\n")
	}

	if usesPanic {
		b.buf.WriteString("\tleas 10,s\n")
	}
	b.buf.WriteString("\tldd #0\n")
	b.buf.WriteString("\tldx #0\n")
	b.buf.WriteString("\trts\n")

	rawCode := b.buf.String() + "\n" + b.rodataBuf.String() + "\n" + b.dataBuf.String()
	return peepholeOptimize(rawCode)
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.stackSize = 0
	b.pushedBytes = 0
	b.slots = make(map[int]int)
	b.slotSizes = make(map[int]int)
	b.paramSlots = make(map[string]int)

	var firstWord *ir.Parameter
	var firstByte *ir.Parameter

	fmt.Fprintf(&b.buf, "\t\t; =========== EMIT FUNC %q\n", f.Name)

	for _, p := range f.Parameters {
		sz := b.getTypeSizeByType(p.Typ)
		if sz == 2 && firstWord == nil {
			firstWord = p
			fmt.Fprintf(&b.buf, "\t\t; Note: param %q type %q is first size=2\n", p.Name, p.Type())
		} else if sz == 1 && firstByte == nil {
			firstByte = p
			fmt.Fprintf(&b.buf, "\t\t; Note: param %q type %q is first size=2\n", p.Name, p.Type())
		}
	}

	b.paramPseudoIDs = make(map[string]int)
	for i, p := range f.Parameters {
		size := b.getTypeSizeByType(p.Typ)
		aligned := align(size)
		b.stackSize += aligned
		b.paramSlots[p.Name] = -(b.frameOffset + b.stackSize)
		pseudoID := -(i + 1)
		b.paramPseudoIDs[p.Name] = pseudoID
		b.slots[pseudoID] = b.paramSlots[p.Name]
		b.slotSizes[pseudoID] = size
		fmt.Fprintf(&b.buf, "\t\t; Note: with param %q, type %q, size %d, b.stackSize becomes %d, slot becomes %v\n", p.Name, p.Type(), aligned, b.stackSize, b.paramSlots[p.Name])
	}
	// Pre-scan: collect cast IDs that are targeted by addrof_local (they need a real stack slot).
	castNeedsSlot := make(map[int]bool)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if addrLocal, ok := instr.(*ir.AddressOfLocal); ok {
				if localInstr, isInstr := addrLocal.Local.(ir.Instruction); isInstr {
					if cast, isCast := localInstr.(*ir.Cast); isCast && (cast.Op == "word_to_ptr" || cast.Op == "ptr_to_word" || cast.Op == "bitcast") {
						castNeedsSlot[cast.GetID()] = true
					}
				}
			}
		}
	}

	b.jmpSlots = make(map[int]int)
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if setJmp, ok := instr.(*ir.SetJmp); ok {
				b.stackSize += 10
				b.jmpSlots[setJmp.GetID()] = -(b.frameOffset + b.stackSize)
			}
		}
	}

	// Pre-scan for all other instructions (in program order).
	// Casts get slots only if addrof_local targets them (castNeedsSlot).
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if cast, ok := instr.(*ir.Cast); ok && (cast.Op == "word_to_ptr" || cast.Op == "ptr_to_word" || cast.Op == "bitcast") {
				if castNeedsSlot[cast.GetID()] {
					b.getSlot(cast.GetID(), cast.Type()) // allocate in program order
				}
				continue
			}
			if !instr.Type().Equals(ir.TypeVoid) && !instr.Type().Equals(ir.TypeUnknown) {
				b.getSlot(instr.GetID(), instr.Type())
			}
		}
	}

	//no-section// b.buf.WriteString(fmt.Sprintf("\n\texport f_%s\n", f.Name))
	b.buf.WriteString(fmt.Sprintf("%s:\n", f.EmitName()))
	if b.useFramePointer {
		b.buf.WriteString("\tpshs u\n")
		b.buf.WriteString("\ttfr s,u\n")
	}
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", b.stackSize))
	}

	stackArgOffset := 2
	retSize := b.getTypeSizeByType(f.ReturnType)
	b.retSlot = -1
	b.buf.WriteString("\t; --- Function parameters ---\n")
	if retSize > 2 {
		aligned := align(retSize)
		b.retSlot = stackArgOffset
		b.buf.WriteString(fmt.Sprintf("\t; Return value: size=%d, stack_offset=%d\n", retSize, stackArgOffset))
		stackArgOffset += aligned
	}

	for _, p := range f.Parameters {
		if p == firstWord {
			b.buf.WriteString(fmt.Sprintf("\t; Param %s passed in X (tracked in register)\n", firstWord.Name))
			b.buf.WriteString(fmt.Sprintf("\tstx %s\n", b.memAccess(b.paramSlots[p.Name])))
		}
		if p == firstByte {
			b.buf.WriteString(fmt.Sprintf("\t; Param %s passed in B\n", firstByte.Name))
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", b.memAccess(b.paramSlots[firstByte.Name])))
		}
	}

	xClobbered := false
	for _, p := range f.Parameters {
		size := b.getTypeSizeByType(p.Typ)
		if p == firstWord || p == firstByte {
			continue
		}

		aligned := align(size)
		b.buf.WriteString(fmt.Sprintf("\t; Param %s: size=%d, stack_offset=%d\n", p.Name, size, stackArgOffset))
		if size <= 2 {
			if size == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", b.memAccess(stackArgOffset)))
				b.buf.WriteString(fmt.Sprintf("\tstb %s\n", b.memAccess(b.paramSlots[p.Name])))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(stackArgOffset)))
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.paramSlots[p.Name])))
			}
		} else {
			b.flushRegisters()
			b.emitLoadAddr("y", b.memAccess(stackArgOffset))
			b.emitLoadAddr("x", b.memAccess(b.paramSlots[p.Name]))
			b.emitCopyYX(size)
			xClobbered = true
		}
		stackArgOffset += aligned
	}

	for i, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", f.Name, blk.ID))

		b.activeRegs = map[string]int{}
		b.valInReg = map[int]string{}
		b.slotOwner = map[int]int{}
		b.freeRegs = b.availableRegisters()

		if i == 0 && firstWord != nil && !xClobbered {
			pseudoID := b.paramPseudoIDs[firstWord.Name]
			b.activeRegs["X"] = pseudoID
			b.valInReg[pseudoID] = "X"
			var newFree []string
			for _, r := range b.freeRegs {
				if r != "X" {
					newFree = append(newFree, r)
				}
			}
			b.freeRegs = newFree
		}

		for _, instr := range blk.Instructions {
			if phi, isPhi := instr.(*ir.Phi); isPhi {
				if offset, ok := b.getSlotOffset(phi.GetID()); ok {
					b.slotOwner[offset] = phi.GetID()
				}
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			b.buf.WriteString("\t;;; " + ir.PrintInstruction(instr) + "\n")
			b.emitInstr(instr)
			if offset, ok := b.getSlotOffset(instr.GetID()); ok {
				b.slotOwner[offset] = instr.GetID()
			}
		}

		b.flushRegisters()

		if blk.Terminator != nil {
			b.buf.WriteString("\t; " + blk.Terminator.String() + " ;;; Block_Terminator\n")
		}

		switch term := blk.Terminator.(type) {
		case *ir.Jump:
			b.emitPhiAssignments(blk, term.Target)
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", f.Name, term.Target.ID))
		case *ir.Branch:
			b.loadVal(term.Condition)

			condType := term.Condition.Type()
			if b.getTypeSizeByType(condType) == 1 {
				b.buf.WriteString("\tcmpb #0 ;;(ir.Branch)\n")
			} else {
				b.buf.WriteString("\tcmpd #0 ;;(ir.Branch)\n")
				panic("bool should be 1 byte")
			}

			b.buf.WriteString(fmt.Sprintf("\tbne .L_%s_b%d_true\n", f.Name, blk.ID))
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d_false\n", f.Name, blk.ID))

			b.buf.WriteString(fmt.Sprintf(".L_%s_b%d_true:\n", f.Name, blk.ID))
			b.emitPhiAssignments(blk, term.TrueBlock)
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", f.Name, term.TrueBlock.ID))

			b.buf.WriteString(fmt.Sprintf(".L_%s_b%d_false:\n", f.Name, blk.ID))
			b.emitPhiAssignments(blk, term.FalseBlock)
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", f.Name, term.FalseBlock.ID))

		case *ir.Return:
			if term.Val != nil {
				retSize := b.getTypeSizeByType(term.Val.Type())
				if retSize <= 2 {
					b.loadVal(term.Val)
					if retSize == 2 {
						b.buf.WriteString("\ttfr d,x\n")
					}
				} else {
					if b.retSlot > 0 {
						b.flushRegisters()
						b.emitLoadAddr("y", b.getAddrStr(term.Val))
						b.emitLoadAddr("x", b.memAccess(b.retSlot))
						b.emitCopyYX(retSize)
					}
				}
			}
			if b.useFramePointer {
				b.buf.WriteString("\tleas 0,u\n")
				b.buf.WriteString("\tpuls u,pc\n")
			} else {
				if b.stackSize > 0 {
					b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", b.stackSize))
				}
				b.buf.WriteString("\trts\n")
			}
		default:
			log.Panicf("bad case: %T / %v", term, term)
		}
	}
}

func (b *Backend) loadVal(val ir.Value) {
	// fmt.Printf("DEBUG loadVal: %v (type %T)\n", val, val)
	val = b.resolveVal(val)
	switch v := val.(type) {
	case *ir.Parameter:
		pseudoID := b.paramPseudoIDs[v.Name]
		if reg, ok := b.valInReg[pseudoID]; ok {
			if reg == "X" {
				b.buf.WriteString("\ttfr x,d\n")
			} else if reg == "Y" {
				b.buf.WriteString("\ttfr y,d\n")
			} else if reg == "U" {
				b.buf.WriteString("\ttfr u,d\n")
			} else if reg == "B" {
				//dont_clra// b.buf.WriteString("\tclra\n")
			} else if reg == "D" {
				// already in D
			}
		} else {
			if b.getTypeSizeByType(v.Typ) == 1 {
				if needs_clra {
					b.buf.WriteString(fmt.Sprintf("\tldb %s\n\tclra\n", b.memAccess(b.paramSlots[v.Name])))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tldb %s\n", b.memAccess(b.paramSlots[v.Name]))) //dont_clra//
				}
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(b.paramSlots[v.Name])))
			}
		}
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", v.Val&0xFFFF))
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tldb #%d\n\tclra\n", v.Val&0xFF))
	case ir.Instruction:
		if reg, ok := b.valInReg[v.GetID()]; ok {
			// fmt.Printf("DEBUG: reg is %q\n", reg)
			if reg == "X" {
				b.buf.WriteString("\ttfr x,d\n")
			} else if reg == "Y" {
				b.buf.WriteString("\ttfr y,d\n")
			} else if reg == "U" {
				b.buf.WriteString("\ttfr u,d\n")
			} else if reg == "B" {
				//dont_clra// b.buf.WriteString("\tclra\n")
			} else if reg == "D" {
				// already in D
			}
		} else {
			if b.slotSizes[v.GetID()] == 1 {
				if needs_clra {
					b.buf.WriteString(fmt.Sprintf("\tldb %s\n\tclra\n", b.memAccess(b.slots[v.GetID()])))
				} else {
					b.buf.WriteString(fmt.Sprintf("\tldb %s\n", b.memAccess(b.slots[v.GetID()])))
				}
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(b.slots[v.GetID()])))
			}
		}
	default:
		log.Panicf("bad case: %T / %v", v, v)
	}
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					size := b.getTypeSizeByType(phi.Typ)
					if size <= 2 {
						b.loadVal(edge.Value)
						if size == 1 {
							b.buf.WriteString("\tclra\n")
							b.buf.WriteString(fmt.Sprintf("\tstb %s\n", b.memAccess(b.slots[phi.GetID()])))
						} else {
							b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.slots[phi.GetID()])))
						}
					} else {
						b.flushRegisters()
						destStr := b.memAccess(b.slots[phi.GetID()])
						srcStr := b.getAddrStr(edge.Value)
						b.emitLoadAddr("x", destStr)
						b.emitLoadAddr("y", srcStr)
						b.buf.WriteString("\tpshs u\n")
						b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
						lbl := b.nextLabel()
						b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
						b.buf.WriteString("\tlda ,y+\n")
						b.buf.WriteString("\tsta ,x+\n")
						b.buf.WriteString("\tleau -1,u\n")
						b.buf.WriteString("\tcmpu #0\n")
						b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
						b.buf.WriteString("\tpuls u\n")
					}
				}
			}
		}
	}
}

func (b *Backend) emitCopyYX(size int) {
	if size == 0 {
		return
	}
	b.buf.WriteString("\tpshs u\n")
	b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
	lbl := b.nextLabel()
	b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
	b.buf.WriteString("\tldb ,y+\n")
	b.buf.WriteString("\tstb ,x+\n")
	b.buf.WriteString("\tleau -1,u\n")
	b.buf.WriteString("\tcmpu #0\n")
	b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
	b.buf.WriteString("\tpuls u\n")
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	if cast, ok := instr.(*ir.Cast); ok && (cast.Op == "word_to_ptr" || cast.Op == "ptr_to_word" || cast.Op == "bitcast") {
		// Only emit this cast if addrof_local gave it a slot; otherwise it is a transparent no-op.
		if _, hasSlot := b.slots[cast.GetID()]; !hasSlot {
			return
		}
	}
	id := instr.GetID()
	offset := b.slots[id]

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))
	case *ir.ConstByte, *ir.ConstWord:
		b.loadVal(i)
		b.storeResult(id)
	case *ir.Sizeof:
		size := b.getTypeSizeByType(i.TargetTyp)
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", size))
		b.storeResult(id)
	case *ir.Load:
		b.flushRegisters()
		size := b.getTypeSizeByType(i.Global.Typ)
		destStr := b.memAccess(offset)
		srcStr := ""
		if b.globalsAtY {
			srcStr = fmt.Sprintf("%d,y", b.globalOffsets[i.Global.Name])
		} else if b.picMode {
			srcStr = fmt.Sprintf("v_%s,pcr", i.Global.Name)
		} else {
			srcStr = fmt.Sprintf("v_%s", i.Global.Name)
		}

		b.emitLoadAddr("y", srcStr)
		switch size {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			//dont_clra// b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		case 2:
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		default:
			b.emitLoadAddr("x", destStr)
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tlda ,y+\n")
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}
	case *ir.Store:
		b.flushRegisters()
		size := b.getTypeSizeByType(i.Global.Typ)
		destStr := ""
		if b.globalsAtY {
			destStr = fmt.Sprintf("%d,y", b.globalOffsets[i.Global.Name])
		} else if b.picMode {
			destStr = fmt.Sprintf("v_%s,pcr", i.Global.Name)
		} else {
			destStr = fmt.Sprintf("v_%s", i.Global.Name)
		}

		b.emitLoadAddr("x", destStr)

		if cVal, ok := i.Val.(*ir.ConstWord); ok {
			if size == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cVal.Val&0xFF))
				b.buf.WriteString("\tstb ,x\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", cVal.Val&0xFFFF))
				b.buf.WriteString("\tstd ,x\n")
			}
		} else if cByte, ok := i.Val.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cByte.Val&0xFF))
			b.buf.WriteString("\tstb ,x\n")
		} else {
			valStr := b.getAddrStr(i.Val)
			switch size {
			case 1:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldb ,y\n")
				b.buf.WriteString("\tstb ,x\n")
			case 2:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldd ,y\n")
				b.buf.WriteString("\tstd ,x\n")
			default:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tpshs u\n")
				b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
				lbl := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
				b.buf.WriteString("\tlda ,y+\n")
				b.buf.WriteString("\tsta ,x+\n")
				b.buf.WriteString("\tleau -1,u\n")
				b.buf.WriteString("\tcmpu #0\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
				b.buf.WriteString("\tpuls u\n")
			}
		}
	case *ir.ZeroInit:
		b.flushRegisters()
		size := b.getTypeSizeByType(i.Typ)
		destStr := b.memAccess(offset)
		fmt.Fprintf(&b.buf, "\t\t; ZeroInit size=%d dest=%v\n", size, destStr)

		if size == 0 {
			break
		}

		if size == 1 {
			//dont_clra// b.buf.WriteString("\tclra\n\tclrb\n")
			b.buf.WriteString("\tclrb\n") //dont_clra//
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		} else if size == 2 {
			b.buf.WriteString("\tclra\n\tclrb\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		} else {
			b.emitLoadAddr("x", destStr)
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", size))
			b.buf.WriteString("\tclra\n")
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}
	case *ir.ExtractElement:
		b.flushRegisters()
		eltSize := b.getEltSizeUsingIrt(i.Array.Type())
		arrayStr := b.getAddrStr(i.Array)
		destStr := b.memAccess(offset)
		fmt.Fprintf(&b.buf, "\t\t; ExtractElement size=%d array=%v dest=%v\n", eltSize, arrayStr, destStr)

		b.emitLoadAddr("y", arrayStr)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
			}
		} else {
			if eltSize == 1 {
				b.loadVal(i.Index)
				b.buf.WriteString("\tleay d,y\n")
			} else if eltSize == 2 {
				b.loadVal(i.Index)
				b.buf.WriteString("\tlslb\n")
				b.buf.WriteString("\trola\n")
				b.buf.WriteString("\tleay d,y\n")
			} else {
				b.loadVal(i.Index)
				b.buf.WriteString("\tcmpd #0\n")
				lblEnd := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblEnd))
				lblLoop := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lblLoop))
				b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", eltSize))
				b.buf.WriteString("\tsubd #1\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblLoop))
				b.buf.WriteString(fmt.Sprintf("%s:\n", lblEnd))
			}
		}

		switch eltSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			//dont_clra// b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		case 2:
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		default:
			b.emitLoadAddr("x", destStr)
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", eltSize))
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tlda ,y+\n")
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}
	case *ir.InsertElement:
		b.flushRegisters()
		arraySize := b.getTypeSizeByType(i.Array.Type())
		arrayStr := b.getAddrStr(i.Array)
		destStr := b.memAccess(offset)

		b.emitLoadAddr("y", arrayStr)
		b.emitLoadAddr("x", destStr)
		if arraySize > 0 {
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", arraySize))
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tlda ,y+\n")
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}

		eltSize := b.getEltSizeUsingIrt(i.Array.Type())
		b.emitLoadAddr("x", destStr)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
			}
		} else {
			if eltSize == 1 {
				b.loadVal(i.Index)
				b.buf.WriteString("\tleax d,x\n")
			} else if eltSize == 2 {
				b.loadVal(i.Index)
				b.buf.WriteString("\tlslb\n")
				b.buf.WriteString("\trola\n")
				b.buf.WriteString("\tleax d,x\n")
			} else {
				b.loadVal(i.Index)
				b.buf.WriteString("\tcmpd #0\n")
				lblEnd := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblEnd))
				lblLoop := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lblLoop))
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", eltSize))
				b.buf.WriteString("\tsubd #1\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblLoop))
				b.buf.WriteString(fmt.Sprintf("%s:\n", lblEnd))
			}
		}

		if cVal, ok := i.Val.(*ir.ConstWord); ok {
			if eltSize == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cVal.Val&0xFF))
				b.buf.WriteString("\tstb ,x\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", cVal.Val&0xFFFF))
				b.buf.WriteString("\tstd ,x\n")
			}
		} else if cByte, ok := i.Val.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cByte.Val&0xFF))
			b.buf.WriteString("\tstb ,x\n")
		} else {
			valStr := b.getAddrStr(i.Val)
			switch eltSize {
			case 1:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldb ,y\n")
				b.buf.WriteString("\tstb ,x\n")
			case 2:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldd ,y\n")
				b.buf.WriteString("\tstd ,x\n")
			default:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tpshs u\n")
				b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", eltSize))
				lbl2 := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lbl2))
				b.buf.WriteString("\tlda ,y+\n")
				b.buf.WriteString("\tsta ,x+\n")
				b.buf.WriteString("\tleau -1,u\n")
				b.buf.WriteString("\tcmpu #0\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl2))
				b.buf.WriteString("\tpuls u\n")
			}
		}
	case *ir.ExtractField:
		b.flushRegisters()
		byteOffset, fieldSize := b.getFieldOffsetAndSizeUsingIrt(i.Struct.Type(), i.FieldIndex)
		structStr := b.getAddrStr(i.Struct)
		destStr := b.memAccess(offset)

		b.emitLoadAddr("y", structStr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
		}

		switch fieldSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			//dont_clra// b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		case 2:
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		default:
			b.emitLoadAddr("x", destStr)
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", fieldSize))
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tlda ,y+\n")
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}
	case *ir.InsertField:
		b.flushRegisters()
		structSize := b.getTypeSizeByType(i.Struct.Type())
		structStr := b.getAddrStr(i.Struct)
		destStr := b.memAccess(offset)

		b.emitLoadAddr("y", structStr)
		b.emitLoadAddr("x", destStr)
		b.buf.WriteString("\tpshs u\n")
		b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", structSize))
		lbl := b.nextLabel()
		b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
		b.buf.WriteString("\tlda ,y+\n")
		b.buf.WriteString("\tsta ,x+\n")
		b.buf.WriteString("\tleau -1,u\n")
		b.buf.WriteString("\tcmpu #0\n")
		b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
		b.buf.WriteString("\tpuls u\n")

		byteOffset, fieldSize := b.getFieldOffsetAndSizeUsingIrt(i.Struct.Type(), i.FieldIndex)
		b.emitLoadAddr("x", destStr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
		}

		if cVal, ok := i.Val.(*ir.ConstWord); ok {
			if fieldSize == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cVal.Val&0xFF))
				b.buf.WriteString("\tstb ,x\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", cVal.Val&0xFFFF))
				b.buf.WriteString("\tstd ,x\n")
			}
		} else if cByte, ok := i.Val.(*ir.ConstByte); ok {
			b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cByte.Val&0xFF))
			b.buf.WriteString("\tstb ,x\n")
		} else {
			valStr := b.getAddrStr(i.Val)
			switch fieldSize {
			case 1:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldb ,y\n")
				b.buf.WriteString("\tstb ,x\n")
			case 2:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldd ,y\n")
				b.buf.WriteString("\tstd ,x\n")
			default:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tpshs u\n")
				b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", fieldSize))
				lbl2 := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lbl2))
				b.buf.WriteString("\tlda ,y+\n")
				b.buf.WriteString("\tsta ,x+\n")
				b.buf.WriteString("\tleau -1,u\n")
				b.buf.WriteString("\tcmpu #0\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl2))
				b.buf.WriteString("\tpuls u\n")
			}
		}
	case *ir.AddressOfGlobal:
		b.buf.WriteString(fmt.Sprintf("\tldd #v_%s\n", i.Global.Name))
		b.buf.WriteString(fmt.Sprintf("\tstd %s\t; ir.AddressOfGlobal(%s)\n", b.memAccess(offset), i.Global.Name))
	case *ir.AddressOfFunc:
		b.buf.WriteString(fmt.Sprintf("\tldd #%s\n", i.Func.EmitName()))
		b.storeResult(id)
	case *ir.AddressOfLocal:
		b.flushRegisters()
		var localOffset int
		var isParam bool
		if p, ok := i.Local.(*ir.Parameter); ok {
			localOffset = b.paramSlots[p.Name]
			isParam = true
		} else {
			localInstr := i.Local.(ir.Instruction)
			localOffset = b.slots[localInstr.GetID()]
		}
		b.emitLoadAddr("x", b.memAccess(localOffset))
		b.buf.WriteString("\ttfr x,d\n")
		if isParam {
			b.buf.WriteString(fmt.Sprintf("\tstd %s\t; ir.AddressOfLocal(param, locOff=%d)\n", b.memAccess(offset), localOffset))
		} else {
			localInstr := i.Local.(ir.Instruction)
			b.buf.WriteString(fmt.Sprintf("\tstd %s\t; ir.AddressOfLocal(%v, locOff=%d ;%v)\n", b.memAccess(offset), localOffset, localInstr.GetID(), localInstr.GetComment()))
		}
	case *ir.AddressOfField:
		structType := i.Ptr.Type().PointedType()
		byteOffset, _ := b.getFieldOffsetAndSizeUsingIrt(structType, i.FieldIndex)
		b.loadVal(i.Ptr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\taddd #%d\t; byteOffset\n", byteOffset))
		}
		b.buf.WriteString(fmt.Sprintf("\tstd %s\t; ir.AddressOfField(%v.%v)\n", b.memAccess(offset), structType.Name, i.FieldIndex))
	case *ir.AddressOfElement:
		b.flushRegisters()
		b.loadVal(i.ArrayPtr)

		eltSize := b.getEltSizeUsingIrt(i.ArrayPtr.Type())
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\taddd #%d\n", byteOffset))
			}
		} else {
			b.buf.WriteString("\ttfr d,y\n")
			b.loadVal(i.Index)
			if eltSize > 1 {
				b.buf.WriteString(fmt.Sprintf("\tldx #%d\n", eltSize))
				b.emitMul16()
			}
			b.buf.WriteString("\tleay d,y\n")
			b.buf.WriteString("\ttfr y,d\n")
		}
		b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(offset)))
	case *ir.ExtractFieldPtr:
		b.flushRegisters()
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSizeUsingIrt(structType, i.FieldIndex)

		destStr := b.memAccess(offset)
		b.loadVal(i.Ptr)
		b.buf.WriteString("\ttfr d,y\t; starting ir.ExtractFieldPtr\n")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
		}

		switch fieldSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			//dont_clra// b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		case 2:
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		default:
			b.emitLoadAddr("x", destStr)
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", fieldSize))
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tlda ,y+\n")
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}
	case *ir.InsertFieldPtr:
		b.flushRegisters()
		structType := i.Ptr.Type().PointedType()
		byteOffset, fieldSize := b.getFieldOffsetAndSizeUsingIrt(structType, i.FieldIndex)
		b.loadVal(i.Ptr)
		b.buf.WriteString("\ttfr d,x\t; starting ir.InsertFieldPtr\n")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
		}

		if cVal, ok := i.Val.(*ir.ConstWord); ok {
			if fieldSize == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cVal.Val&0xFF))
				b.buf.WriteString("\tstb ,x\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", cVal.Val))
				b.buf.WriteString("\tstd ,x\n")
			}
		} else {
			valStr := b.getAddrStr(i.Val)
			switch fieldSize {
			case 1:
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", valStr))
				b.buf.WriteString("\tstb ,x\n")
			case 2:
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", valStr))
				b.buf.WriteString("\tstd ,x\n")
			default:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tpshs u\n")
				b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", fieldSize))
				lbl2 := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lbl2))
				b.buf.WriteString("\tlda ,y+\n")
				b.buf.WriteString("\tsta ,x+\n")
				b.buf.WriteString("\tleau -1,u\n")
				b.buf.WriteString("\tcmpu #0\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl2))
				b.buf.WriteString("\tpuls u\n")
			}
		}
	case *ir.LoadPtr:
		b.flushRegisters()
		destStr := b.memAccess(offset)
		b.loadVal(i.Ptr)
		b.buf.WriteString("\ttfr d,y\t; starting ir.LoadPtr\n")
		fieldSize := b.getTypeSizeByType(i.Typ)
		switch fieldSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			//dont_clra// b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstb %s\n", destStr))
		case 2:
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		default:
			b.emitLoadAddr("x", destStr)
			b.buf.WriteString("\tpshs u\n")
			b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", fieldSize))
			lbl := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			b.buf.WriteString("\tlda ,y+\n")
			b.buf.WriteString("\tsta ,x+\n")
			b.buf.WriteString("\tleau -1,u\n")
			b.buf.WriteString("\tcmpu #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl))
			b.buf.WriteString("\tpuls u\n")
		}
	case *ir.StorePtr:
		b.flushRegisters()
		fieldSize := b.getTypeSizeByType(i.Ptr.Type().PointedType())
		b.loadVal(i.Ptr)
		b.buf.WriteString("\ttfr d,x\t; starting ir.StorePtr\n")

		if cVal, ok := i.Val.(*ir.ConstWord); ok {
			if fieldSize == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb #%d\n", cVal.Val&0xFF))
				b.buf.WriteString("\tstb ,x\t\t; store byte via pointer\n")
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", cVal.Val))
				b.buf.WriteString("\tstd ,x\t\t; store word via pointer\n")
			}
		} else {
			valStr := b.getAddrStr(i.Val)
			switch fieldSize {
			case 1:
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n", valStr))
				b.buf.WriteString("\tstb ,x\t\t; store byte via pointer\n")
			case 2:
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", valStr))
				b.buf.WriteString("\tstd ,x\t\t; store word via pointer\n")
			default:
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tpshs u\n")
				b.buf.WriteString(fmt.Sprintf("\tldu #%d\n", fieldSize))
				lbl2 := b.nextLabel()
				b.buf.WriteString(fmt.Sprintf("%s:\n", lbl2))
				b.buf.WriteString("\tlda ,y+\n")
				b.buf.WriteString("\tsta ,x+\n")
				b.buf.WriteString("\tleau -1,u\n")
				b.buf.WriteString("\tcmpu #0\n")
				b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lbl2))
				b.buf.WriteString("\tpuls u\n")
			}
		}

	case *ir.BinaryOp:
		b.loadVal(i.Right)
		b.buf.WriteString(fmt.Sprintf("\tstd ,--s\t ; starting ir.BinaryOp(%v,%v,%v)\n", i.Left, i.Op, i.Right))
		b.pushBytes(2)
		b.loadVal(i.Left)
		switch i.Op {
		case "add":
			b.buf.WriteString("\taddd ,s++\n")
			b.popBytes(2)
		case "sub":
			b.buf.WriteString("\tsubd ,s++\n")
			b.popBytes(2)
		case "mul":
			// TODO: get a 16-bit MUL subroutine.
			// FOR NOW: assume args are positive, under 256.
			b.buf.WriteString("\tlda 1,s\t; load low byte of Right into A\n")
			b.buf.WriteString("\tmul\t; unsigned multiply A * B, result in D\n")
			b.buf.WriteString("\tleas 2,s\n")
			b.popBytes(2)
		case "div", "mod":
			b.buf.WriteString(fmt.Sprintf("\t; unimplemented %s\n", i.Op))
			b.buf.WriteString("\tleas 2,s\n")
			b.popBytes(2)
		case "shl":
			lblLoop := b.nextLabel()
			lblDone := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\ttst 1,s\t; test shift amount\n"))
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblDone))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblLoop))
			b.buf.WriteString("\taslb\n")
			if b.getTypeSizeByType(i.Typ) != 1 {
				b.buf.WriteString("\trola\n")
			}
			b.buf.WriteString("\tdec 1,s\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblLoop))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
			b.buf.WriteString("\tleas 2,s\n")
			b.popBytes(2)
		case "shr":
			lblLoop := b.nextLabel()
			lblDone := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\ttst 1,s\t; test shift amount\n"))
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblDone))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblLoop))
			if b.getTypeSizeByType(i.Typ) != 1 {
				b.buf.WriteString("\tlsra\n")
				b.buf.WriteString("\trorb\n")
			} else {
				b.buf.WriteString("\tlsrb\n")
			}
			b.buf.WriteString("\tdec 1,s\n")
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblLoop))
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblDone))
			b.buf.WriteString("\tleas 2,s\n")
			b.popBytes(2)
		case "and":
			b.buf.WriteString("\tanda 0,s\n\tandb 1,s\n\tleas 2,s\n")
			b.popBytes(2)
		case "or":
			b.buf.WriteString("\tora 0,s\n\torb 1,s\n\tleas 2,s\n")
			b.popBytes(2)
		case "xor":
			b.buf.WriteString("\teora 0,s\n\teorb 1,s\n\tleas 2,s\n")
			b.popBytes(2)
		case "andnot":
			b.buf.WriteString("\tcom 0,s\n\tcom 1,s\n\tanda 0,s\n\tandb 1,s\n\tleas 2,s\n")
			b.popBytes(2)
		default:
			log.Panicf("Unknown BinaryOp in M6809: %q", i.Op)
		}
		if b.getTypeSizeByType(i.Typ) == 1 {
			//dont_clra// b.buf.WriteString("\tclra\n")
		}
		b.storeResult(id)

	case *ir.Compare:

		leftType := i.Left.Type()
		rightType := i.Right.Type()
		sizeOne := false
		if b.getTypeSizeUsingIrt(&leftType) == 1 {
			b.buf.WriteString(fmt.Sprintf("\t\t*CMP* Left %q is size 1\n", i.Left.String()))
			sizeOne = true
		}
		if b.getTypeSizeUsingIrt(&rightType) == 1 {
			b.buf.WriteString(fmt.Sprintf("\t\t*CMP* Right %q is size 1\n", i.Right.String()))
			Assert(sizeOne)
		} else {
			Assert(!sizeOne)
		}

		if sizeOne {

			b.loadVal(i.Right)
			b.buf.WriteString(fmt.Sprintf("\tstb ,-s\t; starting ir.Compare(%v,%v,%v) 1-byte\n", i.Left, i.Op, i.Right))
			b.pushBytes(1)
			b.loadVal(i.Left)
			b.buf.WriteString("\tcmpb ,s+\n")
			b.popBytes(1)

		} else {

			b.loadVal(i.Right)
			b.buf.WriteString(fmt.Sprintf("\tstd ,--s\t; starting ir.Compare(%v,%v,%v) 2-byte\n", i.Left, i.Op, i.Right))
			b.pushBytes(2)
			b.loadVal(i.Left)
			b.buf.WriteString("\tcmpd ,s++\n")
			b.popBytes(2)
		}

		lblTrue := b.nextLabel()
		lblEnd := b.nextLabel()

		isInt := i.Left.Type().Equals(ir.TypeInt)
		switch i.Op {
		case "eq":
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblTrue))
		case "neq":
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblTrue))
		case "lt":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tblt %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tblo %s\n", lblTrue))
			}
		case "lte":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tble %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tbls %s\n", lblTrue))
			}
		case "gt":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tbgt %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tbhi %s\n", lblTrue))
			}
		case "gte":
			if isInt {
				b.buf.WriteString(fmt.Sprintf("\tbge %s\n", lblTrue))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tbhs %s\n", lblTrue))
			}
		default:
			log.Panicf("Unknown Compare Op in M6809: %q", i.Op)
		}
		b.buf.WriteString("\tclrb\n\tbra " + lblEnd + "\n")
		b.buf.WriteString(lblTrue + ":\n\tldb #1\n")
		//dont_clra// b.buf.WriteString(lblEnd + ":\n\tclra\n")
		b.buf.WriteString(lblEnd + ":\n") //dont_clra//
		b.storeResult(id)

	case *ir.Call:
		b.flushRegisters()
		b.buf.WriteString(fmt.Sprintf("\t; --- Calling %q\n", i.Func.Name))
		var firstWordArg ir.Value
		var firstByteArg ir.Value
		var firstWordIdx = -1
		var firstByteIdx = -1

		for idx, arg := range i.Args {
			sz := b.getTypeSizeByType(i.Func.Parameters[idx].Typ)
			_ = sz
			if sz == 2 && firstWordArg == nil {
				firstWordArg = arg
				firstWordIdx = idx
			} else if sz == 1 && firstByteArg == nil {
				firstByteArg = arg
				firstByteIdx = idx
			}
		}

		var pushedBytes int
		b.buf.WriteString("\t; --- Setup call arguments ---\n")
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			if idx == firstWordIdx {
				b.buf.WriteString(fmt.Sprintf("\t\t; --- first size=2 arg: %q %q\n", i.Args[idx].String(), i.Args[idx].Type()))
				continue
			}
			if idx == firstByteIdx {
				b.buf.WriteString(fmt.Sprintf("\t\t; --- first size=1 arg: %q %q\n", i.Args[idx].String(), i.Args[idx].Type()))
				continue
			}
			argSize := b.getTypeSizeByType(i.Args[idx].Type())
			aligned := align(argSize)

			b.buf.WriteString(fmt.Sprintf("\t\t\t; Push arg %d: size=%d\n", idx, argSize))
			if argSize == 1 {
				b.loadVal(i.Args[idx])
				b.buf.WriteString("\tpshs b\n")
				b.pushBytes(aligned)
			} else if argSize == 2 {
				b.loadVal(i.Args[idx])
				b.buf.WriteString("\tstd ,--s\n")
				b.pushBytes(aligned)
			} else {
				b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", aligned))
				b.pushBytes(aligned)
				addr := b.getAddrStr(i.Args[idx])
				b.emitLoadAddr("y", addr)
				b.buf.WriteString("\tleax ,s\n")
				b.emitCopyYX(argSize)
			}
			pushedBytes += aligned
		}
		b.buf.WriteString(fmt.Sprintf("\t; --- Pushed args, pushedBytes=%d", pushedBytes))

		retSize := b.getTypeSizeByType(i.Func.ReturnType)
		if retSize > 2 {
			aligned := align(retSize)
			b.buf.WriteString(fmt.Sprintf("\t; Allocate space for return value: size=%d\n", retSize))
			b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", aligned))
			b.pushBytes(aligned)
			pushedBytes += aligned
		}
		b.buf.WriteString(fmt.Sprintf("\t; pushedBytes total %d bytes\n", pushedBytes))

		if firstWordArg != nil {
			b.buf.WriteString(fmt.Sprintf("\t; Load arg %d into X (first size=2 arg)\n", firstWordIdx))
			b.loadVal(firstWordArg)
			b.buf.WriteString("\ttfr d,x\n")
		}
		if firstByteArg != nil {
			b.buf.WriteString(fmt.Sprintf("\t; Load arg %d into B (first size=1 arg)\n", firstByteIdx))
			b.loadVal(firstByteArg)
		}

		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tlbsr %s\t\t; CALL (PIC)\n", i.Func.EmitName()))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tjsr %s\t\t; CALL\n", i.Func.EmitName()))
		}

		if retSize > 2 {
			b.buf.WriteString(fmt.Sprintf("\t\t\t; doing emitCopyXY(%d)\n", retSize))
			dest := b.getAddrStr(i)
			b.emitLoadAddr("x", dest)
			b.buf.WriteString(fmt.Sprintf("\tleay ,s\t; for emitCopyXY(%d)\n", retSize))
			b.emitCopyYX(retSize)
			b.buf.WriteString(fmt.Sprintf("\t\t\t; done emitCopyXY(%d)\n", retSize))
		}

		if pushedBytes > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleas %d,s  ; unpushing bytes\n", pushedBytes))
			b.popBytes(pushedBytes)
		}

		if retSize == 2 {
			b.buf.WriteString("\ttfr x,d\n")
		} else if retSize == 1 {
			//dont_clra// b.buf.WriteString("\tclra\n")
		}

		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 2 {
			b.storeResult(id)
		}

	case *ir.IndirectCall:
		b.flushRegisters()
		b.buf.WriteString("\t; --- Indirect Call\n")
		var firstWordArg ir.Value
		var firstByteArg ir.Value
		var firstWordIdx = -1
		var firstByteIdx = -1

		for idx, arg := range i.Args {
			sz := b.getTypeSizeByType(arg.Type())
			if sz == 2 && firstWordArg == nil {
				firstWordArg = arg
				firstWordIdx = idx
			} else if sz == 1 && firstByteArg == nil {
				firstByteArg = arg
				firstByteIdx = idx
			}
		}

		var pushedBytes int
		b.buf.WriteString("\t; --- Setup call arguments ---\n")
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			if idx == firstWordIdx || idx == firstByteIdx {
				continue
			}
			argSize := b.getTypeSizeByType(i.Args[idx].Type())
			aligned := align(argSize)
			if argSize == 1 {
				b.loadVal(i.Args[idx])
				b.buf.WriteString("\tpshs b\n")
				b.pushBytes(aligned)
			} else if argSize == 2 {
				b.loadVal(i.Args[idx])
				b.buf.WriteString("\tstd ,--s\n")
				b.pushBytes(aligned)
			} else {
				b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", aligned))
				b.pushBytes(aligned)
				addr := b.getAddrStr(i.Args[idx])
				b.emitLoadAddr("y", addr)
				b.buf.WriteString("\tleax ,s\n")
				b.emitCopyYX(argSize)
			}
			pushedBytes += aligned
		}

		retSize := 0
		if !i.Typ.Equals(ir.TypeVoid) {
			retSize = b.getTypeSizeByType(i.Typ)
		}
		if retSize > 2 {
			aligned := align(retSize)
			b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", aligned))
			b.pushBytes(aligned)
			pushedBytes += aligned
		}

		b.loadVal(i.FuncPtr)
		b.buf.WriteString("\ttfr d,y\n")

		if firstWordArg != nil {
			b.loadVal(firstWordArg)
			b.buf.WriteString("\ttfr d,x\n")
		}
		if firstByteArg != nil {
			b.loadVal(firstByteArg)
		}

		b.buf.WriteString("\tjsr ,y\t\t; INDIRECT CALL\n")

		if retSize > 2 {
			dest := b.getAddrStr(i)
			b.emitLoadAddr("x", dest)
			b.buf.WriteString(fmt.Sprintf("\tleay ,s\t; for emitCopyXY(%d)\n", retSize))
			b.emitCopyYX(retSize)
		}

		if pushedBytes > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleas %d,s  ; unpushing bytes\n", pushedBytes))
			b.popBytes(pushedBytes)
		}

		if retSize == 2 {
			b.buf.WriteString("\ttfr x,d\n")
		} else if retSize == 1 {
			//dont_clra// b.buf.WriteString("\tclra\n")
		}

		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 2 {
			b.storeResult(id)
		}

	case *ir.SetJmp:
		b.flushRegisters()
		jmpSlot := b.jmpSlots[id]
		// jumper.prev = _jmp_chain_
		b.emitLoadAddr("x", b.memAccess(jmpSlot))
		if b.picMode {
			b.buf.WriteString("\tldd v_prelude._jmp_chain_,pcr\n")
		} else {
			b.buf.WriteString("\tldd v_prelude._jmp_chain_\n")
		}
		b.buf.WriteString("\tstd 0,x\n")

		// _jmp_chain_ = &jumper
		if b.picMode {
			b.buf.WriteString("\tstx v_prelude._jmp_chain_,pcr\n")
		} else {
			b.buf.WriteString("\tstx v_prelude._jmp_chain_\n")
		}

		// save S, U, Y, PC
		lblNext := b.nextLabel()
		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tleay %s,pcr\n", lblNext))
			b.buf.WriteString("\tsty 2,x\n") // PC
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd #%s\n", lblNext))
			b.buf.WriteString("\tstd 2,x\n") // PC
		}

		b.buf.WriteString("\ttfr s,d\n")
		b.buf.WriteString("\tstd 4,x\n") // S

		b.buf.WriteString("\ttfr u,d\n")
		b.buf.WriteString("\tstd 6,x\n") // U

		b.buf.WriteString("\ttfr y,d\n")
		b.buf.WriteString("\tstd 8,x\n") // Y

		b.buf.WriteString("\tclra\n")
		b.buf.WriteString("\tclrb\n")

		b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext))
		b.storeResult(id)

	case *ir.LongJmp:
		b.flushRegisters()
		b.loadVal(i.JmpBuf)
		b.buf.WriteString("\ttfr d,x\n") // X = jumper

		// Return 1
		b.buf.WriteString("\tclra\n")
		b.buf.WriteString("\tldb #1\n")

		b.buf.WriteString("\tldy 8,x\n")
		b.buf.WriteString("\tldu 6,x\n")
		b.buf.WriteString("\tlds 4,x\n")
		b.buf.WriteString("\tjmp [2,x]\n")

	case *ir.BuiltinCall:
		b.flushRegisters()
		if i.Name == "print" || i.Name == "println" {
			b.emitPrint(i.Name == "println", i.Args)
		} else if i.Name == "panic" {
			if len(i.Args) > 0 {
				if strLit, ok := i.Args[0].(*ir.StringLiteral); ok {
					b.fmtCount++
					lbl := fmt.Sprintf(".Lfmt%d", b.fmtCount)
					b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", lbl, strLit.Value))
					if b.picMode {
						b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lbl))
						b.buf.WriteString("\tstx v_prelude._panic_,pcr\n")
					} else {
						b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lbl))
						b.buf.WriteString("\tstx v_prelude._panic_\n")
					}
				} else {
					b.loadVal(i.Args[0])
					if b.picMode {
						b.buf.WriteString("\tstd v_prelude._panic_,pcr\n")
					} else {
						b.buf.WriteString("\tstd v_prelude._panic_\n")
					}
				}
			} else {
				b.buf.WriteString("\tldd #0\n")
				if b.picMode {
					b.buf.WriteString("\tstd v_prelude._panic_,pcr\n")
				} else {
					b.buf.WriteString("\tstd v_prelude._panic_\n")
				}
			}

			b.fmtCount++
			lblPanicMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*PANIC* %%s\\n\"\n", lblPanicMsg))

			if b.picMode {
				b.buf.WriteString("\tldd v_prelude._panic_,pcr\n")
			} else {
				b.buf.WriteString("\tldd v_prelude._panic_\n")
			}
			b.buf.WriteString("\tstd ,--s\n")
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblPanicMsg))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblPanicMsg))
			}
			b.buf.WriteString("\tstx ,--s\n")
			if b.picMode {
				b.buf.WriteString("\tlbsr _printf\n")
			} else {
				b.buf.WriteString("\tjsr _printf\n")
			}
			b.buf.WriteString("\tleas 4,s\n")

			if b.picMode {
				b.buf.WriteString("\tldx v_prelude._jmp_chain_,pcr\n")
			} else {
				b.buf.WriteString("\tldx v_prelude._jmp_chain_\n")
			}
			b.buf.WriteString("\tcmpx #0\n")
			lblNext2 := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext2))
			b.buf.WriteString("\tclra\n")
			b.buf.WriteString("\tldb #1\n")
			b.buf.WriteString("\tldy 8,x\n")
			b.buf.WriteString("\tldu 6,x\n")
			b.buf.WriteString("\tlds 4,x\n")
			b.buf.WriteString("\tjmp [2,x]\n")
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext2))

			b.fmtCount++
			lblAbortMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"\n", lblAbortMsg))
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblAbortMsg))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblAbortMsg))
			}
			b.buf.WriteString("\tstx ,--s\n")
			if b.picMode {
				b.buf.WriteString("\tlbsr _printf\n")
			} else {
				b.buf.WriteString("\tjsr _printf\n")
			}
			b.buf.WriteString("\tleas 2,s\n")
			b.buf.WriteString("\tldx #1\n")
			b.buf.WriteString("\tjmp __exit\n")

		} else if i.Name == "_unlink_jmp_" {
			if b.picMode {
				b.buf.WriteString("\tldx v_prelude._jmp_chain_,pcr\n")
			} else {
				b.buf.WriteString("\tldx v_prelude._jmp_chain_\n")
			}
			b.buf.WriteString("\tcmpx #0\n")
			lblNext2 := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext2))
			b.buf.WriteString("\tldd 0,x\n") // prev
			if b.picMode {
				b.buf.WriteString("\tstd v_prelude._jmp_chain_,pcr\n")
			} else {
				b.buf.WriteString("\tstd v_prelude._jmp_chain_\n")
			}
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext2))

		} else if i.Name == "_propagate_panic_" {
			if b.picMode {
				b.buf.WriteString("\tldd v_prelude._panic_,pcr\n")
			} else {
				b.buf.WriteString("\tldd v_prelude._panic_\n")
			}
			b.buf.WriteString("\tcmpd #0\n")
			lblNext3 := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext3))

			if b.picMode {
				b.buf.WriteString("\tldx v_prelude._jmp_chain_,pcr\n")
			} else {
				b.buf.WriteString("\tldx v_prelude._jmp_chain_\n")
			}
			b.buf.WriteString("\tcmpx #0\n")
			lblNext2 := b.nextLabel()
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblNext2))

			b.buf.WriteString("\tclra\n")
			b.buf.WriteString("\tldb #1\n")
			b.buf.WriteString("\tldy 8,x\n")
			b.buf.WriteString("\tldu 6,x\n")
			b.buf.WriteString("\tlds 4,x\n")
			b.buf.WriteString("\tjmp [2,x]\n")
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext2))

			b.fmtCount++
			lblAbortMsg := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"\\n*** ABORT\\n\\n*** EMPTY_RE_CHAIN\\n\"\n", lblAbortMsg))
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lblAbortMsg))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lblAbortMsg))
			}
			b.buf.WriteString("\tstx ,--s\n")
			if b.picMode {
				b.buf.WriteString("\tlbsr _printf\n")
			} else {
				b.buf.WriteString("\tjsr _printf\n")
			}
			b.buf.WriteString("\tleas 2,s\n")
			b.buf.WriteString("\tldx #1\n")
			b.buf.WriteString("\tjmp __exit\n")
			b.buf.WriteString(fmt.Sprintf("%s:\n", lblNext3))

		} else if i.Name == "exit" {
			b.loadVal(i.Args[0])
			b.buf.WriteString("\tldx #1\n")
			b.buf.WriteString("\tjmp __exit\n")
		}

	case *ir.Cast:
		b.loadVal(i.Operand)
		if i.Op == "trunc" {
			//dont_clra// b.buf.WriteString("\tclra\n")
		}
		b.storeResult(id)
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
			formatStrs = append(formatStrs, "%d")
			dataArgs = append(dataArgs, arg)
		} else if strings.HasSuffix(arg.Type().Name, "slice_byte") {
			formatStrs = append(formatStrs, "%s")
			dataArgs = append(dataArgs, arg)
		} else if arg.Type().Name == "*byte" {
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

	if b.picMode {
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", fmtLabel, format))
	} else {
		if b.dataBuf.Len() == 0 {
			//no-section// b.dataBuf.WriteString("\tsection data\n")
		}
		b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", fmtLabel, format))
	}

	for i := len(dataArgs) - 1; i >= 0; i-- {
		if strLit, ok := dataArgs[i].(*ir.StringLiteral); ok {
			b.fmtCount++
			lbl := fmt.Sprintf(".Lfmt%d", b.fmtCount)
			if b.picMode {
				b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", lbl, strLit.Value))
			} else {
				b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz %q\n", lbl, strLit.Value))
			}
			if b.picMode {
				b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", lbl))
			} else {
				b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", lbl))
			}
			b.buf.WriteString("\tstx ,--s\n")
			b.pushBytes(2)
		} else {
			b.loadVal(dataArgs[i])
			b.buf.WriteString("\tstd ,--s\n")
			b.pushBytes(2)
		}
	}

	if b.picMode {
		b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", fmtLabel))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tldx #%s\n", fmtLabel))
	}

	b.buf.WriteString("\tstx ,--s\n")
	b.pushBytes(2)

	if b.picMode {
		b.buf.WriteString("\tlbsr _printf\n")
	} else {
		b.buf.WriteString("\tjsr _printf\n")
	}

	cleanup := 2 + len(dataArgs)*2
	b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", cleanup))
	b.popBytes(cleanup)
}

func (b *Backend) emitMul16() {
	fmt.Fprintln(&b.buf, "\t pshs D,X // BEGIN emitMul16(D,X)->D {")

	fmt.Fprintln(&b.buf, "\t lda 1,s")
	fmt.Fprintln(&b.buf, "\t ldb 3,s")
	fmt.Fprintln(&b.buf, "\t mul")
	fmt.Fprintln(&b.buf, "\t tfr d,x // first partial")

	fmt.Fprintln(&b.buf, "\t lda 0,s")
	fmt.Fprintln(&b.buf, "\t ldb 3,s")
	fmt.Fprintln(&b.buf, "\t mul")
	fmt.Fprintln(&b.buf, "\t tfr b,a")
	fmt.Fprintln(&b.buf, "\t clrb")
	fmt.Fprintln(&b.buf, "\t leax d,x // second partial")

	fmt.Fprintln(&b.buf, "\t lda 1,s")
	fmt.Fprintln(&b.buf, "\t ldb 2,s")
	fmt.Fprintln(&b.buf, "\t mul")
	fmt.Fprintln(&b.buf, "\t tfr b,a")
	fmt.Fprintln(&b.buf, "\t clrb")
	fmt.Fprintln(&b.buf, "\t leax d,x // third partial")

	fmt.Fprintln(&b.buf, "\t tfr x,d")
	fmt.Fprintln(&b.buf, "\t leas 4,s // END emitMul16(D,X)->D }")
}

func (b *Backend) emitData(val ir.Value) {
	switch v := val.(type) {
	case *ir.ConstByte:
		b.dataBuf.WriteString(fmt.Sprintf("\tfcb %d\n", v.Val))
	case *ir.ConstWord:
		b.dataBuf.WriteString(fmt.Sprintf("\tfdb %d\n", v.Val))
	case *ir.AddressOfGlobal:
		b.dataBuf.WriteString(fmt.Sprintf("\tfdb v_%s\n", v.Global.Name))
	case *ir.ConstStruct:
		for _, f := range v.Fields {
			b.emitData(f)
		}
	case *ir.ConstArray:
		for _, el := range v.Elements {
			b.emitData(el)
		}
	default:
		log.Panicf("unsupported init value type %T", val)
	}
}

func Assert(pred bool) {
	if !pred {
		panic("Assertion Failed")
	}
}
