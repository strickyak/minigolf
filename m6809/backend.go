package m6809

import (
	"bytes"
	"fmt"
	"minigo/ir"
	"strconv"
	"strings"
)

func getTypeSize(typ string) int {
	if typ == "byte" { return 1 }
	if typ == "word" { return 2 }
	if strings.HasPrefix(typ, "[") {
		idx := strings.Index(typ, "]")
		if idx != -1 {
			length, _ := strconv.Atoi(typ[1:idx])
			eltSize := getTypeSize(typ[idx+1:])
			return length * eltSize
		}
	}
	return 2
}

func getEltSize(arrType string) int {
	if strings.HasPrefix(arrType, "[") {
		idx := strings.Index(arrType, "]")
		if idx != -1 {
			return getTypeSize(arrType[idx+1:])
		}
	}
	return 2
}

type Backend struct {
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
	paramSlots      map[string]int
	globalOffsets   map[string]int
	activeRegs      map[string]int
	valInReg        map[int]string
	freeRegs        []string
	fmtCount        int
	lblCount        int
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
	b.buf.WriteString("\t; flush registers\n")
	for reg, id := range b.activeRegs {
		if reg == "X" { b.buf.WriteString("\ttfr x,d\n") }
		if reg == "Y" { b.buf.WriteString("\ttfr y,d\n") }
		if reg == "U" { b.buf.WriteString("\ttfr u,d\n") }
		b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.slots[id])))
	}
	b.activeRegs = map[string]int{}
	b.valInReg = map[int]string{}
	b.freeRegs = b.availableRegisters()
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
	for r, i := range b.activeRegs {
		regToSpill = r
		spilledId = i
		break
	}

	b.buf.WriteString(fmt.Sprintf("\t; spilling %s (val %d) to stack\n", regToSpill, spilledId))
	b.buf.WriteString("\tpshs d\n")
	b.pushBytes(2)
	if regToSpill == "X" { b.buf.WriteString("\ttfr x,d\n") }
	if regToSpill == "Y" { b.buf.WriteString("\ttfr y,d\n") }
	if regToSpill == "U" { b.buf.WriteString("\ttfr u,d\n") }
	b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.slots[spilledId])))
	b.buf.WriteString("\tpuls d\n")
	b.popBytes(2)

	delete(b.valInReg, spilledId)
	b.activeRegs[regToSpill] = id
	b.valInReg[id] = regToSpill
	return regToSpill
}

func (b *Backend) storeResult(id int) {
	reg := b.allocateReg(id)
	if reg == "X" { b.buf.WriteString("\ttfr d,x\n") }
	if reg == "Y" { b.buf.WriteString("\ttfr d,y\n") }
	if reg == "U" { b.buf.WriteString("\ttfr d,u\n") }
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
	}
	return ""
}

func (b *Backend) pushBytes(n int) {
	b.pushedBytes += n
}
func (b *Backend) popBytes(n int) {
	b.pushedBytes -= n
}

func (b *Backend) getSlot(id int, typ string) int {
	if offset, ok := b.slots[id]; ok {
		return offset
	}
	size := getTypeSize(typ)
	aligned := size
	if aligned < 2 {
		aligned = 2
	} else if aligned % 2 != 0 {
		aligned++
	}
	b.stackSize += aligned
	offset := -(b.frameOffset + b.stackSize)
	b.slots[id] = offset
	return offset
}

func (b *Backend) Generate(program *ir.Program) string {
	b.buf.WriteString("\tpragma cescapes\n")
	b.buf.WriteString("\tpragma undefextern\n")
	b.buf.WriteString("\tsection code\n")

	b.globalOffsets = make(map[string]int)
	if !b.globalsAtY && len(program.Globals) > 0 {
		b.dataBuf.WriteString("\tsection data\n")
		for _, g := range program.Globals {
			b.dataBuf.WriteString(fmt.Sprintf("\texport v_%s\n", g.Name))
			b.dataBuf.WriteString(fmt.Sprintf("v_%s:\n", g.Name))
			size := getTypeSize(string(g.Typ))
			for j := 0; j < size; j++ {
				b.dataBuf.WriteString("\tfcb 0\n")
			}
		}
	} else if b.globalsAtY {
		offset := 0
		for _, g := range program.Globals {
			b.globalOffsets[g.Name] = offset
			size := getTypeSize(string(g.Typ))
			offset += size
		}
	}

	for _, f := range program.Functions {
		b.emitFunc(f)
	}

	b.buf.WriteString("\n\texport _main\n")
	b.buf.WriteString("_main:\n")
	if b.picMode {
		b.buf.WriteString("\tlbsr f_main\n")
	} else {
		b.buf.WriteString("\tjsr f_main\n")
	}
	b.buf.WriteString("\tldx #0\n")
	b.buf.WriteString("\trts\n")

	return b.buf.String() + "\n" + b.rodataBuf.String() + "\n" + b.dataBuf.String()
}

func (b *Backend) emitFunc(f *ir.Function) {
	b.stackSize = 0
	b.pushedBytes = 0
	b.slots = make(map[int]int)
	b.paramSlots = make(map[string]int)

	var firstWord *ir.Parameter
	var firstByte *ir.Parameter

	for _, p := range f.Parameters {
		if p.Typ == ir.TypeWord && firstWord == nil {
			firstWord = p
		} else if p.Typ == ir.TypeByte && firstByte == nil {
			firstByte = p
		}
	}

	for _, p := range f.Parameters {
		size := getTypeSize(string(p.Typ))
		aligned := size
		if aligned < 2 { aligned = 2 } else if aligned % 2 != 0 { aligned++ }
		b.stackSize += aligned
		b.paramSlots[p.Name] = -(b.frameOffset + b.stackSize)
	}
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if instr.Type() != ir.TypeVoid && instr.Type() != ir.TypeUnknown {
				b.getSlot(instr.GetID(), string(instr.Type()))
			}
		}
	}

	b.buf.WriteString(fmt.Sprintf("\n\texport f_%s\n", f.Name))
	b.buf.WriteString(fmt.Sprintf("f_%s:\n", f.Name))
	if b.useFramePointer {
		b.buf.WriteString("\tpshs u\n")
		b.buf.WriteString("\ttfr s,u\n")
	}
	if b.stackSize > 0 {
		b.buf.WriteString(fmt.Sprintf("\tleas -%d,s\n", b.stackSize))
	}

	stackArgOffset := 2
	for _, p := range f.Parameters {
		if p == firstWord {
			b.buf.WriteString(fmt.Sprintf("\tstx %s\n", b.memAccess(b.paramSlots[p.Name])))
		} else if p == firstByte {
			b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.paramSlots[p.Name])))
		} else {
			// Array passing as arguments in 6809 not fully supported yet if > 2 bytes
			size := getTypeSize(string(p.Typ))
			if size <= 2 {
				b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(stackArgOffset)))
				b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.paramSlots[p.Name])))
			}
			aligned := size
			if aligned < 2 { aligned = 2 } else if aligned % 2 != 0 { aligned++ }
			stackArgOffset += aligned
		}
	}

	for _, blk := range f.Blocks {
		b.buf.WriteString(fmt.Sprintf(".Lb%d:\n", blk.ID))

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
			b.buf.WriteString(fmt.Sprintf("\tlbra .Lb%d\n", term.Target.ID))
		case *ir.Branch:
			b.loadVal(term.Condition)
			b.buf.WriteString("\tcmpd #0\n")
			b.buf.WriteString(fmt.Sprintf("\tbne .Lb%d_true\n", blk.ID))
			b.buf.WriteString(fmt.Sprintf("\tlbra .Lb%d_false\n", blk.ID))

			b.buf.WriteString(fmt.Sprintf(".Lb%d_true:\n", blk.ID))
			b.emitPhiAssignments(blk, term.TrueBlock)
			b.buf.WriteString(fmt.Sprintf("\tlbra .Lb%d\n", term.TrueBlock.ID))

			b.buf.WriteString(fmt.Sprintf(".Lb%d_false:\n", blk.ID))
			b.emitPhiAssignments(blk, term.FalseBlock)
			b.buf.WriteString(fmt.Sprintf("\tlbra .Lb%d\n", term.FalseBlock.ID))

		case *ir.Return:
			if term.Val != nil {
				b.loadVal(term.Val)
				if term.Val.Type() == ir.TypeWord {
					b.buf.WriteString("\ttfr d,x\n")
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
		}
	}
}

func (b *Backend) loadVal(val ir.Value) {
	switch v := val.(type) {
	case *ir.Parameter:
		b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(b.paramSlots[v.Name])))
	case *ir.ConstWord:
		b.buf.WriteString(fmt.Sprintf("\tldd #%d\n", v.Val&0xFFFF))
	case *ir.ConstByte:
		b.buf.WriteString(fmt.Sprintf("\tldb #%d\n\tclra\n", v.Val&0xFF))
	case ir.Instruction:
		if reg, ok := b.valInReg[v.GetID()]; ok {
			if reg == "X" { b.buf.WriteString("\ttfr x,d\n") }
			if reg == "Y" { b.buf.WriteString("\ttfr y,d\n") }
			if reg == "U" { b.buf.WriteString("\ttfr u,d\n") }
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(b.slots[v.GetID()])))
		}
	}
}

func (b *Backend) emitPhiAssignments(from, to *ir.BasicBlock) {
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*ir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					size := getTypeSize(string(phi.Typ))
					if size <= 2 {
						b.loadVal(edge.Value)
						if phi.Type() == ir.TypeByte {
							b.buf.WriteString("\tclra\n")
						}
						b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.slots[phi.GetID()])))
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

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()
	offset := b.slots[id]

	switch i := instr.(type) {
	case *ir.ConstByte, *ir.ConstWord:
		b.loadVal(i)
		b.storeResult(id)
	case *ir.Load:
		b.flushRegisters()
		size := getTypeSize(string(i.Global.Typ))
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
		if size == 1 {
			b.buf.WriteString("\tldb ,y\n")
			b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		} else if size == 2 {
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		} else {
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
		size := getTypeSize(string(i.Global.Typ))
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
			if size == 1 {
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldb 1,y\n")
				b.buf.WriteString("\tstb ,x\n")
			} else if size == 2 {
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldd ,y\n")
				b.buf.WriteString("\tstd ,x\n")
			} else {
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
		size := getTypeSize(string(i.Typ))
		destStr := b.memAccess(offset)
		if size == 1 || size == 2 {
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
		eltSize := getEltSize(string(i.Array.Type()))
		arrayStr := b.getAddrStr(i.Array)
		destStr := b.memAccess(offset)

		b.emitLoadAddr("y", arrayStr)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleay %d,y\n", byteOffset))
			}
		} else {
			panic("Dynamic array indexing not yet implemented for 6809")
		}

		if eltSize == 1 {
			b.buf.WriteString("\tldb ,y\n")
			b.buf.WriteString("\tclra\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		} else if eltSize == 2 {
			b.buf.WriteString("\tldd ,y\n")
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", destStr))
		} else {
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
		arraySize := getTypeSize(string(i.Array.Type()))
		arrayStr := b.getAddrStr(i.Array)
		destStr := b.memAccess(offset)
		
		b.emitLoadAddr("y", arrayStr)
		b.emitLoadAddr("x", destStr)
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

		eltSize := getEltSize(string(i.Array.Type()))
		b.emitLoadAddr("x", destStr)
		if cIdx, ok := i.Index.(*ir.ConstWord); ok {
			byteOffset := int(cIdx.Val) * eltSize
			if byteOffset > 0 {
				b.buf.WriteString(fmt.Sprintf("\tleax %d,x\n", byteOffset))
			}
		} else {
			panic("Dynamic array indexing not yet implemented for 6809")
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
			if eltSize == 1 {
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldb 1,y\n")
				b.buf.WriteString("\tstb ,x\n")
			} else if eltSize == 2 {
				b.emitLoadAddr("y", valStr)
				b.buf.WriteString("\tldd ,y\n")
				b.buf.WriteString("\tstd ,x\n")
			} else {
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
	case *ir.BinaryOp:
		b.loadVal(i.Right)
		b.buf.WriteString("\tstd ,--s\n")
		b.pushBytes(2)
		b.loadVal(i.Left)
		switch i.Op {
		case "add":
			b.buf.WriteString("\taddd ,s++\n")
			b.popBytes(2)
		case "sub":
			b.buf.WriteString("\tsubd ,s++\n")
			b.popBytes(2)
		case "mul", "div", "mod", "shl", "shr":
			b.buf.WriteString(fmt.Sprintf("\t; unimplemented %s\n", i.Op))
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
		}
		if i.Typ == ir.TypeByte {
			b.buf.WriteString("\tclra\n")
		}
		b.storeResult(id)
	case *ir.Compare:
		b.loadVal(i.Right)
		b.buf.WriteString("\tstd ,--s\n")
		b.pushBytes(2)
		b.loadVal(i.Left)
		b.buf.WriteString("\tcmpd ,s++\n")
		b.popBytes(2)

		lblTrue := b.nextLabel()
		lblEnd := b.nextLabel()

		switch i.Op {
		case "eq":
			b.buf.WriteString(fmt.Sprintf("\tbeq %s\n", lblTrue))
		case "neq":
			b.buf.WriteString(fmt.Sprintf("\tbne %s\n", lblTrue))
		case "lt":
			b.buf.WriteString(fmt.Sprintf("\tblo %s\n", lblTrue))
		case "lte":
			b.buf.WriteString(fmt.Sprintf("\tbls %s\n", lblTrue))
		case "gt":
			b.buf.WriteString(fmt.Sprintf("\tbhi %s\n", lblTrue))
		case "gte":
			b.buf.WriteString(fmt.Sprintf("\tbhs %s\n", lblTrue))
		}
		b.buf.WriteString("\tclrb\n\tbra " + lblEnd + "\n")
		b.buf.WriteString(lblTrue + ":\n\tldb #1\n")
		b.buf.WriteString(lblEnd + ":\n\tclra\n")
		b.storeResult(id)
	case *ir.Call:
		b.flushRegisters()
		var firstWordArg ir.Value
		var firstByteArg ir.Value
		var firstWordIdx = -1
		var firstByteIdx = -1

		for idx, arg := range i.Args {
			if arg.Type() == ir.TypeWord && firstWordArg == nil {
				firstWordArg = arg
				firstWordIdx = idx
			} else if arg.Type() == ir.TypeByte && firstByteArg == nil {
				firstByteArg = arg
				firstByteIdx = idx
			}
		}

		var pushedBytes int
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			if idx == firstWordIdx || idx == firstByteIdx {
				continue
			}
			b.loadVal(i.Args[idx])
			b.buf.WriteString("\tstd ,--s\n")
			b.pushBytes(2)
			pushedBytes += 2
		}

		if firstWordArg != nil {
			b.loadVal(firstWordArg)
			b.buf.WriteString("\ttfr d,x\n")
		}
		if firstByteArg != nil {
			b.loadVal(firstByteArg)
		}

		if b.picMode {
			b.buf.WriteString(fmt.Sprintf("\tlbsr f_%s\n", i.Func.Name))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tjsr f_%s\n", i.Func.Name))
		}

		if pushedBytes > 0 {
			b.buf.WriteString(fmt.Sprintf("\tleas %d,s\n", pushedBytes))
			b.popBytes(pushedBytes)
		}

		if i.Typ == ir.TypeWord {
			b.buf.WriteString("\ttfr x,d\n")
		} else if i.Typ == ir.TypeByte {
			b.buf.WriteString("\tclra\n")
		}
		if i.Typ != ir.TypeVoid {
			b.storeResult(id)
		}
	case *ir.BuiltinCall:
		b.flushRegisters()
		if i.Name == "print" || i.Name == "println" {
			b.emitPrint(i.Name == "println", i.Args)
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
			formatStrs = append(formatStrs, strLit.Value)
		} else {
			formatStrs = append(formatStrs, "%u")
			dataArgs = append(dataArgs, arg)
		}
	}

	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\\n"
	}

	if b.picMode {
		b.rodataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"%s\"\n", fmtLabel, format))
	} else {
		if b.dataBuf.Len() == 0 {
			b.dataBuf.WriteString("\tsection data\n")
		}
		b.dataBuf.WriteString(fmt.Sprintf("%s:\n\t.asciz \"%s\"\n", fmtLabel, format))
	}

	for i := len(dataArgs) - 1; i >= 0; i-- {
		b.loadVal(dataArgs[i])
		b.buf.WriteString("\tstd ,--s\n")
		b.pushBytes(2)
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
