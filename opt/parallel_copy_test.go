package opt

import (
	"testing"
)

func TestSequentializeIdentity(t *testing.T) {
	moves := []ParallelMove{
		{Dest: "a", Src: "a", Size: 2},
		{Dest: "b", Src: "b", Size: 1},
	}
	steps := SequentializeParallelCopies(moves)
	if len(steps) != 0 {
		t.Fatalf("expected 0 steps for identity moves, got %d", len(steps))
	}
}

func TestSequentializeAcyclicChain(t *testing.T) {
	// a <- b
	// b <- c
	// c <- 10
	// Correct order: a <- b FIRST, then b <- c, then c <- 10
	moves := []ParallelMove{
		{Dest: "b", Src: "c", Size: 2},
		{Dest: "c", Src: "10", Size: 2},
		{Dest: "a", Src: "b", Size: 2},
	}

	steps := SequentializeParallelCopies(moves)
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	// Verify order
	if steps[0].Dest != "a" || steps[0].Src != "b" {
		t.Errorf("step 0 should be a <- b, got %v <- %v", steps[0].Dest, steps[0].Src)
	}
	if steps[1].Dest != "b" || steps[1].Src != "c" {
		t.Errorf("step 1 should be b <- c, got %v <- %v", steps[1].Dest, steps[1].Src)
	}
	if steps[2].Dest != "c" || steps[2].Src != "10" {
		t.Errorf("step 2 should be c <- 10, got %v <- %v", steps[2].Dest, steps[2].Src)
	}
}

func TestSequentializeTwoCycleSwap(t *testing.T) {
	// a <- b
	// b <- a
	moves := []ParallelMove{
		{Dest: "a", Src: "b", Size: 2},
		{Dest: "b", Src: "a", Size: 2},
	}

	steps := SequentializeParallelCopies(moves)
	if len(steps) != 1 {
		t.Fatalf("expected 1 swap step, got %d", len(steps))
	}

	if steps[0].Kind != StepSwap {
		t.Fatalf("expected StepSwap, got %v", steps[0].Kind)
	}
	if (steps[0].Loc1 == "a" && steps[0].Loc2 == "b") || (steps[0].Loc1 == "b" && steps[0].Loc2 == "a") {
		// Valid swap between a and b
	} else {
		t.Errorf("expected swap between a and b, got %v and %v", steps[0].Loc1, steps[0].Loc2)
	}
}

func TestSequentializeSwapWithExternalReader(t *testing.T) {
	// a <- b
	// b <- a
	// c <- a  (reads initial value of a)
	// c must read a BEFORE the swap between a and b occurs!
	moves := []ParallelMove{
		{Dest: "a", Src: "b", Size: 2},
		{Dest: "b", Src: "a", Size: 2},
		{Dest: "c", Src: "a", Size: 2},
	}

	steps := SequentializeParallelCopies(moves)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps (copy c <- a then swap a <-> b), got %d", len(steps))
	}

	if steps[0].Kind != StepCopy || steps[0].Dest != "c" || steps[0].Src != "a" {
		t.Errorf("step 0 must copy c <- a before swap, got %v", steps[0])
	}
	if steps[1].Kind != StepSwap {
		t.Errorf("step 1 must be StepSwap, got %v", steps[1])
	}
}

func TestSequentializeThreeCycle(t *testing.T) {
	// a <- b
	// b <- c
	// c <- a
	moves := []ParallelMove{
		{Dest: "a", Src: "b", Size: 2},
		{Dest: "b", Src: "c", Size: 2},
		{Dest: "c", Src: "a", Size: 2},
	}

	steps := SequentializeParallelCopies(moves)
	// Expected: 1 Save, 2 Copy, 1 Restore = 4 steps
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps for 3-cycle, got %d: %v", len(steps), steps)
	}

	if steps[0].Kind != StepSave {
		t.Errorf("step 0 should be StepSave, got %v", steps[0].Kind)
	}
	if steps[1].Kind != StepCopy || steps[2].Kind != StepCopy {
		t.Errorf("steps 1 and 2 should be StepCopy, got %v and %v", steps[1].Kind, steps[2].Kind)
	}
	if steps[3].Kind != StepRestore {
		t.Errorf("step 3 should be StepRestore, got %v", steps[3].Kind)
	}
}

func TestSequentializeMultipleDisjointCycles(t *testing.T) {
	// Swap 1: a <-> b
	// Swap 2: x <-> y
	moves := []ParallelMove{
		{Dest: "a", Src: "b", Size: 1},
		{Dest: "x", Src: "y", Size: 2},
		{Dest: "b", Src: "a", Size: 1},
		{Dest: "y", Src: "x", Size: 2},
	}

	steps := SequentializeParallelCopies(moves)
	if len(steps) != 2 {
		t.Fatalf("expected 2 swap steps, got %d", len(steps))
	}
	for i, s := range steps {
		if s.Kind != StepSwap {
			t.Errorf("step %d should be StepSwap, got %v", i, s.Kind)
		}
	}
}
