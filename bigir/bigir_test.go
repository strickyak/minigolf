package bigir

import (
	"testing"
)

func TestTypeSizes(t *testing.T) {
	if TypeFarString.Size != 8 {
		t.Fatalf("expected string size 8, got %d", TypeFarString.Size)
	}
	sliceInt := MakeFarSlice(TypeInt)
	if sliceInt.Size != 8 {
		t.Fatalf("expected slice[int] size 8, got %d", sliceInt.Size)
	}
	if TypeFarRef.Size != 2 {
		t.Fatalf("expected far_ref size 2, got %d", TypeFarRef.Size)
	}
}

func TestPackProgram(t *testing.T) {
	fn1 := &Function{
		Name:  "main",
		IsFar: true,
		Blocks: []*BasicBlock{
			{
				ID: 0,
				Instructions: []Instruction{
					&FarCall{
						BaseInstruction: BaseInstruction{Typ: TypeVoid},
						Callee:          "helper",
					},
				},
				Terminator: &FarReturn{BaseInstruction: BaseInstruction{Typ: TypeNoReturn}},
			},
		},
	}

	fn2 := &Function{
		Name:  "helper",
		IsFar: true,
		Blocks: []*BasicBlock{
			{
				ID: 0,
				Instructions: []Instruction{
					&BinaryOp{
						BaseInstruction: BaseInstruction{Typ: TypeWord},
						Op:              "add",
						Left:            &ConstWord{Val: 10},
						Right:           &ConstWord{Val: 20},
					},
				},
				Terminator: &FarReturn{BaseInstruction: BaseInstruction{Typ: TypeNoReturn}},
			},
		},
	}

	prog := &Program{
		Functions: []*Function{fn1, fn2},
	}

	if err := PackProgram(prog); err != nil {
		t.Fatalf("PackProgram failed: %v", err)
	}

	if fn1.BlockID != 8 {
		t.Fatalf("expected main in Block 8, got Block %d", fn1.BlockID)
	}
	if fn2.BlockID != 8 {
		t.Fatalf("expected helper in Block 8, got Block %d", fn2.BlockID)
	}

	if len(prog.Trampolines) != 2 {
		t.Fatalf("expected 2 trampolines, got %d", len(prog.Trampolines))
	}

	t0 := prog.Trampolines[0]
	if t0.FuncName != "main" || t0.TargetBlock != 8 || t0.TargetAddr != 0xA000 {
		t.Fatalf("unexpected trampoline for main: %+v", t0)
	}

	// Verify intra-block call devirtualization:
	// Since main and helper are both in Block 8, the FarCall in main should have been devirtualized to NearCall!
	firstInstr := fn1.Blocks[0].Instructions[0]
	nc, ok := firstInstr.(*NearCall)
	if !ok {
		t.Fatalf("expected devirtualized NearCall, got %T (%s)", firstInstr, firstInstr.Opcode())
	}
	if nc.GetComment() != "devirtualized intra-block call" {
		t.Fatalf("expected devirtualized comment, got %q", nc.GetComment())
	}
}
