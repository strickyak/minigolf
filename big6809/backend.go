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
			sz := g.Typ.Size
			if sz <= 0 {
				sz = 2
			}
			buf.WriteString(fmt.Sprintf("v_%s:\n", mName))
			buf.WriteString(fmt.Sprintf("    rmb   %d\n\n", sz))
		}
	}

	// 4. Emit Far Code Blocks (Staged Data for Blocks 8..127) into codeBuf
	codeBuf.WriteString("; ==============================================================================\n")
	codeBuf.WriteString("; Staged Far Code Blocks (Packed into 8KB physical blocks)\n")
	codeBuf.WriteString("; ==============================================================================\n\n")

	for _, blk := range blockIDs {
		codeBuf.WriteString(fmt.Sprintf("; --- Physical Block %d Data ---\n", blk))
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

	// 5. Emit Unpack Loop in Slot 6 (after sizes are known)
	buf.WriteString("; --- Unpack Far Blocks into Physical Blocks 8..127 ---\n")
	buf.WriteString("__unpack_far_blocks:\n")

	for _, blk := range blockIDs {
		buf.WriteString(fmt.Sprintf("    ; Unpack Block %d\n", blk))
		buf.WriteString(fmt.Sprintf("    lda   #%d\n", blk))
		buf.WriteString("    sta   $FF45             ; Map block into Slot 5\n")
		buf.WriteString(fmt.Sprintf("    ldx   #_block_%d_data\n", blk))
		buf.WriteString("    ldy   #$A000            ; Slot 5 base address\n")
		buf.WriteString(fmt.Sprintf(".copy_blk_%d:\n", blk))
		buf.WriteString("    lda   ,x+\n")
		buf.WriteString("    sta   ,y+\n")
		buf.WriteString(fmt.Sprintf("    cmpx  #_block_%d_end\n", blk))
		buf.WriteString(fmt.Sprintf("    bne   .copy_blk_%d\n", blk))
	}
	buf.WriteString("    rts\n\n")

	// Trailer
	buf.WriteString("    end cstart_embiggen\n")

	return buf.String(), nil
}

func (b *Backend) emitFunction(buf *bytes.Buffer, fn *bigir.Function, stringDescs map[string]string) {
	mName := MangleName(fn.Name)
	buf.WriteString(fmt.Sprintf("; Function: %s (Block %d, Slot 5 Offset 0x%04X)\n", fn.Name, fn.BlockID, fn.Slot5Offset))
	buf.WriteString(fmt.Sprintf("fn_%s:\n", mName))

	// Parameter stack offsets relative to U:
	// 0,u = saved U (2B)
	// 2,u = return address (2B)
	// First parameter is at 4,u
	paramOffsets := make(map[int]int)
	curParamOffset := 4
	for _, p := range fn.Parameters {
		sz := p.Typ.Size
		if sz < 2 {
			sz = 2 // 2-byte stack alignment
		}
		paramOffsets[p.ID] = curParamOffset
		curParamOffset += sz
	}

	// Local SSA instruction offsets relative to U:
	localOffsets := make(map[int]int)
	curLocalOffset := 0
	for _, bb := range fn.Blocks {
		for _, instr := range bb.Instructions {
			sz := instr.Type().Size
			if sz <= 0 {
				continue
			}
			if sz < 2 {
				sz = 2
			}
			curLocalOffset += sz
			localOffsets[instr.GetID()] = curLocalOffset
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
		switch i.Op {
		case "add":
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
		case "sub":
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
		case "mul":
			b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    tfr   d,x\n")
			buf.WriteString("    puls  d\n")
			buf.WriteString("    jsr   __mul16\n")
		case "div":
			b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    puls  x\n")
			buf.WriteString("    jsr   __div16\n")
		case "mod":
			b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    puls  x\n")
			buf.WriteString("    jsr   __mod16\n")
		case "and":
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
		case "or":
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
		case "xor":
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
		case "shl":
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
		case "shr":
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
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.SliceMake:
		buf.WriteString("    ; Make 8-byte slice\n")

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
		buf.WriteString("    ldd   ,x\n")
		if slot, ok := localOffsets[i.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    std   -%d,u\n", slot))
		}

	case *bigir.NearStore:
		b.loadValToD(buf, i.Val, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
		b.loadValToD(buf, i.Addr, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    tfr   d,x\n")
		buf.WriteString("    puls  d\n")
		buf.WriteString("    std   ,x\n")

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
					b.loadSliceDescToReg(buf, args[0], "x", stringDescs)
				} else {
					buf.WriteString("    ldx   #0\n")
				}
				buf.WriteString("    jsr   builtin_panic\n")
			} else {
				buf.WriteString(fmt.Sprintf("    jsr   %s\n", callee))
			}
		} else if callee == "prelude.streq" || callee == "streq" {
			if len(args) >= 2 {
				b.loadSliceDescToReg(buf, args[0], "x", stringDescs)
				b.loadSliceDescToReg(buf, args[1], "y", stringDescs)
			}
			buf.WriteString("    jsr   __far_streq\n")
		} else {
			// Push arguments in reverse order (right to left)
			totalArgBytes := 0
			for idx := len(args) - 1; idx >= 0; idx-- {
				b.loadValToD(buf, args[idx], paramOffsets, localOffsets, stringDescs)
				buf.WriteString("    pshs  d\n")
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

			if instr.Type().Size > 0 {
				if slot, ok := localOffsets[instr.GetID()]; ok {
					buf.WriteString(fmt.Sprintf("    std   -%d,u             ; store return value v%d\n", slot, instr.GetID()))
				}
			}
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
	case bigir.Instruction:
		if off, ok := localOffsets[v.GetID()]; ok {
			buf.WriteString(fmt.Sprintf("    ldd   -%d,u              ; v%d\n", off, v.GetID()))
		} else {
			buf.WriteString("    clra\n    clrb\n")
		}
	case *bigir.Global:
		if v.InitString != "" {
			if desc, ok := stringDescs[v.InitString]; ok {
				buf.WriteString(fmt.Sprintf("    ldd   #%s\n", desc))
				return
			}
		}
		buf.WriteString(fmt.Sprintf("    ldd   v_%s\n", MangleName(v.Name)))
	case *bigir.AddressOfGlobal:
		if v.Global != nil {
			if v.Global.InitString != "" {
				if desc, ok := stringDescs[v.Global.InitString]; ok {
					buf.WriteString(fmt.Sprintf("    ldd   #%s\n", desc))
					return
				}
			}
			buf.WriteString(fmt.Sprintf("    ldd   #v_%s\n", MangleName(v.Global.Name)))
		}
	case *bigir.AddressOfLocal:
		if instr, ok := v.Local.(bigir.Instruction); ok {
			if off, ok := localOffsets[instr.GetID()]; ok {
				buf.WriteString(fmt.Sprintf("    leau  -%d,u\n    tfr   u,d\n", off))
				return
			}
		}
		buf.WriteString("    clra\n    clrb\n")
	default:
		buf.WriteString("    clra\n    clrb\n")
	}
}

func (b *Backend) loadSliceDescToReg(buf *bytes.Buffer, arg bigir.Value, reg string, stringDescs map[string]string) {
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
	}
	var pending []pendingPhi
	for _, instr := range to.Instructions {
		if phi, ok := instr.(*bigir.Phi); ok {
			for _, edge := range phi.Edges {
				if edge.Block == from {
					if slot, exists := localOffsets[phi.GetID()]; exists {
						pending = append(pending, pendingPhi{
							val:  edge.Value,
							slot: slot,
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
		b.loadValToD(buf, p.val, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    pshs  d\n")
	}

	// 2. Pop in reverse order into the respective phi stack slots
	for idx := len(pending) - 1; idx >= 0; idx-- {
		buf.WriteString("    puls  d\n")
		buf.WriteString(fmt.Sprintf("    std   -%d,u             ; phi assign\n", pending[idx].slot))
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
	case *bigir.Return:
		if t.Val != nil {
			b.loadValToD(buf, t.Val, paramOffsets, localOffsets, stringDescs)
		}
		buf.WriteString(fmt.Sprintf("    bra   .L_%s_epilogue\n", mName))
	case *bigir.FarReturn:
		if t.Val != nil {
			b.loadValToD(buf, t.Val, paramOffsets, localOffsets, stringDescs)
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

