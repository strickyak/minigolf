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
type Backend struct{}

// New creates a new EMBIGGEN 6809 backend.
func New() *Backend {
	return &Backend{}
}

// Generate transforms a BIGIR Program into valid 6809 assembly.
func (b *Backend) Generate(prog *bigir.Program) (string, error) {
	var buf bytes.Buffer

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

	// 4. Emit Far Code Blocks (Staged Data for Blocks 8..127)
	buf.WriteString("; ==============================================================================\n")
	buf.WriteString("; Staged Far Code Blocks (Packed into 8KB physical blocks)\n")
	buf.WriteString("; ==============================================================================\n\n")

	for _, blk := range blockIDs {
		buf.WriteString(fmt.Sprintf("; --- Physical Block %d Data ---\n", blk))
		buf.WriteString(fmt.Sprintf("_block_%d_data:\n", blk))

		funcs := prog.FarBlocks[blk]
		for _, fn := range funcs {
			b.emitFunction(&buf, fn, stringDescs)
		}

		buf.WriteString(fmt.Sprintf("_block_%d_end:\n", blk))
		buf.WriteString(fmt.Sprintf("_block_%d_size equ _block_%d_end - _block_%d_data\n\n", blk, blk, blk))
	}

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

	// Emit Basic Blocks
	for _, bb := range fn.Blocks {
		buf.WriteString(fmt.Sprintf(".L_%s_bb%d:\n", mName, bb.ID))

		for _, instr := range bb.Instructions {
			b.emitInstruction(buf, fn, instr, paramOffsets, localOffsets, stringDescs)
		}

		if bb.Terminator != nil {
			b.emitTerminator(buf, fn, bb.Terminator, paramOffsets, localOffsets, stringDescs)
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

	case *bigir.BinaryOp:
		b.loadValToD(buf, i.Left, paramOffsets, localOffsets, stringDescs)
		switch i.Op {
		case "add":
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
			buf.WriteString(fmt.Sprintf("    subd  #%d\n", r.Val))
		case *bigir.ConstByte:
			buf.WriteString(fmt.Sprintf("    subd  #%d\n", r.Val))
		default:
			buf.WriteString("    pshs  d\n")
			b.loadValToD(buf, i.Right, paramOffsets, localOffsets, stringDescs)
			buf.WriteString("    pshs  d\n")
			buf.WriteString("    ldd   2,s\n")
			buf.WriteString("    subd  ,s\n")
			buf.WriteString("    leas  4,s\n")
		}
		lblTrue := fmt.Sprintf(".Lcmp_true_%d", i.GetID())
		lblEnd := fmt.Sprintf(".Lcmp_end_%d", i.GetID())
		switch i.Op {
		case "eq":
			buf.WriteString(fmt.Sprintf("    beq   %s\n", lblTrue))
		case "neq":
			buf.WriteString(fmt.Sprintf("    bne   %s\n", lblTrue))
		case "lt":
			buf.WriteString(fmt.Sprintf("    blt   %s\n", lblTrue))
		case "lte":
			buf.WriteString(fmt.Sprintf("    ble   %s\n", lblTrue))
		case "gt":
			buf.WriteString(fmt.Sprintf("    bgt   %s\n", lblTrue))
		case "gte":
			buf.WriteString(fmt.Sprintf("    bge   %s\n", lblTrue))
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
			if strings.HasPrefix(callee, "builtin_print") || callee == "builtin_panic" {
				if len(args) > 0 {
					b.loadSliceDescToReg(buf, args[0], "x", stringDescs)
				} else {
					buf.WriteString("    ldx   #0\n")
				}
			}
			buf.WriteString(fmt.Sprintf("    jsr   %s\n", callee))
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

func (b *Backend) emitTerminator(
	buf *bytes.Buffer,
	fn *bigir.Function,
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
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.Target.ID))
	case *bigir.CondBranch:
		b.loadValToD(buf, t.Cond, paramOffsets, localOffsets, stringDescs)
		buf.WriteString("    cmpd  #0\n")
		buf.WriteString(fmt.Sprintf("    lbne  .L_%s_bb%d\n", mName, t.TrueTarget.ID))
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.FalseTarget.ID))
	}
}
