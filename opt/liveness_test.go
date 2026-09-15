package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestLivenessLinear(t *testing.T) {
	// Function:
	//   v1 = 10
	//   v2 = 20
	//   v3 = v1 + v2
	//   return v3
	v1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 10}
	v2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 20}
	v3 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord}, Op: "+", Left: v1, Right: v2}
	ret := &ir.Return{Val: v3}

	b0 := &ir.BasicBlock{
		ID:           0,
		Instructions: []ir.Instruction{v1, v2, v3},
		Terminator:   ret,
	}

	fn := &ir.Function{
		Name:   "test_linear",
		Blocks: []*ir.BasicBlock{b0},
	}

	l := ComputeLiveness(fn)

	// v1 is defined at pos 0 and used at pos 4 (v3 definition)
	inv1 := l.Intervals[1]
	if inv1 == nil || inv1.Start != 0 || inv1.End != 4 {
		t.Fatalf("v1 interval expected [0, 4], got: %+v", inv1)
	}

	// v2 is defined at pos 2 and used at pos 4
	inv2 := l.Intervals[2]
	if inv2 == nil || inv2.Start != 2 || inv2.End != 4 {
		t.Fatalf("v2 interval expected [2, 4], got: %+v", inv2)
	}

	// v3 is defined at pos 4 and used at pos 6 (return)
	inv3 := l.Intervals[3]
	if inv3 == nil || inv3.Start != 4 || inv3.End != 6 {
		t.Fatalf("v3 interval expected [4, 6], got: %+v", inv3)
	}

	// v1 and v2 overlap at pos 2..4 -> they must interfere
	if !l.Interferes(1, 2) {
		t.Errorf("v1 and v2 should interfere")
	}

	// v1 and v3: v1 dies at pos 4 where v3 is defined -> they do not interfere
	if l.Interferes(1, 3) {
		t.Errorf("v1 and v3 should not interfere")
	}

	// Max pressure should be 2 (at pos 2, v1 and v2 are simultaneously live)
	p := l.MaxPressure()
	if p != 2 {
		t.Errorf("Expected max pressure 2, got %d", p)
	}
}

func TestLivenessLoop(t *testing.T) {
	// Loop CFG:
	// b0 (entry):
	//   v1 = 0
	//   jump b1
	// b1 (header):
	//   phi v2 = [v1, b0], [v4, b2]
	//   v3 = v2 < 10
	//   branch v3 ? b2 : b3
	// b2 (body):
	//   v4 = v2 + 1
	//   jump b1
	// b3 (exit):
	//   return v2

	v1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 0}

	phi2 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}}

	v4 := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeWord}, Op: "+", Left: phi2, Right: &ir.ConstWord{Val: 1}}

	phi2.Edges = []ir.PhiEdge{
		{Value: v1}, // from b0
		{Value: v4}, // from b2
	}

	cmp3 := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeBool}, Op: "lt", Left: phi2, Right: &ir.ConstWord{Val: 10}}

	b0 := &ir.BasicBlock{ID: 0, Instructions: []ir.Instruction{v1}}
	b1 := &ir.BasicBlock{ID: 1, Instructions: []ir.Instruction{phi2, cmp3}}
	b2 := &ir.BasicBlock{ID: 2, Instructions: []ir.Instruction{v4}}
	b3 := &ir.BasicBlock{ID: 3}

	b0.Terminator = &ir.Jump{Target: b1}
	b0.Successors = []*ir.BasicBlock{b1}

	b1.Terminator = &ir.Branch{Condition: cmp3, TrueBlock: b2, FalseBlock: b3}
	b1.Predecessors = []*ir.BasicBlock{b0, b2}
	b1.Successors = []*ir.BasicBlock{b2, b3}

	b2.Terminator = &ir.Jump{Target: b1}
	b2.Predecessors = []*ir.BasicBlock{b1}
	b2.Successors = []*ir.BasicBlock{b1}

	b3.Terminator = &ir.Return{Val: phi2}
	b3.Predecessors = []*ir.BasicBlock{b1}

	phi2.Edges[0].Block = b0
	phi2.Edges[1].Block = b2

	fn := &ir.Function{
		Name:   "test_loop",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3},
	}

	l := ComputeLiveness(fn)

	// v4 must be live at the end of b2 (to feed phi2 in b1)
	bl2 := l.LiveOut[b2]
	if !bl2[4] {
		t.Errorf("v4 should be live out of block 2 (loop body)")
	}

	// phi2 (v2) must be live into b2 and live into b3
	if !l.LiveIn[b2][2] {
		t.Errorf("phi2 (v2) should be live into block 2")
	}
	if !l.LiveIn[b3][2] {
		t.Errorf("phi2 (v2) should be live into block 3 (loop exit)")
	}
}
