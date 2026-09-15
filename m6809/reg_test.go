package m6809

import (
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestRegMaskOverlapsAndContains(t *testing.T) {
	// D overlaps with A and B
	if !RegD.Overlaps(RegA) {
		t.Errorf("RegD should overlap with RegA")
	}
	if !RegD.Overlaps(RegB) {
		t.Errorf("RegD should overlap with RegB")
	}
	if RegD.Overlaps(RegX) {
		t.Errorf("RegD should not overlap with RegX")
	}

	// D contains both A and B
	if !RegD.Contains(RegA) {
		t.Errorf("RegD should contain RegA")
	}
	if !RegD.Contains(RegB) {
		t.Errorf("RegD should contain RegB")
	}
	if RegA.Contains(RegD) {
		t.Errorf("RegA should not contain RegD")
	}

	// Disjoint index registers
	if RegX.Overlaps(RegY) {
		t.Errorf("RegX should not overlap with RegY")
	}
	if RegX.Overlaps(RegU) {
		t.Errorf("RegX should not overlap with RegU")
	}
}

func TestAllocatableRegistersVariants(t *testing.T) {
	// Default variant: all registers (A, B, X, Y, U) allocatable
	def := AllocatableRegisters(false, false)
	if !def.Contains(RegA) || !def.Contains(RegB) || !def.Contains(RegX) || !def.Contains(RegY) || !def.Contains(RegU) {
		t.Errorf("Default variant should have A, B, X, Y, U allocatable, got %s", def.String())
	}

	// globalsAtY: Y must NOT be allocatable
	gay := AllocatableRegisters(true, false)
	if gay.Overlaps(RegY) {
		t.Errorf("globalsAtY should exclude RegY")
	}
	if !gay.Contains(RegX) || !gay.Contains(RegU) {
		t.Errorf("globalsAtY should still allow X and U")
	}

	// framePointer: U must NOT be allocatable
	fp := AllocatableRegisters(false, true)
	if fp.Overlaps(RegU) {
		t.Errorf("framePointer should exclude RegU")
	}
	if !fp.Contains(RegX) || !fp.Contains(RegY) {
		t.Errorf("framePointer should still allow X and Y")
	}

	// Both globalsAtY and framePointer: only A, B, X available
	both := AllocatableRegisters(true, true)
	if both.Overlaps(RegY) || both.Overlaps(RegU) {
		t.Errorf("both flags should exclude both RegY and RegU")
	}
	if !both.Contains(RegX) || !both.Contains(RegA) || !both.Contains(RegB) {
		t.Errorf("both flags should still allow X, A, B")
	}
}

func TestAssemblyNameAndSize(t *testing.T) {
	if AssemblyName(RegA) != "a" || AssemblyName(RegB) != "b" || AssemblyName(RegD) != "d" {
		t.Errorf("Incorrect accumulator names")
	}
	if AssemblyName(RegX) != "x" || AssemblyName(RegY) != "y" || AssemblyName(RegU) != "u" {
		t.Errorf("Incorrect index register names")
	}

	if RegisterSize(RegA) != 1 || RegisterSize(RegB) != 1 {
		t.Errorf("A and B must be 1 byte")
	}
	if RegisterSize(RegD) != 2 || RegisterSize(RegX) != 2 || RegisterSize(RegY) != 2 || RegisterSize(RegU) != 2 {
		t.Errorf("D, X, Y, U must be 2 bytes")
	}
}

func TestAllocateRegistersLoop(t *testing.T) {
	// Build IR:
	// b0: entry
	//   goto b1
	// b1: loop header
	//   i = phi [0, b0], [i_next, b1]
	//   acc = phi [0, b0], [acc_next, b1]
	//   acc_next = add acc, i
	//   i_next = add i, 1
	//   cond = cmp i_next, 10
	//   br cond, b1, b2
	// b2: exit
	//   ret acc_next

	entry := &ir.BasicBlock{ID: 0}
	header := &ir.BasicBlock{ID: 1}
	exit := &ir.BasicBlock{ID: 2}

	entry.Successors = []*ir.BasicBlock{header}
	entry.Terminator = &ir.Jump{Target: header}

	header.Predecessors = []*ir.BasicBlock{entry, header}
	header.Successors = []*ir.BasicBlock{header, exit}

	phiI := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	phiAcc := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 11, Typ: ir.TypeWord}}

	loadVal := &ir.LoadPtr{BaseInstruction: ir.BaseInstruction{ID: 12, Typ: ir.TypeWord}, Ptr: phiI}
	storeVal := &ir.StorePtr{BaseInstruction: ir.BaseInstruction{ID: 13, Typ: ir.TypeVoid}, Ptr: phiAcc, Val: loadVal}
	iNext := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 14, Typ: ir.TypeWord}, Op: "add", Left: phiI, Right: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord}, Val: 1}}
	accNext := &ir.BinaryOp{BaseInstruction: ir.BaseInstruction{ID: 15, Typ: ir.TypeWord}, Op: "add", Left: phiAcc, Right: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 21, Typ: ir.TypeWord}, Val: 1}}
	cond := &ir.Compare{BaseInstruction: ir.BaseInstruction{ID: 16, Typ: ir.TypeByte}, Op: "lt", Left: iNext, Right: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 22, Typ: ir.TypeWord}, Val: 10}}

	phiI.Edges = []ir.PhiEdge{
		{Block: entry, Value: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 23, Typ: ir.TypeWord}, Val: 0}},
		{Block: header, Value: iNext},
	}
	phiAcc.Edges = []ir.PhiEdge{
		{Block: entry, Value: &ir.ConstWord{BaseInstruction: ir.BaseInstruction{ID: 24, Typ: ir.TypeWord}, Val: 0}},
		{Block: header, Value: accNext},
	}

	header.Instructions = []ir.Instruction{phiI, phiAcc, loadVal, storeVal, accNext, iNext, cond}
	header.Terminator = &ir.Branch{Condition: cond, TrueBlock: header, FalseBlock: exit}

	exit.Predecessors = []*ir.BasicBlock{header}
	exit.Terminator = &ir.Return{Val: loadVal}

	f := &ir.Function{
		Name:       "loop_func",
		ReturnType: ir.TypeWord,
		Blocks:     []*ir.BasicBlock{entry, header, exit},
	}

	// 1. Default backend (both U and Y available)
	bDef := New(false, false, false)
	allocsDef := bDef.AllocateRegisters(f)
	if len(allocsDef) == 0 {
		t.Fatalf("Default backend should have allocated registers, got none")
	}
	for id, reg := range allocsDef {
		if reg != "u" && reg != "y" {
			t.Errorf("Unexpected register %q for var %d", reg, id)
		}
	}

	// 2. Disabled flag
	bDisabled := New(false, false, false)
	bDisabled.NoGlobalRegAlloc = true
	if allocs := bDisabled.AllocateRegisters(f); len(allocs) != 0 {
		t.Errorf("Disabled allocator should return nil, got %v", allocs)
	}

	// 3. globalsAtY (only U available)
	bGay := New(false, true, false)
	allocsGay := bGay.AllocateRegisters(f)
	for id, reg := range allocsGay {
		if reg != "u" {
			t.Errorf("globalsAtY should only allocate U, got %q for var %d", reg, id)
		}
	}

	// 4. framePointer (only Y available)
	bFp := New(true, false, false)
	allocsFp := bFp.AllocateRegisters(f)
	for id, reg := range allocsFp {
		if reg != "y" {
			t.Errorf("framePointer should only allocate Y, got %q for var %d", reg, id)
		}
	}
}
