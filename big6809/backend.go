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
			if _, exists := stringDescs[g.InitString]; exists {
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
	var markUse func(val bigir.Value)
	markUse = func(val bigir.Value) {
		if val == nil {
			return
		}
		if bop, ok := val.(*bigir.BinaryOp); ok && bop.GetID() == 0 {
			markUse(bop.Left)
			markUse(bop.Right)
			return
		}
		if aol, ok := val.(*bigir.AddressOfLocal); ok {
			markUse(resolveRootLocal(aol.Local))
			return
		}
		if sm, ok := val.(*bigir.SliceMake); ok && sm.GetID() == 0 {
			markUse(sm.FarRef)
			markUse(sm.Offset)
			markUse(sm.Length)
			markUse(sm.Capacity)
			return
		}
		if sf, ok := val.(*bigir.SliceField); ok && sf.GetID() == 0 {
			markUse(sf.Slice)
			return
		}
		if instr, ok := val.(bigir.Instruction); ok {
			usedInstrs[instr.GetID()] = true
		}
	}

	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			switch i := instr.(type) {
			case *bigir.UnaryOp:
				markUse(i.Operand)
			case *bigir.BinaryOp:
				markUse(i.Left)
				markUse(i.Right)
			case *bigir.Compare:
				markUse(i.Left)
				markUse(i.Right)
			case *bigir.NearStore:
				markUse(i.Addr)
				markUse(i.Val)
			case *bigir.NearLoad:
				markUse(i.Addr)
			case *bigir.FarStore:
				markUse(i.FarRef)
				markUse(i.Offset)
				markUse(i.Val)
			case *bigir.FarLoad:
				markUse(i.FarRef)
				markUse(i.Offset)
			case *bigir.SliceMake:
				markUse(i.FarRef)
				markUse(i.Offset)
				markUse(i.Length)
				markUse(i.Capacity)
			case *bigir.SliceField:
				markUse(i.Slice)
			case *bigir.SliceGet:
				markUse(i.Slice)
				markUse(i.Index)
			case *bigir.SlicePut:
				markUse(i.Slice)
				markUse(i.Index)
				markUse(i.Val)
			case *bigir.SliceChop:
				markUse(i.Slice)
				markUse(i.Start)
				markUse(i.Limit)
			case *bigir.AddressOfLocal:
				markUse(i.Local)
			case *bigir.FarCall:
				for _, arg := range i.Args {
					markUse(arg)
				}
			case *bigir.NearCall:
				for _, arg := range i.Args {
					markUse(arg)
				}
			case *bigir.IndirectCall:
				markUse(i.FuncPtr)
				for _, arg := range i.Args {
					markUse(arg)
				}
			case *bigir.Phi:
				for _, edge := range i.Edges {
					markUse(edge.Value)
				}
			}
		}
		if bb.Terminator != nil {
			switch t := bb.Terminator.(type) {
			case *bigir.Return:
				markUse(t.Val)
			case *bigir.FarReturn:
				markUse(t.Val)
			case *bigir.CondBranch:
				markUse(t.Cond)
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

	// Pass 2: Allocate stack slots for instructions that produce used values or phis
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			id := instr.GetID()
			if addressTaken[id] {
				continue
			}

			// Constants and addresses are loaded immediately by loadValToD and never read from stack slots
			switch instr.(type) {
			case *bigir.ConstByte, *bigir.ConstWord, *bigir.AddressOfGlobal, *bigir.AddressOfLocal, *bigir.FuncRef:
				continue
			}

			// Phis must always have slots so incoming edges can write to them
			isPhi := false
			if _, ok := instr.(*bigir.Phi); ok {
				isPhi = true
			}

			typ := instr.Type()
			isCallWithStructRet := false
			switch instr.(type) {
			case *bigir.NearCall, *bigir.FarCall, *bigir.IndirectCall:
				if typ.Size > 2 {
					isCallWithStructRet = true
				}
			}
			if !usedInstrs[id] && !isPhi && !isCallWithStructRet {
				continue // Value never read; no stack slot needed!
			}

			if typ.Size <= 0 {
				continue
			}

			sz := 2
			if _, ok := instr.(*bigir.SliceMake); ok {
				sz = 8
			} else if typ.Kind == bigir.KindFarSlice || typ.Kind == bigir.KindFarString {
				sz = 8
			} else if typ.Kind == bigir.KindArray {
				// Arrays are never values on stack unless address-taken (handled in Pass 1).
				// Any phi or temporary with KindArray is at most a word handle.
				sz = 2
			} else if typ.Size > 2 {
				sz = (typ.Size + 1) & ^1
				if sz < 8 {
					sz = 8
				}
			} else if phi, ok := instr.(*bigir.Phi); ok {
				for _, edge := range phi.Edges {
					if edge.Value.Type().Size > 2 {
						sz = 8
						break
					}
				}
			}

			curLocalOffset += sz
			localOffsets[id] = curLocalOffset
		}
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
	for _, bb := range fn.Blocks {
		buf.WriteString(fmt.Sprintf(".L_%s_bb%d:\n", mName, bb.ID))

		for _, instr := range bb.Instructions {
			b.emitInstruction(buf, fn, instr, paramOffsets, localOffsets, stringDescs)
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
) {
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
		} else if i.Type().Size > 2 {
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
		} else {
			buf.WriteString("    ldd   ,x\n")
			if slot, ok := localOffsets[i.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
			}
		}

	case *bigir.NearStore:
		isByteStore := i.Val.Type().Size == 1 || (i.Addr.Type().ElementType != nil && i.Addr.Type().ElementType.Size == 1)
		if isByteStore {
			b.loadValToD(buf, i.Val, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  b\n")
			b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    puls  b\n")
			buf.WriteString("    stb   ,x\n")
		} else if i.Val.Type().Size > 2 {
			b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			b.storeSliceToPtr(buf, i.Val, paramOffsets, localOffsets, stringDescs)
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
				b.emitPrint(buf, callee == "builtin_println", args, paramOffsets, localOffsets, stringDescs)
			} else if callee == "builtin_panic" {
				if len(args) > 0 {
					b.loadSliceDescToReg(buf, args[0], "x", paramOffsets, localOffsets, stringDescs)
				} else {
					buf.WriteString("    ldx   #0\n")
				}
				buf.WriteString("    jsr   builtin_panic\n")
			} else {
				buf.WriteString(fmt.Sprintf("    jsr   %s\n", callee))
			}
		} else if callee == "prelude.streq" || callee == "streq" {
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
		} else {
			retSize := instr.Type().Size

			// Push arguments in reverse order (right to left)
			totalArgBytes := 0
			for idx := len(args) - 1; idx >= 0; idx-- {
				arg := args[idx]
				if arg.Type().Size > 2 {
					b.pushSliceArg(buf, arg, paramOffsets, localOffsets, stringDescs)
					totalArgBytes += 8
				} else {
					b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
					buf.WriteString("    pshs  d\n")
					totalArgBytes += 2
				}
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
		retSize := instr.Type().Size

		// Push arguments in reverse order (right to left)
		totalArgBytes := 0
		for idx := len(i.Args) - 1; idx >= 0; idx-- {
			arg := i.Args[idx]
			if arg.Type().Size > 2 {
				b.pushSliceArg(buf, arg, paramOffsets, localOffsets, stringDescs)
				totalArgBytes += 8
			} else {
				b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
				buf.WriteString("    pshs  d\n")
				totalArgBytes += 2
			}
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
				buf.WriteString(fmt.Sprintf("    leax  %d,u\n    tfr   x,d\n", off))
				return
			}
		}
		if instr, ok := loc.(bigir.Instruction); ok {
			if off, ok := localOffsets[instr.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    leax  -%d,u\n    tfr   x,d\n", off))
				return
			}
		}
		buf.WriteString("    clra\n    clrb\n")
	case *bigir.FuncRef:
		buf.WriteString(fmt.Sprintf("    ldd   #f_%s\n", MangleName(v.FuncName)))
		return
	case *bigir.SliceField:
		b.loadSliceFieldToD(buf, v, paramOffsets, localOffsets, stringDescs)
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

func (b *Backend) pushSliceArg(
	buf *bytes.Buffer,
	arg bigir.Value,
	paramOffsets, localOffsets map[int]int,
	stringDescs map[string]string,
) {
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
	if instr, ok := arg.(bigir.Instruction); ok {
		if slot, ok := localOffsets[instr.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-6))
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-4))
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot-2))
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u\n    pshs  d\n", slot))
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
	} else if g, ok := arg.(*bigir.Global); ok {
		if descLbl, ok := stringDescs[g.InitString]; ok {
			buf.WriteString(fmt.Sprintf("    ld%s   #%s\n", reg, descLbl))
			return
		}
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
	b.fmtCount++
	fmtLabel := fmt.Sprintf(".Lfmt_%d", b.fmtCount)

	var formatStrs []string
	for _, arg := range args {
		if arg == nil {
			formatStrs = append(formatStrs, "%d")
			continue
		}
		typ := arg.Type()
		if typ.Kind == bigir.KindFarString || strings.Contains(typ.Name, "string") {
			formatStrs = append(formatStrs, "%s")
		} else if typ.Kind == bigir.KindInt {
			formatStrs = append(formatStrs, "%d")
		} else {
			formatStrs = append(formatStrs, "%u")
		}
	}
	format := strings.Join(formatStrs, " ")
	if newline {
		format += "\n"
	}
	b.fmtStrings[fmtLabel] = format

	// Push arguments in reverse order (right to left)
	for i := len(args) - 1; i >= 0; i-- {
		arg := args[i]
		if sm, ok := arg.(*bigir.SliceMake); ok {
			if ag, ok := sm.Offset.(*bigir.AddressOfGlobal); ok && ag.Global != nil && ag.Global.InitString != "" {
				if desc, ok := stringDescs[ag.Global.InitString]; ok {
					dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
					buf.WriteString(fmt.Sprintf("    ldd   #%s\n", dataLbl))
				} else {
					b.loadValToD(buf, sm.Offset, paramOffsets, localOffsets, stringDescs)
				}
			} else if g, ok := sm.Offset.(*bigir.Global); ok && g.InitString != "" {
				if desc, ok := stringDescs[g.InitString]; ok {
					dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
					buf.WriteString(fmt.Sprintf("    ldd   #%s\n", dataLbl))
				} else {
					b.loadValToD(buf, sm.Offset, paramOffsets, localOffsets, stringDescs)
				}
			} else {
				b.loadValToD(buf, sm.Offset, paramOffsets, localOffsets, stringDescs)
			}
		} else if ag, ok := arg.(*bigir.AddressOfGlobal); ok && ag.Global != nil && ag.Global.InitString != "" {
			if desc, ok := stringDescs[ag.Global.InitString]; ok {
				dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
				buf.WriteString(fmt.Sprintf("    ldd   #%s\n", dataLbl))
			} else {
				b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
			}
		} else if g, ok := arg.(*bigir.Global); ok && g.InitString != "" {
			if desc, ok := stringDescs[g.InitString]; ok {
				dataLbl := strings.Replace(desc, "_desc_", "_data_", 1)
				buf.WriteString(fmt.Sprintf("    ldd   #%s\n", dataLbl))
			} else {
				b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
			}
		} else if arg.Type().Kind == bigir.KindFarString {
			b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
			// If it's a dynamic string slice loaded into D, offset is at 2,d
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    ldd   2,x\n")
		} else {
			b.loadValToD(buf, arg, paramOffsets, localOffsets, stringDescs)
		}
		buf.WriteString("    pshs  d\n")
	}

	buf.WriteString(fmt.Sprintf("    ldx   #%s\n", fmtLabel))
	buf.WriteString("    pshs  x\n")
	buf.WriteString("    jsr   _printf\n")
	cleanup := 2 + len(args)*2
	buf.WriteString(fmt.Sprintf("    leas  %d,s             ; clean up printf args\n", cleanup))
}

func (b *Backend) emitPhiAssignments(
	buf *bytes.Buffer,
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

	// 1. Evaluate and push all incoming phi values
	for _, p := range pending {
		if p.size > 2 {
			b.pushSliceArg(buf, p.val, paramOffsets, localOffsets, stringDescs)
		} else {
			b.loadValToD(buf, p.val, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
		}
	}

	// 2. Pop in reverse order into the respective phi stack slots
	for idx := len(pending) - 1; idx >= 0; idx-- {
		p := pending[idx]
		if p.size > 2 {
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (far_ref)\n", p.slot))
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (offset)\n", p.slot-2))
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (length)\n", p.slot-4))
			buf.WriteString("    puls  d\n")
			buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign (capacity)\n", p.slot-6))
		} else {
			buf.WriteString("    puls  d\n")
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
			buf.WriteString("    ldx   4,u              ; invisible return pointer\n")
			b.storeSliceToPtr(buf, val, paramOffsets, localOffsets, stringDescs)
		} else if val != nil {
			b.loadValToD(buf, val, paramOffsets, localOffsets, stringDescs)
		}
		buf.WriteString(fmt.Sprintf("    bra   .L_%s_epilogue\n", mName))
	case *bigir.Branch:
		b.emitPhiAssignments(buf, bb, t.Target, paramOffsets, localOffsets, stringDescs)
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.Target.ID))
	case *bigir.CondBranch:
		b.loadValToD(buf, t.Cond, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    cmpd  #0\n")
		lblTrue := fmt.Sprintf(".L_%s_cbr_true_%d", mName, t.GetID())
		buf.WriteString(fmt.Sprintf("    lbne  %s\n", lblTrue))
		// False target
		b.emitPhiAssignments(buf, bb, t.FalseTarget, paramOffsets, localOffsets, stringDescs)
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.FalseTarget.ID))
		// True target
		buf.WriteString(fmt.Sprintf("%s:\n", lblTrue))
		b.emitPhiAssignments(buf, bb, t.TrueTarget, paramOffsets, localOffsets, stringDescs)
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.TrueTarget.ID))
	}
}

