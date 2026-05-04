package m6809

import (
	"bytes"
	"fmt"
	"minigo/ir"
	"strings"
)

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

func (b *Backend) pushBytes(n int) {
	b.pushedBytes += n
}
func (b *Backend) popBytes(n int) {
	b.pushedBytes -= n
}

func (b *Backend) getSlot(id int) int {
	if offset, ok := b.slots[id]; ok {
		return offset
	}
	b.stackSize += 2
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
			b.dataBuf.WriteString("\tfdb 0\n")
		}
	} else if b.globalsAtY {
		offset := 0
		for _, g := range program.Globals {
			b.globalOffsets[g.Name] = offset
			offset += 2
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
		b.stackSize += 2
		b.paramSlots[p.Name] = -(b.frameOffset + b.stackSize)
	}
	for _, blk := range f.Blocks {
		for _, instr := range blk.Instructions {
			if instr.Type() != ir.TypeVoid && instr.Type() != ir.TypeUnknown {
				b.getSlot(instr.GetID())
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
			b.buf.WriteString(fmt.Sprintf("\tldd %s\n", b.memAccess(stackArgOffset)))
			b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.paramSlots[p.Name])))
			stackArgOffset += 2
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
					b.loadVal(edge.Value)
					if phi.Type() == ir.TypeByte {
						b.buf.WriteString("\tclra\n")
					}
					b.buf.WriteString(fmt.Sprintf("\tstd %s\n", b.memAccess(b.slots[phi.GetID()])))
				}
			}
		}
	}
}

func (b *Backend) emitInstr(instr ir.Instruction) {
	id := instr.GetID()

	switch i := instr.(type) {
	case *ir.ConstByte, *ir.ConstWord:
		b.loadVal(i)
		b.storeResult(id)
	case *ir.Load:
		if b.globalsAtY {
			b.buf.WriteString(fmt.Sprintf("\tldd %d,y\n", b.globalOffsets[i.Global.Name]))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tldd v_%s\n", i.Global.Name))
		}
		b.storeResult(id)
	case *ir.Store:
		b.loadVal(i.Val)
		if i.Global.Typ == ir.TypeByte {
			b.buf.WriteString("\tclra\n")
		}
		if b.globalsAtY {
			b.buf.WriteString(fmt.Sprintf("\tstd %d,y\n", b.globalOffsets[i.Global.Name]))
		} else {
			b.buf.WriteString(fmt.Sprintf("\tstd v_%s\n", i.Global.Name))
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
	b.buf.WriteString(fmt.Sprintf("\tleax %s,pcr\n", fmtLabel))
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
