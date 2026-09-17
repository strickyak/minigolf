package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestEscapeAnalysis(t *testing.T) {
	// Case 1: Local variable used only via LoadPtr and StorePtr does NOT escape.
	local1 := &ir.ZeroInit{
		BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord},
	}
	aol1 := &ir.AddressOfLocal{
		BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord.PointerTo()},
		Local:           local1,
	}
	c1 := &ir.ConstWord{
		BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord},
		Val:             42,
	}
	store1 := &ir.StorePtr{
		BaseInstruction: ir.BaseInstruction{ID: 13, Typ: ir.TypeVoid},
		Ptr:             aol1,
		Val:             c1,
	}
	load1 := &ir.LoadPtr{
		BaseInstruction: ir.BaseInstruction{ID: 14, Typ: ir.TypeWord},
		Ptr:             aol1,
	}
	ret1 := &ir.Return{
		BaseInstruction: ir.BaseInstruction{ID: 15, Typ: ir.TypeVoid},
		Val:             load1,
	}

	b0 := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{local1, aol1, c1, store1, load1},
		Terminator:   ret1,
	}
	fn1 := &ir.Function{
		Name:   "fn1",
		Blocks: []*ir.BasicBlock{b0},
	}

	res1 := AnalyzeEscape(fn1)
	if res1.EscapingLocals[local1.GetID()] {
		t.Errorf("expected local1 not to escape")
	}
	if res1.EscapingAOL[aol1.GetID()] {
		t.Errorf("expected aol1 not to escape")
	}

	// Case 2: Local variable passed to a Call DOES escape.
	local2 := &ir.ZeroInit{
		BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord},
	}
	aol2 := &ir.AddressOfLocal{
		BaseInstruction: ir.BaseInstruction{ID: 21, Typ: ir.TypeWord.PointerTo()},
		Local:           local2,
	}
	call := &ir.Call{
		BaseInstruction: ir.BaseInstruction{ID: 22, Typ: ir.TypeVoid},
		Func:            &ir.Function{Name: "external_fn"},
		Args:            []ir.Value{aol2},
	}
	ret2 := &ir.Return{
		BaseInstruction: ir.BaseInstruction{ID: 23, Typ: ir.TypeVoid},
	}
	b1 := &ir.BasicBlock{
		ID:           1,
		Instructions: []ir.Instruction{local2, aol2, call},
		Terminator:   ret2,
	}
	fn2 := &ir.Function{
		Name:   "fn2",
		Blocks: []*ir.BasicBlock{b1},
	}

	res2 := AnalyzeEscape(fn2)
	if !res2.EscapingLocals[local2.GetID()] {
		t.Errorf("expected local2 to escape due to Call argument")
	}
	if !res2.EscapingAOL[aol2.GetID()] {
		t.Errorf("expected aol2 to escape due to Call argument")
	}
}

func TestConstFoldCast(t *testing.T) {
	trunc := &ir.Cast{
		BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeByte},
		Op:              "trunc",
		Operand:         &ir.ConstWord{Val: 64},
	}
	b := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{trunc},
		Terminator:   &ir.Return{},
	}
	fn := &ir.Function{
		Name:   "fn",
		Blocks: []*ir.BasicBlock{b},
	}
	pass := &ConstFoldPass{}
	if !pass.Run(fn) {
		t.Fatalf("expected ConstFoldPass to fold trunc")
	}
	cB, ok := b.Instructions[0].(*ir.ConstByte)
	if !ok || cB.Val != 64 {
		t.Fatalf("expected ConstByte(64), got %v", b.Instructions[0])
	}
}

