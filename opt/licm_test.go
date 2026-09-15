package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestLICMHoisting(t *testing.T) {
	// Loop structure:
	// b0 (preheader)
	//   p1 = 100
	//   p2 = 200
	//   goto b1
	// b1 (header)
	//   phi: i = (b0: 0, b2: i_next)
	//   inv1 = p1 + p2      <-- Invariant! Should be hoisted to b0!
	//   inv2 = inv1 * 2     <-- Invariant! Should be hoisted to b0!
	//   i_next = i + 1
	//   if i < 10 goto b2 else goto b3
	// b2 (latch)
	//   goto b1
	// b3 (exit)
	//   return

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b2 := &ir.BasicBlock{ID: 2}
	b3 := &ir.BasicBlock{ID: 3}

	b0.Successors = []*ir.BasicBlock{b1}
	b1.Predecessors = []*ir.BasicBlock{b0, b2}
	b1.Successors = []*ir.BasicBlock{b2, b3}
	b2.Predecessors = []*ir.BasicBlock{b1}
	b2.Successors = []*ir.BasicBlock{b1}
	b3.Predecessors = []*ir.BasicBlock{b1}

	p1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 100}
	p2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 200}
	b0.Instructions = []ir.Instruction{p1, p2}

	phi := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	inv1 := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord},
		Op:              "add",
		Left:            p1,
		Right:           p2,
	}
	const2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord}, Val: 2}
	inv2 := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 13, Typ: ir.TypeWord},
		Op:              "mul",
		Left:            inv1,
		Right:           const2,
	}
	iNext := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 14, Typ: ir.TypeWord},
		Op:              "add",
		Left:            phi,
		Right:           &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 15, Typ: ir.TypeWord}, Val: 1},
	}

	phi.Edges = []ir.PhiEdge{
		{Block: b0, Value: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 16, Typ: ir.TypeWord}, Val: 0}},
		{Block: b2, Value: iNext},
	}

	b1.Instructions = []ir.Instruction{phi, inv1, const2, inv2, iNext}

	fn := &ir.Function{
		Name:   "test_licm",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3},
	}

	pass := &LICMPass{}
	changed := pass.Run(fn)

	if !changed {
		t.Fatalf("expected LICMPass to hoist invariant instructions")
	}

	// inv1, const2, and inv2 should now be in b0 (preheader)
	b0HasInv1 := false
	b0HasInv2 := false
	for _, instr := range b0.Instructions {
		if instr == inv1 {
			b0HasInv1 = true
		}
		if instr == inv2 {
			b0HasInv2 = true
		}
	}

	if !b0HasInv1 || !b0HasInv2 {
		t.Errorf("expected inv1 and inv2 hoisted to b0; got b0 instrs: %v", b0.Instructions)
	}

	// b1 should NOT contain inv1 or inv2
	for _, instr := range b1.Instructions {
		if instr == inv1 || instr == inv2 {
			t.Errorf("inv1/inv2 should have been removed from b1")
		}
	}
}
