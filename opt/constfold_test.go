package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestConstFoldAlgebraicIdentities(t *testing.T) {
	cf := &ConstFoldPass{WordSize: 2}

	// Test foldSizeof
	szByte := &ir.Sizeof{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, TargetTyp: ir.TypeByte}
	f1 := cf.foldInstruction(szByte, nil)
	if cw, ok := f1.(*ir.ConstWord); !ok || cw.Val != 1 {
		t.Errorf("Expected sizeof(byte) to fold to 1, got %#v", f1)
	}

	szInt := &ir.Sizeof{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, TargetTyp: ir.TypeInt}
	f2 := cf.foldInstruction(szInt, nil)
	if cw, ok := f2.(*ir.ConstWord); !ok || cw.Val != 2 {
		t.Errorf("Expected sizeof(int) with WordSize=2 to fold to 2, got %#v", f2)
	}

	// Test ConstByte arithmetic
	cb1 := &ir.ConstByte{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeByte}, Val: 10}
	cb2 := &ir.ConstByte{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeByte}, Val: 20}
	binAdd := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeByte}, Op: "add", Left: cb1, Right: cb2}
	f3 := cf.foldInstruction(binAdd, nil)
	if cb, ok := f3.(*ir.ConstByte); !ok || cb.Val != 30 {
		t.Errorf("Expected 10 + 20 (byte) to fold to 30, got %#v", f3)
	}

	// Test x * 0 = 0
	x := &ir.Parameter{ID: 100, Name: "x", Typ: ir.TypeWord}
	c0 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 101, Typ: ir.TypeWord}, Val: 0}
	binMul0 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 102, Typ: ir.TypeWord}, Op: "mul", Left: x, Right: c0}
	f4 := cf.foldInstruction(binMul0, nil)
	if cw, ok := f4.(*ir.ConstWord); !ok || cw.Val != 0 {
		t.Errorf("Expected x * 0 to fold to 0, got %#v", f4)
	}

	// Test x - x = 0
	binSubSelf := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 103, Typ: ir.TypeWord}, Op: "sub", Left: x, Right: x}
	f5 := cf.foldInstruction(binSubSelf, nil)
	if cw, ok := f5.(*ir.ConstWord); !ok || cw.Val != 0 {
		t.Errorf("Expected x - x to fold to 0, got %#v", f5)
	}

	// Test x == x = 1
	cmpEqSelf := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 104, Typ: ir.TypeByte}, Op: "eq", Left: x, Right: x}
	f6 := cf.foldInstruction(cmpEqSelf, nil)
	if cb, ok := f6.(*ir.ConstByte); !ok || cb.Val != 1 {
		t.Errorf("Expected x == x to fold to 1, got %#v", f6)
	}

	// Test x != x = 0
	cmpNeqSelf := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 105, Typ: ir.TypeByte}, Op: "neq", Left: x, Right: x}
	f7 := cf.foldInstruction(cmpNeqSelf, nil)
	if cb, ok := f7.(*ir.ConstByte); !ok || cb.Val != 0 {
		t.Errorf("Expected x != x to fold to 0, got %#v", f7)
	}
}

func TestCopyPropAlgebraicIdentities(t *testing.T) {
	cp := &CopyPropPass{}

	x := &ir.Parameter{ID: 200, Name: "x", Typ: ir.TypeWord}
	c0 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 201, Typ: ir.TypeWord}, Val: 0}
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 202, Typ: ir.TypeWord}, Val: 1}

	// x + 0 -> x
	add0 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 203, Typ: ir.TypeWord}, Op: "add", Left: x, Right: c0}
	if v := cp.simplifyBinaryOp(add0); v != x {
		t.Errorf("Expected x + 0 to simplify to x, got %v", v)
	}

	// x * 1 -> x
	mul1 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 204, Typ: ir.TypeWord}, Op: "mul", Left: x, Right: c1}
	if v := cp.simplifyBinaryOp(mul1); v != x {
		t.Errorf("Expected x * 1 to simplify to x, got %v", v)
	}

	// 1 * x -> x
	mul1L := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 205, Typ: ir.TypeWord}, Op: "mul", Left: c1, Right: x}
	if v := cp.simplifyBinaryOp(mul1L); v != x {
		t.Errorf("Expected 1 * x to simplify to x, got %v", v)
	}

	// x << 0 -> x
	shl0 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 206, Typ: ir.TypeWord}, Op: "shl", Left: x, Right: c0}
	if v := cp.simplifyBinaryOp(shl0); v != x {
		t.Errorf("Expected x << 0 to simplify to x, got %v", v)
	}

	// x & x -> x
	andSelf := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 207, Typ: ir.TypeWord}, Op: "and", Left: x, Right: x}
	if v := cp.simplifyBinaryOp(andSelf); v != x {
		t.Errorf("Expected x & x to simplify to x, got %v", v)
	}
}

func TestStrengthReductionPowerOf2(t *testing.T) {
	sr := &StrengthReductionPass{}

	x := &ir.Parameter{ID: 300, Name: "x", Typ: ir.TypeWord}

	// x * 1 should NOT reduce to x << 0 (leave for copyprop)
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 301, Typ: ir.TypeWord}, Val: 1}
	binMul1 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 302, Typ: ir.TypeWord}, Op: "mul", Left: x, Right: c1}
	if sr.reduce(binMul1) {
		t.Errorf("Expected x * 1 to not reduce in StrengthReductionPass")
	}

	// x * 2 should reduce to x << 1
	c2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 303, Typ: ir.TypeWord}, Val: 2}
	binMul2 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 304, Typ: ir.TypeWord}, Op: "mul", Left: x, Right: c2}
	if !sr.reduce(binMul2) {
		t.Fatalf("Expected x * 2 to reduce in StrengthReductionPass")
	}
	if binMul2.Op != "shl" {
		t.Errorf("Expected op to become shl, got %s", binMul2.Op)
	}
	if cw, ok := binMul2.Right.(*ir.ConstWord); !ok || cw.Val != 1 {
		t.Errorf("Expected right operand to be ConstWord(1), got %#v", binMul2.Right)
	}
}

func TestConstFoldSignedCompare(t *testing.T) {
	// 64-bit word size
	cf64 := &ConstFoldPass{WordSize: 8}
	var neg42 int64 = -42
	cNeg42 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeInt}, Val: uint64(neg42)}
	cZero := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeInt}, Val: 0}

	cmpLt := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeByte}, Op: "lt", Left: cNeg42, Right: cZero}
	f1 := cf64.foldCompare(cmpLt)
	if cb, ok := f1.(*ir.ConstByte); !ok || cb.Val != 1 {
		t.Errorf("Expected -42 < 0 to fold to 1 (true) on 64-bit, got %#v", f1)
	}

	cmpGt := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeByte}, Op: "gt", Left: cNeg42, Right: cZero}
	f2 := cf64.foldCompare(cmpGt)
	if cb, ok := f2.(*ir.ConstByte); !ok || cb.Val != 0 {
		t.Errorf("Expected -42 > 0 to fold to 0 (false) on 64-bit, got %#v", f2)
	}

	// 16-bit word size (M6809)
	cf16 := &ConstFoldPass{WordSize: 2}
	var neg42_16 int16 = -42
	cNeg42_16 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 5, Typ: ir.TypeInt}, Val: uint64(uint16(neg42_16))}
	cmpLt16 := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 6, Typ: ir.TypeByte}, Op: "lt", Left: cNeg42_16, Right: cZero}
	f3 := cf16.foldCompare(cmpLt16)
	if cb, ok := f3.(*ir.ConstByte); !ok || cb.Val != 1 {
		t.Errorf("Expected -42 < 0 to fold to 1 (true) on 16-bit, got %#v", f3)
	}
}

