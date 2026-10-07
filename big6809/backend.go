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

	// Prologue
	buf.WriteString("    pshs  u\n")
	buf.WriteString("    tfr   s,u\n")
	buf.WriteString("    leas  -32,s             ; Allocate local stack frame\n")

	// Emit Basic Blocks
	for _, bb := range fn.Blocks {
		buf.WriteString(fmt.Sprintf(".L_%s_bb%d:\n", mName, bb.ID))

		for _, instr := range bb.Instructions {
			b.emitInstruction(buf, fn, instr, stringDescs)
		}

		if bb.Terminator != nil {
			b.emitTerminator(buf, fn, bb.Terminator)
		}
	}

	// Epilogue
	buf.WriteString(fmt.Sprintf(".L_%s_epilogue:\n", mName))
	buf.WriteString("    leas  ,u\n")
	buf.WriteString("    puls  u\n")
	buf.WriteString("    rts\n\n")
}

func (b *Backend) emitInstruction(buf *bytes.Buffer, fn *bigir.Function, instr bigir.Instruction, stringDescs map[string]string) {
	switch i := instr.(type) {
	case *bigir.ConstByte:
		// Constants are operands in SSA; do not emit standalone register clobber
	case *bigir.ConstWord:
		// Constants are operands in SSA; do not emit standalone register clobber

	case *bigir.BinaryOp:
		switch i.Op {
		case "add":
			buf.WriteString("    addd  2,s\n")
		case "sub":
			buf.WriteString("    subd  2,s\n")
		}

	case *bigir.SliceMake:
		buf.WriteString("    ; Make 8-byte slice\n")

	case *bigir.Compare:
		buf.WriteString("    tstb\n")

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

		if strings.HasPrefix(callee, "builtin_print") {
			if len(args) > 0 {
				b.loadSliceDescToReg(buf, args[0], "x", stringDescs)
			} else {
				buf.WriteString("    ldx   #0\n")
			}
			buf.WriteString(fmt.Sprintf("    jsr   %s\n", callee))
		} else if callee == "prelude.streq" || callee == "streq" {
			if len(args) >= 2 {
				b.loadSliceDescToReg(buf, args[0], "x", stringDescs)
				b.loadSliceDescToReg(buf, args[1], "y", stringDescs)
			}
			buf.WriteString("    jsr   __far_streq\n")
		} else if comment == "devirtualized intra-block call" {
			mCallee := MangleName(callee)
			buf.WriteString(fmt.Sprintf("    jsr   $A000+fn_%s-_block_%d_data    ; direct intra-block call in Slot 5\n", mCallee, fn.BlockID))
		} else {
			buf.WriteString(fmt.Sprintf("    jsr   f_%s\n", MangleName(callee)))
		}
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

func (b *Backend) emitTerminator(buf *bytes.Buffer, fn *bigir.Function, term bigir.Terminator) {
	mName := MangleName(fn.Name)
	switch t := term.(type) {
	case *bigir.Return, *bigir.FarReturn:
		buf.WriteString(fmt.Sprintf("    bra   .L_%s_epilogue\n", mName))
	case *bigir.Branch:
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.Target.ID))
	case *bigir.CondBranch:
		buf.WriteString(fmt.Sprintf("    lbne  .L_%s_bb%d\n", mName, t.TrueTarget.ID))
		buf.WriteString(fmt.Sprintf("    lbra  .L_%s_bb%d\n", mName, t.FalseTarget.ID))
	}
}
