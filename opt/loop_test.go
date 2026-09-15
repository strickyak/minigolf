package opt

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestDominatorTreeDiamond(t *testing.T) {
	// CFG:
	//       b0 (entry)
	//      /  \
	//     b1   b2
	//      \  /
	//       b3 (exit)

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b2 := &ir.BasicBlock{ID: 2}
	b3 := &ir.BasicBlock{ID: 3}

	b0.Successors = []*ir.BasicBlock{b1, b2}
	b1.Predecessors = []*ir.BasicBlock{b0}
	b1.Successors = []*ir.BasicBlock{b3}

	b2.Predecessors = []*ir.BasicBlock{b0}
	b2.Successors = []*ir.BasicBlock{b3}

	b3.Predecessors = []*ir.BasicBlock{b1, b2}

	fn := &ir.Function{
		Name:   "diamond",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3},
	}

	dt := ComputeDominators(fn)

	// b0 dominates all blocks
	for _, b := range fn.Blocks {
		if !dt.Dominates(b0, b) {
			t.Errorf("b0 should dominate b%d", b.ID)
		}
	}

	// b1 does NOT dominate b3, b2 does NOT dominate b3
	if dt.Dominates(b1, b3) {
		t.Errorf("b1 should not dominate b3")
	}
	if dt.Dominates(b2, b3) {
		t.Errorf("b2 should not dominate b3")
	}

	// Immediate dominator of b1 and b2 is b0
	if dt.ImmediateDominator(b1) != b0 {
		t.Errorf("IDom(b1) expected b0, got: %v", dt.ImmediateDominator(b1))
	}
	if dt.ImmediateDominator(b2) != b0 {
		t.Errorf("IDom(b2) expected b0, got: %v", dt.ImmediateDominator(b2))
	}
	// Immediate dominator of b3 is b0
	if dt.ImmediateDominator(b3) != b0 {
		t.Errorf("IDom(b3) expected b0, got: %v", dt.ImmediateDominator(b3))
	}
}

func TestNaturalLoopDetection(t *testing.T) {
	// Simple for loop:
	//   b0 (preheader)
	//   -> b1 (header)
	//      phi: i = (b0: 0, b2: i_next)
	//      if i < 10 -> b2 (body/latch), else b3 (exit)
	//   b2 (body/latch):
	//      i_next = i + 1
	//      goto b1
	//   b3 (exit):
	//      return

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

	initVal := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 0}
	stepVal := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 1}

	phi := &ir.Phi{
		BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord},
	}
	iNext := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord},
		Op:              "add",
		Left:            phi,
		Right:           stepVal,
	}

	phi.Edges = []ir.PhiEdge{
		{Block: b0, Value: initVal},
		{Block: b2, Value: iNext},
	}

	b0.Instructions = []ir.Instruction{initVal, stepVal}
	b1.Instructions = []ir.Instruction{phi}
	b2.Instructions = []ir.Instruction{iNext}

	fn := &ir.Function{
		Name:   "simple_loop",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3},
	}

	loops := FindLoops(fn)
	if len(loops) != 1 {
		t.Fatalf("expected 1 loop, found %d", len(loops))
	}

	loop := loops[0]
	if loop.Header != b1 {
		t.Errorf("expected header b1, got: b%d", loop.Header.ID)
	}
	if loop.Preheader != b0 {
		t.Errorf("expected preheader b0, got: %v", loop.Preheader)
	}
	if len(loop.Latches) != 1 || loop.Latches[0] != b2 {
		t.Errorf("expected latch b2, got: %v", loop.Latches)
	}
	if !loop.Contains(b1) || !loop.Contains(b2) {
		t.Errorf("loop should contain b1 and b2")
	}
	if loop.Contains(b0) || loop.Contains(b3) {
		t.Errorf("loop should not contain b0 or b3")
	}

	// Induction variables
	if len(loop.IndVars) != 1 {
		t.Fatalf("expected 1 induction variable, found %d", len(loop.IndVars))
	}

	iv := loop.IndVars[0]
	if iv.Phi != phi {
		t.Errorf("IV phi mismatch")
	}
	if iv.Initial != initVal {
		t.Errorf("IV initial mismatch")
	}
	if iv.Step != 1 {
		t.Errorf("expected IV step 1, got %d", iv.Step)
	}

	// Loop invariants: stepVal and initVal are defined in b0 (outside loop)
	foundStepVal := false
	for _, inv := range loop.Invariants {
		if inv == stepVal {
			foundStepVal = true
			break
		}
	}
	if !foundStepVal {
		t.Errorf("stepVal should be identified as loop-invariant")
	}
}

func TestNestedLoops(t *testing.T) {
	// Nested loops: Outer loop b1..b4, Inner loop b2..b3
	// b0 -> b1 (outer header)
	//       -> b2 (inner header)
	//          -> b3 (inner latch -> b2)
	//          -> b4 (outer latch -> b1)
	//       -> b5 (exit)

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b2 := &ir.BasicBlock{ID: 2}
	b3 := &ir.BasicBlock{ID: 3}
	b4 := &ir.BasicBlock{ID: 4}
	b5 := &ir.BasicBlock{ID: 5}

	b0.Successors = []*ir.BasicBlock{b1}
	b1.Predecessors = []*ir.BasicBlock{b0, b4}
	b1.Successors = []*ir.BasicBlock{b2, b5}

	b2.Predecessors = []*ir.BasicBlock{b1, b3}
	b2.Successors = []*ir.BasicBlock{b3, b4}

	b3.Predecessors = []*ir.BasicBlock{b2}
	b3.Successors = []*ir.BasicBlock{b2}

	b4.Predecessors = []*ir.BasicBlock{b2}
	b4.Successors = []*ir.BasicBlock{b1}

	b5.Predecessors = []*ir.BasicBlock{b1}

	fn := &ir.Function{
		Name:   "nested_loops",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3, b4, b5},
	}

	loops := FindLoops(fn)
	if len(loops) != 2 {
		t.Fatalf("expected 2 loops, got %d", len(loops))
	}

	var outer, inner *Loop
	for _, l := range loops {
		if l.Header == b1 {
			outer = l
		} else if l.Header == b2 {
			inner = l
		}
	}

	if outer == nil || inner == nil {
		t.Fatalf("could not identify outer and inner loops")
	}

	if outer.IsInnermost() {
		t.Errorf("outer loop should not be innermost")
	}
	if !inner.IsInnermost() {
		t.Errorf("inner loop should be innermost")
	}

	if len(outer.SubLoops) != 1 || outer.SubLoops[0] != inner {
		t.Errorf("outer.SubLoops expected [inner], got: %v", outer.SubLoops)
	}
}
