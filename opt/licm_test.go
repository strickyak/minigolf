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
	b0.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 90, Typ: ir.TypeVoid}, Target: b1}
	b1.Predecessors = []*ir.BasicBlock{b0, b2}
	b1.Successors = []*ir.BasicBlock{b2, b3}
	b1.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 91, Typ: ir.TypeVoid}, TrueBlock: b2, FalseBlock: b3}
	b2.Predecessors = []*ir.BasicBlock{b1}
	b2.Successors = []*ir.BasicBlock{b1}
	b2.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 92, Typ: ir.TypeVoid}, Target: b1}
	b3.Predecessors = []*ir.BasicBlock{b1}
	b3.Terminator = &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 93, Typ: ir.TypeVoid}}

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

func TestLICMPreheaderInsertion_CriticalEdge(t *testing.T) {
	// CFG:
	// b0: if cond goto b1 (header) else goto b4 (bypass)
	// b1: (header)
	//   phi: i = (b0: 0, b2: i_next)
	//   inv = p1 + p2
	//   i_next = i + 1
	//   if i < 10 goto b2 else goto b3
	// b2: (latch) goto b1
	// b3: (exit) return
	// b4: (bypass) return
	//
	// Here b0 has 2 successors (b1 and b4), so b0 is NOT a dedicated preheader!
	// Previously, this loop was skipped because loop.Preheader == nil.
	// With preheader insertion, a new dedicated preheader block must be inserted
	// on the b0->b1 edge and inv must be hoisted into it.

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b2 := &ir.BasicBlock{ID: 2}
	b3 := &ir.BasicBlock{ID: 3}
	b4 := &ir.BasicBlock{ID: 4}

	p1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 10}
	p2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 20}
	b0.Instructions = []ir.Instruction{p1, p2}
	b0.Successors = []*ir.BasicBlock{b1, b4}
	b0.Terminator = &ir.Branch{
		BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeVoid},
		TrueBlock:       b1,
		FalseBlock:      b4,
	}

	phi := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	inv := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord},
		Op:              "add",
		Left:            p1,
		Right:           p2,
	}
	iNext := &ir.BinaryOp{
		BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord},
		Op:              "add",
		Left:            phi,
		Right:           &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 13, Typ: ir.TypeWord}, Val: 1},
	}
	phi.Edges = []ir.PhiEdge{
		{Block: b0, Value: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 14, Typ: ir.TypeWord}, Val: 0}},
		{Block: b2, Value: iNext},
	}
	b1.Instructions = []ir.Instruction{phi, inv, iNext}
	b1.Predecessors = []*ir.BasicBlock{b0, b2}
	b1.Successors = []*ir.BasicBlock{b2, b3}
	b1.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 15, Typ: ir.TypeVoid}, TrueBlock: b2, FalseBlock: b3}

	b2.Predecessors = []*ir.BasicBlock{b1}
	b2.Successors = []*ir.BasicBlock{b1}
	b2.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 16, Typ: ir.TypeVoid}, Target: b1}

	b3.Predecessors = []*ir.BasicBlock{b1}
	b3.Terminator = &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 17, Typ: ir.TypeVoid}}

	b4.Predecessors = []*ir.BasicBlock{b0}
	b4.Terminator = &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 18, Typ: ir.TypeVoid}}

	fn := &ir.Function{
		Name:   "test_critical_edge",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3, b4},
	}

	// Verify that initially FindLoops finds Preheader == nil
	initialLoops := FindLoops(fn)
	if len(initialLoops) != 1 {
		t.Fatalf("expected 1 loop, found %d", len(initialLoops))
	}
	if initialLoops[0].Preheader != nil {
		t.Fatalf("expected initial loop.Preheader == nil due to critical edge, got %v", initialLoops[0].Preheader)
	}

	pass := &LICMPass{}
	changed := pass.Run(fn)

	if !changed {
		t.Fatalf("expected LICMPass to insert dedicated preheader and hoist inv")
	}

	// Check that a new preheader was inserted
	if len(fn.Blocks) != 6 {
		t.Fatalf("expected 6 blocks after preheader insertion, got %d", len(fn.Blocks))
	}

	// Find the new preheader block
	var preheader *ir.BasicBlock
	for _, b := range fn.Blocks {
		if b != b0 && b != b1 && b != b2 && b != b3 && b != b4 {
			preheader = b
			break
		}
	}
	if preheader == nil {
		t.Fatalf("could not find newly inserted preheader block")
	}

	// Verify preheader contains the hoisted instruction
	hasInv := false
	for _, instr := range preheader.Instructions {
		if instr == inv {
			hasInv = true
		}
	}
	if !hasInv {
		t.Errorf("expected inv in preheader.Instructions, got: %v", preheader.Instructions)
	}

	// Verify b0 branches to preheader (not b1)
	br0 := b0.Terminator.(*ir.Branch)
	if br0.TrueBlock != preheader {
		t.Errorf("expected b0 TrueBlock to be preheader, got %v", br0.TrueBlock)
	}

	// Verify b1 phi edge from b0 was redirected to preheader
	foundPreEdge := false
	for _, edge := range phi.Edges {
		if edge.Block == preheader {
			foundPreEdge = true
			break
		}
	}
	if !foundPreEdge {
		t.Errorf("expected a phi edge from preheader, got %v", phi.Edges)
	}
}

func TestLICMPreheaderInsertion_MultiplePredecessors(t *testing.T) {
	// CFG:
	// b0: if cond goto b1 else goto b2
	// b1: goto b3 (header) with val 10
	// b2: goto b3 (header) with val 20
	// b3: (header)
	//   phi: x = (b1: 10, b2: 20, b4: x_next)
	//   inv = const_word 5 + const_word 6
	//   ...
	// b4: (latch) goto b3
	// b5: (exit) return
	//
	// Multiple outside predecessors entering b3 -> Preheader is nil.
	// Dedicated preheader must be inserted with combining Phi node.

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b2 := &ir.BasicBlock{ID: 2}
	b3 := &ir.BasicBlock{ID: 3}
	b4 := &ir.BasicBlock{ID: 4}
	b5 := &ir.BasicBlock{ID: 5}

	b0.Successors = []*ir.BasicBlock{b1, b2}
	b0.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeVoid}, TrueBlock: b1, FalseBlock: b2}

	val1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 10}
	b1.Instructions = []ir.Instruction{val1}
	b1.Predecessors = []*ir.BasicBlock{b0}
	b1.Successors = []*ir.BasicBlock{b3}
	b1.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeVoid}, Target: b3}

	val2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeWord}, Val: 20}
	b2.Instructions = []ir.Instruction{val2}
	b2.Predecessors = []*ir.BasicBlock{b0}
	b2.Successors = []*ir.BasicBlock{b3}
	b2.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 5, Typ: ir.TypeVoid}, Target: b3}

	phi := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	c5 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord}, Val: 5}
	c6 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord}, Val: 6}
	inv := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 13, Typ: ir.TypeWord}, Op: "add", Left: c5, Right: c6}
	xNext := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 14, Typ: ir.TypeWord}, Op: "add", Left: phi, Right: c5}

	phi.Edges = []ir.PhiEdge{
		{Block: b1, Value: val1},
		{Block: b2, Value: val2},
		{Block: b4, Value: xNext},
	}

	b3.Instructions = []ir.Instruction{phi, c5, c6, inv, xNext}
	b3.Predecessors = []*ir.BasicBlock{b1, b2, b4}
	b3.Successors = []*ir.BasicBlock{b4, b5}
	b3.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 15, Typ: ir.TypeVoid}, TrueBlock: b4, FalseBlock: b5}

	b4.Predecessors = []*ir.BasicBlock{b3}
	b4.Successors = []*ir.BasicBlock{b3}
	b4.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 16, Typ: ir.TypeVoid}, Target: b3}

	b5.Predecessors = []*ir.BasicBlock{b3}
	b5.Terminator = &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 17, Typ: ir.TypeVoid}}

	fn := &ir.Function{
		Name:   "test_multi_pred",
		Blocks: []*ir.BasicBlock{b0, b1, b2, b3, b4, b5},
	}

	pass := &LICMPass{}
	changed := pass.Run(fn)

	if !changed {
		t.Fatalf("expected LICMPass to succeed")
	}

	// Find the new preheader block
	var preheader *ir.BasicBlock
	for _, b := range fn.Blocks {
		if b != b0 && b != b1 && b != b2 && b != b3 && b != b4 && b != b5 {
			preheader = b
			break
		}
	}
	if preheader == nil {
		t.Fatalf("could not find newly inserted preheader block")
	}

	// Verify preheader contains a new combining Phi node AND the hoisted instructions
	hasPhi := false
	hasInv := false
	for _, instr := range preheader.Instructions {
		if _, ok := instr.(*ir.Phi); ok {
			hasPhi = true
		}
		if instr == inv {
			hasInv = true
		}
	}

	if !hasPhi {
		t.Errorf("expected preheader to contain a combined Phi for b1 and b2")
	}
	if !hasInv {
		t.Errorf("expected preheader to contain hoisted inv")
	}

	// Verify b3's phi now has 2 edges: one from preheader, one from b4 (latch)
	if len(phi.Edges) != 2 {
		t.Fatalf("expected b3 phi to have exactly 2 edges, got %d", len(phi.Edges))
	}
	if phi.Edges[0].Block != preheader && phi.Edges[1].Block != preheader {
		t.Errorf("expected one phi edge from preheader, got %v", phi.Edges)
	}
}

func TestLICMPreheaderInsertion_EntryBlockHeader(t *testing.T) {
	// CFG:
	// b0: (header and function entry)
	//   inv = const_word 42 * const_word 2
	//   if cond goto b1 else goto b2
	// b1: (latch) goto b0
	// b2: (exit) return
	//
	// Here b0 is f.Blocks[0], so it has 0 outside predecessors.
	// Dedicated preheader must be created as new f.Blocks[0].

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b2 := &ir.BasicBlock{ID: 2}

	c42 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeWord}, Val: 42}
	c2 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 2, Typ: ir.TypeWord}, Val: 2}
	inv := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 3, Typ: ir.TypeWord}, Op: "mul", Left: c42, Right: c2}

	b0.Instructions = []ir.Instruction{c42, c2, inv}
	b0.Predecessors = []*ir.BasicBlock{b1}
	b0.Successors = []*ir.BasicBlock{b1, b2}
	b0.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 4, Typ: ir.TypeVoid}, TrueBlock: b1, FalseBlock: b2}

	b1.Predecessors = []*ir.BasicBlock{b0}
	b1.Successors = []*ir.BasicBlock{b0}
	b1.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 5, Typ: ir.TypeVoid}, Target: b0}

	b2.Predecessors = []*ir.BasicBlock{b0}
	b2.Terminator = &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 6, Typ: ir.TypeVoid}}

	fn := &ir.Function{
		Name:   "test_entry_header",
		Blocks: []*ir.BasicBlock{b0, b1, b2},
	}

	pass := &LICMPass{}
	changed := pass.Run(fn)

	if !changed {
		t.Fatalf("expected LICMPass to hoist inv")
	}

	// fn.Blocks[0] must now be the new preheader
	if fn.Blocks[0] == b0 {
		t.Errorf("expected fn.Blocks[0] to be new preheader, not b0")
	}

	newEntry := fn.Blocks[0]
	hasInv := false
	for _, instr := range newEntry.Instructions {
		if instr == inv {
			hasInv = true
		}
	}
	if !hasInv {
		t.Errorf("expected hoisted inv in new entry preheader, got: %v", newEntry.Instructions)
	}
}

func TestLICMPreheaderInsertion_NestedLoops(t *testing.T) {
	// CFG:
	// b0: (entry) goto b1
	// b1: (outer header)
	//   phi_i = (b0: 0, b5: i_next)
	//   if i < 10 goto b3 else goto b6 (critical edge to inner loop!)
	// b3: (inner header)
	//   phi_j = (b1: 0, b4: j_next)
	//   inv_inner = phi_i + 100   <-- invariant in inner loop, but varies with i
	//   inv_both  = 123 * 456      <-- invariant in BOTH inner and outer loop
	//   j_next = phi_j + 1
	//   if j < 5 goto b4 else goto b5
	// b4: (inner latch) goto b3
	// b5: (outer latch)
	//   i_next = phi_i + 1
	//   goto b1
	// b6: (outer exit) return

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}
	b3 := &ir.BasicBlock{ID: 3}
	b4 := &ir.BasicBlock{ID: 4}
	b5 := &ir.BasicBlock{ID: 5}
	b6 := &ir.BasicBlock{ID: 6}

	// b0 -> b1
	b0.Successors = []*ir.BasicBlock{b1}
	b0.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 1, Typ: ir.TypeVoid}, Target: b1}

	// b1 (outer header) -> b3, b6
	phiI := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	b1.Instructions = []ir.Instruction{phiI}
	b1.Predecessors = []*ir.BasicBlock{b0, b5}
	b1.Successors = []*ir.BasicBlock{b3, b6}
	b1.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeVoid}, TrueBlock: b3, FalseBlock: b6}

	// b3 (inner header) -> b4, b5
	phiJ := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord}}
	c100 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 21, Typ: ir.TypeWord}, Val: 100}
	invInner := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 22, Typ: ir.TypeWord}, Op: "add", Left: phiI, Right: c100}
	c123 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 23, Typ: ir.TypeWord}, Val: 123}
	c456 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 24, Typ: ir.TypeWord}, Val: 456}
	invBoth := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 25, Typ: ir.TypeWord}, Op: "mul", Left: c123, Right: c456}
	c1 := &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 26, Typ: ir.TypeWord}, Val: 1}
	jNext := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 27, Typ: ir.TypeWord}, Op: "add", Left: phiJ, Right: c1}

	phiJ.Edges = []ir.PhiEdge{
		{Block: b1, Value: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 28, Typ: ir.TypeWord}, Val: 0}},
		{Block: b4, Value: jNext},
	}
	b3.Instructions = []ir.Instruction{phiJ, c100, invInner, c123, c456, invBoth, c1, jNext}
	b3.Predecessors = []*ir.BasicBlock{b1, b4}
	b3.Successors = []*ir.BasicBlock{b4, b5}
	b3.Terminator = &ir.Branch{BaseInstruction: ir.BaseInstruction{ID: 29, Typ: ir.TypeVoid}, TrueBlock: b4, FalseBlock: b5}

	// b4 (inner latch) -> b3
	b4.Predecessors = []*ir.BasicBlock{b3}
	b4.Successors = []*ir.BasicBlock{b3}
	b4.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 30, Typ: ir.TypeVoid}, Target: b3}

	// b5 (outer latch) -> b1
	iNext := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 40, Typ: ir.TypeWord}, Op: "add", Left: phiI, Right: c1}
	b5.Instructions = []ir.Instruction{iNext}
	b5.Predecessors = []*ir.BasicBlock{b3}
	b5.Successors = []*ir.BasicBlock{b1}
	b5.Terminator = &ir.Jump{BaseInstruction: ir.BaseInstruction{ID: 41, Typ: ir.TypeVoid}, Target: b1}

	phiI.Edges = []ir.PhiEdge{
		{Block: b0, Value: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 42, Typ: ir.TypeWord}, Val: 0}},
		{Block: b5, Value: iNext},
	}

	// b6 (exit)
	b6.Predecessors = []*ir.BasicBlock{b1}
	b6.Terminator = &ir.Return{BaseInstruction: ir.BaseInstruction{ID: 50, Typ: ir.TypeVoid}}

	fn := &ir.Function{
		Name:   "test_nested",
		Blocks: []*ir.BasicBlock{b0, b1, b3, b4, b5, b6},
	}

	pass := &LICMPass{}
	changed := pass.Run(fn)

	if !changed {
		t.Fatalf("expected LICMPass to optimize nested loops")
	}

	// invBoth should have been hoisted all the way out to b0 (outer preheader)
	b0HasInvBoth := false
	for _, instr := range b0.Instructions {
		if instr == invBoth {
			b0HasInvBoth = true
		}
	}
	if !b0HasInvBoth {
		t.Errorf("expected invBoth to be hoisted to outer preheader b0")
	}

	// invInner should have been hoisted to inner preheader (not b3, and not b0)
	b0HasInvInner := false
	for _, instr := range b0.Instructions {
		if instr == invInner {
			b0HasInvInner = true
		}
	}
	if b0HasInvInner {
		t.Errorf("invInner depends on phiI, should NOT be in b0")
	}

	b3HasInvInner := false
	for _, instr := range b3.Instructions {
		if instr == invInner {
			b3HasInvInner = true
		}
	}
	if b3HasInvInner {
		t.Errorf("invInner should have been hoisted out of b3")
	}
}

