package m6809

import (
	"strings"
	"testing"

	"github.com/strickyak/minigolf/ir"
)

func TestEmitPhiAssignmentsSwap16(t *testing.T) {
	b := New(false, false, false)
	f := &ir.Function{Name: "test_swap"}
	b.f = f

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}

	// Two 16-bit phis that swap values from b0
	// phi1 (ID 10) gets phi2
	// phi2 (ID 20) gets phi1
	phi1 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	phi2 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord}}

	phi1.Edges = []ir.PhiEdge{{Block: b0, Value: phi2}}
	phi2.Edges = []ir.PhiEdge{{Block: b0, Value: phi1}}

	b1.Instructions = []ir.Instruction{phi1, phi2}

	// Set up slots
	b.slots[10] = 0
	b.slotSizes[10] = 2
	b.slots[20] = 2
	b.slotSizes[20] = 2
	b.stackSize = 4

	b.emitPhiAssignments(b0, b1)
	output := b.buf.String()

	// Must contain hardware exg d,x for 16-bit swap
	if !strings.Contains(output, "exg d,x") {
		t.Fatalf("expected hardware 'exg d,x' in 16-bit swap, got:\n%s", output)
	}
}

func TestEmitPhiAssignmentsSwap8(t *testing.T) {
	b := New(false, false, false)
	f := &ir.Function{Name: "test_swap8"}
	b.f = f

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}

	// Two 8-bit phis that swap values from b0
	phi1 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeByte}}
	phi2 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeByte}}

	phi1.Edges = []ir.PhiEdge{{Block: b0, Value: phi2}}
	phi2.Edges = []ir.PhiEdge{{Block: b0, Value: phi1}}

	b1.Instructions = []ir.Instruction{phi1, phi2}

	// Set up slots
	b.slots[10] = 0
	b.slotSizes[10] = 1
	b.slots[20] = 1
	b.slotSizes[20] = 1
	b.stackSize = 2

	b.emitPhiAssignments(b0, b1)
	output := b.buf.String()

	// Must contain hardware exg a,b for 8-bit swap
	if !strings.Contains(output, "exg a,b") {
		t.Fatalf("expected hardware 'exg a,b' in 8-bit swap, got:\n%s", output)
	}
}

func TestEmitPhiAssignmentsAcyclicOrder(t *testing.T) {
	b := New(false, false, false)
	f := &ir.Function{Name: "test_acyclic"}
	b.f = f

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}

	// Chain:
	// phi1 (ID 10, slot 0) gets phi2 (ID 20, slot 2)
	// phi2 (ID 20, slot 2) gets const 42
	// Regardless of definition order in b1, phi1 <- phi2 must be emitted BEFORE phi2 <- 42!
	phi1 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	phi2 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord}}

	phi1.Edges = []ir.PhiEdge{{Block: b0, Value: phi2}}
	phi2.Edges = []ir.PhiEdge{{Block: b0, Value: &ir.ConstWord{Val: 42}}}

	// Place phi2 BEFORE phi1 in block instructions to stress-test ordering
	b1.Instructions = []ir.Instruction{phi2, phi1}

	b.slots[10] = 0
	b.slotSizes[10] = 2
	b.slots[20] = 2
	b.slotSizes[20] = 2
	b.stackSize = 4

	b.emitPhiAssignments(b0, b1)
	output := b.buf.String()

	// In the output:
	// Storing to 0,s (phi1) must happen before storing #42 to 2,s (phi2)
	idxStore0 := strings.Index(output, "std 0,s")
	idxStore2 := strings.Index(output, "std 2,s")

	if idxStore0 == -1 || idxStore2 == -1 {
		t.Fatalf("expected std 0,s and std 2,s in output, got:\n%s", output)
	}
	if idxStore0 > idxStore2 {
		t.Fatalf("Lost Copy bug detected: std 2,s occurred before std 0,s in output:\n%s", output)
	}
}

func TestEmitPhiAssignmentsThreeCycle(t *testing.T) {
	b := New(false, false, false)
	f := &ir.Function{Name: "test_three_cycle"}
	b.f = f

	b0 := &ir.BasicBlock{ID: 0}
	b1 := &ir.BasicBlock{ID: 1}

	// 3-Cycle:
	// phi1 (ID 10, slot 0) gets phi2 (ID 20, slot 2)
	// phi2 (ID 20, slot 2) gets phi3 (ID 30, slot 4)
	// phi3 (ID 30, slot 4) gets phi1 (ID 10, slot 0)
	phi1 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 10, Typ: ir.TypeWord}}
	phi2 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 20, Typ: ir.TypeWord}}
	phi3 := &ir.Phi{BaseInstruction: ir.BaseInstruction{ID: 30, Typ: ir.TypeWord}}

	phi1.Edges = []ir.PhiEdge{{Block: b0, Value: phi2}}
	phi2.Edges = []ir.PhiEdge{{Block: b0, Value: phi3}}
	phi3.Edges = []ir.PhiEdge{{Block: b0, Value: phi1}}

	b1.Instructions = []ir.Instruction{phi1, phi2, phi3}

	b.slots[10] = 0
	b.slotSizes[10] = 2
	b.slots[20] = 2
	b.slotSizes[20] = 2
	b.slots[30] = 4
	b.slotSizes[30] = 2

	// Allocate scratch slot at offset 6
	b.cssaScratchOffset = 6
	b.stackSize = 8

	b.emitPhiAssignments(b0, b1)
	output := b.buf.String()

	// Must save to scratch slot 6,s and restore from scratch slot 6,s
	if !strings.Contains(output, "std 6,s") {
		t.Fatalf("expected save to scratch slot 'std 6,s', got:\n%s", output)
	}
	if !strings.Contains(output, "ldd 6,s") {
		t.Fatalf("expected restore from scratch slot 'ldd 6,s', got:\n%s", output)
	}
}

