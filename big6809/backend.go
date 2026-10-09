package big6809

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/strickyak/minigolf/bigir"
)

// MangleName converts characters like '.' into valid assembly identifiers.
func MangleName(s string) string {
	var bbuf bytes.Buffer
	for _, c := range s {
		if ('0' <= c && c <= '9') ||
			('a' <= c && c <= 'z') ||
			('A' <= c && c <= 'Z') ||
			c == '_' {
			bbuf.WriteByte(byte(c))
		} else {
			bbuf.WriteByte('_')
			bbuf.WriteByte('_')
		}
	}
	return bbuf.String()
}

// Backend generates Motorola 6809 assembly code from BIGIR for EMBIGGEN mode.
type Backend struct {
	fmtCount   int
	fmtStrings map[string]string
}

// New creates a new EMBIGGEN 6809 backend.
func New() *Backend {
	return &Backend{
		fmtStrings: make(map[string]string),
	}
}

// Generate transforms a BIGIR Program into valid 6809 assembly.
func (b *Backend) Generate(prog *bigir.Program) (string, error) {
	var buf bytes.Buffer
	var codeBuf bytes.Buffer

	// 1. Emit CStart and Core Runtime in Slot 6
	buf.WriteString(CStartTemplate)
	buf.WriteString("\n")

	// 2. Emit Slot 6 Trampolines
	buf.WriteString("; ==============================================================================\n")
	buf.WriteString("; Slot 6 Far Function Trampolines ($C000..$DFFF)\n")
	buf.WriteString("; ==============================================================================\n\n")

	for _, tramp := range prog.Trampolines {
		mName := MangleName(tramp.FuncName)
		buf.WriteString(fmt.Sprintf("f_%s:\n", mName))
		buf.WriteString(fmt.Sprintf("    ldb   #%d\n", tramp.TargetBlock))
		buf.WriteString(fmt.Sprintf("    ldx   #$A000+fn_%s-_block_%d_data\n", mName, tramp.TargetBlock))
		buf.WriteString("    jmp   __far_call_dispatcher\n\n")
	}

	// Sort block IDs for deterministic emission
	var blockIDs []int
	for blk := range prog.FarBlocks {
		blockIDs = append(blockIDs, blk)
	}
	sort.Ints(blockIDs)

	// 3. Emit String Literals and Global Data
	buf.WriteString("; ==============================================================================\n")
	buf.WriteString("; Global Data and 8-Byte String Descriptors\n")
	buf.WriteString("; ==============================================================================\n\n")

	strCount := 0
	stringDescs := make(map[string]string)

	for _, g := range prog.Globals {
		if g.InitString != "" {
			mName := MangleName(g.Name)
			if descLbl, exists := stringDescs[g.InitString]; exists {
				dataLbl := strings.Replace(descLbl, "_desc_", "_data_", 1)
				buf.WriteString(fmt.Sprintf("v_%s equ %s\n", mName, dataLbl))
				continue
			}
			lbl := fmt.Sprintf("_str_data_%d", strCount)
			descLbl := fmt.Sprintf("_str_desc_%d", strCount)
			strCount++
			stringDescs[g.InitString] = descLbl

			strLen := len(g.InitString)
			cleanStr := g.InitString
			if strings.HasSuffix(cleanStr, "\x00") {
				cleanStr = strings.TrimSuffix(cleanStr, "\x00")
				strLen = len(cleanStr)
			}

			buf.WriteString(fmt.Sprintf("%s:\n", lbl))
			buf.WriteString(fmt.Sprintf("v_%s:\n", mName))
			buf.WriteString(fmt.Sprintf("    fcc   %q\n", cleanStr))
			buf.WriteString("    fcb   0\n")
			buf.WriteString(fmt.Sprintf("%s:\n", descLbl))
			buf.WriteString("    fdb   0                 ; far_ref = 0 (Near / fixed literal)\n")
			buf.WriteString(fmt.Sprintf("    fdb   %s            ; offset = virtual address of text\n", lbl))
			buf.WriteString(fmt.Sprintf("    fdb   %d                ; length\n", strLen))
			buf.WriteString(fmt.Sprintf("    fdb   %d                ; capacity\n\n", strLen))
		} else {
			mName := MangleName(g.Name)
			buf.WriteString(fmt.Sprintf("v_%s:\n", mName))
			if g.InitVal != nil {
				b.emitData(&buf, g.InitVal)
				buf.WriteString("\n")
			} else {
				sz := g.Typ.Size
				if sz <= 0 {
					sz = 2
				}
				buf.WriteString(fmt.Sprintf("    rmb   %d\n\n", sz))
			}
		}
	}

	// 4. Emit Far Code Blocks (Staged Data for Blocks 8..127) into codeBuf
	codeBuf.WriteString("; ==============================================================================\n")
	codeBuf.WriteString("; Staged Far Code Blocks (Packed into 8KB physical blocks)\n")
	codeBuf.WriteString("; ==============================================================================\n\n")

	for _, blk := range blockIDs {
		codeBuf.WriteString(fmt.Sprintf("; --- Physical Block %d Data ---\n", blk))
		codeBuf.WriteString(fmt.Sprintf("    org   $%X\n", uint32(blk)*8192))
		codeBuf.WriteString(fmt.Sprintf("_block_%d_data:\n", blk))

		funcs := prog.FarBlocks[blk]
		for _, fn := range funcs {
			b.emitFunction(&codeBuf, fn, stringDescs)
		}

		codeBuf.WriteString(fmt.Sprintf("_block_%d_end:\n", blk))
		codeBuf.WriteString(fmt.Sprintf("_block_%d_size equ _block_%d_end - _block_%d_data\n\n", blk, blk, blk))
	}

	// Emit format strings collected during function emission
	if len(b.fmtStrings) > 0 {
		var fmtKeys []string
		for k := range b.fmtStrings {
			fmtKeys = append(fmtKeys, k)
		}
		sort.Strings(fmtKeys)
		for _, k := range fmtKeys {
			buf.WriteString(fmt.Sprintf("%s:\n    .asciz %q\n", k, b.fmtStrings[k]))
		}
		buf.WriteString("\n")
	}

	// Append Far Code Blocks to main buffer
	buf.Write(codeBuf.Bytes())

	// Trailer
	buf.WriteString("    end cstart_embiggen\n")

	return buf.String(), nil
}

func (b *Backend) emitData(buf *bytes.Buffer, val bigir.Value) {
	switch v := val.(type) {
	case *bigir.ConstByte:
		buf.WriteString(fmt.Sprintf("    fcb   %d\n", v.Val))
	case *bigir.ConstWord:
		buf.WriteString(fmt.Sprintf("    fdb   %d\n", v.Val))
	case *bigir.Global:
		buf.WriteString(fmt.Sprintf("    fdb   v_%s\n", MangleName(v.Name)))
	case *bigir.AddressOfGlobal:
		buf.WriteString(fmt.Sprintf("    fdb   v_%s\n", MangleName(v.Global.Name)))
	case *bigir.FuncRef:
		buf.WriteString(fmt.Sprintf("    fdb   f_%s\n", MangleName(v.FuncName)))
	case *bigir.ConstStruct:
		for _, f := range v.Fields {
			b.emitData(buf, f)
		}
	case *bigir.ConstArray:
		for _, el := range v.Elements {
			b.emitData(buf, el)
		}
	default:
		panic(fmt.Sprintf("unsupported init value type %T", val))
	}
}

func resolveRootLocal(v bigir.Value) bigir.Value {
	visited := make(map[int]bool)
	for {
		phi, ok := v.(*bigir.Phi)
		if !ok || phi.GetID() == 0 || visited[phi.GetID()] {
			break
		}
		visited[phi.GetID()] = true
		found := false
		for _, edge := range phi.Edges {
			if edge.Value != nil && edge.Value != phi {
				v = edge.Value
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return v
}

func getBigIROperands(instr bigir.Instruction) []bigir.Value {
	var ops []bigir.Value
	switch i := instr.(type) {
	case *bigir.UnaryOp:
		ops = append(ops, i.Operand)
	case *bigir.BinaryOp:
		ops = append(ops, i.Left, i.Right)
	case *bigir.Compare:
		ops = append(ops, i.Left, i.Right)
	case *bigir.NearStore:
		ops = append(ops, i.Addr, i.Val)
	case *bigir.NearLoad:
		ops = append(ops, i.Addr)
	case *bigir.FarStore:
		ops = append(ops, i.FarRef, i.Offset, i.Val)
	case *bigir.FarLoad:
		ops = append(ops, i.FarRef, i.Offset)
	case *bigir.SliceMake:
		ops = append(ops, i.FarRef, i.Offset, i.Length, i.Capacity)
	case *bigir.SliceField:
		ops = append(ops, i.Slice)
	case *bigir.ZeroInit:
		// no operands
	case *bigir.SliceToPtr:
		ops = append(ops, i.Slice)
	case *bigir.BitCast:
		ops = append(ops, i.Operand)
	case *bigir.ExtractField:
		ops = append(ops, i.Struct)
	case *bigir.InsertField:
		ops = append(ops, i.Struct, i.Val)
	case *bigir.SliceGet:
		ops = append(ops, i.Slice, i.Index)
	case *bigir.SlicePut:
		ops = append(ops, i.Slice, i.Index, i.Val)
	case *bigir.SliceChop:
		ops = append(ops, i.Slice, i.Start, i.Limit)
	case *bigir.AddressOfLocal:
		ops = append(ops, i.Local)
	case *bigir.FarCall:
		ops = append(ops, i.Args...)
	case *bigir.NearCall:
		ops = append(ops, i.Args...)
	case *bigir.IndirectCall:
		ops = append(ops, i.FuncPtr)
		ops = append(ops, i.Args...)
	case *bigir.Phi:
		for _, e := range i.Edges {
			ops = append(ops, e.Value)
		}
	}
	return ops
}

func getBigIRTerminatorOperands(t bigir.Terminator) []bigir.Value {
	var ops []bigir.Value
	switch term := t.(type) {
	case *bigir.Return:
		if term.Val != nil {
			ops = append(ops, term.Val)
		}
	case *bigir.FarReturn:
		if term.Val != nil {
			ops = append(ops, term.Val)
		}
	case *bigir.CondBranch:
		if term.Cond != nil {
			ops = append(ops, term.Cond)
		}
	}
	return ops
}

func getInstrSlotSize(instr bigir.Instruction) int {
	typ := instr.Type()
	if _, ok := instr.(*bigir.SliceMake); ok {
		return 8
	} else if typ.Kind == bigir.KindFarSlice || typ.Kind == bigir.KindFarString {
		return 8
	} else if typ.Kind == bigir.KindArray {
		return 2
	} else if typ.Size > 2 {
		sz := (typ.Size + 1) & ^1
		if sz < 8 {
			sz = 8
		}
		return sz
	} else if phi, ok := instr.(*bigir.Phi); ok {
		maxSz := 2
		if phi.Type().Size > maxSz {
			maxSz = phi.Type().Size
		}
		for _, edge := range phi.Edges {
			if edge.Value.Type().Size > maxSz {
				maxSz = edge.Value.Type().Size
			}
		}
		if maxSz > 2 {
			sz := (maxSz + 1) & ^1
			if sz < 8 {
				sz = 8
			}
			return sz
		}
	}
	return 2
}

// WindowSlotTracker tracks which FarRef/slice/buffer is currently mapped into
// physical MMAP window slots 2 ($4000, $FF42), 3 ($6000, $FF43), and 4 ($8000, $FF44)
// within a straight-line basic block.
type WindowSlotTracker struct {
	mappedKey [5]string
}

func (t *WindowSlotTracker) Reset() {
	for i := range t.mappedKey {
		t.mappedKey[i] = ""
	}
}

func (t *WindowSlotTracker) Invalidate(slot int) {
	if slot >= 0 && slot < len(t.mappedKey) {
		t.mappedKey[slot] = ""
	}
}

func (t *WindowSlotTracker) InvalidateKey(key string) {
	if key == "" {
		return
	}
	for i := range t.mappedKey {
		if t.mappedKey[i] == key {
			t.mappedKey[i] = ""
		}
	}
}

func (t *WindowSlotTracker) InvalidateAll() {
	t.Reset()
}

func (t *WindowSlotTracker) IsMapped(slot int, key string) bool {
	if key == "" || slot < 0 || slot >= len(t.mappedKey) {
		return false
	}
	return t.mappedKey[slot] == key
}

func (t *WindowSlotTracker) SetMapped(slot int, key string) {
	if key != "" && slot >= 0 && slot < len(t.mappedKey) {
		t.mappedKey[slot] = key
	}
}

func valKey(v bigir.Value) string {
	if v == nil {
		return ""
	}
	v = resolveRootLocal(v)
	if aol, ok := v.(*bigir.AddressOfLocal); ok {
		root := resolveRootLocal(aol.Local)
		if instr, ok := root.(bigir.Instruction); ok {
			return fmt.Sprintf("instr:%d", instr.GetID())
		}
	}
	if aog, ok := v.(*bigir.AddressOfGlobal); ok && aog.Global != nil {
		return fmt.Sprintf("global:%s", aog.Global.Name)
	}
	if g, ok := v.(*bigir.Global); ok {
		return fmt.Sprintf("global:%s", g.Name)
	}
	if p, ok := v.(*bigir.Parameter); ok {
		return fmt.Sprintf("param:%d", p.ID)
	}
	if sm, ok := v.(*bigir.SliceMake); ok {
		if cw, ok := sm.FarRef.(*bigir.ConstWord); ok {
			return fmt.Sprintf("far_const:%d", cw.Val)
		}
		if cb, ok := sm.FarRef.(*bigir.ConstByte); ok {
			return fmt.Sprintf("far_const:%d", cb.Val)
		}
		return fmt.Sprintf("instr:%d", sm.GetID())
	}
	if cw, ok := v.(*bigir.ConstWord); ok {
		return fmt.Sprintf("const:%d", cw.Val)
	}
	if cb, ok := v.(*bigir.ConstByte); ok {
		return fmt.Sprintf("const:%d", cb.Val)
	}
	if instr, ok := v.(bigir.Instruction); ok && instr.GetID() > 0 {
		return fmt.Sprintf("instr:%d", instr.GetID())
	}
	return ""
}

func (b *Backend) emitFunction(buf *bytes.Buffer, fn *bigir.Function, stringDescs map[string]string) {
	mName := MangleName(fn.Name)
	buf.WriteString(fmt.Sprintf("; Function: %s (Block %d, Slot 5 Offset 0x%04X)\n", fn.Name, fn.BlockID, fn.Slot5Offset))
	buf.WriteString(fmt.Sprintf("fn_%s:\n", mName))

	// Parameter stack offsets relative to U:
	// 0,u = saved U (2B)
	// 2,u = return address (2B)
	// If fn.ReturnType.Size > 2, 4,u is the invisible return pointer ret_ptr,
	// and user parameters start at 6,u.
	paramOffsets := make(map[int]int)
	curParamOffset := 4
	if fn.ReturnType.Size > 2 {
		curParamOffset = 6
	}
	for _, p := range fn.Parameters {
		sz := p.Typ.Size
		if sz < 2 {
			sz = 2 // 2-byte stack alignment
		}
		sz = (sz + 1) & ^1
		paramOffsets[p.ID] = curParamOffset
		curParamOffset += sz
	}

	// 1. Collect instructions whose address is taken via AddressOfLocal.
	addressTaken := make(map[int]bool)
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			if aol, ok := instr.(*bigir.AddressOfLocal); ok {
				root := resolveRootLocal(aol.Local)
				if target, ok := root.(bigir.Instruction); ok {
					addressTaken[target.GetID()] = true
				}
			}
		}
	}

	// 2. Collect instructions whose values are actually read.
	usedInstrs := make(map[int]bool)
	var getLeafInstrIDs func(val bigir.Value, fnVisit func(int))
	getLeafInstrIDs = func(val bigir.Value, fnVisit func(int)) {
		if val == nil {
			return
		}
		if bop, ok := val.(*bigir.BinaryOp); ok && bop.GetID() == 0 {
			getLeafInstrIDs(bop.Left, fnVisit)
			getLeafInstrIDs(bop.Right, fnVisit)
			return
		}
		if aol, ok := val.(*bigir.AddressOfLocal); ok {
			root := resolveRootLocal(aol.Local)
			if target, ok := root.(bigir.Instruction); ok {
				fnVisit(target.GetID())
			}
			return
		}
		if sm, ok := val.(*bigir.SliceMake); ok && sm.GetID() == 0 {
			getLeafInstrIDs(sm.FarRef, fnVisit)
			getLeafInstrIDs(sm.Offset, fnVisit)
			getLeafInstrIDs(sm.Length, fnVisit)
			getLeafInstrIDs(sm.Capacity, fnVisit)
			return
		}
		if sf, ok := val.(*bigir.SliceField); ok && sf.GetID() == 0 {
			getLeafInstrIDs(sf.Slice, fnVisit)
			return
		}
		if stp, ok := val.(*bigir.SliceToPtr); ok && stp.GetID() == 0 {
			getLeafInstrIDs(stp.Slice, fnVisit)
			return
		}
		if bc, ok := val.(*bigir.BitCast); ok && bc.GetID() == 0 {
			getLeafInstrIDs(bc.Operand, fnVisit)
			return
		}
		if instr, ok := val.(bigir.Instruction); ok {
			fnVisit(instr.GetID())
		}
	}

	var markUse func(val bigir.Value)
	markUse = func(val bigir.Value) {
		getLeafInstrIDs(val, func(id int) {
			usedInstrs[id] = true
		})
	}

	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			for _, op := range getBigIROperands(instr) {
				markUse(op)
			}
		}
		if bb.Terminator != nil {
			for _, op := range getBigIRTerminatorOperands(bb.Terminator) {
				markUse(op)
			}
		}
	}

	// Local SSA instruction offsets relative to U:
	localOffsets := make(map[int]int)
	curLocalOffset := 0

	// Pass 1: Allocate stack buffers for variables whose address is taken
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			if addressTaken[instr.GetID()] {
				sz := instr.Type().Size
				if sz < 2 {
					sz = 2
				}
				sz = (sz + 1) & ^1 // align to 2 bytes
				curLocalOffset += sz
				localOffsets[instr.GetID()] = curLocalOffset
			}
		}
	}

	// Pass 2: Allocate stack slots for instructions that produce used values or phis.
	// To minimize stack frame size, purely intra-block temporaries share scratch slots.
	defBlock := make(map[int]*bigir.BasicBlock)
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			defBlock[instr.GetID()] = bb
		}
	}

	crossBlock := make(map[int]bool)
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			id := instr.GetID()
			if phi, ok := instr.(*bigir.Phi); ok {
				crossBlock[phi.GetID()] = true
				for _, edge := range phi.Edges {
					getLeafInstrIDs(edge.Value, func(eid int) {
						crossBlock[eid] = true
					})
				}
			}
			if typ := instr.Type(); typ.Size > 2 {
				switch instr.(type) {
				case *bigir.NearCall, *bigir.FarCall, *bigir.IndirectCall, *bigir.SliceGet, *bigir.FarLoad:
					crossBlock[id] = true
				}
			}
			sz := getInstrSlotSize(instr)
			if sz != 2 && sz != 8 {
				crossBlock[id] = true
			}
			for _, op := range getBigIROperands(instr) {
				getLeafInstrIDs(op, func(opID int) {
					if db, ok := defBlock[opID]; ok && db != bb {
						crossBlock[opID] = true
					}
				})
			}
		}
		if bb.Terminator != nil {
			for _, op := range getBigIRTerminatorOperands(bb.Terminator) {
				getLeafInstrIDs(op, func(opID int) {
					if db, ok := defBlock[opID]; ok && db != bb {
						crossBlock[opID] = true
					}
				})
			}
		}
	}

	// Allocate dedicated slots for cross-block instructions
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			id := instr.GetID()
			if addressTaken[id] || !crossBlock[id] {
				continue
			}

			switch instr.(type) {
			case *bigir.ConstByte, *bigir.ConstWord, *bigir.AddressOfGlobal, *bigir.AddressOfLocal, *bigir.FuncRef:
				continue
			}

			isPhi := false
			if _, ok := instr.(*bigir.Phi); ok {
				isPhi = true
			}

			typ := instr.Type()
			isCallWithStructRet := false
			switch instr.(type) {
			case *bigir.NearCall, *bigir.FarCall, *bigir.IndirectCall, *bigir.SliceGet, *bigir.FarLoad:
				if typ.Size > 2 {
					isCallWithStructRet = true
				}
			}
			if !usedInstrs[id] && !isPhi && !isCallWithStructRet {
				continue
			}

			if typ.Size <= 0 {
				continue
			}

			sz := getInstrSlotSize(instr)
			curLocalOffset += sz
			localOffsets[id] = curLocalOffset
		}
	}

	// Compute live ranges and allocate intra-block scratch slots
	type intraColor struct {
		freeAt int
	}
	maxScratch2 := 0
	maxScratch8 := 0
	blockScratch2Assigned := make(map[int]int)
	blockScratch8Assigned := make(map[int]int)

	for _, bb := range fn.Blocks {
		startIdx := make(map[int]int)
		endIdx := make(map[int]int)

		for i, instr := range bb.Instructions {
			id := instr.GetID()
			if addressTaken[id] || crossBlock[id] || !usedInstrs[id] {
				continue
			}
			switch instr.(type) {
			case *bigir.ConstByte, *bigir.ConstWord, *bigir.AddressOfGlobal, *bigir.AddressOfLocal, *bigir.FuncRef:
				continue
			}
			if instr.Type().Size <= 0 {
				continue
			}
			startIdx[id] = i
			endIdx[id] = i
		}

		for i, instr := range bb.Instructions {
			for _, op := range getBigIROperands(instr) {
				getLeafInstrIDs(op, func(opID int) {
					if _, ok := startIdx[opID]; ok {
						if i > endIdx[opID] {
							endIdx[opID] = i
						}
					}
				})
			}
		}

		if bb.Terminator != nil {
			termIdx := len(bb.Instructions)
			for _, op := range getBigIRTerminatorOperands(bb.Terminator) {
				getLeafInstrIDs(op, func(opID int) {
					if _, ok := startIdx[opID]; ok {
						if termIdx > endIdx[opID] {
							endIdx[opID] = termIdx
						}
					}
				})
			}
		}

		var active2 []*intraColor
		var active8 []*intraColor

		for _, instr := range bb.Instructions {
			id := instr.GetID()
			if _, ok := startIdx[id]; !ok {
				continue
			}
			sz := getInstrSlotSize(instr)
			if sz == 2 {
				assigned := -1
				for slotIdx, c := range active2 {
					if startIdx[id] > c.freeAt {
						assigned = slotIdx
						c.freeAt = endIdx[id]
						break
					}
				}
				if assigned == -1 {
					assigned = len(active2)
					active2 = append(active2, &intraColor{freeAt: endIdx[id]})
				}
				blockScratch2Assigned[id] = assigned
			} else if sz == 8 {
				assigned := -1
				for slotIdx, c := range active8 {
					if startIdx[id] > c.freeAt {
						assigned = slotIdx
						c.freeAt = endIdx[id]
						break
					}
				}
				if assigned == -1 {
					assigned = len(active8)
					active8 = append(active8, &intraColor{freeAt: endIdx[id]})
				}
				blockScratch8Assigned[id] = assigned
			}
		}

		if len(active2) > maxScratch2 {
			maxScratch2 = len(active2)
		}
		if len(active8) > maxScratch8 {
			maxScratch8 = len(active8)
		}
	}

	scratch2Offsets := make([]int, maxScratch2)
	for i := 0; i < maxScratch2; i++ {
		curLocalOffset += 2
		scratch2Offsets[i] = curLocalOffset
	}

	scratch8Offsets := make([]int, maxScratch8)
	for i := 0; i < maxScratch8; i++ {
		curLocalOffset += 8
		scratch8Offsets[i] = curLocalOffset
	}

	for id, slotIdx := range blockScratch2Assigned {
		localOffsets[id] = scratch2Offsets[slotIdx]
	}
	for id, slotIdx := range blockScratch8Assigned {
		localOffsets[id] = scratch8Offsets[slotIdx]
	}
	frameSize := (curLocalOffset + 3) & ^3
	if frameSize < 16 {
		frameSize = 16
	}

	// Prologue
	buf.WriteString("    pshs  u\n")
	buf.WriteString("    tfr   s,u\n")
	buf.WriteString(fmt.Sprintf("    leas  -%d,s            ; Allocate local stack frame\n", frameSize))

	// Special handling for intrinsic magic functions without an AST body:
	if fn.Name == "prelude.mul_byte" {
		buf.WriteString("    lda   5,u               ; param a (low byte)\n")
		buf.WriteString("    ldb   7,u               ; param b (low byte)\n")
		buf.WriteString("    mul                     ; D = A * B (16-bit word)\n")
		buf.WriteString(fmt.Sprintf(".L_%s_epilogue:\n", mName))
		buf.WriteString("    leas  ,u\n")
		buf.WriteString("    puls  u\n")
		buf.WriteString("    rts\n\n")
		return
	}

	// Emit Basic Blocks
	tracker := &WindowSlotTracker{}
	for _, bb := range fn.Blocks {
		tracker.Reset()
		buf.WriteString(fmt.Sprintf(".L_%s_bb%d:\n", mName, bb.ID))

		for _, instr := range bb.Instructions {
			b.emitInstruction(buf, fn, instr, paramOffsets, localOffsets, stringDescs, tracker)
		}

		if bb.Terminator != nil {
			b.emitTerminator(buf, fn, bb, bb.Terminator, paramOffsets, localOffsets, stringDescs)
		}
	}

	// Epilogue
	buf.WriteString(fmt.Sprintf(".L_%s_epilogue:\n", mName))
	buf.WriteString("    leas  ,u\n")
	buf.WriteString("    puls  u\n")
	buf.WriteString("    rts\n\n")
}

func (b *Backend) emitInstruction(
	buf *bytes.Buffer,
	fn *bigir.Function,
	instr bigir.Instruction,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
	tracker *WindowSlotTracker,
) {
	mName := MangleName(fn.Name)
	switch i := instr.(type) {
	case *bigir.ConstByte:
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   #%d\n", i.Val))
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}
	case *bigir.ConstWord:
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   #%d\n", i.Val))
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.Phi:
		// Target stack slot is already assigned at edge transition; no-op inside block.

	case *bigir.UnaryOp:
		b.loadValToD(buf, i.Operand, paramOffsets, localOffsets, stringDescs)
		switch i.Op {
		case "neg":
			buf.WriteString("    coma\n    comb\n    addd  #1\n")
		case "not":
			buf.WriteString("    coma\n    comb\n")
		}
		if i.Type().Size == 1 {
			buf.WriteString("    clra\n")
		}
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.BinaryOp:
		b.emitBinaryOp(buf, i, paramOffsets, localOffsets, stringDescs)
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.SliceMake:
		if slot, ok := localOffsets[i.GetID()]; ok {
			b.loadValToD(buf, i.FarRef, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; far_ref\n", slot))
			b.loadValToD(buf, i.Offset, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; offset\n", slot-2))
			b.loadValToD(buf, i.Length, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; length\n", slot-4))
			b.loadValToD(buf, i.Capacity, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; capacity\n", slot-6))
		}

	case *bigir.SliceField:
		b.loadSliceFieldToD(buf, i, paramOffsets, localOffsets, stringDescs)
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store slice field v%d\n", slot, i.GetID()))
		}

	case *bigir.SliceToPtr:
		b.loadSliceDescToReg(buf, i.Slice, "x", paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    jsr   __slice_to_ptr\n")
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store ptr v%d\n", slot, i.GetID()))
		}
		if tracker != nil {
			tracker.Invalidate(2)
		}

	case *bigir.BitCast:
		if slot, ok := localOffsets[i.GetID()]; ok {
			b.loadValToD(buf, i.Operand, paramOffsets, localOffsets, stringDescs)
			if i.Type().Size == 1 {
				buf.WriteString("    clra\n")
			}
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store bitcast v%d\n", slot, i.GetID()))
		}


	case *bigir.SliceGet:
		b.emitSliceGet(buf, fn, i, i.Slice, i.Index, 2, paramOffsets, localOffsets, stringDescs, tracker)

	case *bigir.SlicePut:
		b.emitSlicePut(buf, fn, i, i.Slice, i.Index, i.Val, 3, paramOffsets, localOffsets, stringDescs, tracker)

	case *bigir.FarLoad:
		b.emitFarLoad(buf, fn, i, i.FarRef, i.Offset, 2, paramOffsets, localOffsets, stringDescs, tracker)

	case *bigir.FarStore:
		b.emitFarStore(buf, fn, i, i.FarRef, i.Offset, i.Val, 3, paramOffsets, localOffsets, stringDescs, tracker)

	case *bigir.ZeroInit:
		if slot, ok := localOffsets[i.GetID()]; ok {
			sz := (i.Type().Size + 1) & ^1
			if sz <= 8 {
				for off := 0; off < sz; off += 2 {
					buf.WriteString("    clra\n    clrb\n")
					buf.WriteString(fmt.Sprintf("    std   -%d,u             ; zero init\n", slot-off))
				}
			} else {
				lbl := fmt.Sprintf(".L_%s_zero_%d", mName, i.GetID())
				buf.WriteString(fmt.Sprintf("    leax  -%d,u             ; zero init large struct/array\n", slot))
				buf.WriteString(fmt.Sprintf("    ldy   #%d\n", sz))
				buf.WriteString(fmt.Sprintf("%s:\n", lbl))
				buf.WriteString("    clr   ,x+\n")
				buf.WriteString("    leay  -1,y\n")
				buf.WriteString(fmt.Sprintf("    bne   %s\n", lbl))
			}
		}

	case *bigir.ExtractField:
		if destSlot, ok := localOffsets[i.GetID()]; ok {
			if i.FieldSize == 1 {
				b.loadExtractFieldToD(buf, i, paramOffsets, localOffsets, stringDescs)
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; extract field %d\n", destSlot, i.FieldIndex))
			} else if i.FieldSize == 2 {
				b.loadExtractFieldToD(buf, i, paramOffsets, localOffsets, stringDescs)
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; extract field %d\n", destSlot, i.FieldIndex))
			} else {
				b.copyFieldBytes(buf, i.Struct, i.ByteOffset, destSlot, i.FieldSize, paramOffsets, localOffsets)
			}
		}

	case *bigir.InsertField:
		if destSlot, ok := localOffsets[i.GetID()]; ok {
			structSize := (i.Type().Size + 1) & ^1
			// If base struct is in another slot, copy it to destSlot first
			if srcInstr, ok := i.Struct.(bigir.Instruction); ok {
				if srcSlot, ok := localOffsets[srcInstr.GetID()]; ok && srcSlot != destSlot {
					for off := 0; off < structSize; off += 2 {
						buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n", srcSlot-off))
						buf.WriteString(fmt.Sprintf("    std   -%d,u\n", destSlot-off))
					}
				}
			} else if p, ok := i.Struct.(*bigir.Parameter); ok {
				if paramOff, ok := paramOffsets[p.ID]; ok {
					for off := 0; off < structSize; off += 2 {
						buf.WriteString(fmt.Sprintf("    ldd   %d,u\n", paramOff+off))
						buf.WriteString(fmt.Sprintf("    std   -%d,u\n", destSlot-off))
					}
				}
			}
			fieldDestOffset := destSlot - i.ByteOffset
			if i.FieldSize == 1 {
				b.loadValToD(buf, i.Val, paramOffsets, localOffsets, stringDescs)
				buf.WriteString(fmt.Sprintf("    stb   -%d,u             ; insert field %d (1B)\n", fieldDestOffset, i.FieldIndex))
			} else if i.FieldSize == 2 {
				b.loadValToD(buf, i.Val, paramOffsets, localOffsets, stringDescs)
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; insert field %d (2B)\n", fieldDestOffset, i.FieldIndex))
			} else if i.FieldSize == 8 {
				b.storeSliceToOffset(buf, i.Val, -fieldDestOffset, paramOffsets, localOffsets, stringDescs)
			} else {
				if valInstr, ok := i.Val.(bigir.Instruction); ok {
					if valSlot, ok := localOffsets[valInstr.GetID()]; ok {
						for off := 0; off < i.FieldSize; off += 2 {
							if off+2 <= i.FieldSize {
								buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n", valSlot-off))
								buf.WriteString(fmt.Sprintf("    std   -%d,u\n", fieldDestOffset-off))
							} else {
								buf.WriteString(fmt.Sprintf("    ldb   -%d,u\n", valSlot-off))
								buf.WriteString(fmt.Sprintf("    stb   -%d,u\n", fieldDestOffset-off))
							}
						}
					}
				} else if p, ok := i.Val.(*bigir.Parameter); ok {
					if pOff, ok := paramOffsets[p.ID]; ok {
						for off := 0; off < i.FieldSize; off += 2 {
							if off+2 <= i.FieldSize {
								buf.WriteString(fmt.Sprintf("    ldd   %d,u\n", pOff+off))
								buf.WriteString(fmt.Sprintf("    std   -%d,u\n", fieldDestOffset-off))
							} else {
								buf.WriteString(fmt.Sprintf("    ldb   %d,u\n", pOff+off))
								buf.WriteString(fmt.Sprintf("    stb   -%d,u\n", fieldDestOffset-off))
							}
						}
					}
				}
			}
		}

	case *bigir.Compare:
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			buf.WriteString(fmt.Sprintf("    cmpd  #%d\n", r.Val))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    cmpd  #%d\n", r.Val))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			buf.WriteString("    ldd   2,s\n")
			buf.WriteString("    cmpd  ,s\n")
			buf.WriteString("    leas  4,s\n")
		}
		mName := MangleName(fn.Name)
		lblTrue := fmt.Sprintf(".L_%s_cmp_true_%d", mName, i.GetID())
		lblEnd := fmt.Sprintf(".L_%s_cmp_end_%d", mName, i.GetID())
		isInt := i.Left.Type().Kind == bigir.KindInt
		switch i.Op {
		case "eq":
			buf.WriteString(fmt.Sprintf("    beq   %s\n", lblTrue))
		case "neq":
			buf.WriteString(fmt.Sprintf("    bne   %s\n", lblTrue))
		case "lt":
			if isInt {
				buf.WriteString(fmt.Sprintf("    blt   %s\n", lblTrue))
			} else {
				buf.WriteString(fmt.Sprintf("    blo   %s\n", lblTrue))
			}
		case "lte":
			if isInt {
				buf.WriteString(fmt.Sprintf("    ble   %s\n", lblTrue))
			} else {
				buf.WriteString(fmt.Sprintf("    bls   %s\n", lblTrue))
			}
		case "gt":
			if isInt {
				buf.WriteString(fmt.Sprintf("    bgt   %s\n", lblTrue))
			} else {
				buf.WriteString(fmt.Sprintf("    bhi   %s\n", lblTrue))
			}
		case "gte":
			if isInt {
				buf.WriteString(fmt.Sprintf("    bge   %s\n", lblTrue))
			} else {
				buf.WriteString(fmt.Sprintf("    bhs   %s\n", lblTrue))
			}
		default:
			buf.WriteString(fmt.Sprintf("    beq   %s\n", lblTrue))
		}
		buf.WriteString("    clra\n    clrb\n")
		buf.WriteString(fmt.Sprintf("    bra   %s\n", lblEnd))
		buf.WriteString(fmt.Sprintf("%s:\n    ldd   #1\n%s:\n", lblTrue, lblEnd))
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.NearLoad:
		b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    tfr   d,x\n")
		isByteLoad := i.Type().Size == 1 || (i.Addr.Type().ElementType != nil && i.Addr.Type().ElementType.Size == 1)
		if isByteLoad {
			buf.WriteString("    clra\n    ldb   ,x\n")
			if slot, ok := localOffsets[i.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
			}
		} else if i.Type().Size == 8 && (i.Type().Kind == bigir.KindFarSlice || i.Type().Kind == bigir.KindFarString) {
			if slot, ok := localOffsets[i.GetID()]; ok {
				buf.WriteString("    ldd   0,x\n")
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; far_ref\n", slot))
				buf.WriteString("    ldd   2,x\n")
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; offset\n", slot-2))
				buf.WriteString("    ldd   4,x\n")
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; length\n", slot-4))
				buf.WriteString("    ldd   6,x\n")
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; capacity\n", slot-6))
			}
		} else if i.Type().Size > 2 {
			if slot, ok := localOffsets[i.GetID()]; ok {
				sz := i.Type().Size
				if sz <= 32 {
					for off := 0; off < sz; off += 2 {
						if off+2 <= sz {
							buf.WriteString(fmt.Sprintf("    ldd   %d,x\n    std   -%d,u\n", off, slot-off))
						} else {
							buf.WriteString(fmt.Sprintf("    ldb   %d,x\n    stb   -%d,u\n", off, slot-off))
						}
					}
				} else {
					wordCount := sz / 2
					buf.WriteString(fmt.Sprintf("    leay  -%d,u\n", slot))
					if wordCount > 0 {
						if wordCount <= 16 {
							for off := 0; off < wordCount*2; off += 2 {
								buf.WriteString(fmt.Sprintf("    ldd   %d,x\n    std   %d,y\n", off, off))
							}
						} else {
							lbl := fmt.Sprintf(".L_%s_loadcpy_%d", mName, instr.GetID())
							buf.WriteString("    pshs  u\n")
							buf.WriteString(fmt.Sprintf("    ldu   #%d\n", wordCount))
							buf.WriteString(fmt.Sprintf("%s:\n", lbl))
							buf.WriteString("    ldd   ,x++\n")
							buf.WriteString("    std   ,y++\n")
							buf.WriteString("    leau  -1,u\n")
							buf.WriteString("    cmpu  #0\n")
							buf.WriteString(fmt.Sprintf("    bne   %s\n", lbl))
							buf.WriteString("    puls  u\n")
						}
					}
					if sz%2 != 0 {
						buf.WriteString(fmt.Sprintf("    ldb   %d,x\n    stb   %d,y\n", wordCount*2, wordCount*2))
					}
				}
			}
		} else {
			buf.WriteString("    ldd   ,x\n")
			if slot, ok := localOffsets[i.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
			}
		}

	case *bigir.NearStore:
		if aol, ok := i.Addr.(*bigir.AddressOfLocal); ok {
			if tracker != nil {
				tracker.InvalidateKey(valKey(aol))
			}
		}
		isByteStore := i.Val.Type().Size == 1 || (i.Addr.Type().ElementType != nil && i.Addr.Type().ElementType.Size == 1)
		if isByteStore {
			b.loadValToD(buf, i.Val, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  b\n")
			b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    puls  b\n")
			buf.WriteString("    stb   ,x\n")
		} else if i.Val.Type().Size == 8 && (i.Val.Type().Kind == bigir.KindFarSlice || i.Val.Type().Kind == bigir.KindFarString) {
			b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			b.storeSliceToPtr(buf, i.Val, paramOffsets, localOffsets, stringDescs)
		} else if i.Val.Type().Size > 2 {
			sz := i.Val.Type().Size
			b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			if srcInstr, ok := i.Val.(bigir.Instruction); ok {
				if srcSlot, ok := localOffsets[srcInstr.GetID()]; ok {
					if sz <= 32 {
						for off := 0; off < sz; off += 2 {
							if off+2 <= sz {
								buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    std   %d,x\n", srcSlot-off, off))
							} else {
								buf.WriteString(fmt.Sprintf("    ldb   -%d,u\n    stb   %d,x\n", srcSlot-off, off))
							}
						}
					} else {
						wordCount := sz / 2
						buf.WriteString(fmt.Sprintf("    leay  -%d,u\n", srcSlot))
						if wordCount > 0 {
							lbl := fmt.Sprintf(".L_%s_storecpy_%d", mName, instr.GetID())
							buf.WriteString("    pshs  u\n")
							buf.WriteString(fmt.Sprintf("    ldu   #%d\n", wordCount))
							buf.WriteString(fmt.Sprintf("%s:\n", lbl))
							buf.WriteString("    ldd   ,y++\n")
							buf.WriteString("    std   ,x++\n")
							buf.WriteString("    leau  -1,u\n")
							buf.WriteString("    cmpu  #0\n")
							buf.WriteString(fmt.Sprintf("    bne   %s\n", lbl))
							buf.WriteString("    puls  u\n")
						}
						if sz%2 != 0 {
							buf.WriteString("    ldb   ,y\n    stb   ,x\n")
						}
					}
				}
			} else if p, ok := i.Val.(*bigir.Parameter); ok {
				if pOff, ok := paramOffsets[p.ID]; ok {
					for off := 0; off < sz; off += 2 {
						if off+2 <= sz {
							buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    std   %d,x\n", pOff+off, off))
						} else {
							buf.WriteString(fmt.Sprintf("    ldb   %d,u\n    stb   %d,x\n", pOff+off, off))
						}
					}
				}
			} else if cs, ok := i.Val.(*bigir.ConstStruct); ok {
				for off, f := range cs.Fields {
					b.loadValToD(buf, f, paramOffsets, localOffsets, stringDescs)
					if f.Type().Size == 1 {
						buf.WriteString(fmt.Sprintf("    stb   %d,x\n", off))
					} else {
						buf.WriteString(fmt.Sprintf("    std   %d,x\n", off))
					}
				}
			} else if g, ok := i.Val.(*bigir.Global); ok {
				mName := MangleName(g.Name)
				for off := 0; off < sz; off += 2 {
					if off+2 <= sz {
						buf.WriteString(fmt.Sprintf("    ldd   v_%s+%d\n    std   %d,x\n", mName, off, off))
					} else {
						buf.WriteString(fmt.Sprintf("    ldb   v_%s+%d\n    stb   %d,x\n", mName, off, off))
					}
				}
			}
		} else {
			b.loadValToD(buf, i.Val, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    puls  d\n")
			buf.WriteString("    std   ,x\n")
		}

	case *bigir.AddressOfGlobal:
		if slot, ok := localOffsets[i.GetID()]; ok {
			b.loadValToD(buf, i, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.AddressOfLocal:
		if slot, ok := localOffsets[i.GetID()]; ok {
			b.loadValToD(buf, i, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.NearCall, *bigir.FarCall:
		var callee string
		var args []bigir.Value
		var comment string
		if nc, ok := instr.(*bigir.NearCall); ok {
			callee = nc.Callee
			args = nc.Args
			comment = nc.GetComment()
		} else if fc, ok := instr.(*bigir.FarCall); ok {
			callee = fc.Callee
			args = fc.Args
			comment = fc.GetComment()
		}

		if strings.HasPrefix(callee, "builtin_") {
			if callee == "builtin_println" || callee == "builtin_print" {
				hasString := false
				for _, arg := range args {
					if arg != nil {
						typ := arg.Type()
						if typ.Kind == bigir.KindFarString ||
							strings.Contains(typ.Name, "string") ||
							strings.HasSuffix(typ.Name, "slice_byte") ||
							typ.Name == "*byte" || typ.Name == "*char" ||
							(typ.Kind == bigir.KindNearPtr && typ.ElementType != nil && typ.ElementType.Size == 1) ||
							(typ.Kind == bigir.KindFarSlice && typ.ElementType != nil && typ.ElementType.Size == 1) {
							hasString = true
							break
						}
					}
				}
				b.emitPrint(buf, callee == "builtin_println", args, paramOffsets, localOffsets, stringDescs)
				if tracker != nil && hasString {
					tracker.Invalidate(2) // Slot 2 is clobbered by builtin_print_string
				}
			} else if callee == "builtin_panic" {
				if tracker != nil {
					tracker.InvalidateAll()
				}
				if len(args) > 0 {
					b.loadSliceDescToReg(buf, args[0], "x", paramOffsets, localOffsets, stringDescs)
				} else {
					buf.WriteString("    ldx   #0\n")
				}
				buf.WriteString("    jsr   builtin_panic\n")
			} else {
				if tracker != nil {
					tracker.InvalidateAll()
				}
				buf.WriteString(fmt.Sprintf("    jsr   %s\n", callee))
			}
		} else if callee == "prelude.streq" || callee == "streq" {
			if tracker != nil {
				tracker.InvalidateAll()
			}
			if len(args) >= 2 {
				b.loadSliceDescToReg(buf, args[0], "x", paramOffsets, localOffsets, stringDescs)
				b.loadSliceDescToReg(buf, args[1], "y", paramOffsets, localOffsets, stringDescs)
			}
			buf.WriteString("    jsr   __far_streq\n")
			if instr.Type().Size > 0 {
				if slot, ok := localOffsets[instr.GetID()]; ok {
					buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store return value v%d\n", slot, instr.GetID()))
				}
			}
		} else if callee == "prelude.MapWindow0" || callee == "MapWindow0" {
			if len(args) > 0 {
				b.loadValToD(buf, args[0], paramOffsets, localOffsets, stringDescs)
				buf.WriteString("    stb   $FF42\n")
				if tracker != nil {
					tracker.SetMapped(2, valKey(args[0]))
				}
			}
		} else if callee == "prelude.MapWindow1" || callee == "MapWindow1" {
			if len(args) > 0 {
				b.loadValToD(buf, args[0], paramOffsets, localOffsets, stringDescs)
				buf.WriteString("    stb   $FF43\n")
				if tracker != nil {
					tracker.SetMapped(3, valKey(args[0]))
				}
			}
		} else if callee == "prelude.MapWindow2" || callee == "MapWindow2" {
			if len(args) > 0 {
				b.loadValToD(buf, args[0], paramOffsets, localOffsets, stringDescs)
				buf.WriteString("    stb   $FF44\n")
				if tracker != nil {
					tracker.SetMapped(4, valKey(args[0]))
				}
			}
		} else if strings.Contains(callee, "slice_") && (strings.HasSuffix(callee, "_Get") || strings.HasSuffix(callee, "_Get1")) {
			if len(args) >= 2 {
				slotNum := 2
				if strings.HasSuffix(callee, "_Get1") {
					slotNum = 3
				}
				b.emitSliceGet(buf, fn, instr, args[0], args[1], slotNum, paramOffsets, localOffsets, stringDescs, tracker)
				return
			}
		} else if strings.Contains(callee, "slice_") && (strings.HasSuffix(callee, "_Put") || strings.HasSuffix(callee, "_Put1")) {
			if len(args) >= 3 {
				slotNum := 3
				if strings.HasSuffix(callee, "_Put1") {
					slotNum = 2
				}
				b.emitSlicePut(buf, fn, instr, args[0], args[1], args[2], slotNum, paramOffsets, localOffsets, stringDescs, tracker)
				return
			}
		} else {
			if tracker != nil {
				tracker.InvalidateAll()
			}
			retSize := instr.Type().Size

			// Push arguments in reverse order (right to left)
			totalArgBytes := 0
			for idx := len(args) - 1; idx >= 0; idx-- {
				arg := args[idx]
				totalArgBytes += b.pushArg(buf, arg, paramOffsets, localOffsets, stringDescs)
			}

			// If retSize > 2, push invisible return pointer last (at 4,u in callee)
			if retSize > 2 {
				slot := localOffsets[instr.GetID()]
				buf.WriteString(fmt.Sprintf("    leax  -%d,u             ; pass invisible return pointer\n", slot))
				buf.WriteString("    pshs  x\n")
				totalArgBytes += 2
			}

			if comment == "devirtualized intra-block call" {
				mCallee := MangleName(callee)
				buf.WriteString(fmt.Sprintf("    jsr   $A000+fn_%s-_block_%d_data    ; direct intra-block call in Slot 5\n", mCallee, fn.BlockID))
			} else {
				buf.WriteString(fmt.Sprintf("    jsr   f_%s\n", MangleName(callee)))
			}

			if totalArgBytes > 0 {
				buf.WriteString(fmt.Sprintf("    leas  %d,s             ; clean up call arguments\n", totalArgBytes))
			}

			if retSize <= 2 && retSize > 0 {
				if slot, ok := localOffsets[instr.GetID()]; ok {
					buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store return value v%d\n", slot, instr.GetID()))
				}
			}
		}

	case *bigir.FuncRef:
		if slot, ok := localOffsets[i.GetID()]; ok {
			b.loadValToD(buf, i, paramOffsets, localOffsets, stringDescs)
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.IndirectCall:
		if tracker != nil {
			tracker.InvalidateAll()
		}
		retSize := instr.Type().Size

		// Push arguments in reverse order (right to left)
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			totalArgBytes += b.pushArg(buf, arg, paramOffsets, localOffsets, stringDescs)
		}

		if retSize > 2 {
			slot := localOffsets[instr.GetID()]
			buf.WriteString(fmt.Sprintf("    leax  -%d,u             ; pass invisible return pointer\n", slot))
			buf.WriteString("    pshs  x\n")
			totalArgBytes += 2
		}

		// Load function pointer and call indirectly
		b.loadValToD(buf, i.FuncPtr, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    tfr   d,x\n")
		buf.WriteString("    jsr   ,x\n")

		if totalArgBytes > 0 {
			buf.WriteString(fmt.Sprintf("    leas  %d,s             ; clean up call arguments\n", totalArgBytes))
		}

		if retSize <= 2 && retSize > 0 {
			if slot, ok := localOffsets[instr.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store return value v%d\n", slot, instr.GetID()))
			}
		}
	}
}

func (b *Backend) emitBinaryOp(
	buf *bytes.Buffer,
	i *bigir.BinaryOp,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	switch i.Op {
	case "add", "+":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			buf.WriteString(fmt.Sprintf("    addd  #%d\n", r.Val))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    addd  #%d\n", r.Val))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    addd  ,s++\n")
		}
	case "sub", "-":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			buf.WriteString(fmt.Sprintf("    subd  #%d\n", r.Val))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    subd  #%d\n", r.Val))
		default:
			buf.WriteString("    pshs  d\n") // push Left
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n") // push Right
			buf.WriteString("    ldd   2,s\n") // D = Left
			buf.WriteString("    subd  ,s\n")  // D = Left - Right
			buf.WriteString("    leas  4,s\n")
		}
	case "mul", "*":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    tfr   d,x\n")
		buf.WriteString("    puls  d\n")
		buf.WriteString("    jsr   __mul16\n")
	case "div", "/":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    puls  x\n")
		buf.WriteString("    jsr   __div16\n")
	case "mod", "%":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    puls  x\n")
		buf.WriteString("    jsr   __mod16\n")
	case "and", "&":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			buf.WriteString(fmt.Sprintf("    anda  #%d\n    andb  #%d\n", (r.Val>>8)&0xFF, r.Val&0xFF))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    anda  #0\n    andb  #%d\n", r.Val&0xFF))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    anda  0,s\n    andb  1,s\n    leas  2,s\n")
		}
	case "or", "|":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			buf.WriteString(fmt.Sprintf("    ora   #%d\n    orb   #%d\n", (r.Val>>8)&0xFF, r.Val&0xFF))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    orb   #%d\n", r.Val&0xFF))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    ora   0,s\n    orb   1,s\n    leas  2,s\n")
		}
	case "xor", "^":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			buf.WriteString(fmt.Sprintf("    eora  #%d\n    eorb  #%d\n", (r.Val>>8)&0xFF, r.Val&0xFF))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    eorb  #%d\n", r.Val&0xFF))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    eora  0,s\n    eorb  1,s\n    leas  2,s\n")
		}
	case "andnot", "&^":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			inv := ^r.Val
			buf.WriteString(fmt.Sprintf("    anda  #%d\n    andb  #%d\n", (inv>>8)&0xFF, inv&0xFF))
		case *bigir.ConstByte:
			inv := ^uint16(r.Val)
			buf.WriteString(fmt.Sprintf("    anda  #%d\n    andb  #%d\n", (inv>>8)&0xFF, inv&0xFF))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    coma\n    comb\n")
			buf.WriteString("    anda  0,s\n    andb  1,s\n    leas  2,s\n")
		}
	case "shl", "<<":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			for k := uint64(0); k < r.Val; k++ {
				buf.WriteString("    aslb\n    rola\n")
			}
		case *bigir.ConstByte:
			for k := uint8(0); k < r.Val; k++ {
				buf.WriteString("    aslb\n    rola\n")
			}
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    puls  d\n")
			buf.WriteString("    jsr   __shl16\n")
		}
	case "shr", ">>":
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch r := i.Right.(type) {
		case *bigir.ConstWord:
			for k := uint64(0); k < r.Val; k++ {
				buf.WriteString("    lsra\n    rorb\n")
			}
		case *bigir.ConstByte:
			for k := uint8(0); k < r.Val; k++ {
				buf.WriteString("    lsra\n    rorb\n")
			}
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    puls  d\n")
			buf.WriteString("    jsr   __shr16\n")
		}
	}
	if i.Type().Size == 1 {
		buf.WriteString("    clra\n")
	}
}

func (b *Backend) loadValToD(
	buf *bytes.Buffer,
	val bigir.Value,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	if val == nil {
		buf.WriteString("    clra\n    clrb\n")
		return
	}
	switch v := val.(type) {
	case *bigir.ConstByte:
		buf.WriteString(fmt.Sprintf("    ldd   #%d\n", v.Val))
	case *bigir.ConstWord:
		buf.WriteString(fmt.Sprintf("    ldd   #%d\n", v.Val))
	case *bigir.Parameter:
		if off, ok := paramOffsets[v.ID]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   %d,u               ; param %s\n", off, v.Name))
			if v.Type().Size == 1 {
				buf.WriteString("    clra\n")
			}
		} else {
			buf.WriteString("    clra\n    clrb\n")
		}
	case *bigir.Global:
		if v.InitString != "" {
			if desc, ok := stringDescs[v.InitString]; ok {
				dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
				buf.WriteString(fmt.Sprintf("    ldd   #%s\n", dataLbl))
				return
			}
		}
		buf.WriteString(fmt.Sprintf("    ldd   v_%s\n", MangleName(v.Name)))
	case *bigir.AddressOfGlobal:
		if v.Global != nil {
			if v.Global.InitString != "" {
				if desc, ok := stringDescs[v.Global.InitString]; ok {
					dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
					buf.WriteString(fmt.Sprintf("    ldd   #%s\n", dataLbl))
					return
				}
			}
			buf.WriteString(fmt.Sprintf("    ldd   #v_%s\n", MangleName(v.Global.Name)))
		}
	case *bigir.AddressOfLocal:
		loc := resolveRootLocal(v.Local)
		if param, ok := loc.(*bigir.Parameter); ok {
			if off, ok := paramOffsets[param.ID]; ok {
				if param.Type().Size == 1 {
					buf.WriteString(fmt.Sprintf("    leax  %d,u\n    tfr   x,d\n", off+1))
				} else {
					buf.WriteString(fmt.Sprintf("    leax  %d,u\n    tfr   x,d\n", off))
				}
				return
			}
		}
		if instr, ok := loc.(bigir.Instruction); ok {
			if off, ok := localOffsets[instr.GetID()]; ok {
				if instr.Type().Size == 1 {
					buf.WriteString(fmt.Sprintf("    leax  -%d,u\n    tfr   x,d\n", off-1))
				} else {
					buf.WriteString(fmt.Sprintf("    leax  -%d,u\n    tfr   x,d\n", off))
				}
				return
			}
		}
		buf.WriteString("    clra\n    clrb\n")
	case *bigir.BitCast:
		if v.GetID() > 0 {
			if off, ok := localOffsets[v.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
				if v.Type().Size == 1 {
					buf.WriteString("    clra\n")
				}
				return
			}
		}
		b.loadValToD(buf, v.Operand, paramOffsets, localOffsets, stringDescs)
		if v.Type().Size == 1 {
			buf.WriteString("    clra\n")
		}
	case *bigir.FuncRef:
		buf.WriteString(fmt.Sprintf("    ldd   #f_%s\n", MangleName(v.FuncName)))
		return
	case *bigir.SliceField:
		if off, ok := localOffsets[v.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
			return
		}
		b.loadSliceFieldToD(buf, v, paramOffsets, localOffsets, stringDescs)
	case *bigir.SliceToPtr:
		if off, ok := localOffsets[v.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
			return
		}
		b.loadSliceDescToReg(buf, v.Slice, "x", paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    jsr   __slice_to_ptr\n")
		return
	case *bigir.ExtractField:
		if off, ok := localOffsets[v.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
			return
		}
		if v.FieldSize <= 2 {
			b.loadExtractFieldToD(buf, v, paramOffsets, localOffsets, stringDescs)
			return
		}
		buf.WriteString("    clra\n    clrb\n")
	case *bigir.BinaryOp:
		if v.GetID() > 0 {
			if off, ok := localOffsets[v.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
				return
			}
		}
		b.emitBinaryOp(buf, v, paramOffsets, localOffsets, stringDescs)
	case bigir.Instruction:
		if off, ok := localOffsets[v.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
			if v.Type().Size == 1 {
				buf.WriteString("    clra\n")
			}
		} else {
			buf.WriteString("    clra\n    clrb\n")
		}
	default:
		buf.WriteString("    clra\n    clrb\n")
	}
}

func (b *Backend) loadSliceFieldToD(
	buf *bytes.Buffer,
	sf *bigir.SliceField,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	fieldByteOffset := sf.FieldIdx * 2
	switch s := sf.Slice.(type) {
	case *bigir.Parameter:
		if off, ok := paramOffsets[s.ID]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   %d,u               ; param %s field %d\n", off+fieldByteOffset, s.Name, sf.FieldIdx))
			return
		}
	case bigir.Instruction:
		if off, ok := localOffsets[s.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d field %d\n", off-fieldByteOffset, s.GetID(), sf.FieldIdx))
			return
		}
	case *bigir.Global:
		if s.InitString != "" {
			if desc, ok := stringDescs[s.InitString]; ok {
				buf.WriteString(fmt.Sprintf("    ldd   %s+%d\n", desc, fieldByteOffset))
				return
			}
		}
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+%d\n", MangleName(s.Name), fieldByteOffset))
		return
	case *bigir.AddressOfGlobal:
		if s.Global != nil {
			if s.Global.InitString != "" {
				if desc, ok := stringDescs[s.Global.InitString]; ok {
					buf.WriteString(fmt.Sprintf("    ldd   %s+%d\n", desc, fieldByteOffset))
					return
				}
			}
			buf.WriteString(fmt.Sprintf("    ldd   v_%s+%d\n", MangleName(s.Global.Name), fieldByteOffset))
			return
		}
	}
	buf.WriteString("    clra\n    clrb\n")
}

func (b *Backend) loadExtractFieldToD(
	buf *bytes.Buffer,
	ef *bigir.ExtractField,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	if sInstr, ok := ef.Struct.(bigir.Instruction); ok {
		if slot, ok := localOffsets[sInstr.GetID()]; ok {
			fieldOffset := slot - ef.ByteOffset
			if ef.FieldSize == 1 {
				buf.WriteString(fmt.Sprintf("    clra\n    ldb   -%d,u              ; v%d field %d\n", fieldOffset, sInstr.GetID(), ef.FieldIndex))
			} else {
				buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d field %d\n", fieldOffset, sInstr.GetID(), ef.FieldIndex))
			}
			return
		}
	} else if p, ok := ef.Struct.(*bigir.Parameter); ok {
		if off, ok := paramOffsets[p.ID]; ok {
			fieldOffset := off + ef.ByteOffset
			if ef.FieldSize == 1 {
				buf.WriteString(fmt.Sprintf("    clra\n    ldb   %d,u               ; param %s field %d\n", fieldOffset, p.Name, ef.FieldIndex))
			} else {
				buf.WriteString(fmt.Sprintf("    ldd   %d,u               ; param %s field %d\n", fieldOffset, p.Name, ef.FieldIndex))
			}
			return
		}
	} else if g, ok := ef.Struct.(*bigir.Global); ok {
		mName := MangleName(g.Name)
		if ef.FieldSize == 1 {
			buf.WriteString(fmt.Sprintf("    clra\n    ldb   v_%s+%d\n", mName, ef.ByteOffset))
		} else {
			buf.WriteString(fmt.Sprintf("    ldd   v_%s+%d\n", mName, ef.ByteOffset))
		}
		return
	}
	buf.WriteString("    clra\n    clrb\n")
}

func (b *Backend) copyFieldBytes(
	buf *bytes.Buffer,
	strct bigir.Value,
	byteOffset int,
	destSlot int,
	size int,
	paramOffsets, localOffsets map[int]int,
) {
	if sInstr, ok := strct.(bigir.Instruction); ok {
		if slot, ok := localOffsets[sInstr.GetID()]; ok {
			srcBase := slot - byteOffset
			for off := 0; off < size; off += 2 {
				if off+2 <= size {
					buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n", srcBase-off))
					buf.WriteString(fmt.Sprintf("    std   -%d,u\n", destSlot-off))
				} else {
					buf.WriteString(fmt.Sprintf("    ldb   -%d,u\n", srcBase-off))
					buf.WriteString(fmt.Sprintf("    stb   -%d,u\n", destSlot-off))
				}
			}
			return
		}
	} else if p, ok := strct.(*bigir.Parameter); ok {
		if pOff, ok := paramOffsets[p.ID]; ok {
			srcBase := pOff + byteOffset
			for off := 0; off < size; off += 2 {
				if off+2 <= size {
					buf.WriteString(fmt.Sprintf("    ldd   %d,u\n", srcBase+off))
					buf.WriteString(fmt.Sprintf("    std   -%d,u\n", destSlot-off))
				} else {
					buf.WriteString(fmt.Sprintf("    ldb   %d,u\n", srcBase+off))
					buf.WriteString(fmt.Sprintf("    stb   -%d,u\n", destSlot-off))
				}
			}
			return
		}
	}
}

func (b *Backend) pushSliceArg(
	buf *bytes.Buffer,
	arg bigir.Value,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	if instr, ok := arg.(bigir.Instruction); ok {
		if slot, ok := localOffsets[instr.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-6))
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-4))
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-2))
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot))
			return
		}
	}
	if sm, ok := arg.(*bigir.SliceMake); ok {
		b.loadValToD(buf, sm.Capacity, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, sm.Length, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, sm.Offset, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, sm.FarRef, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		return
	}
	if cs, ok := arg.(*bigir.ConstStruct); ok {
		for idx := len(cs.Fields) - 1; idx >= 0; idx-- {
			b.loadValToD(buf, cs.Fields[idx], paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
		}
		for i := len(cs.Fields); i < 4; i++ {
			buf.WriteString("    clra\n    clrb\n    pshs  d\n")
		}
		return
	}
	if p, ok := arg.(*bigir.Parameter); ok {
		if off, ok := paramOffsets[p.ID]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    pshs  d\n", off+6))
			buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    pshs  d\n", off+4))
			buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    pshs  d\n", off+2))
			buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    pshs  d\n", off))
			return
		}
	}
	if g, ok := arg.(*bigir.Global); ok {
		if g.InitString != "" {
			if desc, ok := stringDescs[g.InitString]; ok {
				dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
				strLen := len(strings.TrimSuffix(g.InitString, "\x00"))
				buf.WriteString(fmt.Sprintf("    ldd   #%d\n    pshs  d\n", strLen))
				buf.WriteString(fmt.Sprintf("    ldd   #%d\n    pshs  d\n", strLen))
				buf.WriteString(fmt.Sprintf("    ldd   #%s\n    pshs  d\n", dataLbl))
				buf.WriteString("    clra\n    clrb\n    pshs  d\n")
				return
			}
		}
		mName := MangleName(g.Name)
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+6\n    pshs  d\n", mName))
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+4\n    pshs  d\n", mName))
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+2\n    pshs  d\n", mName))
		buf.WriteString(fmt.Sprintf("    ldd   v_%s\n    pshs  d\n", mName))
		return
	}
	if ag, ok := arg.(*bigir.AddressOfGlobal); ok && ag.Global != nil {
		if ag.Global.InitString != "" {
			if desc, ok := stringDescs[ag.Global.InitString]; ok {
				dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
				strLen := len(strings.TrimSuffix(ag.Global.InitString, "\x00"))
				buf.WriteString(fmt.Sprintf("    ldd   #%d\n    pshs  d\n", strLen))
				buf.WriteString(fmt.Sprintf("    ldd   #%d\n    pshs  d\n", strLen))
				buf.WriteString(fmt.Sprintf("    ldd   #%s\n    pshs  d\n", dataLbl))
				buf.WriteString("    clra\n    clrb\n    pshs  d\n")
				return
			}
		}
		mName := MangleName(ag.Global.Name)
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+6\n    pshs  d\n", mName))
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+4\n    pshs  d\n", mName))
		buf.WriteString(fmt.Sprintf("    ldd   v_%s+2\n    pshs  d\n", mName))
		buf.WriteString(fmt.Sprintf("    ldd   v_%s\n    pshs  d\n", mName))
		return
	}
	buf.WriteString("    clra\n    clrb\n    pshs  d\n    pshs  d\n    pshs  d\n    pshs  d\n")
}

func (b *Backend) pushArg(
	buf *bytes.Buffer,
	arg bigir.Value,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) int {
	sz := arg.Type().Size
	if sz <= 2 {
		b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		return 2
	}
	if sz == 8 && (arg.Type().Kind == bigir.KindFarSlice || arg.Type().Kind == bigir.KindFarString) {
		b.pushSliceArg(buf, arg, paramOffsets, localOffsets, stringDescs)
		return 8
	}
	aligned := (sz + 1) & ^1
	if instr, ok := arg.(bigir.Instruction); ok {
		if srcSlot, ok := localOffsets[instr.GetID()]; ok {
			for off := aligned - 2; off >= 0; off -= 2 {
				buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", srcSlot-off))
			}
			return aligned
		}
	} else if p, ok := arg.(*bigir.Parameter); ok {
		if pOff, ok := paramOffsets[p.ID]; ok {
			for off := aligned - 2; off >= 0; off -= 2 {
				buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    pshs  d\n", pOff+off))
			}
			return aligned
		}
	} else if cs, ok := arg.(*bigir.ConstStruct); ok {
		for idx := len(cs.Fields) - 1; idx >= 0; idx-- {
			b.loadValToD(buf, cs.Fields[idx], paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
		}
		return aligned
	}
	b.pushSliceArg(buf, arg, paramOffsets, localOffsets, stringDescs)
	return 8
}

func (b *Backend) storeSliceToOffset(
	buf *bytes.Buffer,
	val bigir.Value,
	destOffset int,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	b.pushSliceArg(buf, val, paramOffsets, localOffsets, stringDescs)
	buf.WriteString("    puls  d\n")
	buf.WriteString(fmt.Sprintf("    std   %d,u\n", destOffset))
	buf.WriteString("    puls  d\n")
	buf.WriteString(fmt.Sprintf("    std   %d,u\n", destOffset+2))
	buf.WriteString("    puls  d\n")
	buf.WriteString(fmt.Sprintf("    std   %d,u\n", destOffset+4))
	buf.WriteString("    puls  d\n")
	buf.WriteString(fmt.Sprintf("    std   %d,u\n", destOffset+6))
}

func (b *Backend) storeSliceToPtr(
	buf *bytes.Buffer,
	val bigir.Value,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	buf.WriteString("    pshs  x\n")
	b.pushSliceArg(buf, val, paramOffsets, localOffsets, stringDescs)
	buf.WriteString("    ldx   8,s\n")
	buf.WriteString("    puls  d\n    std   0,x\n")
	buf.WriteString("    puls  d\n    std   2,x\n")
	buf.WriteString("    puls  d\n    std   4,x\n")
	buf.WriteString("    puls  d\n    std   6,x\n")
	buf.WriteString("    leas  2,s\n")
}

func (b *Backend) emitSliceGet(
	buf *bytes.Buffer,
	fn *bigir.Function,
	instr bigir.Instruction,
	sliceArg, indexArg bigir.Value,
	slotNum int,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
	tracker *WindowSlotTracker,
) {
	id := instr.GetID()
	elemSize := instr.Type().Size
	if elemSize <= 0 {
		elemSize = 2
	}
	mmapReg := "$FF42"
	winBase := "$4000"
	if slotNum == 3 {
		mmapReg = "$FF43"
		winBase = "$6000"
	} else if slotNum == 4 {
		mmapReg = "$FF44"
		winBase = "$8000"
	}
	mName := MangleName(fn.Name)
	lblInbounds := fmt.Sprintf(".L_%s_sget_inbounds_%d", mName, id)
	lblNear := fmt.Sprintf(".L_%s_sget_near_%d", mName, id)
	lblCalc := fmt.Sprintf(".L_%s_sget_calc_%d", mName, id)

	// 1. Evaluate index and push to stack
	b.loadValToD(buf, indexArg, paramOffsets, localOffsets, stringDescs)
	buf.WriteString("    pshs  d             ; push index\n")

	// 2. Load slice descriptor address into X
	if sliceArg.Type().Kind == bigir.KindNearPtr || sliceArg.Type().Size == 2 {
		b.loadValToD(buf, sliceArg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    tfr   d,x           ; X = slice pointer\n")
	} else {
		b.loadSliceDescToReg(buf, sliceArg, "x", paramOffsets, localOffsets, stringDescs)
	}

	// 3. Bounds check: index on stack against length at 4,x
	buf.WriteString("    ldd   ,s            ; D = index\n")
	buf.WriteString("    cmpd  4,x           ; compare with slice.length\n")
	buf.WriteString(fmt.Sprintf("    blo   %s\n", lblInbounds))
	buf.WriteString("    ldx   #__str_panic_2002\n")
	buf.WriteString("    jsr   builtin_panic\n")
	buf.WriteString(fmt.Sprintf("%s:\n", lblInbounds))

	// 4. Check FarRef: Slot
	sliceKey := valKey(sliceArg)
	alreadyMapped := tracker != nil && sliceKey != "" && tracker.IsMapped(slotNum, sliceKey)

	buf.WriteString("    ldd   ,x            ; D = far_ref\n")
	buf.WriteString(fmt.Sprintf("    beq   %s\n", lblNear))
	if !alreadyMapped {
		buf.WriteString(fmt.Sprintf("    stb   %s         ; map block into Slot\n", mmapReg))
		if tracker != nil && sliceKey != "" {
			tracker.SetMapped(slotNum, sliceKey)
		}
	}
	buf.WriteString("    ldd   2,x           ; D = slice.offset\n")
	buf.WriteString(fmt.Sprintf("    addd  #%s        ; add Slot base\n", winBase))
	buf.WriteString(fmt.Sprintf("    bra   %s\n", lblCalc))
	buf.WriteString(fmt.Sprintf("%s:\n", lblNear))
	buf.WriteString("    ldd   2,x           ; D = slice.offset (near)\n")
	buf.WriteString(fmt.Sprintf("%s:\n", lblCalc))
	buf.WriteString("    tfr   d,y           ; Y = element base address\n")
	buf.WriteString("    puls  d             ; D = index\n")

	// 5. Compute element address and load
	if elemSize == 1 {
		buf.WriteString("    leax  d,y           ; X = base + index\n")
		buf.WriteString("    clra\n    ldb   ,x            ; D = byte\n")
		if slot, ok := localOffsets[id]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}
	} else if elemSize == 2 {
		buf.WriteString("    aslb\n    rola          ; D = index * 2\n")
		buf.WriteString("    leax  d,y           ; X = base + index * 2\n")
		buf.WriteString("    ldd   ,x            ; D = word\n")
		if slot, ok := localOffsets[id]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}
	} else if elemSize == 8 {
		buf.WriteString("    aslb\n    rola\n    aslb\n    rola\n    aslb\n    rola          ; D = index * 8\n")
		buf.WriteString("    leax  d,y           ; X = base + index * 8\n")
		if slot, ok := localOffsets[id]; ok {
			buf.WriteString("    ldd   0,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; far_ref\n", slot))
			buf.WriteString("    ldd   2,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; offset\n", slot-2))
			buf.WriteString("    ldd   4,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; length\n", slot-4))
			buf.WriteString("    ldd   6,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; capacity\n", slot-6))
		}
	} else {
		buf.WriteString(fmt.Sprintf("    ldx   #%d\n", elemSize))
		buf.WriteString("    jsr   __mul16\n")
		buf.WriteString("    leax  d,y           ; X = base + index * size\n")
		if slot, ok := localOffsets[id]; ok {
			for off := 0; off < elemSize; off += 2 {
				if off+2 <= elemSize {
					buf.WriteString(fmt.Sprintf("    ldd   %d,x\n    std   -%d,u\n", off, slot-off))
				} else {
					buf.WriteString(fmt.Sprintf("    ldb   %d,x\n    stb   -%d,u\n", off, slot-off))
				}
			}
		}
	}
}

func (b *Backend) emitSlicePut(
	buf *bytes.Buffer,
	fn *bigir.Function,
	instr bigir.Instruction,
	sliceArg, indexArg, valArg bigir.Value,
	slotNum int,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
	tracker *WindowSlotTracker,
) {
	id := instr.GetID()
	elemSize := valArg.Type().Size
	if elemSize <= 0 {
		elemSize = 2
	}
	mmapReg := "$FF43"
	winBase := "$6000"
	if slotNum == 2 {
		mmapReg = "$FF42"
		winBase = "$4000"
	} else if slotNum == 4 {
		mmapReg = "$FF44"
		winBase = "$8000"
	}
	mName := MangleName(fn.Name)
	lblInbounds := fmt.Sprintf(".L_%s_sput_inbounds_%d", mName, id)
	lblNear := fmt.Sprintf(".L_%s_sput_near_%d", mName, id)
	lblCalc := fmt.Sprintf(".L_%s_sput_calc_%d", mName, id)

	// 1. Push value to store
	if elemSize == 1 {
		b.loadValToD(buf, valArg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  b             ; push val byte\n")
	} else if elemSize == 2 {
		b.loadValToD(buf, valArg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d             ; push val word\n")
	} else if elemSize == 8 {
		b.pushSliceArg(buf, valArg, paramOffsets, localOffsets, stringDescs)
	} else {
		if instrVal, ok := valArg.(bigir.Instruction); ok {
			if slot, ok := localOffsets[instrVal.GetID()]; ok {
				aligned := (elemSize + 1) & ^1
				for off := aligned - 2; off >= 0; off -= 2 {
					buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-off))
				}
			} else {
				b.pushSliceArg(buf, valArg, paramOffsets, localOffsets, stringDescs)
			}
		} else {
			b.pushSliceArg(buf, valArg, paramOffsets, localOffsets, stringDescs)
		}
	}

	// 2. Evaluate index and push
	b.loadValToD(buf, indexArg, paramOffsets, localOffsets, stringDescs)
	buf.WriteString("    pshs  d             ; push index\n")

	// 3. Load slice descriptor pointer into X
	if sliceArg.Type().Kind == bigir.KindNearPtr || sliceArg.Type().Size == 2 {
		b.loadValToD(buf, sliceArg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    tfr   d,x           ; X = slice pointer\n")
	} else {
		b.loadSliceDescToReg(buf, sliceArg, "x", paramOffsets, localOffsets, stringDescs)
	}

	// 4. Bounds check: index on stack against length at 4,x
	buf.WriteString("    ldd   ,s            ; D = index\n")
	buf.WriteString("    cmpd  4,x           ; compare with slice.length\n")
	buf.WriteString(fmt.Sprintf("    blo   %s\n", lblInbounds))
	buf.WriteString("    ldx   #__str_panic_2003\n")
	buf.WriteString("    jsr   builtin_panic\n")
	buf.WriteString(fmt.Sprintf("%s:\n", lblInbounds))

	// 5. Check FarRef: Slot
	sliceKey := valKey(sliceArg)
	alreadyMapped := tracker != nil && sliceKey != "" && tracker.IsMapped(slotNum, sliceKey)

	buf.WriteString("    ldd   ,x            ; D = far_ref\n")
	buf.WriteString(fmt.Sprintf("    beq   %s\n", lblNear))
	if !alreadyMapped {
		buf.WriteString(fmt.Sprintf("    stb   %s         ; map block into Slot\n", mmapReg))
		if tracker != nil && sliceKey != "" {
			tracker.SetMapped(slotNum, sliceKey)
		}
	}
	buf.WriteString("    ldd   2,x           ; D = slice.offset\n")
	buf.WriteString(fmt.Sprintf("    addd  #%s        ; add Slot base\n", winBase))
	buf.WriteString(fmt.Sprintf("    bra   %s\n", lblCalc))
	buf.WriteString(fmt.Sprintf("%s:\n", lblNear))
	buf.WriteString("    ldd   2,x           ; D = slice.offset (near)\n")
	buf.WriteString(fmt.Sprintf("%s:\n", lblCalc))
	buf.WriteString("    tfr   d,y           ; Y = element base address\n")
	buf.WriteString("    puls  d             ; D = index\n")

	// 6. Compute element address and store
	if elemSize == 1 {
		buf.WriteString("    leax  d,y           ; X = base + index\n")
		buf.WriteString("    puls  b             ; B = val\n")
		buf.WriteString("    stb   ,x\n")
	} else if elemSize == 2 {
		buf.WriteString("    aslb\n    rola          ; D = index * 2\n")
		buf.WriteString("    leax  d,y           ; X = base + index * 2\n")
		buf.WriteString("    puls  d             ; D = val\n")
		buf.WriteString("    std   ,x\n")
	} else if elemSize == 8 {
		buf.WriteString("    aslb\n    rola\n    aslb\n    rola\n    aslb\n    rola          ; D = index * 8\n")
		buf.WriteString("    leax  d,y           ; X = base + index * 8\n")
		buf.WriteString("    puls  d\n    std   0,x           ; far_ref\n")
		buf.WriteString("    puls  d\n    std   2,x           ; offset\n")
		buf.WriteString("    puls  d\n    std   4,x           ; length\n")
		buf.WriteString("    puls  d\n    std   6,x           ; capacity\n")
	} else {
		buf.WriteString(fmt.Sprintf("    ldx   #%d\n", elemSize))
		buf.WriteString("    jsr   __mul16\n")
		buf.WriteString("    leax  d,y           ; X = base + index * size\n")
		aligned := (elemSize + 1) & ^1
		for off := 0; off < aligned; off += 2 {
			buf.WriteString(fmt.Sprintf("    puls  d\n    std   %d,x\n", off))
		}
	}
}

func (b *Backend) emitFarLoad(
	buf *bytes.Buffer,
	fn *bigir.Function,
	instr bigir.Instruction,
	farRefVal, offsetVal bigir.Value,
	slotNum int,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
	tracker *WindowSlotTracker,
) {
	id := instr.GetID()
	elemSize := instr.Type().Size
	if elemSize <= 0 {
		elemSize = 2
	}
	mmapReg := "$FF42"
	winBase := "$4000"
	if slotNum == 3 {
		mmapReg = "$FF43"
		winBase = "$6000"
	} else if slotNum == 4 {
		mmapReg = "$FF44"
		winBase = "$8000"
	}
	mName := MangleName(fn.Name)
	lblNear := fmt.Sprintf(".L_%s_fld_near_%d", mName, id)
	lblCalc := fmt.Sprintf(".L_%s_fld_calc_%d", mName, id)

	refKey := valKey(farRefVal)
	alreadyMapped := tracker != nil && refKey != "" && tracker.IsMapped(slotNum, refKey)

	// 1. Evaluate FarRef
	b.loadValToD(buf, farRefVal, paramOffsets, localOffsets, stringDescs)
	buf.WriteString(fmt.Sprintf("    beq   %s\n", lblNear))
	if !alreadyMapped {
		buf.WriteString(fmt.Sprintf("    stb   %s         ; map FarRef into Slot\n", mmapReg))
		if tracker != nil && refKey != "" {
			tracker.SetMapped(slotNum, refKey)
		}
	}
	// Far branch: add Slot base
	b.loadValToD(buf, offsetVal, paramOffsets, localOffsets, stringDescs)
	buf.WriteString(fmt.Sprintf("    addd  #%s        ; add Slot base\n", winBase))
	buf.WriteString(fmt.Sprintf("    bra   %s\n", lblCalc))

	// Near branch: direct RAM
	buf.WriteString(fmt.Sprintf("%s:\n", lblNear))
	b.loadValToD(buf, offsetVal, paramOffsets, localOffsets, stringDescs)

	buf.WriteString(fmt.Sprintf("%s:\n", lblCalc))
	buf.WriteString("    tfr   d,x           ; X = target address\n")

	// 2. Perform load
	if elemSize == 1 {
		buf.WriteString("    clra\n    ldb   ,x\n")
		if slot, ok := localOffsets[id]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}
	} else if elemSize == 2 {
		buf.WriteString("    ldd   ,x\n")
		if slot, ok := localOffsets[id]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}
	} else if elemSize == 8 {
		if slot, ok := localOffsets[id]; ok {
			buf.WriteString("    ldd   0,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; far_ref\n", slot))
			buf.WriteString("    ldd   2,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; offset\n", slot-2))
			buf.WriteString("    ldd   4,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; length\n", slot-4))
			buf.WriteString("    ldd   6,x\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; capacity\n", slot-6))
		}
	} else {
		if slot, ok := localOffsets[id]; ok {
			for off := 0; off < elemSize; off += 2 {
				if off+2 <= elemSize {
					buf.WriteString(fmt.Sprintf("    ldd   %d,x\n    std   -%d,u\n", off, slot-off))
				} else {
					buf.WriteString(fmt.Sprintf("    ldb   %d,x\n    stb   -%d,u\n", off, slot-off))
				}
			}
		}
	}
}

func (b *Backend) emitFarStore(
	buf *bytes.Buffer,
	fn *bigir.Function,
	instr bigir.Instruction,
	farRefVal, offsetVal, valArg bigir.Value,
	slotNum int,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
	tracker *WindowSlotTracker,
) {
	elemSize := valArg.Type().Size
	if elemSize <= 0 {
		elemSize = 2
	}
	mmapReg := "$FF43"
	winBase := "$6000"
	if slotNum == 2 {
		mmapReg = "$FF42"
		winBase = "$4000"
	} else if slotNum == 4 {
		mmapReg = "$FF44"
		winBase = "$8000"
	}
	id := instr.GetID()
	mName := MangleName(fn.Name)
	lblNear := fmt.Sprintf(".L_%s_fst_near_%d", mName, id)
	lblCalc := fmt.Sprintf(".L_%s_fst_calc_%d", mName, id)

	refKey := valKey(farRefVal)
	alreadyMapped := tracker != nil && refKey != "" && tracker.IsMapped(slotNum, refKey)

	// 1. Push value
	if elemSize == 1 {
		b.loadValToD(buf, valArg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  b             ; push val byte\n")
	} else if elemSize == 2 {
		b.loadValToD(buf, valArg, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d             ; push val word\n")
	} else if elemSize == 8 {
		b.pushSliceArg(buf, valArg, paramOffsets, localOffsets, stringDescs)
	} else {
		b.pushSliceArg(buf, valArg, paramOffsets, localOffsets, stringDescs)
	}

	// 2. Evaluate FarRef
	b.loadValToD(buf, farRefVal, paramOffsets, localOffsets, stringDescs)
	buf.WriteString(fmt.Sprintf("    beq   %s\n", lblNear))
	if !alreadyMapped {
		buf.WriteString(fmt.Sprintf("    stb   %s         ; map FarRef into Slot\n", mmapReg))
		if tracker != nil && refKey != "" {
			tracker.SetMapped(slotNum, refKey)
		}
	}
	b.loadValToD(buf, offsetVal, paramOffsets, localOffsets, stringDescs)
	buf.WriteString(fmt.Sprintf("    addd  #%s        ; add Slot base\n", winBase))
	buf.WriteString(fmt.Sprintf("    bra   %s\n", lblCalc))

	buf.WriteString(fmt.Sprintf("%s:\n", lblNear))
	b.loadValToD(buf, offsetVal, paramOffsets, localOffsets, stringDescs)

	buf.WriteString(fmt.Sprintf("%s:\n", lblCalc))
	buf.WriteString("    tfr   d,x           ; X = target address\n")

	// 3. Perform store
	if elemSize == 1 {
		buf.WriteString("    puls  b\n    stb   ,x\n")
	} else if elemSize == 2 {
		buf.WriteString("    puls  d\n    std   ,x\n")
	} else if elemSize == 8 {
		buf.WriteString("    puls  d\n    std   0,x           ; far_ref\n")
		buf.WriteString("    puls  d\n    std   2,x           ; offset\n")
		buf.WriteString("    puls  d\n    std   4,x           ; length\n")
		buf.WriteString("    puls  d\n    std   6,x           ; capacity\n")
	} else {
		aligned := (elemSize + 1) & ^1
		for off := 0; off < aligned; off += 2 {
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   %d,x\n", off))
		}
	}
}

func (b *Backend) loadSliceDescToReg(
	buf *bytes.Buffer,
	arg bigir.Value,
	reg string,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	if sm, ok := arg.(*bigir.SliceMake); ok {
		if ag, ok := sm.Offset.(*bigir.AddressOfGlobal); ok && ag.Global != nil {
			if descLbl, ok := stringDescs[ag.Global.InitString]; ok {
				buf.WriteString(fmt.Sprintf("    ld%s   #%s\n", reg, descLbl))
				return
			}
		}
		if g, ok := sm.Offset.(*bigir.Global); ok {
			if descLbl, ok := stringDescs[g.InitString]; ok {
				buf.WriteString(fmt.Sprintf("    ld%s   #%s\n", reg, descLbl))
				return
			}
		}
	} else if ag, ok := arg.(*bigir.AddressOfGlobal); ok && ag.Global != nil {
		if descLbl, ok := stringDescs[ag.Global.InitString]; ok {
			buf.WriteString(fmt.Sprintf("    ld%s   #%s\n", reg, descLbl))
			return
		}
		buf.WriteString(fmt.Sprintf("    ld%s   #v_%s\n", reg, MangleName(ag.Global.Name)))
		return
	} else if g, ok := arg.(*bigir.Global); ok {
		if descLbl, ok := stringDescs[g.InitString]; ok {
			buf.WriteString(fmt.Sprintf("    ld%s   #%s\n", reg, descLbl))
			return
		}
		buf.WriteString(fmt.Sprintf("    ld%s   #v_%s\n", reg, MangleName(g.Name)))
		return
	}
	if p, ok := arg.(*bigir.Parameter); ok {
		if off, ok := paramOffsets[p.ID]; ok {
			buf.WriteString(fmt.Sprintf("    lea%s  %d,u\n", reg, off))
			return
		}
	}
	if instr, ok := arg.(bigir.Instruction); ok {
		if off, ok := localOffsets[instr.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    lea%s  -%d,u\n", reg, off))
			return
		}
	}
	buf.WriteString(fmt.Sprintf("    ld%s   #0\n", reg))
}

func (b *Backend) emitPrint(
	buf *bytes.Buffer,
	newline bool,
	args []bigir.Value,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	for i, arg := range args {
		if i > 0 {
			buf.WriteString("    ldb   #' '\n")
			buf.WriteString("    jsr   putchar\n")
		}
		if arg == nil {
			buf.WriteString("    clra\n    clrb\n")
			buf.WriteString("    pshs  d\n")
			buf.WriteString("    ldx   #__fmt_d\n")
			buf.WriteString("    pshs  x\n")
			buf.WriteString("    jsr   _printf\n")
			buf.WriteString("    leas  4,s             ; clean up printf args\n")
			continue
		}
		typ := arg.Type()
		isString := typ.Kind == bigir.KindFarString ||
			strings.Contains(typ.Name, "string") ||
			strings.HasSuffix(typ.Name, "slice_byte") ||
			(typ.Kind == bigir.KindFarSlice && typ.ElementType != nil && typ.ElementType.Size == 1)

		if isString {
			b.loadSliceDescToReg(buf, arg, "x", paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    jsr   builtin_print_string\n")
		} else {
			fmtLabel := "__fmt_d"
			if typ.Name == "*byte" || typ.Name == "*char" || (typ.Kind == bigir.KindNearPtr && typ.ElementType != nil && typ.ElementType.Size == 1) {
				fmtLabel = "__fmt_s"
			} else if typ.Kind != bigir.KindInt {
				fmtLabel = "__fmt_u"
			}
			b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			buf.WriteString(fmt.Sprintf("    ldx   #%s\n", fmtLabel))
			buf.WriteString("    pshs  x\n")
			buf.WriteString("    jsr   _printf\n")
			buf.WriteString("    leas  4,s             ; clean up printf args\n")
		}
	}
	if newline {
		buf.WriteString("    ldb   #10              ; newline\n")
		buf.WriteString("    jsr   putchar\n")
	}
}

func (b *Backend) emitPhiAssignments(
	buf *bytes.Buffer,
	mName string,
	from, to *bigir.BasicBlock,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	if to == nil {
		return
	}
	type pendingPhi struct {
		val  bigir.Value
		slot int
		size int
	}
	var pending []pendingPhi
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*bigir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					if slot, exists := localOffsets[phi.GetID()]; exists {
						if edgeInstr, ok := edge.Value.(bigir.Instruction); ok {
							if srcSlot, exists := localOffsets[edgeInstr.GetID()]; exists && srcSlot == slot {
								break
							}
						}
						sz := phi.Type().Size
						if edge.Value.Type().Size > sz {
							sz = edge.Value.Type().Size
						}
						pending = append(pending, pendingPhi{
							val:  edge.Value,
							slot: slot,
							size: sz,
						})
					}
					break
				}
			}
		}
	}
	if len(pending) == 0 {
		return
	}

	// 1. Evaluate and copy/push incoming phi values
	for _, p := range pending {
		if p.size > 32 {
			if instr, ok := p.val.(bigir.Instruction); ok {
				if srcSlot, ok := localOffsets[instr.GetID()]; ok {
					if srcSlot != p.slot {
						lbl := fmt.Sprintf(".L_%s_phicpy_%d_%d_%d", mName, from.ID, to.ID, instr.GetID())
						wordCount := (p.size + 1) / 2
						buf.WriteString(fmt.Sprintf("    leax  -%d,u             ; phi copy src\n", srcSlot))
						buf.WriteString(fmt.Sprintf("    leay  -%d,u             ; phi copy dest\n", p.slot))
						buf.WriteString("    pshs  u\n")
						buf.WriteString(fmt.Sprintf("    ldu   #%d\n", wordCount))
						buf.WriteString(fmt.Sprintf("%s:\n", lbl))
						buf.WriteString("    ldd   ,x++\n")
						buf.WriteString("    std   ,y++\n")
						buf.WriteString("    leau  -1,u\n")
						buf.WriteString("    cmpu  #0\n")
						buf.WriteString(fmt.Sprintf("    bne   %s\n", lbl))
						buf.WriteString("    puls  u\n")
					}
				}
			}
		} else if p.size == 8 && (p.val.Type().Kind == bigir.KindFarSlice || p.val.Type().Kind == bigir.KindFarString) {
			b.pushSliceArg(buf, p.val, paramOffsets, localOffsets, stringDescs)
		} else if p.size > 2 {
			aligned := (p.size + 1) & ^1
			if instr, ok := p.val.(bigir.Instruction); ok {
				if slot, ok := localOffsets[instr.GetID()]; ok {
					for off := aligned - 2; off >= 0; off -= 2 {
						buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-off))
					}
				} else if aligned == 8 {
					b.pushSliceArg(buf, p.val, paramOffsets, localOffsets, stringDescs)
				}
			} else if param, ok := p.val.(*bigir.Parameter); ok {
				if pOff, ok := paramOffsets[param.ID]; ok {
					for off := aligned - 2; off >= 0; off -= 2 {
						buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    pshs  d\n", pOff+off))
					}
				}
			} else if cs, ok := p.val.(*bigir.ConstStruct); ok {
				for idx := len(cs.Fields) - 1; idx >= 0; idx-- {
					b.loadValToD(buf, cs.Fields[idx], paramOffsets, localOffsets, stringDescs)
					buf.WriteString("    pshs  d\n")
				}
			} else if g, ok := p.val.(*bigir.Global); ok {
				mName := MangleName(g.Name)
				for off := aligned - 2; off >= 0; off -= 2 {
					buf.WriteString(fmt.Sprintf("    ldd   v_%s+%d\n    pshs  d\n", mName, off))
				}
			} else if aligned == 8 {
				b.pushSliceArg(buf, p.val, paramOffsets, localOffsets, stringDescs)
			}
		} else {
			b.loadValToD(buf, p.val, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
		}
	}

	// 2. Pop in reverse order into the respective phi stack slots
	for idx := len(pending) - 1; idx >= 0; idx-- {
		p := pending[idx]
		if p.size > 32 {
			continue // Already copied directly in step 1
		} else if p.size == 8 && (p.val.Type().Kind == bigir.KindFarSlice || p.val.Type().Kind == bigir.KindFarString) {
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (far_ref)\n", p.slot))
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (offset)\n", p.slot-2))
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (length)\n", p.slot-4))
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (capacity)\n", p.slot-6))
		} else if p.size > 2 {
			aligned := (p.size + 1) & ^1
			for off := 0; off < aligned; off += 2 {
				buf.WriteString("    puls  d\n")
				buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign struct +%d\n", p.slot-off, off))
			}
		} else {
			buf.WriteString("    puls  d\n")
			if p.size == 1 {
				buf.WriteString("    clra\n")
			}
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign\n", p.slot))
		}
	}
}

func (b *Backend) emitTerminator(
	buf *bytes.Buffer,
	fn *bigir.Function,
	bb *bigir.BasicBlock,
	term bigir.Terminator,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
	mName := MangleName(fn.Name)
	switch t := term.(type) {
	case *bigir.Return, *bigir.FarReturn:
		var val bigir.Value
		if r, ok := term.(*bigir.Return); ok {
			val = r.Val
		} else if fr, ok := term.(*bigir.FarReturn); ok {
			val = fr.Val
		}
		if fn.ReturnType.Size > 2 {
			retSize := fn.ReturnType.Size
			buf.WriteString("    ldx   4,u              ; invisible return pointer\n")
			if sm, ok := val.(*bigir.SliceMake); ok && retSize == 8 {
				b.storeSliceToPtr(buf, sm, paramOffsets, localOffsets, stringDescs)
			} else if instr, ok := val.(bigir.Instruction); ok {
				if slot, ok := localOffsets[instr.GetID()]; ok {
					buf.WriteString(fmt.Sprintf("    leay  -%d,u\n", slot))
					for off := 0; off < retSize; off += 2 {
						if off+2 <= retSize {
							buf.WriteString(fmt.Sprintf("    ldd   %d,y\n", off))
							buf.WriteString(fmt.Sprintf("    std   %d,x\n", off))
						} else {
							buf.WriteString(fmt.Sprintf("    ldb   %d,y\n", off))
							buf.WriteString(fmt.Sprintf("    stb   %d,x\n", off))
						}
					}
				} else if retSize == 8 {
					b.storeSliceToPtr(buf, val, paramOffsets, localOffsets, stringDescs)
				}
			} else if p, ok := val.(*bigir.Parameter); ok {
				if pOff, ok := paramOffsets[p.ID]; ok {
					for off := 0; off < retSize; off += 2 {
						if off+2 <= retSize {
							buf.WriteString(fmt.Sprintf("    ldd   %d,u\n    std   %d,x\n", pOff+off, off))
						} else {
							buf.WriteString(fmt.Sprintf("    ldb   %d,u\n    stb   %d,x\n", pOff+off, off))
						}
					}
				}
			} else if cs, ok := val.(*bigir.ConstStruct); ok {
				for off, f := range cs.Fields {
					b.loadValToD(buf, f, paramOffsets, localOffsets, stringDescs)
					if f.Type().Size == 1 {
						buf.WriteString(fmt.Sprintf("    stb   %d,x\n", off))
					} else {
						buf.WriteString(fmt.Sprintf("    std   %d,x\n", off))
					}
				}
			} else if g, ok := val.(*bigir.Global); ok {
				mName := MangleName(g.Name)
				for off := 0; off < retSize; off += 2 {
					if off+2 <= retSize {
						buf.WriteString(fmt.Sprintf("    ldd   v_%s+%d\n    std   %d,x\n", mName, off, off))
					} else {
						buf.WriteString(fmt.Sprintf("    ldb   v_%s+%d\n    stb   %d,x\n", mName, off, off))
					}
				}
			} else if retSize == 8 {
				b.storeSliceToPtr(buf, val, paramOffsets, localOffsets, stringDescs)
			}
		} else if val != nil {
			b.loadValToD(buf, val, paramOffsets, localOffsets, stringDescs)
		}
		buf.WriteString(fmt.Sprintf("    bra   .L_%s_epilogue\n", mName))
	case *bigir.Branch:
		b.emitPhiAssignments(buf, mName, bb, t.Target, paramOffsets, localOffsets, stringDescs)
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.Target.ID))
	case *bigir.CondBranch:
		b.loadValToD(buf, t.Cond, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    cmpd  #0\n")
		lblTrue := fmt.Sprintf(".L_%s_cbr_true_%d", mName, t.GetID())
		buf.WriteString(fmt.Sprintf("    lbne  %s\n", lblTrue))
		// False target
		b.emitPhiAssignments(buf, mName, bb, t.FalseTarget, paramOffsets, localOffsets, stringDescs)
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.FalseTarget.ID))
		// True target
		buf.WriteString(fmt.Sprintf("%s:\n", lblTrue))
		b.emitPhiAssignments(buf, mName, bb, t.TrueTarget, paramOffsets, localOffsets, stringDescs)
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.TrueTarget.ID))
	}
}

