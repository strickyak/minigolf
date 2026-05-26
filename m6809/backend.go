package m6809

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/strickyak/minigolf/ir"
)

var GLOBAL_VAR_OFFSET = flag.Int("global_var_offset", 16, "must be positive, so address 0 is not used, that is nil")

func align(sz int) int {
	if sz == 0 {
		return 1
	}
	return sz
}

func (b *Backend) getTypeSize(typ string) int {
	switch typ {
	case "void":
		return 1
	case "byte":
		return 1
	case "word":
		return 2
	case "int":
		return 2
	case "uint":
		return 2
	case "const_integer":
		return 2
	default:
		// fallthrough
	}

	if (ir.Type{Name: typ}).IsAPointer() {
		return 2
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
	if typ == "byte" {
		return 1
	}
	if typ == "word" || typ == "int" || typ == "bool" || strings.HasPrefix(typ, "*") || strings.HasPrefix(typ, "func") {
		return 2
	}
	if strings.HasPrefix(typ, "prelude.slice_") || strings.HasPrefix(typ, "slice_") {
		return 6
	}
	if (ir.Type{Name: typ}).IsAStruct() {
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
	log.Panicf("getTypeSize: unknown type: %q", typ)
	panic(0)
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
	return b.getTypeSize(arrType)
}

func (b *Backend) getFieldOffsetAndSize(structName string, fieldIndex int) (int, int) {
	content := ""
	if def, ok := b.program.TypeDefs[structName]; ok {
		content = def.Name[7 : len(def.Name)-1]
	} else if (ir.Type{Name: structName}).IsAStruct() {
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
	globalOffsets   map[string]int
	activeRegs      map[string]int
	valInReg        map[int]string
	freeRegs        []string
	fmtCount        int
	lblCount        int
	retSlot         int
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
		globalOffsets:   make(map[string]int),
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
		default:
			panic(reg)
		}
		sz := b.slotSizes[id]
		if sz == 1 {
			b.buf.WriteString(fmt.Sprintf("\tstb %s\t; reg=%v id=%v\n", b.memAccess(b.slots[id]), reg, id))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tstd %s\t; reg=%v id=%v\n", b.memAccess(b.slots[id]), reg, id))
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
	if b.slotSizes[spilledId] == 1 {
		b.buf.WriteString(fmt.Sprintf("\tstb %s\n", b.memAccess(b.slots[spilledId])))
	} else {
		b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.slots[spilledId])))
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

func (b *Backend) getAddrStr(val ir.Value) string {
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

func (b *Backend) getSlot(id int, typ string) int {
	if offset, ok := b.slots[id]; ok {
		fmt.Fprintf(&b.buf, "\t\t\t; getSlot(%d, %q): found: offset=%d\n", id, typ, offset)
		return offset
	}
	size := b.getTypeSize(typ)
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

				size := b.getTypeSize(g.Typ.Name)
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
					offset += b.getTypeSize(g.Typ.Name)
				} else {
					offset += len(g.InitString)
				}
			} else {
				size := b.getTypeSize(g.Typ.Name)
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

	//no-section// b.buf.WriteString("\n\texport _main\n")
	b.buf.WriteString("_main:\n")
	if b.picMode {
		b.buf.WriteString("\tlbsr f_main\n")
	} else {
		b.buf.WriteString("\tjsr f_main\n")
	}
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
		sz := b.getTypeSize(p.Typ.Name)
		if sz == 2 && firstWord == nil {
			firstWord = p
			fmt.Fprintf(&b.buf, "\t\t; Note: param %q type %q is first size=2\n", p.Name, p.Type())
		} else if sz == 1 && firstByte == nil {
			firstByte = p
			fmt.Fprintf(&b.buf, "\t\t; Note: param %q type %q is first size=2\n", p.Name, p.Type())
		}
	}

	for _, p := range f.Parameters {
		size := b.getTypeSize(p.Typ.Name)
		aligned := align(size)
		b.stackSize += aligned
		b.paramSlots[p.Name] = -(b.frameOffset + b.stackSize)
		fmt.Fprintf(&b.buf, "\t\t; Note: with param %q, type %q, size %d, b.stackSize becomes %d, slot becomes %v\n", p.Name, p.Type(), aligned, b.stackSize, b.paramSlots[p.Name])
	}
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if !instr.Type().Equals(ir.TypeVoid) && !instr.Type().Equals(ir.TypeUnknown) {
				b.getSlot(instr.GetID(), instr.Type().Name)
			}
		}
	}

	//no-section// b.buf.WriteString(fmt.Sprintf("\n\texport f_%s\n", f.Name))
	b.buf.WriteString(fmt.Sprintf("f_%s:\n", f.Name))
	if b.useFramePointer {
		b.buf.WriteString("\tpshs u\n")
		b.buf.WriteString("\ttfr s,u\n")
	}
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", b.stackSize))
	}

	stackArgOffset := 2
	retSize := b.getTypeSize(f.ReturnType.Name)
	b.retSlot = -1
	b.buf.WriteString("\t; --- Function parameters ---\n")
	if retSize > 2 {
		aligned := align(retSize)
		b.retSlot = stackArgOffset
		b.buf.WriteString(fmt.Sprintf("\t; Return value: size=%d, stack_offset=%d\n", retSize, stackArgOffset))
		stackArgOffset += aligned
	}

	if firstWord != nil {
		b.buf.WriteString(fmt.Sprintf("\t; Param %s passed in X\n", firstWord.Name))
		b.buf.WriteString(fmt.Sprintf("\tstx %s\n", b.memAccess(b.paramSlots[firstWord.Name])))
	}
	if firstByte != nil {
		b.buf.WriteString(fmt.Sprintf("\t; Param %s passed in B\n", firstByte.Name))
		b.buf.WriteString(fmt.Sprintf("\tstb %s\n", b.memAccess(b.paramSlots[firstByte.Name])))
	}

	for _, p := range f.Parameters {
		size := b.getTypeSize(p.Typ.Name)
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
			b.buf.WriteString(fmt.Sprintf("\tleay %s\n", b.memAccess(stackArgOffset)))
			b.emitLoadAddr("x", b.memAccess(b.paramSlots[p.Name]))
			b.emitCopyYX(size)
		}
		stackArgOffset += aligned
	}

	for _, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".L_%s_b%d:\n", f.Name, blk.ID))

		b.activeRegs = map[string]int{}
		b.valInReg = map[int]string{}
		b.freeRegs = b.availableRegisters()

		for _, instr := range blk.Instructions {
			if _, isPhi := instr.(*ir.Phi); isPhi {
				continue
			}
			if _, isTerm := instr.(ir.Terminator); isTerm {
				continue
			}
			b.emitInstr(instr)
		}

		b.flushRegisters()

		switch term := blk.Terminator.(type) {
		case *ir.Jump:
			b.emitPhiAssignments(blk, term.Target)
			b.buf.WriteString(fmt.Sprintf("\tlbra .L_%s_b%d\n", f.Name, term.Target.ID))
		case *ir.Branch:
			b.loadVal(term.Condition)
			b.buf.WriteString("\tcmpd #0\n")
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
				retSize := b.getTypeSize(term.Val.Type().Name)
				if retSize <= 2 {
					b.loadVal(term.Val)
					if retSize == 2 {
						b.buf.WriteString("\ttfr d,x\n")
					}
				} else {
					if b.retSlot > 0 {
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
	switch v := val.(type) {
	case *ir.Parameter:
		if b.getTypeSize(v.Typ.Name) == 1 {
			b.buf.WriteString(fmt.Sprintf("\tldb %s\n\tclra\n", b.memAccess(b.paramSlots[v.Name])))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(b.paramSlots[v.Name])))
		}
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", v.Val&0xFFFF))
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tldb #%d\n\tclra\n", v.Val&0xFF))
	case ir.Instruction:
		if reg, ok := b.valInReg[v.GetID()]; ok {
			if reg == "X" {
				b.buf.WriteString("\ttfr x,d\n")
			}
			if reg == "Y" {
				b.buf.WriteString("\ttfr y,d\n")
			}
			if reg == "U" {
				b.buf.WriteString("\ttfr u,d\n")
			}
		} else {
			if b.slotSizes[v.GetID()] == 1 {
				b.buf.WriteString(fmt.Sprintf("\tldb %s\n\tclra\n", b.memAccess(b.slots[v.GetID()])))
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
					size := b.getTypeSize(phi.Typ.Name)
					if size <= 2 {
						b.loadVal(edge.Value)
						if phi.Type().Equals(ir.TypeByte) {
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
	id := instr.GetID()
	offset := b.slots[id]

	switch i := instr.(type) {
	case *ir.SourceMarker:
		b.buf.WriteString(fmt.Sprintf("\t; %s\n", i.Comment))
	case *ir.ConstByte, *ir.ConstWord:
		b.loadVal(i)
		b.storeResult(id)
	case *ir.Sizeof:
		size := b.getTypeSize(i.TargetTyp.Name)
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", size))
		b.storeResult(id)
	case *ir.Load:
		b.flushRegisters()
		size := b.getTypeSize(i.Global.Typ.Name)
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
			b.buf.WriteString("\tclra\n")
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
		size := b.getTypeSize(i.Global.Typ.Name)
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
		size := b.getTypeSize(i.Typ.Name)
		destStr := b.memAccess(offset)
		fmt.Fprintf(&b.buf, "\t\t; ZeroInit size=%d dest=%v\n", size, destStr)

		if size == 0 {
			break
		}

		if size == 1 {
			b.buf.WriteString("\tclra\n\tclrb\n")
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
		eltSize := b.getEltSize(i.Array.Type().Name)
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
			b.buf.WriteString("\tclra\n")
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
		arraySize := b.getTypeSize(i.Array.Type().Name)
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

		eltSize := b.getEltSize(i.Array.Type().Name)
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
		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type().Name, i.FieldIndex)
		structStr := b.getAddrStr(i.Struct)
		destStr := b.memAccess(offset)

		b.emitLoadAddr("y", structStr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
		}

		switch fieldSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			b.buf.WriteString("\tclra\n")
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
		structSize := b.getTypeSize(i.Struct.Type().Name)
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

		byteOffset, fieldSize := b.getFieldOffsetAndSize(i.Struct.Type().Name, i.FieldIndex)
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
		b.buf.WriteString(fmt.Sprintf("\tldd #f_%s\n", i.Func.Name))
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
		structName := i.Ptr.Type().Name
		structName = strings.TrimPrefix(structName, "*")
		byteOffset, _ := b.getFieldOffsetAndSize(structName, i.FieldIndex)
		b.loadVal(i.Ptr)
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\taddd #%d\t; byteOffset\n", byteOffset))
		}
		b.buf.WriteString(fmt.Sprintf("\tstd %s\t; ir.AddressOfField(%v.%v)\n", b.memAccess(offset), structName, i.FieldIndex))
	case *ir.AddressOfElement:
		b.flushRegisters()
		b.loadVal(i.ArrayPtr)

		eltSize := b.getEltSize(i.ArrayPtr.Type().Name)
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
		structName := i.Ptr.Type().Name
		structName = strings.TrimPrefix(structName, "*")
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structName, i.FieldIndex)

		destStr := b.memAccess(offset)
		b.loadVal(i.Ptr)
		b.buf.WriteString("\ttfr d,y\t; starting ir.ExtractFieldPtr\n")
		if byteOffset > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
		}

		switch fieldSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			b.buf.WriteString("\tclra\n")
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
		structName := i.Ptr.Type().Name
		structName = strings.TrimPrefix(structName, "*")
		byteOffset, fieldSize := b.getFieldOffsetAndSize(structName, i.FieldIndex)
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
		fieldSize := b.getTypeSize(i.Typ.Name)
		switch fieldSize {
		case 1:
			b.buf.WriteString("\tldb ,y\n")
			b.buf.WriteString("\tclra\n")
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
		ptrType := i.Ptr.Type().Name
		pointeeType := "word"
		if (ir.Type{Name: ptrType}).IsAPointer() {
			pointeeType = ptrType[1:]
		}
		fieldSize := b.getTypeSize(pointeeType)
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
			b.buf.WriteString(fmt.Sprintf("\ttst 1,s\t; test shift amount\n"))
			b.buf.WriteString(fmt.Sprintf("\tbeq shl_done_%d\n", id))
			b.buf.WriteString(fmt.Sprintf("shl_loop_%d:\n", id))
			b.buf.WriteString("\taslb\n")
			if !i.Typ.Equals(ir.TypeByte) {
				b.buf.WriteString("\trola\n")
			}
			b.buf.WriteString("\tdec 1,s\n")
			b.buf.WriteString(fmt.Sprintf("\tbne shl_loop_%d\n", id))
			b.buf.WriteString(fmt.Sprintf("shl_done_%d:\n", id))
			b.buf.WriteString("\tleas 2,s\n")
			b.popBytes(2)
		case "shr":
			b.buf.WriteString(fmt.Sprintf("\ttst 1,s\t; test shift amount\n"))
			b.buf.WriteString(fmt.Sprintf("\tbeq shr_done_%d\n", id))
			b.buf.WriteString(fmt.Sprintf("shr_loop_%d:\n", id))
			if !i.Typ.Equals(ir.TypeByte) {
				b.buf.WriteString("\tlsra\n")
				b.buf.WriteString("\trorb\n")
			} else {
				b.buf.WriteString("\tlsrb\n")
			}
			b.buf.WriteString("\tdec 1,s\n")
			b.buf.WriteString(fmt.Sprintf("\tbne shr_loop_%d\n", id))
			b.buf.WriteString(fmt.Sprintf("shr_done_%d:\n", id))
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
		if i.Typ.Equals(ir.TypeByte) {
			b.buf.WriteString("\tclra\n")
		}
		b.storeResult(id)

	case *ir.Compare:
		b.loadVal(i.Right)
		b.buf.WriteString(fmt.Sprintf("\tstd ,--s\t; starting ir.Compare(%v,%v,%v\n", i.Left, i.Op, i.Right))
		b.pushBytes(2)
		b.loadVal(i.Left)
		b.buf.WriteString("\tcmpd ,s++\n")
		b.popBytes(2)

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
		b.buf.WriteString(lblEnd + ":\n\tclra\n")
		b.storeResult(id)

	case *ir.Call:
		b.flushRegisters()
		b.buf.WriteString(fmt.Sprintf("\t; --- Calling %q\n", i.Func.Name))
		var firstWordArg ir.Value
		var firstByteArg ir.Value
		var firstWordIdx = -1
		var firstByteIdx = -1

		for idx, arg := range i.Args {
			sz := b.getTypeSize(i.Func.Parameters[idx].Typ.Name)
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
			argSize := b.getTypeSize(i.Args[idx].Type().Name)
			aligned := align(argSize)

			b.buf.WriteString(fmt.Sprintf("\t\t\t; Push arg %d: size=%d\n", idx, argSize))
			if argSize == 1 {
				b.loadVal(i.Args[idx])
				b.buf.WriteString("\tstb ,-s\n")
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

		retSize := b.getTypeSize(i.Func.ReturnType.Name)
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
			b.buf.WriteString(fmt.Sprintf("\tlbsr f_%s\t\t; CALL (PIC)\n", i.Func.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tjsr f_%s\t\t; CALL\n", i.Func.Name))
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
			b.buf.WriteString("\tclra\n")
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
			sz := b.getTypeSize(arg.Type().Name)
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
			argSize := b.getTypeSize(i.Args[idx].Type().Name)
			aligned := align(argSize)
			if argSize == 1 {
				b.loadVal(i.Args[idx])
				b.buf.WriteString("\tstb ,--s\n")
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
			retSize = b.getTypeSize(i.Typ.Name)
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
			b.buf.WriteString("\tclra\n")
		}

		if !i.Typ.Equals(ir.TypeVoid) && retSize <= 2 {
			b.storeResult(id)
		}

	case *ir.BuiltinCall:
		b.flushRegisters()
		if i.Name == "print" || i.Name == "println" {
			b.emitPrint(i.Name == "println", i.Args)
		} else if i.Name == "exit" {
			b.buf.WriteString("\tfcb 1\n")
		}

	case *ir.Cast:
		b.loadVal(i.Operand)
		if i.Op == "trunc" {
			b.buf.WriteString("\tclra\n")
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
	default:
		log.Panicf("unsupported init value type %T", val)
	}
}
