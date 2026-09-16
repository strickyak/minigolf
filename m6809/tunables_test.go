package m6809

import (
	"strings"
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestInlineMul16(t *testing.T) {
	op1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 10}
	op2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 20}
	mulOp := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord},
		Op:              "mul",
		Left:            op1,
		Right:           op2,
	}

	// 1. Without InlineMul16 (default): calls __mul16
	b1 := New(false, false, false)
	b1.f = &ir.Function{Name: "test_mul"}
	b1.slots[1] = 0
	b1.slotSizes[1] = 2
	b1.slots[2] = 2
	b1.slotSizes[2] = 2
	b1.slots[3] = 4
	b1.slotSizes[3] = 2
	b1.stackSize = 6
	b1.emitBinaryOp(mulOp)
	out1 := b1.buf.String()
	if !strings.Contains(out1, "jsr __mul16") && !strings.Contains(out1, "lbsr __mul16") {
		t.Errorf("Expected out-of-line helper call for __mul16, got:\n%s", out1)
	}

	// 2. With InlineMul16: inlines multiplication body, no jsr __mul16
	b2 := New(false, false, false)
	b2.InlineMul16 = true
	b2.f = &ir.Function{Name: "test_mul_inline"}
	b2.slots[1] = 0
	b2.slotSizes[1] = 2
	b2.slots[2] = 2
	b2.slotSizes[2] = 2
	b2.slots[3] = 4
	b2.slotSizes[3] = 2
	b2.stackSize = 6
	b2.emitBinaryOp(mulOp)
	out2 := b2.buf.String()
	if strings.Contains(out2, "__mul16") {
		t.Errorf("Did not expect __mul16 helper call when InlineMul16 is true, got:\n%s", out2)
	}
	if !strings.Contains(out2, "pshs d,x") || !strings.Contains(out2, "leas 4,s") {
		t.Errorf("Expected inline 16-bit multiply instructions, got:\n%s", out2)
	}
}

func TestInlineDivMod16(t *testing.T) {
	op1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 100}
	op2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 7}
	divOp := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord},
		Op:              "div",
		Left:            op1,
		Right:           op2,
	}

	// 1. Without InlineDivMod16 (default): calls __div16
	b1 := New(false, false, false)
	b1.f = &ir.Function{Name: "test_div"}
	b1.slots[1] = 0
	b1.slotSizes[1] = 2
	b1.slots[2] = 2
	b1.slotSizes[2] = 2
	b1.slots[3] = 4
	b1.slotSizes[3] = 2
	b1.stackSize = 6
	b1.emitBinaryOp(divOp)
	out1 := b1.buf.String()
	if !strings.Contains(out1, "jsr __div16") && !strings.Contains(out1, "lbsr __div16") {
		t.Errorf("Expected out-of-line helper call for __div16, got:\n%s", out1)
	}

	// 2. With InlineDivMod16: inlines division loop, branches to __div0_error on 0
	b2 := New(false, false, false)
	b2.InlineDivMod16 = true
	b2.f = &ir.Function{Name: "test_div_inline"}
	b2.slots[1] = 0
	b2.slotSizes[1] = 2
	b2.slots[2] = 2
	b2.slotSizes[2] = 2
	b2.slots[3] = 4
	b2.slotSizes[3] = 2
	b2.stackSize = 6
	b2.emitBinaryOp(divOp)
	out2 := b2.buf.String()
	if strings.Contains(out2, "jsr __div16") || strings.Contains(out2, "lbsr __div16") {
		t.Errorf("Did not expect __div16 helper call when InlineDivMod16 is true, got:\n%s", out2)
	}
	if !strings.Contains(out2, "exg d,x") || !strings.Contains(out2, "tfr x,d") || !strings.Contains(out2, "__div0_error") {
		t.Errorf("Expected inline division instructions and __div0_error branch, got:\n%s", out2)
	}
}

func TestShiftUnrollThreshold(t *testing.T) {
	val := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 42}
	c6 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 6}
	shlOp := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord},
		Op:              "shl",
		Left:            val,
		Right:           c6,
	}

	// 1. With ShiftUnrollThreshold = 4: shift of 6 is emitted as a loop
	b1 := New(false, false, false)
	b1.ShiftUnrollThreshold = 4
	b1.f = &ir.Function{Name: "test_shl_loop"}
	b1.slots[1] = 0
	b1.slotSizes[1] = 2
	b1.slots[3] = 2
	b1.slotSizes[3] = 2
	b1.stackSize = 4
	b1.emitBinaryOp(shlOp)
	out1 := b1.buf.String()
	if !strings.Contains(out1, "ldx #6") || !strings.Contains(out1, "bne") {
		t.Errorf("Expected loop for shift count 6 with threshold 4, got:\n%s", out1)
	}

	// 2. With ShiftUnrollThreshold = 8: shift of 6 is unrolled straight-line
	b2 := New(false, false, false)
	b2.ShiftUnrollThreshold = 8
	b2.f = &ir.Function{Name: "test_shl_unroll"}
	b2.slots[1] = 0
	b2.slotSizes[1] = 2
	b2.slots[3] = 2
	b2.slotSizes[3] = 2
	b2.stackSize = 4
	b2.emitBinaryOp(shlOp)
	out2 := b2.buf.String()
	if strings.Contains(out2, "bne") || strings.Contains(out2, "ldx #6") {
		t.Errorf("Expected unrolled shift for count 6 with threshold 8, got:\n%s", out2)
	}
	aslbCount := strings.Count(out2, "aslb")
	if aslbCount != 6 {
		t.Errorf("Expected 6 aslb instructions for unrolled shift of 6, got %d:\n%s", aslbCount, out2)
	}
}

func TestMemcpyUnrollThreshold(t *testing.T) {
	// 1. Size 6 with threshold 4: loop or helper
	b1 := New(false, false, false)
	b1.MemcpyUnrollThreshold = 4
	b1.emitCopy("x", "s", 6)
	out1 := b1.buf.String()
	if !strings.Contains(out1, "ldu #6") {
		t.Errorf("Expected loop for memcpy size 6 with threshold 4, got:\n%s", out1)
	}

	// 2. Size 6 with threshold 8: unrolled straight-line
	b2 := New(false, false, false)
	b2.MemcpyUnrollThreshold = 8
	b2.emitCopy("x", "s", 6)
	out2 := b2.buf.String()
	if strings.Contains(out2, "ldu #6") {
		t.Errorf("Expected unrolled straight-line copy with threshold 8, got:\n%s", out2)
	}
	staCount := strings.Count(out2, "sta")
	if staCount != 6 {
		t.Errorf("Expected 6 sta instructions for unrolled copy of 6 bytes, got %d:\n%s", staCount, out2)
	}
}
